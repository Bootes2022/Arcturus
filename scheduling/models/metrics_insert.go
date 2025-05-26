package models

import (
	"database/sql"
	"fmt"
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
