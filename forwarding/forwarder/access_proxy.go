package forwarder

import (
	"bufio"
	"bytes"
	"fmt"
	"forwarding/forwarder/connection"
	packet "forwarding/packet_handler"
	"forwarding/router"
	"io"
	"log"
	"math/rand"
	"net"
	"net/http"
	"net/http/httputil"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/xtaci/smux"
)

var requestCounter uint32 = 0 // Global counter for generating unique request IDs

func generateUniqueRequestID(r *http.Request) uint32 {
	// Atomically increment the global counter
	counter := atomic.AddUint32(&requestCounter, 1)

	now := time.Now()
	seconds := now.Unix()
	nanos := now.Nanosecond()

	// Combine time components for uniqueness
	timeComponent := uint32(seconds) ^ uint32(nanos)
	// Generate a small random number to further reduce collision probability
	randomComponent := uint32(rand.Intn(1000))

	// Combine all parts to form the ID
	// Shift counter to give it more significance and avoid overlap with time/random components
	return timeComponent ^ (counter << 10) ^ randomComponent
}

type RepositoryConfig struct {
	ProcessingInterval time.Duration
	RequestBufferSize  int
	ResponseBufferSize int
}

var DefaultRepositoryConfig = RepositoryConfig{
	ProcessingInterval: 10 * time.Millisecond,
	RequestBufferSize:  100,
	ResponseBufferSize: 100,
}

type Repository struct {
	httpRequestChan  chan *RequestItem
	smuxResponseChan chan *ResponseItem

	done chan struct{}
	wg   sync.WaitGroup

	config       RepositoryConfig
	accessConfig AccessConfig

	bufferManager *BufferManager

	stateManager *RequestStateManager
}

type RequestItem struct {
	Request          *http.Request
	ResponseWriter   http.ResponseWriter
	HeaderBytes      []byte
	RequestID        uint32
	ReceivedAt       time.Time
	ResponseReceived chan struct{}
	IsLastHop        bool
	NextHopIP        string
	HopList          []uint32
}

type ResponseItem struct {
	Data       []byte
	ReceivedAt time.Time
}

type AccessConfig struct {
	HttpPort     string
	ResponsePort string
}

var DefaultAccessConfig = AccessConfig{
	HttpPort:     "50055",
	ResponsePort: "50054",
}

type AccessProxy struct {
	repository *Repository
	config     AccessConfig
}

func CreateAccessProxy(config AccessConfig, repoConfig RepositoryConfig) *AccessProxy {
	// Initialize a new request state manager
	stateManager := NewRequestStateManager(15*time.Minute, 1*time.Minute) // 15 min expiration, 1 min cleanup

	repo := &Repository{
		httpRequestChan:  make(chan *RequestItem, repoConfig.RequestBufferSize),
		smuxResponseChan: make(chan *ResponseItem, repoConfig.ResponseBufferSize),
		done:             make(chan struct{}),
		config:           repoConfig,
		accessConfig:     config,
		stateManager:     stateManager,
	}

	bufferConfig := DefaultBufferConfig()
	repo.bufferManager = NewBufferManager(bufferConfig, stateManager)

	// Set send functions for the buffer manager; nil for forwardResponseToPreviousHop as AccessProxy handles responses differently
	repo.bufferManager.SetSendFunctions(
		repo.sendSingleRequest,
		repo.sendMergedRequest,
		nil, // AccessProxy does not forward responses to a previous hop in the same way RelayProxy does
	)
	log.Printf("[Access-INFO] AccessProxy repository created with RequestBufferSize: %d, ResponseBufferSize: %d", repoConfig.RequestBufferSize, repoConfig.ResponseBufferSize)
	return &AccessProxy{
		repository: repo,
		config:     config,
	}
}

func (r *Repository) StartProcessors() {
	r.wg.Add(2) // Add count for the two goroutines to be launched

	go r.processHttpRequests()
	go r.processSmuxResponses()

	log.Println("[Repository-INFO] Started HTTP request and SMUX response processors.")
}

func (r *Repository) Stop() {
	log.Println("[Repository-INFO] Stopping repository processors...")
	close(r.done) // Signal all worker goroutines to stop
	r.wg.Wait()   // Wait for all worker goroutines to complete

	if r.bufferManager != nil {
		log.Println("[Repository-INFO] Stopping BufferManager.")
		r.bufferManager.Stop()
	}

	if r.stateManager != nil {
		log.Println("[Repository-INFO] Stopping RequestStateManager.")
		r.stateManager.Stop()
	}

	log.Println("[Repository-INFO] All repository processors stopped.")
}

func (r *Repository) processHttpRequests() {
	defer r.wg.Done() // Decrement the WaitGroup counter when this goroutine exits

	log.Printf("[Repository-INFO] Starting HTTP request processing dispatcher.")

	workerCount := runtime.NumCPU() * 2 // Default to twice the number of CPUs
	if workerCount < 4 {
		workerCount = 4 // Ensure at least 4 workers
	}

	log.Printf("[Repository-INFO] Launching %d HTTP request worker goroutines.", workerCount)

	for i := 0; i < workerCount; i++ {
		go func(workerID int) {
			log.Printf("[Repository-DEBUG] HTTP Request Worker #%d started.", workerID)
			for {
				select {
				case <-r.done:
					log.Printf("[Repository-INFO] HTTP Request Worker #%d stopping as done signal received.", workerID)
					return
				case req, ok := <-r.httpRequestChan:
					if !ok {
						log.Printf("[Repository-WARN] HTTP Request Worker #%d: httpRequestChan closed, exiting.", workerID)
						return
					}
					log.Printf("[Access-DEBUG] Worker #%d processing HTTP request ID %d for %s from %s", workerID, req.RequestID, req.Request.URL.Path, req.Request.RemoteAddr)

					reqState := &RequestState{
						RequestID:        req.RequestID,
						OriginalRequest:  req.Request,
						ResponseWriter:   req.ResponseWriter,
						RequestData:      []byte{}, // Will be populated if not a direct proxy
						Size:             0,
						Status:           StatusCreated,
						CreatedAt:        time.Now(),
						LastUpdatedAt:    time.Now(),
						NextHopIP:        req.NextHopIP,
						HopList:          req.HopList,
						IsLastHop:        req.IsLastHop,
						ResponseReceived: req.ResponseReceived,
						BufferID:         "",
						MergeGroupID:     0,
					}

					r.stateManager.AddState(reqState) // StateManager already logs this addition

					if req.IsLastHop {
						log.Printf("[Access-INFO] Request ID %d for %s is the last hop. Handling as direct proxy to %s.", req.RequestID, req.Request.URL.Path, req.NextHopIP)
						go r.handleDirectProxy(reqState)
					} else {
						log.Printf("[Access-INFO] Request ID %d for %s is not the last hop. Processing for buffered forwarding to %s.", req.RequestID, req.Request.URL.Path, req.NextHopIP)
						reqBytes, err := httputil.DumpRequest(req.Request, true)
						if err != nil {
							log.Printf("[Access-ERROR] Failed to dump HTTP request ID %d: %v", req.RequestID, err)
							r.stateManager.UpdateStatus(req.RequestID, StatusFailed) // StateManager logs this update
							r.notifyRequestFailed(reqState, fmt.Errorf("failed to dump request: %w", err))
							continue
						}

						// Safely update RequestData and Size
						reqState.mu.Lock()
						reqState.RequestData = reqBytes
						reqState.Size = len(reqBytes)
						reqState.mu.Unlock()
						log.Printf("[Access-DEBUG] Request ID %d dumped, size: %d bytes.", req.RequestID, reqState.Size)

						err = r.bufferManager.ProcessRequest(reqState)
						if err != nil {
							log.Printf("[Access-ERROR] BufferManager failed to process request ID %d: %v", req.RequestID, err)
							r.stateManager.UpdateStatus(req.RequestID, StatusFailed) // StateManager logs this update
							r.notifyRequestFailed(reqState, fmt.Errorf("buffer manager processing failed: %w", err))
						} else {
							log.Printf("[Access-INFO] Request ID %d successfully submitted to BufferManager for forwarding.", req.RequestID)
						}
					}
				}
			}
		}(i)
	}

	<-r.done // Wait for the done signal to stop the dispatcher itself
	log.Println("[Repository-INFO] HTTP request processing dispatcher stopped.")
}

func (r *Repository) handleDirectProxy(reqState *RequestState) {
	// Update status to Sent, StateManager will log this change
	r.stateManager.UpdateStatus(reqState.RequestID, StatusSent)

	reqState.mu.RLock()
	originalReq := reqState.OriginalRequest
	nextHopIP := reqState.NextHopIP
	requestID := reqState.RequestID
	log.Printf("[Access-INFO] Request ID %d: Handling direct proxy to %s for URL: %s", requestID, nextHopIP, originalReq.URL.Path)
	reqState.mu.RUnlock()

	parts := bytes.Split([]byte(nextHopIP), []byte(":"))
	host := string(parts[0])
	port := "80" // Default port for direct proxy if not specified
	if len(parts) > 1 {
		port = string(parts[1])
	}

	targetURL := fmt.Sprintf("http://%s:%s%s", host, port, originalReq.URL.Path)
	log.Printf("[Access-DEBUG] Request ID %d: Target URL for direct proxy: %s", requestID, targetURL)

	var bodyBytes []byte
	var err error
	if originalReq.Body != nil {
		bodyBytes, err = io.ReadAll(originalReq.Body)
		if err != nil {
			log.Printf("[Access-ERROR] Request ID %d: Failed to read request body for direct proxy: %v", requestID, err)
			r.stateManager.UpdateStatus(requestID, StatusFailed)
			r.notifyRequestFailed(reqState, fmt.Errorf("failed to read request body: %w", err))
			return
		}
		originalReq.Body.Close() // Close the original body after reading
		// Restore the body for the original request in case it's needed elsewhere (though unlikely for direct proxy)
		originalReq.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
	}

	// Create a new request for the target URL
	clonedReq, err := http.NewRequest(originalReq.Method, targetURL, io.NopCloser(bytes.NewBuffer(bodyBytes)))
	if err != nil {
		log.Printf("[Access-ERROR] Request ID %d: Failed to create new HTTP request for direct proxy to %s: %v", requestID, targetURL, err)
		r.stateManager.UpdateStatus(requestID, StatusFailed)
		r.notifyRequestFailed(reqState, fmt.Errorf("failed to create new request: %w", err))
		return
	}

	// Copy headers from the original request
	for key, values := range originalReq.Header {
		for _, value := range values {
			clonedReq.Header.Add(key, value)
		}
	}
	clonedReq.Host = host // Set the Host header to the target host

	client := &http.Client{
		Timeout: 30 * time.Second, // TODO: Make timeout configurable
	}

	log.Printf("[Access-DEBUG] Request ID %d: Sending request to %s", requestID, targetURL)
	resp, err := client.Do(clonedReq)
	if err != nil {
		log.Printf("[Access-ERROR] Request ID %d: Failed to execute direct proxy request to %s: %v", requestID, targetURL, err)
		r.stateManager.UpdateStatus(requestID, StatusFailed)
		r.notifyRequestFailed(reqState, fmt.Errorf("failed to execute request to target: %w", err))
		return
	}
	defer resp.Body.Close()
	log.Printf("[Access-INFO] Request ID %d: Received response %d from %s", requestID, resp.StatusCode, targetURL)

	r.stateManager.UpdateStatus(requestID, StatusResponding) // StateManager logs this

	reqState.mu.RLock()
	responseWriter := reqState.ResponseWriter
	responseChan := reqState.ResponseReceived
	reqState.mu.RUnlock()

	// Copy headers from the target's response to the original client's response writer
	for key, values := range resp.Header {
		for _, value := range values {
			responseWriter.Header().Add(key, value)
		}
	}

	responseWriter.WriteHeader(resp.StatusCode)

	// Copy the response body to the original client
	copiedBytes, err := io.Copy(responseWriter, resp.Body)
	if err != nil {
		log.Printf("[Access-ERROR] Request ID %d: Failed to copy response body to client: %v. Copied %d bytes.", requestID, err, copiedBytes)
		r.stateManager.UpdateStatus(requestID, StatusFailed)
		// Don't call notifyRequestFailed here as headers/status might have already been sent.
		// The client connection will likely be closed by the HTTP server or detect an error.
		return
	}
	log.Printf("[Access-DEBUG] Request ID %d: Successfully copied %d bytes of response body to client.", requestID, copiedBytes)

	r.stateManager.UpdateStatus(requestID, StatusCompleted) // StateManager logs this

	close(responseChan) // Signal that the response has been handled

	log.Printf("[Access-INFO] Request ID %d: Direct proxy handling completed successfully.", requestID)
}

func (r *Repository) notifyRequestFailed(reqState *RequestState, err error) {
	if reqState == nil {
		log.Printf("[Access-ERROR] notifyRequestFailed called with nil reqState. Error: %v", err)
		return
	}

	log.Printf("[Access-WARN] Notifying client of failure for Request ID %d. Error: %v", reqState.RequestID, err)

	reqState.mu.RLock()
	responseWriter := reqState.ResponseWriter
	responseChan := reqState.ResponseReceived
	isAlreadyClosed := false
	// Check if channel is already closed to prevent panic
	select {
	case _, ok := <-responseChan:
		if !ok {
			isAlreadyClosed = true
		}
	default:
	}
	reqState.mu.RUnlock()

	if responseWriter == nil {
		log.Printf("[Access-ERROR] Request ID %d: Cannot notify client of failure, ResponseWriter is nil.", reqState.RequestID)
		// If responseChan is still open, close it to unblock any waiting goroutine
		if !isAlreadyClosed {
			close(responseChan)
		}
		return
	}

	// Construct a generic error response
	// Avoid writing to responseWriter if headers have already been sent, though this function is typically called before that.
	// For simplicity, we assume it's safe here, but in a more complex system, this might need a check.
	bodyMsg := fmt.Sprintf("Request failed due to an internal error. Request ID: %d. Error: %v", reqState.RequestID, err)
	responseWriter.Header().Set("Content-Type", "text/plain; charset=utf-8")
	responseWriter.WriteHeader(http.StatusInternalServerError)
	_, writeErr := responseWriter.Write([]byte(bodyMsg))
	if writeErr != nil {
		log.Printf("[Access-ERROR] Request ID %d: Failed to write error response to client: %v", reqState.RequestID, writeErr)
	}

	// Close the response channel if it's not already closed
	if !isAlreadyClosed {
		close(responseChan)
	}
	log.Printf("[Access-INFO] Request ID %d: Client notification of failure sent.", reqState.RequestID)
}

func (r *Repository) processSmuxResponses() {
	defer r.wg.Done() // Decrement the WaitGroup counter when this goroutine exits

	workerCount := runtime.NumCPU() * 2 // Default to twice the number of CPUs
	if workerCount < 4 {
		workerCount = 4 // Ensure at least 4 workers
	}

	log.Printf("[Repository-INFO] Starting SMUX response processing dispatcher with %d workers.", workerCount)

	var wgProcessors sync.WaitGroup // Use a new WaitGroup for SMUX response workers
	for i := 0; i < workerCount; i++ {
		wgProcessors.Add(1)
		go func(workerID int) {
			defer wgProcessors.Done()
			log.Printf("[Repository-DEBUG] SMUX Response Worker #%d started.", workerID)
			for {
				select {
				case <-r.done:
					log.Printf("[Repository-INFO] SMUX Response Worker #%d stopping as done signal received.", workerID)
					return
				case resp, ok := <-r.smuxResponseChan:
					if !ok {
						log.Printf("[Repository-WARN] SMUX Response Worker #%d: smuxResponseChan closed, exiting.", workerID)
						return
					}

					log.Printf("[Access-DEBUG] Worker #%d received SMUX response. Data size: %d bytes. Chan size: %d/%d.",
						workerID, len(resp.Data), len(r.smuxResponseChan), cap(r.smuxResponseChan))

					if len(resp.Data) < 4 { // Assuming a minimum header size of 4 bytes if packet.MinHeaderSize is not defined
						log.Printf("[Access-ERROR] Worker #%d: Received SMUX response with insufficient data size (%d bytes) for header.", workerID, len(resp.Data))
						continue
					}

					headerLen := uint16(resp.Data[2])<<8 | uint16(resp.Data[3]) // Assuming header length is at bytes 2 and 3
					if int(headerLen) > len(resp.Data) {
						log.Printf("[Access-ERROR] Worker #%d: Invalid header length %d in SMUX response (total data size: %d bytes).", workerID, headerLen, len(resp.Data))
						continue
					}

					headerBytes := resp.Data[:headerLen]
					responseData := resp.Data[headerLen:]

					header, err := packet.Unpack(headerBytes)
					if err != nil {
						log.Printf("[Access-ERROR] Worker #%d: Failed to unpack SMUX response header: %v. Header bytes: %x", workerID, err, headerBytes)
						continue
					}

					log.Printf("[Access-DEBUG] Worker #%d: Unpacked SMUX response header for %d packet(s). Request IDs: %v", workerID, header.PacketCount, header.PacketID)

					positions := packet.GetRequestPositions(header, len(responseData))

					for i := 0; i < int(header.PacketCount); i++ {
						requestID := header.PacketID[i]
						// Boundary check for positions to prevent panic
						if positions[i] > positions[i+1] || positions[i+1] > len(responseData) {
							log.Printf("[Access-ERROR] Worker #%d, Request ID %d: Invalid packet positions [%d:%d] for response data size %d.", workerID, requestID, positions[i], positions[i+1], len(responseData))
							continue
						}
						respData := responseData[positions[i]:positions[i+1]]
						log.Printf("[Access-DEBUG] Worker #%d: Processing response for Request ID %d, data size: %d bytes.", workerID, requestID, len(respData))

						reqState, exists := r.stateManager.GetState(requestID)
						if !exists {
							log.Printf("[Access-WARN] Worker #%d: No state found for Request ID %d in SMUX response. Response might be late or ID is invalid.", workerID, requestID)
							continue
						}

						r.stateManager.UpdateStatus(requestID, StatusResponding) // StateManager logs this

						respReader := bufio.NewReader(bytes.NewReader(respData))
						httpResp, err := http.ReadResponse(respReader, reqState.OriginalRequest) // Pass original request for context if needed by ReadResponse
						if err != nil {
							log.Printf("[Access-ERROR] Worker #%d, Request ID %d: Failed to read HTTP response from SMUX data: %v", workerID, requestID, err)
							r.stateManager.UpdateStatus(requestID, StatusFailed)
							// Potentially call notifyRequestFailed if appropriate, but client might have timed out
							continue
						}
						log.Printf("[Access-DEBUG] Worker #%d, Request ID %d: Parsed HTTP response: %s", workerID, requestID, httpResp.Status)

						reqState.mu.RLock()
						responseWriter := reqState.ResponseWriter
						responseChan := reqState.ResponseReceived
						isAlreadyClosed := false
						// Check if channel is already closed to prevent panic
						select {
						case _, ok := <-responseChan:
							if !ok {
								isAlreadyClosed = true
							}
						default:
						}
						reqState.mu.RUnlock()

						if responseWriter != nil {
							err = r.sendResponseToClient(responseWriter, httpResp)
							if err != nil {
								log.Printf("[Access-ERROR] Worker #%d, Request ID %d: Failed to send response to client: %v", workerID, requestID, err)
								r.stateManager.UpdateStatus(requestID, StatusFailed)
								// Don't call notifyRequestFailed as headers might be partially sent.
								continue
							}

							r.stateManager.UpdateStatus(requestID, StatusCompleted) // StateManager logs this
							if responseChan != nil && !isAlreadyClosed {
								close(responseChan)
							}

							log.Printf("[Access-INFO] Worker #%d: Successfully sent SMUX response for Request ID %d to client.", workerID, requestID)
						} else {
							log.Printf("[Access-WARN] Worker #%d, Request ID %d: ResponseWriter is nil, cannot send response to client.", workerID, requestID)
							// If responseChan is still open, close it
							if responseChan != nil && !isAlreadyClosed {
								close(responseChan)
							}
						}
					}
				}
			}
		}(i)
	}

	<-r.done // Wait for the repository done signal
	log.Printf("[Repository-INFO] SMUX response processing dispatcher signalled to stop. Waiting for workers...")
	wgProcessors.Wait() // Wait for all SMUX response worker goroutines to complete
	log.Println("[Repository-INFO] SMUX response processing dispatcher and all its workers stopped.")
}

func (r *Repository) sendResponseToClient(w http.ResponseWriter, resp *http.Response) error {
	log.Printf("[Access-DEBUG] Sending response to client. Status: %s, Code: %d", resp.Status, resp.StatusCode)

	// Copy headers from the response to the client's ResponseWriter
	for key, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}

	w.WriteHeader(resp.StatusCode)

	if resp.Body != nil {
		defer resp.Body.Close()
		copiedBytes, err := io.Copy(w, resp.Body)
		if err != nil {
			log.Printf("[Access-ERROR] Failed to copy response body to client: %v. Copied %d bytes.", err, copiedBytes)
			return fmt.Errorf("failed to copy response body: %w", err)
		}
		log.Printf("[Access-DEBUG] Successfully copied %d bytes of response body to client.", copiedBytes)
	} else {
		log.Printf("[Access-DEBUG] Response body is nil, nothing to copy to client.")
	}
	log.Printf("[Access-INFO] Successfully sent response headers and body (if any) to client.")
	return nil
}

func (r *Repository) sendSingleRequest(data []byte, nextHopIP string, requestID uint32, request *RequestState) error {
	log.Printf("[Access-INFO] Request ID %d: Preparing to send single request to next hop: %s", requestID, nextHopIP)

	parts := bytes.Split([]byte(nextHopIP), []byte(":"))
	ip := string(parts[0])
	port := "50056" // Default port for relay if not specified
	if len(parts) > 1 {
		port = string(parts[1])
	}
	targetAddr := ip + ":" + port

	log.Printf("[Access-DEBUG] Request ID %d: Sending single request via SMUX to target: %s. Data size (payload only): %d bytes", requestID, targetAddr, len(data))

	session, err := connection.GetOrCreateClientSession(targetAddr)
	if err != nil {
		log.Printf("[Access-ERROR] Request ID %d: Failed to get/create SMUX client session for target %s: %v", requestID, targetAddr, err)
		return fmt.Errorf("failed to get/create SMUX client session to %s: %w", targetAddr, err)
	}

	stream, err := session.OpenStream()
	if err != nil {
		log.Printf("[Access-ERROR] Request ID %d: Failed to open SMUX stream for target %s (session: %p): %v", requestID, targetAddr, session, err)
		connection.RemoveClientSession(targetAddr, session) // Attempt to remove potentially bad session
		// Retry getting/creating session and opening stream once
		log.Printf("[Access-INFO] Request ID %d: Retrying to establish SMUX connection to %s...", requestID, targetAddr)
		session, err = connection.GetOrCreateClientSession(targetAddr)
		if err != nil {
			log.Printf("[Access-ERROR] Request ID %d: Retry failed to get/create SMUX client session for target %s: %v", requestID, targetAddr, err)
			return fmt.Errorf("retry failed to get/create SMUX client session to %s: %w", targetAddr, err)
		}
		stream, err = session.OpenStream()
		if err != nil {
			log.Printf("[Access-ERROR] Request ID %d: Retry failed to open SMUX stream for target %s (session: %p): %v", requestID, targetAddr, session, err)
			return fmt.Errorf("retry failed to open SMUX stream to %s: %w", targetAddr, err)
		}
		log.Printf("[Access-INFO] Request ID %d: Successfully established SMUX stream to %s after retry.", requestID, targetAddr)
	}
	defer stream.Close()

	var headerBytes []byte
	request.mu.RLock()
	hopList := request.HopList
	isLastHop := request.IsLastHop // This should be false if we are sending a request to a next hop
	request.mu.RUnlock()

	if isLastHop {
		// This case should ideally not happen if this function is for sending to a *next* hop.
		// If it's truly the last hop, handleDirectProxy or a similar function should have been called.
		// However, if logic dictates it can occur, we log a warning.
		log.Printf("[Access-WARN] Request ID %d: sendSingleRequest called for what is marked as the last hop to %s. Proceeding without adding a forwarder header.", requestID, nextHopIP)
	} else {
		// Construct the packet header for forwarding
		header := &packet.Packet{
			PacketCount: 1,
			PacketID:    []uint32{requestID},
			HopList:     hopList,
			HopCounts:   0, // HopCounts is 0 when originating from AccessProxy
		}

		headerBytes, err = header.Pack()
		if err != nil {
			log.Printf("[Access-ERROR] Request ID %d: Failed to pack packet header: %v", requestID, err)
			return fmt.Errorf("failed to pack packet header for request %d: %w", requestID, err)
		}
		log.Printf("[Access-DEBUG] Request ID %d: Packet header packed. Header size: %d bytes. HopList: %v", requestID, len(headerBytes), hopList)
	}

	fullData := append(headerBytes, data...)
	log.Printf("[Access-DEBUG] Request ID %d: Sending data to %s. Total size (header + payload): %d bytes.", requestID, targetAddr, len(fullData))

	_, err = stream.Write(fullData)
	if err != nil {
		log.Printf("[Access-ERROR] Request ID %d: Failed to write data to SMUX stream for target %s (stream: %p): %v", requestID, targetAddr, stream, err)
		return fmt.Errorf("failed to write data to SMUX stream for %s: %w", targetAddr, err)
	}

	// Update status to Sent, StateManager will log this change
	r.stateManager.UpdateStatus(requestID, StatusSent)
	log.Printf("[Access-INFO] Request ID %d: Successfully sent single request data to %s.", requestID, targetAddr)
	return nil
}

func (r *Repository) sendMergedRequest(mergedData []byte, nextHopIP string, updatedHeader *packet.Packet) error {
	log.Printf("[Access-INFO] Preparing to send merged request to next hop: %s. Header PacketCount: %d, Request IDs: %v", nextHopIP, updatedHeader.PacketCount, updatedHeader.PacketID)

	parts := bytes.Split([]byte(nextHopIP), []byte(":"))
	ip := string(parts[0])
	port := "50056" // Default port for relay if not specified
	if len(parts) > 1 {
		port = string(parts[1])
	}
	targetAddr := ip + ":" + port

	log.Printf("[Access-DEBUG] Sending merged request via SMUX to target: %s. Original merged data size: %d bytes", targetAddr, len(mergedData))

	var modifiedData []byte
	var newHeaderBytes []byte
	var err error

	// The mergedData already contains an old header. We need to replace it with the updatedHeader.
	if len(mergedData) >= 4 && updatedHeader != nil { // Basic check for existing header structure
		currentHeaderLen := int(uint16(mergedData[2])<<8 | uint16(mergedData[3])) // Assuming header length is at bytes 2 and 3

		if currentHeaderLen > 0 && currentHeaderLen <= len(mergedData) {
			requestBytes := mergedData[currentHeaderLen:] // This is the actual payload (one or more HTTP requests)

			log.Printf("[Access-DEBUG] Current header length in mergedData: %d. Payload size: %d. Updating header with HopCounts=%d", currentHeaderLen, len(requestBytes), updatedHeader.HopCounts)

			newHeaderBytes, err = updatedHeader.Pack()
			if err != nil {
				log.Printf("[Access-ERROR] Failed to pack updated header for merged request (Request IDs: %v): %v", updatedHeader.PacketID, err)
				return fmt.Errorf("failed to pack updated header for merged request: %w", err)
			}
			modifiedData = append(newHeaderBytes, requestBytes...)
			log.Printf("[Access-DEBUG] Successfully updated header for merged request. New header size: %d. Total modified data size: %d.", len(newHeaderBytes), len(modifiedData))
		} else {
			log.Printf("[Access-WARN] Invalid or zero current header length (%d) in mergedData for Request IDs: %v. Using original mergedData.", currentHeaderLen, updatedHeader.PacketID)
			modifiedData = mergedData // Fallback to original data if header structure is unexpected
		}
	} else {
		log.Printf("[Access-WARN] Merged data length insufficient or updatedHeader is nil for Request IDs: %v. Using original mergedData.", updatedHeader.PacketID)
		modifiedData = mergedData // Fallback to original data
	}

	session, err := connection.GetOrCreateClientSession(targetAddr)
	if err != nil {
		log.Printf("[Access-ERROR] Failed to get/create SMUX client session for merged request to target %s (Request IDs: %v): %v", targetAddr, updatedHeader.PacketID, err)
		return fmt.Errorf("failed to get/create SMUX client session for merged request to %s: %w", targetAddr, err)
	}

	stream, err := session.OpenStream()
	if err != nil {
		log.Printf("[Access-ERROR] Failed to open SMUX stream for merged request to target %s (session: %p, Request IDs: %v): %v", targetAddr, session, updatedHeader.PacketID, err)
		connection.RemoveClientSession(targetAddr, session) // Attempt to remove potentially bad session
		// Retry getting/creating session and opening stream once
		log.Printf("[Access-INFO] Retrying to establish SMUX connection for merged request to %s...", targetAddr)
		session, err = connection.GetOrCreateClientSession(targetAddr)
		if err != nil {
			log.Printf("[Access-ERROR] Retry failed to get/create SMUX client session for merged request to target %s (Request IDs: %v): %v", targetAddr, updatedHeader.PacketID, err)
			return fmt.Errorf("retry failed to get/create SMUX client session for merged request to %s: %w", targetAddr, err)
		}
		stream, err = session.OpenStream()
		if err != nil {
			log.Printf("[Access-ERROR] Retry failed to open SMUX stream for merged request to target %s (session: %p, Request IDs: %v): %v", targetAddr, session, updatedHeader.PacketID, err)
			return fmt.Errorf("retry failed to open SMUX stream for merged request to %s: %w", targetAddr, err)
		}
		log.Printf("[Access-INFO] Successfully established SMUX stream for merged request to %s after retry.", targetAddr)
	}
	defer stream.Close()

	log.Printf("[Access-DEBUG] Sending modified merged data to %s. Total size: %d bytes. Request IDs: %v", targetAddr, len(modifiedData), updatedHeader.PacketID)
	_, err = stream.Write(modifiedData)
	if err != nil {
		log.Printf("[Access-ERROR] Failed to write merged data to SMUX stream for target %s (stream: %p, Request IDs: %v): %v", targetAddr, stream, updatedHeader.PacketID, err)
		return fmt.Errorf("failed to write merged data to SMUX stream for %s: %w", targetAddr, err)
	}

	// Update status to Sent for all individual requests within the merged request
	for _, requestID := range updatedHeader.PacketID {
		r.stateManager.UpdateStatus(requestID, StatusSent) // StateManager logs this update
	}

	log.Printf("[Access-INFO] Successfully sent merged request data to %s for Request IDs: %v.", targetAddr, updatedHeader.PacketID)
	return nil
}

func (r *Repository) StartHttpProxy() {
	handler := func(w http.ResponseWriter, req *http.Request) {
		requestReceivedTime := time.Now()
		log.Printf("[Access-INFO] Received HTTP request: %s %s from %s. User-Agent: %s", req.Method, req.URL.Path, req.RemoteAddr, req.UserAgent())

		requestID := generateUniqueRequestID(req)
		log.Printf("[Access-DEBUG] Generated Request ID %d for %s %s", requestID, req.Method, req.URL.Path)

		pathManager := router.GetInstance() // Assuming router.GetInstance() is safe and handles its own initialization logging if any.
		paths := pathManager.GetPaths()
		if len(paths) == 0 {
			log.Printf("[Access-ERROR] Request ID %d: No available paths from PathManager for %s %s. Responding with 503.", requestID, req.Method, req.URL.Path)
			http.Error(w, "Service unavailable: No routing paths found.", http.StatusServiceUnavailable)
			return
		}

		// Debug log for available paths (can be verbose)
		// for i, p := range paths {
		// 	log.Printf("[Access-TRACE] Path %d: %v (Latency: %d ms, Weight: %d)", i, p.IPList, p.Latency, p.Weight)
		// }

		wrr := router.NewWeightedRoundRobin(paths)
		nextPath := wrr.Next()
		if nextPath.IPList == nil || len(nextPath.IPList) == 0 {
			log.Printf("[Access-ERROR] Request ID %d: WeightedRoundRobin returned no valid next path for %s %s. Responding with 503.", requestID, req.Method, req.URL.Path)
			http.Error(w, "Service unavailable: Could not determine next hop.", http.StatusServiceUnavailable)
			return
		}
		log.Printf("[Access-INFO] Request ID %d: Selected path for %s %s: %v (Latency: %d ms, Weight: %d)", requestID, req.Method, req.URL.Path, nextPath.IPList, nextPath.Latency, nextPath.Weight)

		// HopList for the packet should be the selected path from the router
		currentHopList := nextPath.IPList

		header, err := packet.NewPacket(currentHopList, requestID) // Pass the selected path as HopList
		if err != nil {
			log.Printf("[Access-ERROR] Request ID %d: Failed to create new packet header: %v. HopList: %v. Responding with 500.", requestID, err, currentHopList)
			http.Error(w, "Internal server error: Failed to create packet header.", http.StatusInternalServerError)
			return
		}

		nextHopIP, isLastHop, err := header.GetNextHopIP()
		if err != nil {
			log.Printf("[Access-ERROR] Request ID %d: Failed to get next hop IP from header: %v. Header: %+v. Responding with 500.", requestID, err, header)
			http.Error(w, "Internal server error: Failed to determine next hop.", http.StatusInternalServerError)
			return
		}

		log.Printf("[Access-INFO] Request ID %d: Determined next hop: %s, IsLastHop: %v", requestID, nextHopIP, isLastHop)

		reqItem := &RequestItem{
			Request:          req,
			ResponseWriter:   w,
			RequestID:        requestID,
			ReceivedAt:       requestReceivedTime, // Use the time captured at the beginning of the handler
			ResponseReceived: make(chan struct{}),
			IsLastHop:        isLastHop,
			NextHopIP:        nextHopIP,
			HopList:          header.HopList, // This is the full path selected
		}

		if !isLastHop {
			// If not the last hop, we need to pack the header to be sent with the request data
			headerBytes, err := header.Pack()
			if err != nil {
				log.Printf("[Access-ERROR] Request ID %d: Failed to pack header for forwarding: %v. Header: %+v. Responding with 500.", requestID, err, header)
				http.Error(w, "Internal server error: Failed to pack forwarding header.", http.StatusInternalServerError)
				return
			}
			reqItem.HeaderBytes = headerBytes
			log.Printf("[Access-DEBUG] Request ID %d: Header packed for forwarding, size: %d bytes.", requestID, len(headerBytes))
		}

		select {
		case r.httpRequestChan <- reqItem:
			log.Printf("[Access-DEBUG] Request ID %d: Submitted to httpRequestChan for processing.", requestID)
		case <-time.After(5 * time.Second): // TODO: Make this timeout configurable
			log.Printf("[Access-ERROR] Request ID %d: Timeout submitting request to httpRequestChan. Channel may be full or blocked. Responding with 503.", requestID)
			http.Error(w, "Service temporarily unavailable: Request queue timeout.", http.StatusServiceUnavailable)
			return
		}

		// Wait for the response or timeout
		select {
		case <-reqItem.ResponseReceived:
			processingTime := time.Since(requestReceivedTime)
			log.Printf("[Access-INFO] Request ID %d: Response received and processed for %s %s. Total time: %s.", requestID, req.Method, req.URL.Path, processingTime)
		case <-time.After(30 * time.Second): // TODO: Make this timeout configurable
			processingTime := time.Since(requestReceivedTime)
			log.Printf("[Access-ERROR] Request ID %d: Timeout waiting for response for %s %s. Total time waited: %s. Responding with 504.", requestID, req.Method, req.URL.Path, processingTime)
			http.Error(w, "Gateway timeout: No response from upstream server.", http.StatusGatewayTimeout)
			return
		}
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", handler)

	log.Printf("[Access-INFO] Starting HTTP Proxy on port %s", r.accessConfig.HttpPort)

	srv := &http.Server{
		Addr:    ":" + r.accessConfig.HttpPort,
		Handler: mux,
		// TODO: Add other server configurations like ReadTimeout, WriteTimeout, IdleTimeout for robustness
	}

	err := srv.ListenAndServe()
	if err != nil && err != http.ErrServerClosed {
		log.Fatalf("[Access-CRITICAL] HTTP Proxy ListenAndServe on port %s failed: %v", r.accessConfig.HttpPort, err)
	} else if err == http.ErrServerClosed {
		log.Printf("[Access-INFO] HTTP Proxy on port %s has been gracefully shut down.", r.accessConfig.HttpPort)
	}
	log.Printf("[Access-INFO] HTTP Proxy on port %s stopped listening.", r.accessConfig.HttpPort) // This log might be confusing if shutdown was graceful.
}

func (r *Repository) StartTcpResponseProxy() {
	listenAddr := ":" + r.accessConfig.ResponsePort
	// Test if the port is available before attempting to listen indefinitely
	// This is a common pattern to fail fast if the port is already in use.
	testListener, err := net.Listen("tcp", listenAddr)
	if err != nil {
		log.Fatalf("[Access-CRITICAL] Pre-check failed: TCP Response Proxy cannot listen on port %s: %v", r.accessConfig.ResponsePort, err)
		return // Should not be reached due to Fatalf
	}
	testListener.Close() // Close the test listener immediately
	log.Printf("[Access-DEBUG] Port %s is available for TCP Response Proxy.", r.accessConfig.ResponsePort)

	listener, err := net.Listen("tcp", listenAddr)
	if err != nil {
		log.Fatalf("[Access-CRITICAL] TCP Response Proxy failed to listen on port %s: %v", r.accessConfig.ResponsePort, err)
	}
	defer listener.Close()

	log.Printf("[Access-INFO] TCP Response Proxy started and listening on port %s", r.accessConfig.ResponsePort)

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Printf("[Access-ERROR] TCP Response Proxy: Error accepting new connection: %v", err)
			// Check if the error is temporary or if the listener is closed
			if ne, ok := err.(net.Error); ok && !ne.Temporary() {
				log.Printf("[Access-INFO] TCP Response Proxy: Listener on port %s is likely closed. Shutting down accept loop.", r.accessConfig.ResponsePort)
				return // Stop accepting if the error is not temporary (e.g., listener closed)
			}
			// For temporary errors, log and continue
			log.Printf("[Access-WARN] TCP Response Proxy: Temporary error accepting connection: %v. Continuing...", err)
			continue
		}
		log.Printf("[Access-INFO] TCP Response Proxy: Accepted new connection from %s", conn.RemoteAddr().String())

		go r.handleTcpResponse(conn)
	}
}

func (r *Repository) handleTcpResponse(conn net.Conn) {
	remoteAddr := conn.RemoteAddr().String()
	log.Printf("[Access-INFO] Handling TCP response connection from %s", remoteAddr)

	// Use default SMUX config if nil is passed; consider making this configurable
	session, err := smux.Server(conn, connection.DefaultSmuxConfig()) // Using DefaultSmuxConfig from connection package
	if err != nil {
		log.Printf("[Access-ERROR] Failed to establish SMUX server session for %s: %v", remoteAddr, err)
		conn.Close() // Ensure connection is closed on SMUX setup failure
		return
	}
	defer session.Close()

	connection.AddServerSession(remoteAddr, session) // Assuming this logs its own success/failure if necessary
	log.Printf("[Access-INFO] SMUX server session established for %s.", remoteAddr)

	streamCount := 0
	for {
		stream, err := session.AcceptStream()
		if err != nil {
			if session.IsClosed() || err == io.EOF {
				log.Printf("[Access-INFO] SMUX session for %s closed or EOF reached while accepting stream. Streams handled: %d. Error: %v", remoteAddr, streamCount, err)
				break // Exit loop if session is closed or no more streams
			}
			log.Printf("[Access-ERROR] Error accepting SMUX stream for %s (Streams handled: %d): %v", remoteAddr, streamCount, err)
			break // Or continue, depending on desired behavior for other stream accept errors
		}

		streamCount++
		log.Printf("[Access-INFO] Accepted SMUX stream #%d for %s. Stream ID: %d", streamCount, remoteAddr, stream.ID())

		go r.handleResponseFromStream(stream)
	}

	connection.RemoveServerSession(remoteAddr, session) // Assuming this logs if necessary
	log.Printf("[Access-INFO] SMUX server session ended for %s. Total streams handled: %d", remoteAddr, streamCount)
}

func (r *Repository) handleResponseFromStream(stream *smux.Stream) {
	defer stream.Close()
	streamIDInfo := fmt.Sprintf("StreamID:%d (local)", stream.ID())
	log.Printf("[Access-INFO] Handling response from SMUX stream: %s", streamIDInfo)

	// Set a read deadline for the stream to prevent indefinite blocking
	// TODO: Make this timeout configurable
	stream.SetReadDeadline(time.Now().Add(30 * time.Second))

	// Initial buffer size, can be tuned.
	// Using bytes.Buffer for easier appending and final []byte conversion.
	var dataBuffer bytes.Buffer
	readBuf := make([]byte, 8192) // Temporary buffer for each Read call
	var totalRead int64

	for {
		n, err := stream.Read(readBuf)
		if n > 0 {
			if _, writeErr := dataBuffer.Write(readBuf[:n]); writeErr != nil {
				log.Printf("[Access-ERROR] Failed to write to dataBuffer from SMUX stream %s: %v", streamIDInfo, writeErr)
				// Depending on the error, may need to return or break
				return
			}
			totalRead += int64(n)
			log.Printf("[Access-TRACE] Read %d bytes from SMUX stream %s. Total read so far: %d bytes.", n, streamIDInfo, totalRead)
		}
		if err != nil {
			if err == io.EOF {
				log.Printf("[Access-INFO] EOF reached on SMUX stream %s after reading %d bytes.", streamIDInfo, totalRead)
			} else if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				log.Printf("[Access-WARN] Timeout reading from SMUX stream %s after %d bytes. Error: %v", streamIDInfo, totalRead, err)
			} else if err == io.ErrClosedPipe || strings.Contains(err.Error(), "stream closed") || strings.Contains(err.Error(), "session shutdown") {
				log.Printf("[Access-INFO] SMUX stream %s closed or session shut down while reading. Total read: %d bytes. Error: %v", streamIDInfo, totalRead, err)
			} else {
				log.Printf("[Access-ERROR] Error reading from SMUX stream %s after %d bytes: %v", streamIDInfo, totalRead, err)
			}
			break // Exit loop on any error (including EOF)
		}
		// No specific check for `n < len(buf)` as Read might fill the buffer and still not return EOF.
		// The loop continues until EOF or another error.
	}

	if totalRead == 0 {
		log.Printf("[Access-WARN] No data read from SMUX stream %s before it closed or errored.", streamIDInfo)
		// Do not send empty data to smuxResponseChan if nothing was read, unless it's an explicit signal.
		return
	}

	log.Printf("[Access-INFO] Finished reading from SMUX stream %s. Total data size: %d bytes.", streamIDInfo, totalRead)

	respItem := &ResponseItem{
		Data:       dataBuffer.Bytes(), // Get all collected bytes
		ReceivedAt: time.Now(),
	}

	select {
	case r.smuxResponseChan <- respItem:
		log.Printf("[Access-DEBUG] Sent %d bytes from SMUX stream %s to smuxResponseChan.", totalRead, streamIDInfo)
	case <-time.After(5 * time.Second): // TODO: Make this timeout configurable
		log.Printf("[Access-ERROR] Timeout sending data from SMUX stream %s (size: %d) to smuxResponseChan. Channel full or blocked?", streamIDInfo, totalRead)
	}
}

func (ap *AccessProxy) Start() {
	log.Println("[Access-INFO] Starting AccessProxy...")
	ap.repository.StartProcessors()

	go ap.repository.StartTcpResponseProxy()
	// StartHttpProxy is blocking, so it should be called last in this sequence if not run in a goroutine.
	// Or, if it's intended to be the main blocking call for the AccessProxy's Start():
	ap.repository.StartHttpProxy()
	log.Println("[Access-INFO] AccessProxy Start sequence completed (HTTP Proxy is now blocking)...")
}

func (ap *AccessProxy) Stop() {
	log.Println("[Access-INFO] Stopping AccessProxy...")
	ap.repository.Stop() // This already logs its actions
	log.Println("[Access-INFO] AccessProxy stopped.")
}

// AccessProxyfunc is a simple starter, consider renaming to be more descriptive like StartDefaultAccessProxy
func AccessProxyfunc() {
	log.Println("[Access-INFO] Initializing and starting AccessProxy with default configuration...")
	proxy := CreateAccessProxy(DefaultAccessConfig, DefaultRepositoryConfig)
	proxy.Start() // This will block if StartHttpProxy blocks
	// If Start() blocks, this log might not be reached until shutdown, or never if it's an infinite select {}
	log.Println("[Access-INFO] Default AccessProxy started and running.")
}

// AccessProxyWithFullConfig is a starter with explicit configuration
func AccessProxyWithFullConfig(accessConfig AccessConfig, repoConfig RepositoryConfig) {
	log.Printf("[Access-INFO] Initializing and starting AccessProxy with custom configuration. HTTP Port: %s, Response Port: %s", accessConfig.HttpPort, accessConfig.ResponsePort)
	proxy := CreateAccessProxy(accessConfig, repoConfig)
	proxy.Start() // This will block if StartHttpProxy blocks
	log.Println("[Access-INFO] Custom configured AccessProxy started and running.")
}
