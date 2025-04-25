package main

import (
	"context"
	"control/server/heartbeats"
)

func main() {
	//
	ctx := context.Background()
	heartbeats.StartServer(ctx)
}
