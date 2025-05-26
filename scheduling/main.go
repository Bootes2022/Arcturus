package main

import (
	"context"
	"control/controller/heartbeats"
)

func main() {
	//
	ctx := context.Background()
	heartbeats.StartServer(ctx)
}
