package models

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"scheduling/config"
	pb "scheduling/controller/heartbeats/proto"
	"sort"
	"time"

	"github.com/gomodule/redigo/redis"
)

// NodeSystemInfo struct is used to store the results queried from the database
type NodeSystemInfo struct {
	IP        string    `json:"ip"`        // IP Address
	CPUCores  int       `json:"cpu_cores"` // Number of CPU Cores
	CPUUsage  float64   `json:"cpu_usage"` // CPU Usage percentage
	Timestamp time.Time `json:"timestamp"` // Timestamp of the record
}

func QueryIp(db *sql.DB) ([]string, error) {
	rows, err := db.Query("SELECT DISTINCT ip FROM node_region")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ips []string
	for rows.Next() {
		var ip string
		if err := rows.Scan(&ip); err != nil {
			fmt.Println("：", err)
			return nil, err
		}
		ips = append(ips, ip)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return ips, nil
}

func CalculateAvgDelay(conn redis.Conn, db *sql.DB, ip1 string, ip2 string) {
	var totalDelay float64
	totalDelay = 0
	key := fmt.Sprintf("%s:%s", ip1, ip2)

	values, err := redis.Values(conn.Do("LRANGE", key, -10, -1))
	if err != nil {
		log.Fatalf("Failed to retrieve data from Redis: %v", err)
	}

	if len(values) == 0 {
		log.Printf("No data found for key: %s", key)
		return
	}

	for _, value := range values {
		var result config.ProbeResult
		err := json.Unmarshal(value.([]byte), &result)
		if err != nil {
			log.Printf("Failed to parse Redis value: %v", err)
			continue
		}
		totalDelay += float64(result.Delay)
	}

	avgDelay := totalDelay / float64(len(values))

	InsertLinkInfo(db, ip1, ip2, avgDelay, time.Now().Format("2006-01-02 15:04:05"))
}

func GetDelay(db *sql.DB, ip1 string, ip2 string) (float64, error) {
	var delay float64

	query := `
		SELECT tcp_delay
		FROM region_probe_info 
		WHERE source_ip = ? AND target_ip = ? 
		ORDER BY probe_time DESC 
		LIMIT 1;
	`
	err := db.QueryRow(query, ip1, ip2).Scan(&delay)
	if err != nil {
		if err == sql.ErrNoRows {
			return 0, fmt.Errorf("no data found for %s->%s", ip1, ip2)
		}
		return 0, fmt.Errorf("failed to query delay: %v", err)
	}
	return delay, nil
}

func GetCpuStats(db *sql.DB, destinationIP string) (*config.CPUStats, error) {

	query := `
		SELECT cpu_usage
		FROM system_info
		WHERE ip = ?
		ORDER BY created_at DESC
		LIMIT 10;
	`

	rows, err := db.Query(query, destinationIP)
	if err != nil {
		return nil, fmt.Errorf("failed to execute query: %v", err)
	}
	defer rows.Close()
	var (
		cpuUsages   []float64 //
		sum         float64   //  (mean)
		varianceSum float64   //  (variance)
	)

	for rows.Next() {
		var cpuUsage float64
		if err := rows.Scan(&cpuUsage); err != nil {
			return nil, fmt.Errorf("failed to scan row: %v", err)
		}
		cpuUsages = append(cpuUsages, cpuUsage)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration error: %v", err)
	}

	if len(cpuUsages) == 0 {
		return nil, fmt.Errorf("no CPU usage data found for device %s", destinationIP)
	}

	for _, usage := range cpuUsages {
		sum += usage
	}
	mean := sum / float64(len(cpuUsages))

	for _, usage := range cpuUsages {
		varianceSum += math.Pow(usage-mean, 2)
	}
	variance := varianceSum / float64(len(cpuUsages))

	stats := &config.CPUStats{
		DestinationIP: destinationIP,
		Mean:          mean,
		Variance:      variance,
	}
	return stats, nil
}

func QueryVirtualQueueCPUByIP(db *sql.DB, sourceIP string, destinationIP string) (float64, float64, error) {

	query := `
		SELECT 
			virtual_queue_cpu_mean,
			virtual_queue_cpu_variance
		FROM 
			network_metrics
		WHERE 
			source_ip = ? 
			AND destination_ip = ? 
		ORDER BY 
			updated_at DESC 
		LIMIT 1;
	`

	var virtualQueueCPUMean float64
	var virtualQueueCPUVariance float64

	err := db.QueryRow(query, sourceIP, destinationIP).Scan(&virtualQueueCPUMean, &virtualQueueCPUVariance)
	if err != nil {
		if err == sql.ErrNoRows {

			return 0, 0, fmt.Errorf("no records found for sourceIP to destinationIP: %s:%s", sourceIP, destinationIP)
		}

		return 0, 0, fmt.Errorf("error querying database: %w", err)
	}

	return virtualQueueCPUMean, virtualQueueCPUVariance, nil
}

func GetCPUPerformanceList(db *sql.DB, thresholdCpuMean float64, thresholdCpuVar float64) (aboveCpuMeans, belowCpuMeans, aboveCpuVars, belowCpuVars []float64, err error) {

	query := `
	WITH RankedRecords AS (
		SELECT 
			ip,
			cpu_usage,
			ROW_NUMBER() OVER (PARTITION BY ip ORDER BY timestamp DESC) AS rn
		FROM system_info
	)
	SELECT 
		ip,
		AVG(cpu_usage) AS avg_cpu_usage,
		VARIANCE(cpu_usage) AS variance_cpu_usage
	FROM RankedRecords
	WHERE rn <= 10
	GROUP BY ip;
	`

	rows, queryErr := db.Query(query)
	if queryErr != nil {
		return nil, nil, nil, nil, queryErr
	}
	defer rows.Close()

	for rows.Next() {
		var (
			ip               string
			avgCpuUsage      float64
			varianceCpuUsage float64
		)

		scanErr := rows.Scan(&ip, &avgCpuUsage, &varianceCpuUsage)
		if scanErr != nil {
			return nil, nil, nil, nil, scanErr
		}

		if avgCpuUsage > thresholdCpuMean {
			aboveCpuMeans = append(aboveCpuMeans, avgCpuUsage)
		} else {
			belowCpuMeans = append(belowCpuMeans, avgCpuUsage)
		}

		if varianceCpuUsage > thresholdCpuVar {
			aboveCpuVars = append(aboveCpuVars, varianceCpuUsage)
		} else {
			belowCpuVars = append(belowCpuVars, varianceCpuUsage)
		}
	}

	if rowsErr := rows.Err(); rowsErr != nil {
		return nil, nil, nil, nil, rowsErr
	}

	sort.Float64s(aboveCpuMeans)
	sort.Float64s(belowCpuMeans)
	sort.Float64s(aboveCpuVars)
	sort.Float64s(belowCpuVars)
	return aboveCpuMeans, belowCpuMeans, aboveCpuVars, belowCpuVars, nil
}

func QueryOriginIP(db *sql.DB, domain string) (string, error) {
	var originIP string

	err := db.QueryRow("SELECT origin_ip FROM domain_origin WHERE domain = ?", domain).Scan(&originIP)
	if err != nil {
		if err == sql.ErrNoRows {
			// ，
			return "", fmt.Errorf("domain '%s' not found", domain)
		}

		return "", fmt.Errorf("failed to query database for domain '%s': %v", domain, err)
	}

	return originIP, nil
}

func QueryNodeInfo(db *sql.DB) ([]*pb.NodeInfo, error) {

	rows, err := db.Query("SELECT ip, region FROM node_region")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var nodes []*pb.NodeInfo
	for rows.Next() {
		var node pb.NodeInfo
		if err := rows.Scan(&node.Ip, &node.Region); err != nil {
			return nil, err
		}
		nodes = append(nodes, &node)
	}

	return nodes, nil
}

func QueryDomainIPMappings(db *sql.DB) ([]*pb.DomainIPMapping, error) {

	rows, err := db.Query("SELECT domain, origin_ip FROM domain_origin")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var mappings []*pb.DomainIPMapping
	for rows.Next() {
		var mapping pb.DomainIPMapping
		if err := rows.Scan(&mapping.Domain, &mapping.Ip); err != nil {
			return nil, err
		}
		mappings = append(mappings, &mapping)
	}

	return mappings, nil
}

func GetNodeRegion(db *sql.DB, ip string) (string, error) {
	var region string
	query := "SELECT region FROM node_region WHERE ip = ?"
	err := db.QueryRow(query, ip).Scan(&region)
	if err != nil {
		if err == sql.ErrNoRows {
			return "unknown", nil
		}
		return "", err
	}
	return region, nil
}

func GetAllRegions(db *sql.DB) ([]string, error) {
	var regions []string

	rows, err := db.Query("SELECT DISTINCT region FROM node_region")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var region string
		if err := rows.Scan(&region); err != nil {
			return nil, err
		}
		regions = append(regions, region)
	}

	return regions, nil
}

func GetRegionIPs(db *sql.DB, region string) ([]string, error) {
	rows, err := db.Query("SELECT ip FROM node_region WHERE region = ?", region)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ips []string
	for rows.Next() {
		var ip string
		if err := rows.Scan(&ip); err != nil {
			return nil, err
		}
		ips = append(ips, ip)
	}

	return ips, nil
}

func CountMetricsNodes(db *sql.DB) (int, error) {

	query := `SELECT COUNT(DISTINCT ip) FROM system_info`

	var count int
	err := db.QueryRow(query).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf(": %w", err)
	}

	return count, nil
}

func GetMedianVirtual(db *sql.DB) (float64, float64, error) {
	query := `
	WITH latest_records AS (
		SELECT *
		FROM (
			SELECT *,
				   ROW_NUMBER() OVER (PARTITION BY source_ip, destination_ip ORDER BY updated_at DESC) as rn
			FROM network_metrics
		) ranked
		WHERE rn = 1
	)
	SELECT 
		PERCENTILE_CONT(0.5) WITHIN GROUP (ORDER BY virtual_queue_cpu_mean) OVER () as median_virtual_queue_cpu_mean,
		PERCENTILE_CONT(0.5) WITHIN GROUP (ORDER BY virtual_queue_cpu_variance) OVER () as median_virtual_queue_cpu_variance
	FROM latest_records
	LIMIT 1;`

	row := db.QueryRow(query)

	var medianMean, medianVariance float64

	err := row.Scan(&medianMean, &medianVariance)
	if err != nil {

		if err == sql.ErrNoRows {
			return 0, 0, fmt.Errorf("")
		}
		return 0, 0, fmt.Errorf(": %w", err)
	}

	return medianMean, medianVariance, nil
}

// GetLatestSystemInfoByRegion method queries the latest node system information for a given region.
// It no longer fetches the region itself in the result struct.
func GetLatestNodeInfoByRegion(db *sql.DB, region string) ([]NodeSystemInfo, error) {
	// SQL query to get the latest system info for IPs within a specific region.
	// The 'nr.region' column is removed from the SELECT list.
	query := `
        WITH RankedSystemInfo AS (
            SELECT
                ip,
                cpu_cores,
                cpu_usage,
                timestamp, -- Ensure the timestamp column in system_info is DATETIME or TIMESTAMP type
                ROW_NUMBER() OVER (PARTITION BY ip ORDER BY timestamp DESC) as rn
            FROM
                system_info
        )
        SELECT
            rsi.ip,
            rsi.cpu_cores,
            rsi.cpu_usage,
            rsi.timestamp
        FROM
            node_region nr
        INNER JOIN
            RankedSystemInfo rsi ON nr.ip = rsi.ip
        WHERE
            nr.region = ? 
            AND rsi.rn = 1;
    `

	rows, err := db.Query(query, region)
	if err != nil {
		return nil, fmt.Errorf("error querying latest system info by region '%s': %w", region, err)
	}
	defer rows.Close()

	var results []NodeSystemInfo
	for rows.Next() {
		var info NodeSystemInfo
		// Note: The order of Scan arguments must exactly match the order of columns in the SELECT statement
		err := rows.Scan(
			&info.IP,
			&info.CPUCores,
			&info.CPUUsage,
			&info.Timestamp, // MySQL DATETIME/TIMESTAMP can be directly scanned into time.Time
		)
		if err != nil {
			// Consider whether to log the error and continue, or return the error immediately.
			// Here, returning the error immediately is chosen.
			return nil, fmt.Errorf("error scanning row for region '%s': %w", region, err)
		}
		results = append(results, info)
	}

	// Check for errors that occurred during rows iteration
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows for region '%s': %w", region, err)
	}

	if len(results) == 0 {
		// Optionally, you can return a specific error type or (nil, nil) to indicate no records found.
		// fmt.Printf("No records found for region: %s\n", region) // Example log message
	}
	return results, nil
}

// GetDomainConfig method queries the domain config
func GetDomainConfigValues(db *sql.DB, domainName string) (int, float64, error) {
	query := `
        SELECT
            total_req_increment,
            redistribution_proportion
        FROM
            domain_config
        WHERE
            domain_name = ?;
    `

	var totalReqIncrement int
	var redistributionProportion float64

	// db.QueryRow is used because we expect at most one row (due to UNIQUE constraint on domain_name)
	err := db.QueryRow(query, domainName).Scan(
		&totalReqIncrement,
		&redistributionProportion,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			// Domain not found in the table. Return zero values for the params and the error.
			return 0, 0.0, fmt.Errorf("domain '%s' not found in domain_config: %w", domainName, err)
		}
		// Some other error occurred during query or scan. Return zero values and the error.
		return 0, 0.0, fmt.Errorf("error querying domain_config for domain '%s': %w", domainName, err)
	}

	// Successfully fetched the values
	return totalReqIncrement, redistributionProportion, nil
}
