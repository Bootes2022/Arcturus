package proxy

import (
	"bufio"
	"bytes"
	packet "data/packet.go"
	"data/proxy/connection"
	"fmt"
	"github.com/xtaci/smux"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"runtime"
	"strings"
	"sync"
	"time"
)

type ResponseData struct {
	RequestID uint32
	Data      []byte
	HopList   []uint32 // HopList
}

type RelayRepositoryConfig struct {
	ProcessingInterval time.Duration
	RequestBufferSize  int
	ResponseBufferSize int
}

var DefaultRelayRepositoryConfig = RelayRepositoryConfig{
	ProcessingInterval: 10 * time.Millisecond,
	RequestBufferSize:  100,
	ResponseBufferSize: 100,
}

type RelayRepository struct {
	requestChan  chan *RelayRequestItem
	responseChan chan *RelayResponseItem

	done chan struct{}
	wg   sync.WaitGroup

	config      RelayRepositoryConfig
	relayConfig RelayConfig

	bufferManager *BufferManager

	stateManager *RequestStateManager
}

type RelayRequestItem struct {
	Data       []byte
	Stream     *smux.Stream
	ReceivedAt time.Time
	RemoteAddr string
}

type RelayResponseItem struct {
	Data       []byte
	ReceivedAt time.Time
	RemoteAddr string
}

type RelayConfig struct {
	RequestPort  string //  (50056)
	ResponsePort string //  (50057)

	RelayPort  string // Relay (50056)
	SourcePort string //  (8080)

	AccessResponsePort string // Access (50054)
	RelayResponsePort  string // Relay (50057)
}

var DefaultRelayConfig = RelayConfig{
	RequestPort:        "50056",
	ResponsePort:       "50057",
	RelayPort:          "50056",
	SourcePort:         "8080",
	AccessResponsePort: "50054",
	RelayResponsePort:  "50057",
}

type RelayProxy struct {
	repository *RelayRepository
	config     RelayConfig
}

func CreateRelayProxy(relayConfig RelayConfig, repoConfig RelayRepositoryConfig) *RelayProxy {

	stateManager := NewRequestStateManager(15*time.Minute, 1*time.Minute)

	repo := &RelayRepository{
		requestChan:  make(chan *RelayRequestItem, repoConfig.RequestBufferSize),
		responseChan: make(chan *RelayResponseItem, repoConfig.ResponseBufferSize),
		done:         make(chan struct{}),
		config:       repoConfig,
		relayConfig:  relayConfig,
		stateManager: stateManager,
	}

	bufferConfig := DefaultBufferConfig()
	repo.bufferManager = NewBufferManager(bufferConfig, stateManager)

	repo.bufferManager.SetSendFunctions(
		repo.sendSingleRequest,
		repo.sendMergedRequest,
		repo.forwardResponseToPreviousHop,
	)

	return &RelayProxy{
		repository: repo,
		config:     relayConfig,
	}
}

func (r *RelayRepository) StartProcessors() {
	r.wg.Add(2)

	go r.processRequests()

	go r.processResponses()

	log.Println("[RelayRepository] ")
}

func (r *RelayRepository) Stop() {
	close(r.done)
	r.wg.Wait()

	if r.bufferManager != nil {
		r.bufferManager.Stop()
	}

	if r.stateManager != nil {
		r.stateManager.Stop()
	}

	log.Println("[RelayRepository] ")
}

func (r *RelayRepository) processRequests() {
	defer r.wg.Done()

	workerCount := runtime.NumCPU() * 2
	if workerCount < 4 {
	}

	log.Printf("[RelayRepository]  %d ", workerCount)

	for i := 0; i < workerCount; i++ {
		go func(workerID int) {
			for {
				select {
				case <-r.done:
					log.Printf("[RelayRepository]  #%d ", workerID)
					return
				case req := <-r.requestChan:
					go r.processRequestWithTargetRouting(req.Data, req.Stream, req.RemoteAddr)
				}
			}
		}(i)
	}

	<-r.done
	log.Println("[RelayRepository] ")
}

func (r *RelayRepository) processRequestWithTargetRouting(data []byte, responseStream *smux.Stream, remoteAddr string) {

	headerLen := uint16(data[2])<<8 | uint16(data[3])

	headerBytes := data[:headerLen]
	requestBytes := data[headerLen:]

	header, err := packet.Unpack(headerBytes)
	if err != nil {
		log.Printf("[Relay-ERROR] : %v", err)
		return
	}

	log.Printf("[Relay] ，HopCounts=%d", header.HopCounts)

	header.IncrementHopCounts()
	log.Printf("[Relay] ，HopCounts=%d", header.HopCounts)

	nextHopIP, isLastHop, err := header.GetNextHopIP()
	if err != nil {
		log.Printf("[Relay-ERROR] : %v", err)
		return
	}

	var requestStates []*RequestState

	if header.PacketCount > 1 {
		positions := packet.GetRequestPositions(header, len(requestBytes))

		for i := 0; i < int(header.PacketCount); i++ {
			requestID := header.PacketID[i]
			reqData := requestBytes[positions[i]:positions[i+1]]

			reqState := &RequestState{
				RequestID:        requestID,
				OriginalRequest:  nil, // HTTP
				ResponseWriter:   nil, // ResponseWriter
				RequestData:      reqData,
				Size:             len(reqData),
				Status:           StatusCreated,
				CreatedAt:        time.Now(),
				LastUpdatedAt:    time.Now(),
				NextHopIP:        nextHopIP,
				HopList:          header.HopList,
				IsLastHop:        isLastHop,
				ResponseReceived: make(chan struct{}),
				BufferID:         "",
				MergeGroupID:     0,
				UpdatedHeader:    header,
			}

			r.stateManager.AddState(reqState)
			requestStates = append(requestStates, reqState)
		}
	} else {
		requestID := header.PacketID[0]

		reqState := &RequestState{
			RequestID:        requestID,
			OriginalRequest:  nil, // HTTP
			ResponseWriter:   nil, // ResponseWriter
			RequestData:      requestBytes,
			Size:             len(requestBytes),
			Status:           StatusCreated,
			CreatedAt:        time.Now(),
			LastUpdatedAt:    time.Now(),
			NextHopIP:        nextHopIP,
			HopList:          header.HopList,
			IsLastHop:        isLastHop,
			ResponseReceived: make(chan struct{}),
			BufferID:         "",
			MergeGroupID:     0,
			UpdatedHeader:    header,
		}

		r.stateManager.AddState(reqState)
		requestStates = append(requestStates, reqState)
	}

	// (isLastHop)
	if isLastHop {
		log.Printf("[Relay] (=%d)，，", header.PacketCount)

		var wg sync.WaitGroup
		for i, reqState := range requestStates {
			wg.Add(1)
			go func(state *RequestState, idx int) {
				defer wg.Done()

				respData, err := r.handleSingleDirectRequest(state)
				if err != nil {
					log.Printf("[Relay-ERROR] : %v", err)
					r.stateManager.UpdateStatus(state.RequestID, StatusFailed)
					return
				}

				err = r.bufferManager.ProcessResponse(respData)
				if err != nil {
					log.Printf("[Relay-ERROR] : %v", err)
				}

				log.Printf("[Relay]  %d ( %d) ", state.RequestID, idx)
			}(reqState, i)
		}

		wg.Wait()
		log.Printf("[Relay] ，")
	} else {
		log.Printf("[Relay] (=%d)，: %s", header.PacketCount, nextHopIP)

		updatedHeaderBytes, err := header.Pack()
		if err != nil {
			log.Printf("[Relay-ERROR] header: %v", err)
			return
		}

		// header
		requestBytes := append([]byte{}, data[headerLen:]...)
		mergedData := append(updatedHeaderBytes, requestBytes...)

		err = r.forwardToNextHop(mergedData, nextHopIP, header)
		if err != nil {
			log.Printf("[Relay-ERROR] : %v", err)
		}
	}
}

// forwardToNextHop
func (r *RelayRepository) forwardToNextHop(mergedData []byte, nextHopIP string, header *packet.Packet) error {
	// NextHopIP
	parts := strings.Split(nextHopIP, ":")
	ip := parts[0]

	// isLastHop
	_, isLastHop, _ := header.GetNextHopIP()
	var port string

	if len(parts) > 1 {
		port = parts[1]
	} else if isLastHop {
		port = r.relayConfig.SourcePort
	} else {
		port = r.relayConfig.RelayPort
	}

	targetAddr := ip + ":" + port
	log.Printf("[Relay] %s，: %v，: %s",
		ip, isLastHop, port)

	session, err := connection.GetOrCreateClientSession(targetAddr)
	if err != nil {
		log.Printf("[Relay-ERROR] SMUX: %v", err)
		return err
	}

	stream, err := session.OpenStream()
	if err != nil {
		log.Printf("[Relay-ERROR] SMUX: %v", err)

		connection.RemoveClientSession(targetAddr, session)

		session, err = connection.GetOrCreateClientSession(targetAddr)
		if err != nil {
			log.Printf("[Relay-ERROR] SMUX: %v", err)
			return err
		}

		stream, err = session.OpenStream()
		if err != nil {
			log.Printf("[Relay-ERROR] SMUX: %v", err)
			return err
		}
	}
	defer stream.Close()

	_, err = stream.Write(mergedData)
	if err != nil {
		log.Printf("[Relay-ERROR] : %v", err)
		return err
	}

	log.Printf("[Relay]  %s，: %d ",
		targetAddr, len(mergedData))

	return nil
}

func (r *RelayRepository) handleSingleDirectRequest(reqState *RequestState) (*ResponseData, error) {

	r.stateManager.UpdateStatus(reqState.RequestID, StatusSent)

	reqState.mu.RLock()
	requestID := reqState.RequestID
	requestData := reqState.RequestData
	nextHopIP := reqState.NextHopIP
	reqState.mu.RUnlock()

	log.Printf("[Relay] : ID=%d, =%d ",
		requestID, len(requestData))

	reader := bytes.NewReader(requestData)
	bufReader := bufio.NewReader(reader)
	httpReq, err := http.ReadRequest(bufReader)
	if err != nil {
		log.Printf("[Relay-ERROR] HTTP: %v", err)
		r.stateManager.UpdateStatus(requestID, StatusFailed)
		return nil, fmt.Errorf("HTTP: %v", err)
	}

	parts := bytes.Split([]byte(nextHopIP), []byte(":"))
	host := string(parts[0])
	var port string
	if len(parts) > 1 {
		port = string(parts[1])
	} else {

		port = r.relayConfig.SourcePort
	}

	targetURL := fmt.Sprintf("http://%s:%s%s", host, port, httpReq.URL.Path)
	log.Printf("[Relay] URL: %s, ID: %d", targetURL, requestID)

	destURL, err := url.Parse(targetURL)
	if err != nil {
		log.Printf("[Relay-ERROR] URL: %v", err)
		r.stateManager.UpdateStatus(requestID, StatusFailed)
		return nil, fmt.Errorf("URL: %v", err)
	}
	httpReq.URL = destURL
	httpReq.RequestURI = ""

	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	httpResp, err := client.Do(httpReq)
	if err != nil {
		log.Printf("[Relay-ERROR] : %v", err)
		r.stateManager.UpdateStatus(requestID, StatusFailed)
		return nil, fmt.Errorf(": %v", err)
	}
	defer httpResp.Body.Close()

	r.stateManager.UpdateStatus(requestID, StatusResponding)

	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		log.Printf("[Relay-ERROR] : %v", err)
		r.stateManager.UpdateStatus(requestID, StatusFailed)
		return nil, fmt.Errorf(": %v", err)
	}
	log.Printf("[RESP-TRACE] : ID=%d, =%d, =%d",
		requestID, httpResp.StatusCode, len(respBody))

	clonedResp := &http.Response{
		Status:        httpResp.Status,
		StatusCode:    httpResp.StatusCode,
		Proto:         httpResp.Proto,
		ProtoMajor:    httpResp.ProtoMajor,
		ProtoMinor:    httpResp.ProtoMinor,
		Header:        httpResp.Header.Clone(),
		Body:          io.NopCloser(bytes.NewReader(respBody)),
		ContentLength: int64(len(respBody)),
	}

	var respBuffer bytes.Buffer
	err = clonedResp.Write(&respBuffer)
	if err != nil {
		log.Printf("[Relay-ERROR] HTTP: %v", err)

		r.stateManager.UpdateStatus(requestID, StatusFailed)
		return nil, fmt.Errorf("HTTP: %v", err)
	}

	r.stateManager.UpdateStatus(requestID, StatusCompleted)

	log.Printf("[Relay] : ID=%d, =%d ",
		requestID, respBuffer.Len())

	return &ResponseData{
		RequestID: reqState.RequestID,
		Data:      respBuffer.Bytes(),
		HopList:   reqState.HopList, // HopList
	}, nil

}

func (r *RelayRepository) sendSingleRequest(data []byte, nextHopIP string, requestID uint32, request *RequestState) error {

	parts := bytes.Split([]byte(nextHopIP), []byte(":"))
	ip := string(parts[0])

	var port string
	if len(parts) > 1 {

		port = string(parts[1])
	} else {

		var isLastHop bool
		if request != nil {
			request.mu.RLock()
			isLastHop = request.IsLastHop
			request.mu.RUnlock()
		}

		if isLastHop {
			port = "8080"
		} else {
			port = "50056"
		}
	}

	targetAddr := ip + ":" + port
	log.Printf("[Relay] : %s (), ID: %d", targetAddr, requestID)

	session, err := connection.GetOrCreateClientSession(targetAddr)
	if err != nil {
		log.Printf("[Relay-ERROR] SMUX: %v", err)

		return err

	}

	stream, err := session.OpenStream()
	if err != nil {
		log.Printf("[Relay-ERROR] SMUX: %v", err)

		connection.RemoveClientSession(targetAddr, session)

		session, err = connection.GetOrCreateClientSession(targetAddr)
		if err != nil {
			log.Printf("[Relay-ERROR] SMUX: %v", err)

			return err

		}

		stream, err = session.OpenStream()
		if err != nil {
			log.Printf("[Relay-ERROR] SMUX: %v", err)

			return err

		}
	}
	defer stream.Close()

	var finalData []byte
	if request != nil {

		request.mu.RLock()

		originalData := request.RequestData

		request.mu.RUnlock()

		headerLen := uint16(originalData[2])<<8 | uint16(originalData[3])
		if int(headerLen) <= len(originalData) {

			headerBytes := originalData[:headerLen]

			originalHeader, err := packet.Unpack(headerBytes)
			if err == nil {

				originalHeader.PacketID = []uint32{requestID}

				newHeaderBytes, err := originalHeader.Pack()
				if err == nil {

					finalData = append(newHeaderBytes, data...)
					log.Printf("[Relay] header HopCounts: %d",
						originalHeader.HopCounts)
				}
			}
		}
	} else {

		finalData = data
	}

	_, err = stream.Write(finalData)
	if err != nil {

		log.Printf("[Relay-ERROR] : %v", err)
		return err
	}

	if request != nil {
		r.stateManager.UpdateStatus(requestID, StatusSent)

	}

	log.Printf("[Relay] : %d ", len(finalData))
	return nil
}

func (r *RelayRepository) sendMergedRequest(mergedData []byte, nextHopIP string, updatedHeader *packet.Packet) error {

	parts := bytes.Split([]byte(nextHopIP), []byte(":"))
	ip := string(parts[0])

	var port string
	if len(parts) > 1 {

		port = string(parts[1])
	} else {

		_, isLastHop, _ := updatedHeader.GetNextHopIP()

		if isLastHop {
			port = "8080"
		} else {
			port = "50056"
		}
	}

	targetAddr := ip + ":" + port
	log.Printf("[Relay] : %s (), : %d ", targetAddr, len(mergedData))

	var modifiedData []byte

	if len(mergedData) >= 4 && updatedHeader != nil {
		headerLen := uint16(mergedData[2])<<8 | uint16(mergedData[3])
		if int(headerLen) <= len(mergedData) {
			requestBytes := mergedData[headerLen:]

			log.Printf("[Relay] header，HopCounts=%d",
				updatedHeader.HopCounts)

			newHeaderBytes, err := updatedHeader.Pack()
			if err == nil {

				newHeader, verifyErr := packet.Unpack(newHeaderBytes)
				if verifyErr == nil {
					log.Printf("[Relay-%s] header，HopCounts=%d", newHeader.HopCounts)
				}

				modifiedData = append(newHeaderBytes, requestBytes...)
			}
		}
	}

	if modifiedData == nil {
		log.Printf("[Relay-WARN] header，")
		modifiedData = mergedData

	}

	session, err := connection.GetOrCreateClientSession(targetAddr)
	if err != nil {
		log.Printf("[Relay-ERROR] SMUX: %v", err)
		return err
	}

	stream, err := session.OpenStream()
	if err != nil {
		log.Printf("[Relay-ERROR] SMUX: %v", err)

		connection.RemoveClientSession(targetAddr, session)
		session, err = connection.GetOrCreateClientSession(targetAddr)
		if err != nil {

			log.Printf("[Relay-ERROR] SMUX: %v", err)
			return err

		}
		stream, err = session.OpenStream()
		if err != nil {
			log.Printf("[Relay-ERROR] SMUX: %v", err)
			return err
		}
	}
	defer stream.Close()

	_, err = stream.Write(modifiedData)
	if err != nil {
		log.Printf("[Relay-ERROR] : %v", err)
		return err
	}

	log.Printf("[Relay] %s: %d ", targetAddr, len(modifiedData))
	return nil
}

func (r *RelayRepository) forwardResponseToPreviousHop(previousHopIP string, headerBytes []byte, responseBytes []byte) error {

	header, err := packet.Unpack(headerBytes)
	if err != nil {
		return err
	}

	parts := strings.Split(previousHopIP, ":")
	ip := parts[0]

	var port string
	if len(parts) > 1 {
		port = parts[1]
	} else if header.HopCounts == 0 {
		port = r.relayConfig.AccessResponsePort
	} else {
		port = r.relayConfig.RelayResponsePort
	}

	targetAddr := ip + ":" + port
	log.Printf("[Relay] %s，HopCounts: %d，: %s",
		ip, header.HopCounts, port)

	session, err := connection.GetOrCreateClientSession(targetAddr)
	if err != nil {
		log.Printf("[Relay-ERROR] SMUX: %v", err)
		return err
	}
	log.Printf("[RESP-TRACE] : =%s, =%d",
		targetAddr, len(headerBytes)/4)

	stream, err := session.OpenStream()
	if err != nil {
		log.Printf("[Relay-ERROR] SMUX: %v", err)

		connection.RemoveClientSession(targetAddr, session)

		session, err = connection.GetOrCreateClientSession(targetAddr)
		if err != nil {
			log.Printf("[Relay-ERROR] SMUX: %v", err)
			return err
		}

		stream, err = session.OpenStream()
		if err != nil {
			log.Printf("[Relay-ERROR] SMUX: %v", err)
			return err
		}
	}
	defer stream.Close()

	responseData := append(headerBytes, responseBytes...)

	_, err = stream.Write(responseData)
	if err != nil {
		log.Printf("[Relay-ERROR] : %v", err)
		return err

	}
	log.Printf("[RESP-TRACE] : =%d, =%d, =%d",
		len(headerBytes), len(responseBytes), len(responseData))

	log.Printf("[Relay] : %d ，=%s，=%d，=%d",
		len(responseData), targetAddr, len(headerBytes), len(responseBytes))
	return nil
}

func (r *RelayRepository) processResponses() {
	defer r.wg.Done()

	log.Printf("[RelayRepository] ")

	workerCount := runtime.NumCPU() * 2
	if workerCount < 4 {
		workerCount = 4
	}

	for i := 0; i < workerCount; i++ {
		go func(workerID int) {
			for {
				select {
				case <-r.done:

					return
				case resp := <-r.responseChan:

					r.handleResponse(resp.Data, resp.RemoteAddr)
				}
			}
		}(i)
	}

	<-r.done
}

func (r *RelayRepository) handleResponse(data []byte, remoteAddr string) {

	if len(data) < 4 {

		log.Printf("[Relay-ERROR] ，")

		return
	}

	headerLen := uint16(data[2])<<8 | uint16(data[3])

	if int(headerLen) > len(data) {
		log.Printf("[Relay-ERROR] : %d (: %d)", headerLen, len(data))
		return
	}

	headerBytes := data[:headerLen]
	responseBytes := data[headerLen:]

	header, err := packet.Unpack(headerBytes)
	if err != nil {
		log.Printf("[Relay-ERROR] : %v", err)
		return
	}

	log.Printf("[Relay] ，HopCounts=%d", header.HopCounts)

	header.DecrementHopCounts()
	log.Printf("[Relay] ，HopCounts=%d", header.HopCounts)

	updatedHeaderBytes, err := header.Pack()
	if err != nil {
		log.Printf("[Relay-ERROR] header: %v", err)
		return
	}

	previousHopIP, _, err := header.GetPreviousHopIP()
	if err != nil {
		log.Printf("[Relay-ERROR] : %v", err)
		return
	}

	log.Printf("[Relay] : %d，: %s",
		header.PacketCount, previousHopIP)

	err = r.forwardResponseToPreviousHop(previousHopIP, updatedHeaderBytes, responseBytes)
	if err != nil {
		log.Printf("[Relay-ERROR] : %v", err)
	}
	return

}

func (r *RelayRepository) getResponseBuffer(pathHash string, requestID uint32) *ResponseBuffer {
	return r.bufferManager.getResponseBuffer(pathHash, requestID)
}

func (r *RelayRepository) mergeAndSendBufferedResponses(responses []*BufferedResponse, previousHopIP string) {
	if len(responses) == 0 {
		return
	}

	log.Printf("[Relay]  %d ", len(responses))

	commonHopList := responses[0].HopList

	respHeader := &packet.Packet{
		PacketCount: byte(len(responses)),
		PacketID:    make([]uint32, len(responses)),
		HopList:     commonHopList,
	}

	respSizes := make([]int, len(responses))
	respBodies := make([][]byte, len(responses))

	for i, resp := range responses {
		respHeader.PacketID[i] = resp.RequestID
		respSizes[i] = resp.Size
		respBodies[i] = resp.ResponseData
	}

	respHeader.Offsets = packet.CalcRelativeOffsets(respSizes)

	headerBytes, err := respHeader.Pack()
	if err != nil {
		log.Printf("[Relay-ERROR] : %v", err)
		return
	}

	var mergedRespData []byte
	for _, respBody := range respBodies {
		mergedRespData = append(mergedRespData, respBody...)
	}

	log.Printf("[Relay] ，=%d ，=%s",
		len(mergedRespData), previousHopIP)

	err = r.forwardResponseToPreviousHop(previousHopIP, headerBytes, mergedRespData)
	if err != nil {
		log.Printf("[Relay-ERROR] : %v", err)
	}
}

func (r *RelayRepository) StartRequestListener() {

	listenAddr := fmt.Sprintf("0.0.0.0:%s", r.relayConfig.RequestPort)
	log.Printf("[Relay]  %s ", listenAddr)

	listener, err := net.Listen("tcp", listenAddr)
	if err != nil {
		log.Fatalf("[Relay-FATAL] : %v", err)
		return

	}
	defer listener.Close()

	for {
		select {
		case <-r.done:
			log.Println("[Relay] ")
			return
		default:
			conn, err := listener.Accept()
			if err != nil {
				log.Printf("[Relay-ERROR] : %v", err)
				continue
			}

			log.Printf("[Relay]  %s ", conn.RemoteAddr().String())

			go r.handleRequestConnection(conn)
		}

	}
}

func (r *RelayRepository) handleRequestConnection(conn net.Conn) {
	remoteAddr := conn.RemoteAddr().String()
	log.Printf("[Relay]  %s ", remoteAddr)

	session, err := smux.Server(conn, connection.DefaultSmuxConfig())
	if err != nil {

		log.Printf("[Relay-ERROR] SMUX: %v", err)
		conn.Close()
		return
	}

	connection.AddServerSession(remoteAddr, session)

	log.Printf("[Relay] : %s", remoteAddr)

	go func() {
		for {
			stream, err := session.AcceptStream()
			if err != nil {

				if session.IsClosed() {
					log.Printf("[Relay] : %s", remoteAddr)
					break
				}
				log.Printf("[Relay-ERROR] : %v", err)
				continue

			}

			go r.handleRequestStream(stream, remoteAddr)
		}
	}()
}

func (r *RelayRepository) handleRequestStream(stream *smux.Stream, remoteAddr string) {
	log.Printf("[Relay] : %p  %s", stream, remoteAddr)

	defer stream.Close()

	buffer := make([]byte, 16384)
	n, err := stream.Read(buffer)
	if err != nil {

		if err == io.EOF {
			log.Printf("[Relay] : %s", remoteAddr)
		} else {
			log.Printf("[Relay-ERROR] : %v", err)
		}

		return
	}

	log.Printf("[Relay] : %d ", n)

	r.requestChan <- &RelayRequestItem{
		Data:       buffer[:n],
		Stream:     stream,
		ReceivedAt: time.Now(),
		RemoteAddr: remoteAddr,
	}
}

func (r *RelayRepository) StartResponseListener() {

	listenAddr := fmt.Sprintf("0.0.0.0:%s", r.relayConfig.ResponsePort)
	log.Printf("[Relay]  %s ", listenAddr)

	listener, err := net.Listen("tcp", listenAddr)
	if err != nil {

		log.Fatalf("[Relay-FATAL] : %v", err)
		return
	}

	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		defer listener.Close()

		for {
			select {
			case <-r.done:
				log.Println("[Relay] ")
				return
			default:
				conn, err := listener.Accept()
				if err != nil {
					log.Printf("[Relay-ERROR] : %v", err)
					continue
				}

				log.Printf("[Relay]  %s ", conn.RemoteAddr().String())

				go r.handleResponseConnection(conn)
			}
		}
	}()
}

func (r *RelayRepository) handleResponseConnection(conn net.Conn) {
	remoteAddr := conn.RemoteAddr().String()
	log.Printf("[Relay]  %s ", remoteAddr)

	session, err := smux.Server(conn, connection.DefaultSmuxConfig())
	if err != nil {
		log.Printf("[Relay-ERROR] SMUX: %v", err)
		conn.Close()
		return
	}

	connection.AddServerSession(remoteAddr, session)

	for {
		stream, err := session.AcceptStream()
		if err != nil {
			if session.IsClosed() {
				log.Printf("[Relay] : %s", remoteAddr)
				break
			}
			log.Printf("[Relay-ERROR] : %v", err)
			continue
		}

		go r.handleResponseStream(stream, remoteAddr)
	}
}

func (r *RelayRepository) handleResponseStream(stream *smux.Stream, remoteAddr string) {
	log.Printf("[Relay] : %p  %s", stream, remoteAddr)
	defer stream.Close()

	buffer := make([]byte, 65536)
	n, err := stream.Read(buffer)
	if err != nil {
		if err == io.EOF {
			log.Printf("[Relay] : %s", remoteAddr)
		} else {
			log.Printf("[Relay-ERROR] : %v", err)
		}

		return
	}
	log.Printf("[RESP-FLOW] : ID=%p, =%d, =%s",
		stream, n, remoteAddr)
	log.Printf("[Relay] : %d ", n)
	log.Printf("[RESP-FLOW] : =%d, =%v",
		n, time.Now().Format("15:04:05.000"))

	r.responseChan <- &RelayResponseItem{
		Data:       buffer[:n],
		ReceivedAt: time.Now(),
		RemoteAddr: remoteAddr,
	}
}

func (rp *RelayProxy) Start() {

	rp.repository.StartProcessors()

	go rp.repository.StartRequestListener()

	rp.repository.StartResponseListener()
}

func (rp *RelayProxy) Stop() {
	rp.repository.Stop()
}

func RelayProxyWithFullConfig(relayConfig RelayConfig, repoConfig RelayRepositoryConfig) {
	proxy := CreateRelayProxy(relayConfig, repoConfig)
	proxy.Start()

	select {}
}

func RelayProxyfunc() {
	proxy := CreateRelayProxy(DefaultRelayConfig, DefaultRelayRepositoryConfig)
	proxy.Start()

	select {}
}
