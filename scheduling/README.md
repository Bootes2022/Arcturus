# Arcturus Scheduling Plane

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
| Kubernetes        | ≥1.23    | `kubectl version --short`  |
| Terraform         | ≥1.4     | `terraform --version`      |
| Helm              | ≥3.11    | `helm version --short`     |
| MySQL/MariaDB     | ≥8.0     | `mysql --version`          |

### Database Setup

1. Create the database:
```bash
mysql -u root -p
CREATE DATABASE arcturus_scheduling;
USE arcturus_scheduling;
```

2. Initialize all tables using the schemas provided above.

3. Populate initial node data:
```bash
mysql arcturus_scheduling < init_nodes.sql
```

### Installation via Helm

```bash
# Add Arcturus repository
helm repo add arcturus https://charts.arcturus.io/stable

# Install with production configuration
helm install arcturus-scheduling arcturus/scheduling \
  --namespace arcturus-system \
  --create-namespace \
  --values https://raw.githubusercontent.com/your-repo/arcturus/main/config/scheduling-prod.yaml
```

### Verification

Check that all components are running:
```bash
kubectl get pods -n arcturus-system
kubectl get svc -n arcturus-system
```

Verify database connectivity:
```bash
kubectl exec -it scheduling-controller-0 -n arcturus-system -- mysql -h mysql-service -u root -p arcturus_scheduling -e "SELECT COUNT(*) FROM node_region;"
```

## Configuration

The Scheduling Plane configuration is managed through:

1. **Environment Variables**: For runtime parameters
2. **ConfigMaps**: For static configuration
3. **Database**: For dynamic configuration

Key configuration files:
- `scheduling-config.yaml`: Core scheduling parameters
- `node-discovery.yaml`: Node registration settings
- `routing-policy.yaml`: Path selection policies

## License

Arcturus is licensed under the **Apache 2.0 License**, which allows for:
- Commercial use
- Modification
- Patent use
- Private use

### Requirements:
- Preservation of license and copyright notice
- Acknowledgment of changes made
