package config

import "time"

type ConfigInfo struct {
	PoolNum        int
	ReceivePort    string
	DetectPort     string
	DetectCycle    time.Duration // ns *time.Second
	ExpireDuration time.Duration //redis
	CalculateCycle time.Duration // redis
	K              int
	Theta          float64
	Skip           int
	EtcdEndpoints  []string //etcd
	ControllerID   string   //ID
}

type ProbeResult struct {
	SourceIP      string `json:"ip1"`
	DestinationIP string `json:"ip2"`
	Delay         int64  `json:"tcp_delay"`
	Timestamp     string `json:"timestamp"`
}

type CPUStats struct {
	DestinationIP string  `json:"ip2"`      // ip
	Mean          float64 `json:"mean"`     // CPU
	Variance      float64 `json:"variance"` // CPU
}

type Result struct {
	Ip1   string
	Ip2   string
	Value float64
}
