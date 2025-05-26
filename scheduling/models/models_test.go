package models

import (
	"fmt"
	"scheduling/middleware"
	"testing"
)

func TestQueryIp(t *testing.T) {

	db := middleware.ConnectToDB()
	defer db.Close()
	ips, _ := QueryIp(db)
	fmt.Println("Query result:", ips)

	if len(ips) == 0 {
		t.Error("Expected at least one IP, but got none")
	}
}
