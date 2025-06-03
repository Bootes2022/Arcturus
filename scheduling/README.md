# Scheduling

## Overview

The Arcturus Scheduling Plane serves as the **central configuration and coordination hub** for the entire acceleration system. It is the critical component that must be initialized first during system startup, as it manages global system configuration, node registration, and orchestrates the entire distributed acceleration infrastructure. The **Scheduling Plane** is the brain of the Arcturus acceleration system. As the primary configuration center, it:

- **Initializes and registers** all proxy and controller nodes in the network
- **Manages system-wide configuration** and policy distribution
- **Coordinates real-time path selection** and traffic routing decisions
- **Collects and aggregates** performance metrics from all nodes
- **Maintains the global view** of network topology and health status

Without the Scheduling Plane running, no other component of the acceleration system can function properly. It provides the foundation upon which all distributed operations are built.

## Database Schema

The Scheduling Plane relies on several database tables to maintain system state. **Before starting the system**, these tables must be properly initialized.

### Node Region Table

Stores basic information about all nodes in the system:

```sql
CREATE TABLE node_region (
   id INT AUTO_INCREMENT PRIMARY KEY,
   ip VARCHAR(50) NOT NULL UNIQUE COMMENT 'Node IP address',
   region VARCHAR(50) NOT NULL COMMENT 'Node region identifier',
   hostname VARCHAR(100) COMMENT 'Node hostname',
   description VARCHAR(255) COMMENT 'Node description',
   created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP COMMENT 'Creation time'
);
```

**Usage Example:**
```sql
INSERT INTO node_region (ip, region, hostname, description)
VALUES
('192.168.1.1', 'US-East', 'controller-01', 'Primary controller for US-East'),
('192.168.1.2', 'US-West', 'proxy-01', 'Edge proxy for US-West'),
('192.168.1.3', 'EU-Central', 'proxy-02', 'Edge proxy for EU-Central');
```

### System Info Table

Captures detailed hardware and performance metrics for each node:

```sql
CREATE TABLE system_info (
    id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    ip VARCHAR(45) NOT NULL,
    cpu_cores INT NOT NULL,
    cpu_model_name VARCHAR(255) NOT NULL,
    cpu_mhz FLOAT NOT NULL,
    cpu_cache_size INT NOT NULL,
    cpu_usage FLOAT NOT NULL,
    memory_total BIGINT UNSIGNED NOT NULL,
    memory_available BIGINT UNSIGNED NOT NULL,
    memory_used BIGINT UNSIGNED NOT NULL,
    memory_used_percent FLOAT NOT NULL,
    disk_device VARCHAR(255) NOT NULL,
    disk_total BIGINT UNSIGNED NOT NULL,
    disk_free BIGINT UNSIGNED NOT NULL,
    disk_used BIGINT UNSIGNED NOT NULL,
    disk_used_percent FLOAT NOT NULL,
    network_interface_name VARCHAR(255) NOT NULL,
    network_bytes_sent BIGINT UNSIGNED NOT NULL,
    network_bytes_recv BIGINT UNSIGNED NOT NULL,
    network_packets_sent BIGINT UNSIGNED NOT NULL,
    network_packets_recv BIGINT UNSIGNED NOT NULL,
    hostname VARCHAR(255) NOT NULL,
    os VARCHAR(255) NOT NULL,
    platform VARCHAR(255) NOT NULL,
    platform_version VARCHAR(255) NOT NULL,
    uptime BIGINT UNSIGNED NOT NULL,
    load1 FLOAT NOT NULL,
    load5 FLOAT NOT NULL,
    load15 FLOAT NOT NULL,
    timestamp DATETIME NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
);
```

**Usage Notes:**
- Updated automatically by monitoring agents on each node
- Used for capacity planning and load balancing decisions
- Historical data enables trend analysis and prediction

### Region Probe Info Table

Records network latency measurements between regions:

```sql
CREATE TABLE region_probe_info (
    id INT AUTO_INCREMENT PRIMARY KEY,
    source_ip VARCHAR(15) NOT NULL,
    source_region VARCHAR(50) NOT NULL,
    target_ip VARCHAR(15) NOT NULL,
    target_region VARCHAR(50) NOT NULL,
    tcp_delay INT NOT NULL,
    probe_time DATETIME NOT NULL
);
```

**Usage Example:**
```sql
INSERT INTO region_probe_info (source_ip, source_region, target_ip, target_region, tcp_delay, probe_time)
VALUES
('192.168.1.1', 'US-East', '192.168.1.2', 'US-West', 45, NOW()),
('192.168.1.1', 'US-East', '192.168.1.3', 'EU-Central', 120, NOW());
```

### Network Metrics Table

Maintains detailed link quality information for path optimization:

```sql
CREATE TABLE network_metrics (
    id INT AUTO_INCREMENT PRIMARY KEY,
    source_ip VARCHAR(15) NOT NULL,
    destination_ip VARCHAR(15) NOT NULL,
    link_latency FLOAT NOT NULL,
    cpu_mean FLOAT NOT NULL,
    cpu_variance FLOAT NOT NULL,
    virtual_queue_cpu_mean FLOAT NOT NULL,
    virtual_queue_cpu_variance FLOAT NOT NULL,
    C INT,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
);
```

**Key Fields:**
- `link_latency`: Round-trip time in milliseconds
- `cpu_mean/variance`: Destination node CPU statistics
- `virtual_queue_*`: Metrics for queue-based routing algorithms
- `C`: Configuration parameter for advanced routing

### Domain Origin Table

Maps domain names to their origin servers:

```sql
CREATE TABLE domain_origin (
    domain VARCHAR(20) PRIMARY KEY,
    origin_ip VARCHAR(20) NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
);
```

**Usage Example:**
```sql
INSERT INTO domain_origin (domain, origin_ip)
VALUES
('example.com', '10.0.1.5'),
('api.example.com', '10.0.1.6'),
('cdn.example.com', '10.0.1.7');
```

### Domain config Table

This table is designed for the last mile, containing the overall request increase corresponding to each domain and the required redistribution ratio :

```sql
CREATE TABLE domain_config (
    id INT AUTO_INCREMENT PRIMARY KEY,
    domain_name VARCHAR(255) NOT NULL UNIQUE,
    total_req_increment INT NOT NULL,
    redistribution_proportion DOUBLE NOT NULL,
    last_updated TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
);
```

**Usage Example:**
```sql
INSERT INTO domain_config (domain_name, total_req_increment, redistribution_proportion)
VALUES
('example.com', 100, 0.5),
('api.example.com', 500, 0.4),
('cdn.example.com', 1000, 0.5);
```

## Installation

### Prerequisites

| Requirement       | Version  | Verification Command       |
|-------------------|----------|----------------------------|
| go                | ≥1.23    | `go version `  |
| MySQL    | ≥8.0     | `mysql --version`          |



## Custom Settings

If you need to customize the deployment, you can modify the following parameters in the `scheduling_config.toml` file:

## Configuration Structure

```toml
# Database Connection Settings

# ***The database settings must be consistent with setup.conf to maintain proper database connectivity.
[database]
# Database username for application authentication
username = "myapp_user"

# Database password - should be kept secure and rotated periodically
# Note: In production, consider using environment variables or a secrets manager
password = "StrongAppUserPassword456!"

# Name of the database the application will connect to
dbname   = "myapp_db"

# Domain Origin Configuration
# Configure the domains you want to accelerate

[[domain_origins]]
# Domain name that needs acceleration (e.g., website or API endpoint)
domain    = "example.com"

# Origin server IP address where unaccelerated traffic would normally go
# This server receives traffic when acceleration isn't available
origin_ip = "192.168.1.100"

# Node Region Configuration
# Configure your data plane node clusters in node_regions.
[[node_regions]]
# Public IP address of the forwarding node
ip          = "172.16.0.10"

# Geographic region (used for latency-based routing)
region      = "US-East"

# Hostname/FQDN (used for internal DNS resolution)
hostname    = "node-use1-01.mydatacenter.com"

# Human-readable description
description = "Primary API server in US East"

[[node_regions]]
ip          = "172.16.1.20"
region      = "US-East"
hostname    = "node-use2-02.mydatacenter.com"

# Human-readable description
description = "Primary API server in US East"

```

### Example Complete Configuration
```toml
# [database]
# username = "myapp_user"
# password = "StrongAppUserPassword456!"
# dbname   = "myapp_db"

# [[domain_origins]]
# domain    = "example.com"
# origin_ip = "192.168.1.100"

# [[node_regions]]
# ip          = "172.16.0.10"
# region      = "US-East"
# hostname    = "node-use1-01.mydatacenter.com"
# description = "Primary API server in US East"
```

## License

Arcturus is licensed under the **Apache 2.0 License**, which allows for:
- Commercial use
- Modification
- Patent use
- Private use

### Requirements:
- Preservation of license and copyright notice
- Acknowledgment of changes made
