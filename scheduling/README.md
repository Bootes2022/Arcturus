
# Arcturus Global Acceleration Framework

Arcturus is a **cloud-native** global acceleration framework designed to provide high-performance, low-latency path selection and decision-making with real-time network awareness. The framework integrates two main planes—**Forwarding Plane** and **Scheduling Plane**—to optimize data transmission and network resource management in a scalable and distributed manner.

## Table of Contents
- [Overview](#overview)
- [Forwarding Plane](#forwarding-plane)
  - [Forwarding Architecture](#forwarding-architecture)
  - [Key Techniques](#key-techniques)
    - [Enhanced Proxying](#enhanced-proxying)
    - [LinUCB for Dynamic Optimization](#linucb-for-dynamic-optimization)
    - [Bandit-based Decision Making](#bandit-based-decision-making)
- [Scheduling Plane](#scheduling-plane)
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

Arcturus integrates two crucial components—**Forwarding Plane** and **Scheduling Plane**—to achieve **Global Acceleration (GA)** by ensuring high-throughput, low-latency, and scalable networking. By adopting a **distributed approach**, Arcturus optimizes resource utilization and reduces compute bottlenecks, making it an ideal solution for real-time, multi-cloud applications.

## Forwarding Plane

### Forwarding Architecture

Arcturus’ **Forwarding Plane** is responsible for efficient data transport and optimized routing in GA scenarios.

#### Data Proxying
In the **data proxying** layer, the system uses lightweight techniques like **TCP connection pooling** and **multiplexing** to handle high RPS and small packet sizes.

- **Ingress and Egress Nodes**: Represent user connections by assigning specific ports.
- **Intermediate Nodes**: Forward traffic through a unified TCP tunnel, improving connection efficiency and reducing overhead.

This architecture significantly optimizes the utilization of TCP connections, enhancing throughput and reducing latency.

#### Network Routing
In the **network routing** layer, Arcturus employs **segment routing** over the TCP protocol. Unlike label-based routing technologies like **MPLS** and **LDP**, segment routing offers simpler, more efficient routing by eliminating complex timing logic and reducing the risk of network instability.

- **Protocol Stack Unloading and Loading**: Performed at the ingress and egress nodes.
- **Segment Routing**: Enables fine-grained packet forwarding, improving resource utilization and enabling gradient descent control for optimal scheduling.

### Key Techniques

#### Enhanced Proxying
Arcturus implements three key techniques to enhance proxying:
1. **TCP Connection Pooling**: Optimizes the reuse of existing TCP connections to improve efficiency.
2. **Multiplexing**: Allows multiple streams of data to be carried over a single connection, improving throughput and reducing overhead.
3. **Packet Merging**: Merges small packets into larger ones to optimize network resource usage and improve transmission efficiency.

Together, these techniques reduce connection overhead, optimize resource consumption, and increase overall throughput while maintaining control over latency.

#### LinUCB for Dynamic Optimization
To dynamically optimize key parameters such as **multiplexing sessions (Sp)**, **concurrency levels (Cp)**, and **packet merge timeout (Tp)**, Arcturus uses the **LinUCB (Linear Upper Confidence Bound)** algorithm. This algorithm allows the system to adaptively adjust parameters based on network and load conditions in real-time, ensuring efficient data transport and low-latency performance.

##### Key Parameter Ranges:
- **Sp**: Number of multiplexing sessions (range: 1–10)
- **Cp**: Concurrency levels (range: 50–200)
- **Tp**: Packet merge timeout (range: 1–5 ms)

These parameters are adjusted dynamically to achieve a balance between resource consumption and processing capabilities.

#### Bandit-based Decision Making
The decision-making process for parameter optimization uses a **multi-arm bandit** approach, where each parameter set (Sp, Cp, Tp) is considered an "arm" in the bandit model. The system evaluates the performance of each configuration using key metrics such as **Requests Per Second (RPS)** and **Average Request Processing Time (ART)**. These metrics are normalized and combined into a **reward function** to guide the system towards the optimal configuration.

- **Reward Function**: Combines normalized **RQPT (Requests Per Time Unit)** and **ART (Average Request Time)** to guide the system towards low-latency and high-throughput configurations.
  ```text
  Reward = wRQPT × RQPTnorm + wART × (1 − ARTnorm)
  ```

#### Bandit Exploration Strategy
To efficiently explore the search space of possible configurations (Sp, Cp, Tp), the system utilizes **stress testing** to define realistic search ranges. Based on this approach, Arcturus can adjust parameters within these predefined ranges, dynamically finding the best configuration for any given network condition.

## Scheduling Plane

### Scheduling Architecture

The core scheduling architecture consists of **controller nodes** and **proxy nodes**, each playing a crucial role in the global scheduling mechanism.

- **Controller Node**: The controller node aggregates data from across the system, compresses it, and synchronizes summary data across all nodes. It is responsible for global performance and stability decisions.
  
- **Proxy Node**: The proxy node shares the same scheduling capabilities as the controller, but it works at the edge of the network, handling local decisions and interacting with the controller for global state synchronization.

### Key Concepts

#### Controller and Proxy Nodes
The **controller** and **proxy nodes** form the backbone of the scheduling plane. The controller node processes global data, aggregates network performance statistics, and synchronizes the results with proxy nodes.

- **Proxy nodes** perform similar scheduling tasks as the controller but are located at the edge of the network for efficient, local decision-making.
- Every **5 seconds**, the proxy nodes send their compressed performance data to the central controller, which helps mitigate overreaction to network jitter.

#### Data Synchronization
Efficient **data synchronization** is critical in large-scale networks to ensure real-time updates without overwhelming the system with unnecessary traffic.

- **Static Data**: Network topology and user configurations are propagated incrementally to proxy nodes.
- **Dynamic Data**: Metrics like CPU load, memory usage, and latency are synchronized in real-time.

To reduce communication overhead and improve scalability, the **K-Nearest Neighbors (KNN)** approach is employed. This method partitions performance data into singular (outlier) and non-singular components. Non-singular data is aggregated (mean/median), thus significantly reducing synchronization overhead—up to 80% in some cases.

#### K-Nearest Neighbors (KNN)-Based Approach
Arcturus utilizes a **KNN-based approach** to partition performance data into two components:
- **Singular (Outlier) Components**: Data that deviates significantly from the norm.
- **Non-Singular Components**: Data that is more stable and can be compressed.

Only aggregate statistics (e.g., mean, median) of non-singular components are synchronized, which reduces the data transmitted across the network without sacrificing scheduling accuracy.

#### Regional Scheduling Groups
To further optimize performance and scalability, nodes are organized into **regional scheduling groups**. Each group elects a **master node** responsible for distributing tasks within the group, improving load balancing and resource management.

- **Proxy Group**: Groups of proxy nodes synchronize and handle local scheduling decisions.
- **Controller Group**: A group of controllers coordinates high-level scheduling tasks and global data synchronization.

This hierarchical approach ensures that **edge nodes** (proxy nodes) handle local decision-making, while **controller nodes** manage global coordination.

## Node Region Database Table

Before starting the system, you need to populate the `node_region` table in the database with information about the proxy and controller nodes. This table will store details about the nodes, including their IP addresses, regions, hostnames, and other relevant information.

### `node_region` Table Schema

```sql
-- 节点区域表
CREATE TABLE node_region (
   id INT AUTO_INCREMENT PRIMARY KEY,
   ip VARCHAR(50) NOT NULL UNIQUE COMMENT '节点IP地址',
   region VARCHAR(50) NOT NULL COMMENT '节点所属区域',
   hostname VARCHAR(100) COMMENT '节点主机名',
   description VARCHAR(255) COMMENT '节点描述信息',
   created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间'
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
