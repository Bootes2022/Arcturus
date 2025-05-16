# Scheduling

The Arcturus Scheduling Plane serves as the **central configuration and coordination hub** for the entire acceleration system. It is the critical component that must be initialized first during system startup, as it manages global system configuration, node registration, and orchestrates the entire distributed acceleration infrastructure.

## Overview

The **Scheduling Plane** is the brain of the Arcturus acceleration system. As the primary configuration center, it:

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

## Installation

### Prerequisites

| Requirement       | Version  | Verification Command       |
|-------------------|----------|----------------------------|
| go                | ≥1.23    | `go version `  |
| MySQL    | ≥8.0     | `mysql --version`          |

## Deployment Process Overview

1. Download GitHub repository archive
2. Extract the files
3. Execute the deployment script

## Detailed Steps

### 1. Download GitHub Repository Archive

First, we need to download the project archive from GitHub. You can download it directly on your server using the following commands:

```bash
# Create a temporary directory for download
mkdir -p /tmp/deployment

# Navigate to the temporary directory
cd /tmp/deployment

# Download GitHub repository archive (replace with your repository information)
wget https://github.com/Bootes2022/Arcturus/scheduling.tar.gz -O scheduling.tar.gz
```

> Note: You need to replace `Bootes2022/Arcturus` with your actual repository path, and `main` with your desired branch name.

### 2. Extract the Files

Next, extract the downloaded archive:

```bash
# Extract tar.gz format archive
tar -xzf scheduling.tar.gz

# If you downloaded a zip format, use:
# apt-get install unzip -y  # If unzip is not installed on your system
# unzip arcturus.zip
```

### 3. Execute the Deployment Script

After extraction, navigate to the project directory and execute the deployment script:

```bash
# Navigate to the extracted directory (directory name may include branch name)
cd scheduling

# Ensure the deployment script has execution permissions
chmod +x deploy_scheduling.sh

# Execute the deployment script
./deploy_scheduling.sh
```

## Deployment Script Functions

The deployment script (`deploy.sh`) will automatically complete the following tasks:

1. Install Go environment (if not already installed)
2. Install MySQL database (if not already installed)
3. Create necessary database and user
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
