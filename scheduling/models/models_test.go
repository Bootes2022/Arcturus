package models

import (
	"fmt"
	"scheduling/middleware"
	"testing"
)

func TestQueryIp(t *testing.T) {
	cfg, _ := middleware.LoadConfig()
	db := middleware.ConnectToDB(cfg.Database)
	defer db.Close()
	ips, _ := QueryIp(db)
	fmt.Println("Query result:", ips)

	if len(ips) == 0 {
		t.Error("Expected at least one IP, but got none")
	}
}
