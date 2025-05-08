// utils/network.go
package utils

import (
	"context"
	"control/pool_manager"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"time"
)

func CreateGRPCConnection(addr string, timeout time.Duration) (*grpc.ClientConn, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	return grpc.DialContext(
		ctx,
		addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
}

func ReleasePoolResources() {

	pool_manager.ReleaseAllPools()
}

type ShutdownHandler struct {
	handlers []func()
}

func NewShutdownHandler(handlers ...func()) ShutdownHandler {
	return ShutdownHandler{
		handlers: handlers,
	}
}

func (h ShutdownHandler) ExecuteShutdown() {
	for _, handler := range h.handlers {
		handler()
	}
}
