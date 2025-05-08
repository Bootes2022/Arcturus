
# Arcturus Scheduling Plane

Arcturus implements a **Scheduling Plane** designed to provide high-performance, low-latency path selection and decision-making with real-time network awareness. This system leverages a **distributed approach** to achieve scalability and agility, addressing the limitations of traditional centralized architectures.

## Table of Contents
- [Overview](#overview)
- [Scheduling Architecture](#scheduling-architecture)
- [Key Concepts](#key-concepts)
  - [Controller and Proxy Nodes](#controller-and-proxy-nodes)
  - [Data Synchronization](#data-synchronization)
  - [K-Nearest Neighbors (KNN)-Based Approach](#k-nearest-neighbors-knn-based-approach)
  - [Regional Scheduling Groups](#regional-scheduling-groups)
- [Node Region Database Table](#node-region-database-table)
- [Installation](#installation)
- [License](#license)

## Overview

The **Scheduling Plane** in Arcturus enhances the system's global acceleration (GA) capabilities by addressing the need for rapid decision-making, efficient path switching, and real-time network state propagation. Unlike traditional centralized control centers that suffer from **compute bottlenecks** and **limited responsiveness**, the Scheduling Plane embraces edge scheduling to ensure scalability and fast route adjustment across a distributed network of proxy and controller nodes.

## Scheduling Architecture

The core scheduling architecture consists of **controller nodes** and **proxy nodes**, each playing a crucial role in the global scheduling mechanism.

- **Controller Node**: The controller node aggregates data from across the system, compresses it, and synchronizes summary data across all nodes. It is responsible for global performance and stability decisions.
  
- **Proxy Node**: The proxy node shares the same scheduling capabilities as the controller, but it works at the edge of the network, handling local decisions and interacting with the controller for global state synchronization.

### Key Functions:
1. **Data Management**: Synchronizing network topology and user configurations across proxy nodes.
2. **Scheduling**: Selecting optimal paths and adjusting routes in real-time.
3. **Probing**: Probing the network for performance data (e.g., CPU load, memory usage, latency).
4. **Reporting**: Aggregating and reporting performance metrics to the controller node for global coordination.

## Key Concepts

### Controller and Proxy Nodes
The **controller** and **proxy nodes** form the backbone of the scheduling plane. The controller node processes global data, aggregates network performance statistics, and synchronizes the results with proxy nodes.

- **Proxy nodes** perform similar scheduling tasks as the controller but are located at the edge of the network for efficient, local decision-making.
- Every **5 seconds**, the proxy nodes send their compressed performance data to the central controller, which helps mitigate overreaction to network jitter.

### Data Synchronization
Efficient **data synchronization** is critical in large-scale networks to ensure real-time updates without overwhelming the system with unnecessary traffic.

- **Static Data**: Network topology and user configurations are propagated incrementally to proxy nodes.
- **Dynamic Data**: Metrics like CPU load, memory usage, and latency are synchronized in real-time.

To reduce communication overhead and improve scalability, the **K-Nearest Neighbors (KNN)** approach is employed. This method partitions performance data into singular (outlier) and non-singular components. Non-singular data is aggregated (mean/median), thus significantly reducing synchronization overhead—up to 80% in some cases.

### K-Nearest Neighbors (KNN)-Based Approach
Arcturus utilizes a **KNN-based approach** to partition performance data into two components:
- **Singular (Outlier) Components**: Data that deviates significantly from the norm.
- **Non-Singular Components**: Data that is more stable and can be compressed.

Only aggregate statistics (e.g., mean, median) of non-singular components are synchronized, which reduces the data transmitted across the network without sacrificing scheduling accuracy.

### Regional Scheduling Groups
To further optimize performance and scalability, nodes are organized into **regional scheduling groups**. Each group elects a **master node** responsible for distributing tasks within the group, improving load balancing and resource management.

- **Proxy Group**: Groups of proxy nodes synchronize and handle local scheduling decisions.
- **Controller Group**: A group of controllers coordinates high-level scheduling tasks and global data synchronization.

This hierarchical approach ensures that **edge nodes** (proxy nodes) handle local decision-making, while **controller nodes** manage global coordination.

## Node Region Database Table

Before starting the system, you need to populate the `node_region` table in the database with information about the proxy and controller nodes. This table will store details about the nodes, including their IP addresses, regions, hostnames, and other relevant information.

### `node_region` Table Schema

```sql
-- node_region
CREATE TABLE node_region (
   id INT AUTO_INCREMENT PRIMARY KEY,
   ip VARCHAR(50) NOT NULL UNIQUE,
   region VARCHAR(50) NOT NULL ,
   hostname VARCHAR(100),
   description VARCHAR(255),
   created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
```

### Instructions to Populate the Table

To ensure the system functions correctly, you need to insert the appropriate information for all the nodes (both controller and proxy nodes) into the `node_region` table. Here's an example of how to insert data into the table:

```sql
INSERT INTO node_region (ip, region, hostname, description)
VALUES
('192.168.1.1', 'US-East', 'controller-node-1', 'Primary controller node for the US-East region'),
('192.168.1.2', 'US-West', 'proxy-node-1', 'Proxy node handling traffic for US-West'),
('192.168.1.3', 'EU-Central', 'proxy-node-2', 'Proxy node handling traffic for EU-Central');
```

This information is required to ensure that the scheduling and forwarding planes can accurately route traffic and synchronize across the network.

## Installation

### Prerequisites
| Requirement       | Version  | Verification Command       |
|-------------------|----------|----------------------------|
| Kubernetes        | ≥1.23    | `kubectl version --short`  |
| Terraform         | ≥1.4     | `terraform --version`       |
| Helm              | ≥3.11    | `helm version --short`      |

### Installation Method: Helm
```bash
# Add Arcturus repo
helm repo add arcturus https://charts.arcturus.io/stable

# Install with production profile
helm install arcturus arcturus/arcturus   --namespace arcturus-system   --create-namespace   --values https://raw.githubusercontent.com/your-repo/arcturus/main/config/production.yaml
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
