package proxy

import (
	"bufio"
	"bytes"
	packet "data/packet.go"
	"data/path"
	"data/proxy/connection"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net"
	"net/http"
	"net/http/httputil"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/xtaci/smux"
)

var requestCounter uint32 = 0 // ，

func generateUniqueRequestID(r *http.Request) uint32 {

	counter := atomic.AddUint32(&requestCounter, 1)

	now := time.Now()
	seconds := now.Unix()
	nanos := now.Nanosecond()

	timeComponent := uint32(seconds) ^ uint32(nanos)

	randomComponent := uint32(rand.Intn(1000))

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

	stateManager := NewRequestStateManager(15*time.Minute, 1*time.Minute)

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

	repo.bufferManager.SetSendFunctions(
		repo.sendSingleRequest,
		repo.sendMergedRequest,
		nil,
	)

	return &AccessProxy{
		repository: repo,
		config:     config,
	}
}

func (r *Repository) StartProcessors() {
	r.wg.Add(2)

	go r.processHttpRequests()

	go r.processSmuxResponses()

	log.Println("[Repository] ")
}

func (r *Repository) Stop() {
	close(r.done)
	r.wg.Wait()

	if r.bufferManager != nil {
		r.bufferManager.Stop()
	}

	if r.stateManager != nil {
		r.stateManager.Stop()
	}

	log.Println("[Repository] ")
}

func (r *Repository) processHttpRequests() {
	defer r.wg.Done()

	log.Printf("[Repository] HTTP")

	workerCount := runtime.NumCPU() * 2
	if workerCount < 4 {
		workerCount = 4
	}

	log.Printf("[Repository]  %d HTTP", workerCount)

	for i := 0; i < workerCount; i++ {
		go func(workerID int) {
			for {
				select {
				case <-r.done:
					log.Printf("[Repository] HTTP #%d ", workerID)
					return
				case req, ok := <-r.httpRequestChan:
					if !ok {
						//
						return
					}

					reqState := &RequestState{
						RequestID:        req.RequestID,
						OriginalRequest:  req.Request,
						ResponseWriter:   req.ResponseWriter,
						RequestData:      []byte{},
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

					r.stateManager.AddState(reqState)

					if req.IsLastHop {

						go r.handleDirectProxy(reqState)
					} else {

						reqBytes, err := httputil.DumpRequest(req.Request, true)
						if err != nil {
							log.Printf("[Access-ERROR] : %v", err)
							r.stateManager.UpdateStatus(req.RequestID, StatusFailed)
							r.notifyRequestFailed(reqState, err)
							continue
						}

						reqState.mu.Lock()
						reqState.RequestData = reqBytes
						reqState.Size = len(reqBytes)
						reqState.mu.Unlock()

						err = r.bufferManager.ProcessRequest(reqState)
						if err != nil {
							log.Printf("[Access-ERROR] : %v", err)
							r.stateManager.UpdateStatus(req.RequestID, StatusFailed)
							r.notifyRequestFailed(reqState, err)
						}
					}
				}
			}
		}(i)
	}

	<-r.done
	log.Println("[Repository] HTTP")
}

func (r *Repository) handleDirectProxy(reqState *RequestState) {

	r.stateManager.UpdateStatus(reqState.RequestID, StatusSent)

	reqState.mu.RLock()
	req := reqState.OriginalRequest
	nextHopIP := reqState.NextHopIP
	reqState.mu.RUnlock()

	parts := bytes.Split([]byte(nextHopIP), []byte(":"))
	host := string(parts[0])
	port := "8080"
	if len(parts) > 1 {
		port = string(parts[1])
	}

	targetURL := fmt.Sprintf("http://%s:%s%s", host, port, req.URL.Path)

	var bodyBytes []byte
	if req.Body != nil {
		bodyBytes, err := io.ReadAll(req.Body)
		if err != nil {
			log.Printf("[Access-ERROR] : %v", err)
			r.stateManager.UpdateStatus(reqState.RequestID, StatusFailed)
			r.notifyRequestFailed(reqState, err)
			return
		}

		req.Body.Close()

		req.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
	}

	clonedReq, err := http.NewRequest(req.Method, targetURL, io.NopCloser(bytes.NewBuffer(bodyBytes)))
	if err != nil {
		log.Printf("[Access-ERROR] : %v", err)
		r.stateManager.UpdateStatus(reqState.RequestID, StatusFailed)
		r.notifyRequestFailed(reqState, err)
		return
	}

	for key, values := range req.Header {
		for _, value := range values {
			clonedReq.Header.Add(key, value)
		}
	}

	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	resp, err := client.Do(clonedReq)
	if err != nil {
		log.Printf("[Access-ERROR] : %v", err)
		r.stateManager.UpdateStatus(reqState.RequestID, StatusFailed)
		r.notifyRequestFailed(reqState, err)
		return
	}
	defer resp.Body.Close()

	r.stateManager.UpdateStatus(reqState.RequestID, StatusResponding)

	reqState.mu.RLock()
	responseWriter := reqState.ResponseWriter
	responseChan := reqState.ResponseReceived
	reqState.mu.RUnlock()

	for key, values := range resp.Header {
		for _, value := range values {
			responseWriter.Header().Add(key, value)
		}
	}

	responseWriter.WriteHeader(resp.StatusCode)

	_, err = io.Copy(responseWriter, resp.Body)
	if err != nil {
		log.Printf("[Access-ERROR] : %v", err)
		r.stateManager.UpdateStatus(reqState.RequestID, StatusFailed)
		r.notifyRequestFailed(reqState, err)
		return
	}

	r.stateManager.UpdateStatus(reqState.RequestID, StatusCompleted)

	close(responseChan)

	log.Printf("[Access] : %d", reqState.RequestID)
}

func (r *Repository) notifyRequestFailed(req *RequestState, err error) {
	if req == nil {
		log.Printf("[Access-ERROR] : req")
		return
	}

	req.mu.RLock()
	responseWriter := req.ResponseWriter
	responseChan := req.ResponseReceived
	req.mu.RUnlock()

	if responseWriter == nil {
		log.Printf("[Access-ERROR] : responseWriter")
		return
	}

	errorResp := &http.Response{
		Status:     "Internal Server Error",
		StatusCode: http.StatusInternalServerError,
		Body:       io.NopCloser(bytes.NewReader([]byte(fmt.Sprintf(": %v", err)))),
		Header:     make(http.Header),
	}

	responseWriter.WriteHeader(errorResp.StatusCode)
	io.Copy(responseWriter, errorResp.Body)

	close(responseChan)
}

func (r *Repository) processSmuxResponses() {
	defer r.wg.Done()

	workerCount := runtime.NumCPU() * 2
	if workerCount < 4 {
		workerCount = 4
	}

	log.Printf("[Repository] SMUX，: %d", workerCount)

	var wg sync.WaitGroup
	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			for {
				select {
				case <-r.done:
					log.Printf("[Repository] SMUX #%d ", workerID)
					return
				case resp, ok := <-r.smuxResponseChan:
					if !ok {
						//
						return
					}

					log.Printf("[ACCESS-RESP]  #%d SMUX: =%d, =%d/%d",
						workerID, len(resp.Data), len(r.smuxResponseChan), cap(r.smuxResponseChan))

					if len(resp.Data) < 4 {
						log.Printf("[Access-ERROR] ，")
						continue
					}

					headerLen := uint16(resp.Data[2])<<8 | uint16(resp.Data[3])
					if int(headerLen) > len(resp.Data) {
						log.Printf("[Access-ERROR] : %d (: %d)", headerLen, len(resp.Data))
						continue
					}

					headerBytes := resp.Data[:headerLen]
					responseData := resp.Data[headerLen:]

					header, err := packet.Unpack(headerBytes)
					if err != nil {
						log.Printf("[Access-ERROR] : %v", err)
						continue
					}

					log.Printf("[ACCESS-RESP] %d，", header.PacketCount)

					positions := packet.GetRequestPositions(header, len(responseData))

					for i := 0; i < int(header.PacketCount); i++ {
						requestID := header.PacketID[i]
						respData := responseData[positions[i]:positions[i+1]]

						reqState, exists := r.stateManager.GetState(requestID)
						if !exists {
							log.Printf("[ACCESS-WARN] ID %d ", requestID)
							continue
						}

						r.stateManager.UpdateStatus(requestID, StatusResponding)

						respReader := bufio.NewReader(bytes.NewReader(respData))
						httpResp, err := http.ReadResponse(respReader, nil)
						if err != nil {
							log.Printf("[ACCESS-ERROR] HTTP: %v", err)
							r.stateManager.UpdateStatus(requestID, StatusFailed)
							continue
						}

						reqState.mu.RLock()
						responseWriter := reqState.ResponseWriter
						responseChan := reqState.ResponseReceived
						reqState.mu.RUnlock()

						if responseWriter != nil {
							err = r.sendResponseToClient(responseWriter, httpResp)
							if err != nil {
								log.Printf("[ACCESS-ERROR] : %v", err)
								r.stateManager.UpdateStatus(requestID, StatusFailed)
								continue
							}

							r.stateManager.UpdateStatus(requestID, StatusCompleted)
							if responseChan != nil {
								close(responseChan)
							}

							log.Printf("[ACCESS-RESP]  %d ", requestID)
						}
					}
				}
			}
		}(i)
	}

	<-r.done

	wg.Wait()
	log.Println("[Repository] SMUX")
}

func (r *Repository) sendResponseToClient(w http.ResponseWriter, resp *http.Response) error {

	for key, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}

	w.WriteHeader(resp.StatusCode)

	if resp.Body != nil {
		defer resp.Body.Close()
		_, err := io.Copy(w, resp.Body)
		return err
	}

	return nil
}

func (r *Repository) sendSingleRequest(data []byte, nextHopIP string, requestID uint32, request *RequestState) error {

	parts := bytes.Split([]byte(nextHopIP), []byte(":"))

	ip := string(parts[0])
	port := "50056"
	if len(parts) > 1 {
		port = string(parts[1])
	}

	targetAddr := ip + ":" + port

	log.Printf("[Access-DEBUG] SMUX，: %s, ID: %d", targetAddr, requestID)

	session, err := connection.GetOrCreateClientSession(targetAddr)
	if err != nil {
		log.Printf("[Access-ERROR] SMUX: %v", err)
		return err
	}

	stream, err := session.OpenStream()
	if err != nil {
		log.Printf("[Access-ERROR] SMUX: %v", err)

		connection.RemoveClientSession(targetAddr, session)

		session, err = connection.GetOrCreateClientSession(targetAddr)
		if err != nil {
			log.Printf("[Access-ERROR] SMUX: %v", err)
			return err
		}

		stream, err = session.OpenStream()
		if err != nil {
			log.Printf("[Access-ERROR] SMUX: %v", err)
			return err
		}
	}
	defer stream.Close()

	var headerBytes []byte
	request.mu.RLock()
	hopList := request.HopList
	isLastHop := request.IsLastHop
	request.mu.RUnlock()

	if !isLastHop {

		header := &packet.Packet{
			PacketCount: 1,
			PacketID:    []uint32{requestID},
			HopList:     hopList,
			HopCounts:   0,
		}

		var err error
		headerBytes, err = header.Pack()
		if err != nil {
			log.Printf("[Access-ERROR] : %v", err)
			return err
		}
	}

	fullData := append(headerBytes, data...)
	log.Printf("[Access-DEBUG]  %s，: %d", targetAddr, len(fullData))

	_, err = stream.Write(fullData)
	if err != nil {
		log.Printf("[Access-ERROR] : %v", err)
		return err
	}

	log.Printf("[Access-DEBUG] ，，ID: %d", requestID)
	return nil
}

func (r *Repository) sendMergedRequest(mergedData []byte, nextHopIP string, updatedHeader *packet.Packet) error {

	parts := bytes.Split([]byte(nextHopIP), []byte(":"))
	ip := string(parts[0])
	port := "50056"
	if len(parts) > 1 {
		port = string(parts[1])
	}

	targetAddr := ip + ":" + port
	log.Printf("[Access-DEBUG] SMUX，: %s", targetAddr)

	var modifiedData []byte

	if len(mergedData) >= 4 && updatedHeader != nil {
		headerLen := uint16(mergedData[2])<<8 | uint16(mergedData[3])
		if int(headerLen) <= len(mergedData) {
			requestBytes := mergedData[headerLen:]

			log.Printf("[Access-DEBUG] header，HopCounts=%d", updatedHeader.HopCounts)

			newHeaderBytes, err := updatedHeader.Pack()
			if err == nil {

				modifiedData = append(newHeaderBytes, requestBytes...)
				log.Printf("[Access-DEBUG] header, HopCounts=%d", updatedHeader.HopCounts)
			}
		}
	}

	if modifiedData == nil {
		log.Printf("[Access-WARN] header，")
		modifiedData = mergedData
	}

	session, err := connection.GetOrCreateClientSession(targetAddr)
	if err != nil {
		log.Printf("[Access-ERROR] SMUX: %v", err)
		return err
	}

	stream, err := session.OpenStream()
	if err != nil {
		// ...
		return err
	}
	defer stream.Close()

	_, err = stream.Write(modifiedData)
	if err != nil {
		log.Printf("[Access-ERROR] : %v", err)
		return err
	}

	log.Printf("[Access-DEBUG] ，")
	return nil
}

func (r *Repository) StartHttpProxy() {
	handler := func(w http.ResponseWriter, req *http.Request) {
		log.Printf("[Access] : %s %s", req.Method, req.URL.Path)

		requestID := generateUniqueRequestID(req)

		pathManager := path.GetInstance()
		paths := pathManager.GetPaths()

		for i, p := range paths {

			if i == 3 {
				log.Printf(" %d: %v (: %d)", i, p.IPList, p.Latency)
				HopList = p.IPList
			}
		}
		wrr := path.NewWeightedRoundRobin(paths)
		nextPath := wrr.Next()
		log.Printf(": %v, : %d", nextPath.IPList, nextPath.Latency)

		HopList = nextPath.IPList

		header, err := packet.NewPacket(HopList, requestID)
		if err != nil {
			log.Printf("[Access-ERROR] header: %v", err)
			log.Printf("[Access-DEBUG] HopList: %v", HopList)
			http.Error(w, "", http.StatusInternalServerError)
			return
		}

		nextHopIP, isLastHop, err := header.GetNextHopIP()
		if err != nil {
			log.Printf("[Access-ERROR] : %v", err)
			log.Printf("[Access-DEBUG] Header: %+v", header)
			http.Error(w, "", http.StatusInternalServerError)
			return
		}

		log.Printf("[Access] : %s, : %v", nextHopIP, isLastHop)

		reqItem := &RequestItem{
			Request:          req,
			ResponseWriter:   w,
			RequestID:        requestID,
			ReceivedAt:       time.Now(),
			ResponseReceived: make(chan struct{}),
			IsLastHop:        isLastHop,
			NextHopIP:        nextHopIP,
			HopList:          header.HopList,
		}

		if !isLastHop {
			headerBytes, err := header.Pack()
			if err != nil {
				log.Printf("[Access-ERROR] Header: %v", err)
				log.Printf("[Access-DEBUG] Header: %+v", header)
				http.Error(w, "", http.StatusInternalServerError)
				return
			}
			reqItem.HeaderBytes = headerBytes
		}

		select {
		case r.httpRequestChan <- reqItem:
		case <-time.After(5 * time.Second):
			log.Printf("[Access-ERROR] ， %d", requestID)
			http.Error(w, "，", http.StatusServiceUnavailable)
			return
		}

		select {
		case <-reqItem.ResponseReceived:
		case <-time.After(30 * time.Second):
			log.Printf("[Access-ERROR]  %d ", requestID)
			http.Error(w, "", http.StatusGatewayTimeout)
			return
		}
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", handler)

	log.Printf("[Access] HTTP，: %s", r.accessConfig.HttpPort)

	err := http.ListenAndServe(":"+r.accessConfig.HttpPort, mux)
	if err != nil {
		log.Fatalf("[Access-ERROR] HTTP: %v", err)
	}
	log.Printf("[Access] HTTP") // ，ListenAndServe
}

func (r *Repository) StartTcpResponseProxy() {

	testListener, err := net.Listen("tcp", ":"+r.accessConfig.ResponsePort)
	if err != nil {
		log.Fatalf("[Access-ERROR]  %s : %v", r.accessConfig.ResponsePort, err)
		return
	}
	testListener.Close()

	listener, err := net.Listen("tcp", ":"+r.accessConfig.ResponsePort)
	if err != nil {
		log.Fatalf("[Access-ERROR] %s: %v", r.accessConfig.ResponsePort, err)
	}
	defer listener.Close()

	log.Printf("[Access] ，: %s", r.accessConfig.ResponsePort)

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Println("[Access-ERROR] :", err)

			if ne, ok := err.(net.Error); ok {
				log.Printf("[Access-ERROR] : =%v, =%v",
					ne.Temporary(), ne.Timeout())
			}
			continue
		}
		log.Printf("[Access]  %s ", conn.RemoteAddr().String())

		go r.handleTcpResponse(conn)
	}
}

func (r *Repository) handleTcpResponse(conn net.Conn) {
	remoteAddr := conn.RemoteAddr().String()
	log.Printf("[Access]  %s ", remoteAddr)

	session, err := smux.Server(conn, nil)
	if err != nil {
		log.Printf("[Access-ERROR] SMUX: %v", err)
		conn.Close()
		return
	}
	defer session.Close()

	connection.AddServerSession(remoteAddr, session)
	log.Printf("[Access] : %s", remoteAddr)

	streamCount := 0
	for {
		stream, err := session.AcceptStream()
		if err != nil {
			if err == io.EOF || session.IsClosed() {
				log.Printf("[Access] : %s", remoteAddr)
				break
			}
			log.Printf("[Access-ERROR] SMUX: %v", err)
			break
		}

		streamCount++
		log.Printf("[Access]  #%d: %s", streamCount, remoteAddr)

		go r.handleResponseFromStream(stream)
	}

	connection.RemoveServerSession(remoteAddr, session)
	log.Printf("[Access] : %s", remoteAddr)
}

func (r *Repository) handleResponseFromStream(stream *smux.Stream) {
	defer stream.Close()
	streamID := fmt.Sprintf("%p", stream)
	log.Printf("[Access] : %s", streamID)

	stream.SetReadDeadline(time.Now().Add(30 * time.Second))

	buf := make([]byte, 8192)
	var data []byte
	var totalRead int

	for {
		n, err := stream.Read(buf)
		if err != nil {
			if err == io.EOF {
				log.Printf("[Access] : %d ", totalRead)
				break
			}
			log.Printf("[Access-ERROR] : %v", err)
			return
		}
		data = append(data, buf[:n]...)
		totalRead += n
		log.Printf("[Access] : %d ", totalRead)

		if totalRead > len(buf)-1024 {
			newBuf := make([]byte, len(buf)*2)
			copy(newBuf, buf)
			buf = newBuf
		}

		if n < len(buf) {
			break
		}
		log.Printf("[ACCESS-RESP] : =%d, =%d",
			n, totalRead)
	}

	log.Printf("[ACCESS-RESP] : =%d, ",
		totalRead)

	respItem := &ResponseItem{
		Data:       data,
		ReceivedAt: time.Now(),
	}

	r.smuxResponseChan <- respItem
}

func (ap *AccessProxy) Start() {

	ap.repository.StartProcessors()

	go ap.repository.StartTcpResponseProxy()

	ap.repository.StartHttpProxy()
}

func (ap *AccessProxy) Stop() {
	ap.repository.Stop()
}

func AccessProxyfunc() {
	proxy := CreateAccessProxy(DefaultAccessConfig, DefaultRepositoryConfig)
	proxy.Start()
}

func AccessProxyWithFullConfig(accessConfig AccessConfig, repoConfig RepositoryConfig) {
	proxy := CreateAccessProxy(accessConfig, repoConfig)
	proxy.Start()
}
