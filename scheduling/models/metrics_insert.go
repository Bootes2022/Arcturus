package models

import (
	"database/sql"
	"fmt"
	"log"
	"scheduling/config"
	pb "scheduling/controller/heartbeats/proto"
	"time"
)

type ProbeResult struct {
	SourceIP     string
	SourceRegion string
	TargetIP     string
	TargetRegion string
	TCPDelay     int64
	ProbeTime    time.Time
}

func InsertMetricsInfo(db *sql.DB, info *pb.Metrics) error {
	query := `
		INSERT INTO system_info (
			ip, 
			cpu_cores, cpu_model_name, cpu_mhz, cpu_cache_size, cpu_usage,
			memory_total, memory_available, memory_used, memory_used_percent,
			disk_device, disk_total, disk_free, disk_used, disk_used_percent,
			network_interface_name, network_bytes_sent, network_bytes_recv,
			network_packets_sent, network_packets_recv,
			hostname, os, platform, platform_version, uptime,
			load1, load5, load15, timestamp
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	fmt.Printf("Timestamp: %s\n", timestamp)
	_, err := db.Exec(query,
		info.Ip,
		info.CpuInfo.Cores, info.CpuInfo.ModelName, info.CpuInfo.Mhz, info.CpuInfo.CacheSize, info.CpuInfo.Usage, //5
		info.MemoryInfo.Total, info.MemoryInfo.Available, info.MemoryInfo.Used, info.MemoryInfo.UsedPercent, //4
		info.DiskInfo.Device, info.DiskInfo.Total, info.DiskInfo.Free, info.DiskInfo.Used, info.DiskInfo.UsedPercent, //5
		info.NetworkInfo.InterfaceName, info.NetworkInfo.BytesSent, info.NetworkInfo.BytesRecv, //3
		info.NetworkInfo.PacketsSent, info.NetworkInfo.PacketsRecv, //2
		info.HostInfo.Hostname, info.HostInfo.Os, info.HostInfo.Platform, info.HostInfo.PlatformVersion, info.HostInfo.Uptime, //5
		info.LoadInfo.Load1, info.LoadInfo.Load5, info.LoadInfo.Load15, timestamp, //4
	)
	return err
}

func InsertLinkInfo(db *sql.DB, sourceIP string, destinationIP string, delay float64, timestamp string) error {
	query := `
		INSERT INTO link_info (source_ip, destination_ip, latency, Timestamp)
		VALUES (?, ?, ?, ?)
	`
	_, err := db.Exec(query, sourceIP, destinationIP, delay, timestamp)
	return err
}

func UpdateVirtualQueueAndCPUMetrics(db *sql.DB, sourceIP, destinationIP string, latency, mean, variance, VirtualQueueCPUMean, VirtualQueueCPUVariance float64) error {
	query := `
		INSERT INTO network_metrics (
			source_ip, destination_ip, link_latency, 
			cpu_mean, cpu_variance, 
			virtual_queue_cpu_mean, virtual_queue_cpu_variance
		) VALUES (?, ?, ?, ?, ?, ?, ?);
	`

	_, err := db.Exec(query, sourceIP, destinationIP, latency,
		mean, variance, VirtualQueueCPUMean, VirtualQueueCPUVariance)
	if err != nil {
		return fmt.Errorf("failed to insert link data: %v", err)
	}

	return nil
}

func InsertProbeResult(db *sql.DB, result *ProbeResult) error {
	query := `
    INSERT INTO region_probe_info 
    (source_ip, source_region, target_ip, target_region, tcp_delay, probe_time) 
    VALUES (?, ?, ?, ?, ?, ?)
    `
	_, err := db.Exec(
		query,
		result.SourceIP,
		result.SourceRegion,
		result.TargetIP,
		result.TargetRegion,
		result.TCPDelay,
		result.ProbeTime,
	)
	return err
}

// InsertDomainOrigins inserts data into the domain_origin table
func InsertDomainOrigins(db *sql.DB, domains []config.DomainOriginEntry) error {
	if len(domains) == 0 {
		log.Println("No domain origins to insert.")
		return nil
	}

	// Prepare statement for inserting data
	// Using ON DUPLICATE KEY UPDATE to handle cases where the domain might already exist.
	// You can choose to error out instead if that's preferred.
	stmt, err := db.Prepare("INSERT INTO domain_origin (domain, origin_ip) VALUES (?, ?) ON DUPLICATE KEY UPDATE origin_ip = VALUES(origin_ip)")
	if err != nil {
		return fmt.Errorf("error preparing domain_origin insert statement: %w", err)
	}
	defer stmt.Close()

	for _, d := range domains {
		_, err := stmt.Exec(d.Domain, d.OriginIP)
		if err != nil {
			// Log individual errors but continue if possible, or return immediately
			log.Printf("Error inserting domain_origin (domain: %s, ip: %s): %v", d.Domain, d.OriginIP, err)
			// return fmt.Errorf("error executing domain_origin insert for domain %s: %w", d.Domain, err) // Uncomment to stop on first error
		} else {
			log.Printf("Successfully inserted/updated domain_origin: %s -> %s", d.Domain, d.OriginIP)
		}
	}
	return nil // Return nil if we are logging errors but continuing
}

// InsertNodeRegions inserts data into the node_region table
func InsertNodeRegions(db *sql.DB, nodes []config.NodeRegionEntry) error {
	if len(nodes) == 0 {
		log.Println("No node regions to insert.")
		return nil
	}

	// Prepare statement for inserting data
	// Using ON DUPLICATE KEY UPDATE for the unique IP.
	// Note: 'id' is auto-increment, 'created_at' has a default.
	stmt, err := db.Prepare(`
		INSERT INTO node_region (ip, region, hostname, description) 
		VALUES (?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE 
			region = VALUES(region), 
			hostname = VALUES(hostname), 
			description = VALUES(description)
	`)
	if err != nil {
		return fmt.Errorf("error preparing node_region insert statement: %w", err)
	}
	defer stmt.Close()

	for _, n := range nodes {
		// Handle potentially NULL/empty optional fields from TOML
		var hostname sql.NullString
		if n.Hostname != "" {
			hostname.String = n.Hostname
			hostname.Valid = true
		}

		var description sql.NullString
		if n.Description != "" {
			description.String = n.Description
			description.Valid = true
		}

		_, err := stmt.Exec(n.IP, n.Region, hostname, description)
		if err != nil {
			log.Printf("Error inserting node_region (ip: %s, region: %s): %v", n.IP, n.Region, err)
			// return fmt.Errorf("error executing node_region insert for IP %s: %w", n.IP, err) // Uncomment to stop on first error
		} else {
			log.Printf("Successfully inserted/updated node_region: %s (%s)", n.IP, n.Region)
		}
	}
	return nil
}

func SaveOrUpdateDomainConfig(db *sql.DB, domainName string, totalReqIncrement int, redistributionProportion float64) error {
	// SQL statement for INSERT ... ON DUPLICATE KEY UPDATE
	// This is the MySQL way of performing an "UPSERT"
	query := `
        INSERT INTO domain_config (domain_name, total_req_increment, redistribution_proportion) 
        VALUES (?, ?, ?)
        ON DUPLICATE KEY UPDATE
            total_req_increment = VALUES(total_req_increment),
            redistribution_proportion = VALUES(redistribution_proportion);
    `
	// VALUES(column_name) in the ON DUPLICATE KEY UPDATE clause refers to the value
	// that would have been inserted if there was no duplicate.

	stmt, err := db.Prepare(query)
	if err != nil {
		return fmt.Errorf("failed to prepare domain config upsert statement: %w", err)
	}
	defer stmt.Close()

	_, err = stmt.Exec(domainName, totalReqIncrement, redistributionProportion)
	if err != nil {
		return fmt.Errorf("failed to execute domain config upsert for domain %s: %w", domainName, err)
	}
	log.Printf("Configuration for domain '%s' successfully saved/updated.\n", domainName)
	return nil
}
