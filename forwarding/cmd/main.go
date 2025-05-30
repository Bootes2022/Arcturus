package main

import (
	"context"
	"flag"
	"fmt"
	"forwarding/forwarder"
	"forwarding/metrics_processing"
	"forwarding/router"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/BurntSushi/toml"
)

// Config struct to hold configuration from toml file
type ForwardingConfig struct {
	Metrics MetricsConfig `toml:"metrics"`
}

type MetricsConfig struct {
	ServerAddr string `toml:"server_addr"`
}

func loadConfig(path string) (*ForwardingConfig, error) {
	var config ForwardingConfig
	if _, err := toml.DecodeFile(path, &config); err != nil {
		return nil, fmt.Errorf("failed to load config file %s: %w", path, err)
	}
	// Provide a default value if not specified in the config, or handle error
	if config.Metrics.ServerAddr == "" {
		log.Println("Metrics ServerAddr not specified in config, using default or handling error as needed.")
		// For now, let's set a default if empty, or you can make it a fatal error.
		// config.Metrics.ServerAddr = "127.0.0.1:8080" // Example default
	}
	return &config, nil
}

func main() {
	configFile := flag.String("config", "forwarding_config.toml", "Path to the configuration file")
	port := flag.Int("port", 50051, "TCP listener port for the forwarding service")
	flag.Parse()

	cfg, err := loadConfig(*configFile)
	if err != nil {
		log.Fatalf("Error loading configuration: %v", err)
	}
	if cfg.Metrics.ServerAddr == "" {
		log.Fatalf("Metrics server_addr is not configured in %s", *configFile)
	}

	addr := fmt.Sprintf(":%d", *port)

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("TCP: %v", err)
	}
	defer listener.Close()

	log.Printf("TCP， %d", *port)
	log.Printf("Ctrl+C")

	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {

				if ne, ok := err.(net.Error); ok && ne.Temporary() {
					log.Printf(": %v", err)
					continue
				}
				return
			}

			remoteAddr := conn.RemoteAddr().String()
			log.Printf(" %s ", remoteAddr)

			conn.Close()
		}
	}()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go metrics_processing.StartDataPlane(ctx, cfg.Metrics.ServerAddr)

	go func() {
		time.Sleep(1 * time.Minute)
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()

		for range ticker.C {
			pathManager := router.GetInstance()
			paths := pathManager.GetPaths()

			if len(paths) > 0 {
				log.Printf(":  %d ", len(paths))
				for i, p := range paths {
					log.Printf(" %d: %v (: %d)", i+1, p.IPList, p.Latency)
				}
			} else {
				log.Printf(": ")
			}
		}
	}()
	go forwarder.AccessProxyfunc()
	go forwarder.RelayProxyfunc()

	<-signalChan
	log.Println("，...")
	cancel()                    //
	time.Sleep(1 * time.Second) //  goroutine
}

func dataPlane() {
	forwarder.AccessProxyfunc()
	go forwarder.RelayProxyfunc()
}
