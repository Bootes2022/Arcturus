package config

// Config holds the overall configuration structure mapping to conf.toml
type Config struct {
	Database      DatabaseConfig      `toml:"database"`
	DomainOrigins []DomainOriginEntry `toml:"domain_origins"`
	NodeRegions   []NodeRegionEntry   `toml:"node_regions"`
}

// DatabaseConfig holds database connection parameters
type DatabaseConfig struct {
	Username string `toml:"username"`
	Password string `toml:"password"`
	DBName   string `toml:"dbname"`
}

// DomainOriginEntry maps to one [[domain_origins]] item in TOML
type DomainOriginEntry struct {
	Domain   string `toml:"domain"`
	OriginIP string `toml:"origin_ip"`
}

// NodeRegionEntry maps to one [[node_regions]] item in TOML
type NodeRegionEntry struct {
	IP          string `toml:"ip"`
	Region      string `toml:"region"`
	Hostname    string `toml:"hostname,omitempty"`    // omitempty if the field might be missing in TOML
	Description string `toml:"description,omitempty"` // omitempty if the field might be missing in TOML
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
