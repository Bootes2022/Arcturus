package models

import (
	"control/config"
	"control/middleware"
	"encoding/json"
	"fmt"
	"log"
	"testing"
	"time"
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

func TestCalculateAvgDelay(t *testing.T) {

	pool := middleware.CreateRedisPool()
	defer pool.Close()

	conn := pool.Get()
	defer conn.Close()
	db := middleware.ConnectToDB()
	defer middleware.CloseDB()
	var i int64
	for i = 1; i < 4; i++ {
		key := "192.168.1.1:192.168.2.2"
		value, _ := json.Marshal(config.ProbeResult{
			SourceIP:      "192.168.1.1",
			DestinationIP: "192.168.2.2",
			Delay:         i,
			Timestamp:     time.Now().Format("2006-01-02 15:04:05"),
		})
		_, lpushErr := conn.Do("LPUSH", key, value)
		if lpushErr != nil {
			log.Printf("Error storing result in redis: %v", lpushErr)
		}
	}
	CalculateAvgDelay(conn, db, "192.168.1.1", "192.168.2.2")
}
