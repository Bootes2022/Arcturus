package heartbeats

import (
	"context"
	"control/controller/heartbeats/assessment"
	cf "control/controller/heartbeats/config"
	"control/controller/heartbeats/metrics"
	pb "control/controller/heartbeats/proto"
	"control/controller/heartbeats/storage"
	"control/controller/heartbeats/tasks"
	"control/controller/heartbeats/utils"
	"control/middleware"
	"database/sql"
	"fmt"
	"google.golang.org/grpc"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"
)

type ServerConfig struct {
	ListenAddr   string
	DataDir      string
	BufferPeriod time.Duration
}

type HeartbeatServer struct {
	config         ServerConfig
	db             *sql.DB
	grpcServer     *grpc.Server
	fileManager    *storage.FileManager
	metricsHandler *metrics.Handler

	shutdownHandler utils.ShutdownHandler
}

func NewHeartbeatServer(config ServerConfig, db *sql.DB) (*HeartbeatServer, error) {

	if err := os.MkdirAll(config.DataDir, 0755); err != nil {
		return nil, fmt.Errorf(": %v", err)
	}

	fileManager, err := storage.NewFileManager(config.DataDir)
	if err != nil {
		return nil, fmt.Errorf(": %v", err)
	}

	configPusher := cf.NewPusher()

	taskGenerator := tasks.NewTaskGenerator(db, fileManager, 5*time.Minute)

	assessmentCalc := assessment.NewAssessmentCalculator(db, 1*time.Minute)

	metricsHandler := metrics.NewHandler(
		db,
		fileManager,
		taskGenerator,
		configPusher,
		nil,
		assessmentCalc,
		config.BufferPeriod,
	)

	return &HeartbeatServer{
		config:         config,
		db:             db,
		fileManager:    fileManager,
		metricsHandler: metricsHandler,

		shutdownHandler: utils.NewShutdownHandler(func() {

			configPusher.Release()
			utils.ReleasePoolResources()
			middleware.CloseDB()
			log.Println("")
		}),
	}, nil
}

func (s *HeartbeatServer) Start(ctx context.Context) error {

	stopChan := make(chan os.Signal, 1)
	signal.Notify(stopChan, os.Interrupt, syscall.SIGTERM)

	s.grpcServer = grpc.NewServer()
	pb.RegisterMetricsServiceServer(s.grpcServer, s.metricsHandler)

	lis, err := net.Listen("tcp", s.config.ListenAddr)
	if err != nil {
		return fmt.Errorf(": %v", err)
	}

	go s.metricsHandler.StartBackgroundServicesWhenReady(ctx)

	go func() {
		<-stopChan
		log.Println("，...")
		s.Stop()
	}()

	log.Printf(" %s...", s.config.ListenAddr)
	return s.grpcServer.Serve(lis)
}

func (s *HeartbeatServer) Stop() {
	if s.grpcServer != nil {
		s.grpcServer.GracefulStop()
	}
	s.shutdownHandler.ExecuteShutdown()
}

func StartServer(ctx context.Context) {

	db := middleware.ConnectToDB()
	if db == nil {
		log.Fatal("")
	}

	dataDir := "/control/assets/"

	addr := "0.0.0.0:8080"

	config := ServerConfig{
		ListenAddr:   addr,
		DataDir:      dataDir,
		BufferPeriod: 20 * time.Second,
	}

	server, err := NewHeartbeatServer(config, db)
	if err != nil {
		log.Fatalf(": %v", err)
	}

	if err := server.Start(ctx); err != nil {
		log.Fatalf(": %v", err)
	}
}
