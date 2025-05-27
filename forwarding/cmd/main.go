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
)

func main() {

	port := flag.Int("port", 50051, "TCP")
	flag.Parse()

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

	go metrics_processing.StartDataPlane(ctx)

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
