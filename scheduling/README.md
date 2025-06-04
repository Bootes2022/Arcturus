# Scheduling

## Overview

The Arcturus Scheduling Plane is the central configuration and coordination hub for the acceleration system. As a critical component, it must be initialized first during system startup. It manages global system configuration, node registration, and orchestrates the distributed acceleration infrastructure. The Scheduling Plane acts as the brain of the Arcturus system, responsible for:

- Initializing and registering all proxy and controller nodes in the network.
- Managing system-wide configuration and policy distribution.
- Coordinating real-time path selection and traffic routing decisions.
- Collecting and aggregating performance metrics from all nodes.
- Maintaining a global view of the network topology and health status.

If the Scheduling Plane is not running, other components of the acceleration system cannot function properly. It provides the foundation upon which all distributed operations are built.

## Database Schema

The Scheduling Plane relies on several database tables to maintain system state. **Before starting the system**, these tables must be properly initialized.

### Node Region Table

Stores basic information about all nodes in the system:

```sql
CREATE TABLE node_region (
   id INT AUTO_INCREMENT PRIMARY KEY,
   ip VARCHAR(45) NOT NULL UNIQUE COMMENT 'Node IP address (IPv4 or IPv6)',
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
    ip VARCHAR(45) NOT NULL COMMENT 'Node IP address (IPv4 or IPv6)',
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
    source_ip VARCHAR(45) NOT NULL COMMENT 'Source node IP (IPv4 or IPv6)',
    source_region VARCHAR(50) NOT NULL,
    target_ip VARCHAR(45) NOT NULL COMMENT 'Target node IP (IPv4 or IPv6)',
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
    source_ip VARCHAR(45) NOT NULL COMMENT 'Source node IP (IPv4 or IPv6)',
    destination_ip VARCHAR(45) NOT NULL COMMENT 'Destination node IP (IPv4 or IPv6)',
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
    domain VARCHAR(255) PRIMARY KEY COMMENT 'Domain name',
    origin_ip VARCHAR(45) NOT NULL COMMENT 'Origin server IP (IPv4 or IPv6)',
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

## Installation

### Prerequisites

| Requirement       | Version  | Verification Command       |
|-------------------|----------|----------------------------|
| go                | ≥1.22    | `go version`               |
| MySQL             | ≥8.0     | `mysql --version`          |

## Deployment Process Overview

1. Download GitHub repository archive
2. Extract the files
3. Execute the deployment script

## Detailed Steps

### 1. Download GitHub Repository Archive

First, download the project archive from GitHub. You can use a command like `wget` or download it via your browser.

```bash
# Create a temporary directory for download (optional)
# mkdir -p /tmp/arcturus_download
# cd /tmp/arcturus_download

# Download the main repository archive (e.g., main branch as a zip file)
wget https://github.com/Bootes2022/Arcturus/archive/refs/heads/main.zip -O Arcturus-main.zip
```

> Note: Replace `Bootes2022/Arcturus` with the correct repository path if it differs.
> The downloaded file might be named `Arcturus-main.zip` if downloading the main branch.

### 2. Extract the Files

Next, extract the downloaded archive:

```bash
# If you downloaded a zip format (e.g., Arcturus-main.zip):
unzip Arcturus-main.zip
# This will likely create a directory like Arcturus-main

# If you downloaded a tar.gz format archive (e.g., Arcturus-main.tar.gz), use:
# tar -xzf Arcturus-main.tar.gz
```

### 3. Execute the Deployment Script

After extraction, navigate to the `scheduling` project directory and execute the deployment script:

```bash
# Navigate to the extracted directory, then into the scheduling subdirectory
# Adjust the path if your downloaded archive extracts to a different top-level directory name
cd Arcturus-main/scheduling 

# Ensure the deployment script has execution permissions
chmod +x deploy_scheduling.sh

# Execute the deployment script
./deploy_scheduling.sh
```

## Deployment Script Functions

The deployment script (`deploy_scheduling.sh`) will automatically complete the following tasks:

1. Install Go environment (if not already installed)
2. Install MySQL database (if not already installed)
3. Create the necessary database and user
4. Build the application
5. Create configuration file
6. Set up and start the system service

## Post-Deployment Verification

After deployment is complete, you can check the service status with the following command:

```bash
sudo systemctl status scheduling.service
```

The application will run on port 8080. You can check if it's listening properly with:

```bash
netstat -tulpn | grep 8080
```

## Custom Settings

If you need to customize the deployment, you can modify the following parameters in the `deploy_scheduling.sh` file before executing the script:

* `DEPLOY_DIR`: Deployment directory (default: /opt/scheduling)
* `DB_NAME`: Database name (default: scheduling)
* `DB_USER`: Database username (default: scheduling_user)
* `DB_PASSWORD`: Database password (default: StrongPassword123!)

For example:

```bash
# Change database password (recommended for production environments)
sed -i 's/StrongPassword123!/YourSecurePassword456!/' deploy_scheduling.sh

# Modify deployment directory
sed -i 's|/opt/scheduling|/var/www/myapp|g' deploy_scheduling.sh
```

## Common Troubleshooting

1. **Deployment script permission issues**:

```bash
chmod +x deploy_scheduling.sh
```

2. **Port in use**: If port 8080 is already in use, modify the port configuration in `config.toml`.

3. **Service fails to start**: View logs for detailed error information:

```bash
sudo journalctl -u scheduling.service -n 50
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
