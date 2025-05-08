
# Arcturus Forwarding Plane

Arcturus implements a **Forwarding Plane** designed to optimize data proxying, connection management, and network routing, tailored to meet the demanding needs of **Global Acceleration (GA)**. This system leverages lightweight, high-performance techniques such as **TCP connection pooling**, **multiplexing**, and **packet merging**, addressing the unique challenges of resource-constrained environments, high requests per second (RPS), and small packet sizes.

## Table of Contents
- [Overview](#overview)
- [Forwarding Architecture](#forwarding-architecture)
  - [Data Proxying](#data-proxying)
  - [Network Routing](#network-routing)
- [Key Techniques](#key-techniques)
  - [Enhanced Proxying](#enhanced-proxying)
  - [LinUCB for Dynamic Optimization](#linucb-for-dynamic-optimization)
  - [Bandit-based Decision Making](#bandit-based-decision-making)
- [Installation](#installation)
- [License](#license)

## Overview

The **Forwarding Plane** in Arcturus aims to provide efficient data transport and optimized routing for global acceleration scenarios. It overcomes the limitations of traditional proxy technologies, such as **HTTP multiplexing**, **TCP connection pooling**, and **eBPF-based forwarding**, which offer limited flexibility in cloud environments due to strict security policies and resource constraints. Arcturus improves upon these existing solutions by utilizing a **custom-designed proxy** optimized for GA.

## Forwarding Architecture

The **Forwarding Architecture** consists of key elements for **data proxying** and **network routing**:

### Data Proxying
In the **data proxying** layer, the system uses lightweight techniques like **TCP connection pooling** and **multiplexing** to handle high RPS and small packet sizes. 

- **Ingress and Egress Nodes**: Represent user connections by assigning specific ports.
- **Intermediate Nodes**: Forward traffic through a unified TCP tunnel, improving connection efficiency and reducing overhead.

This architecture significantly optimizes the utilization of TCP connections, enhancing throughput and reducing latency.

### Network Routing
In the **network routing** layer, Arcturus employs **segment routing** over the TCP protocol. Unlike label-based routing technologies like **MPLS** and **LDP**, segment routing offers simpler, more efficient routing by eliminating complex timing logic and reducing the risk of network instability.

- **Protocol Stack Unloading and Loading**: Performed at the ingress and egress nodes.
- **Segment Routing**: Enables fine-grained packet forwarding, improving resource utilization and enabling gradient descent control for optimal scheduling.

## Key Techniques

### Enhanced Proxying
Arcturus implements three key techniques to enhance proxying:
1. **TCP Connection Pooling**: Optimizes the reuse of existing TCP connections to improve efficiency.
2. **Multiplexing**: Allows multiple streams of data to be carried over a single connection, improving throughput and reducing overhead.
3. **Packet Merging**: Merges small packets into larger ones to optimize network resource usage and improve transmission efficiency.

Together, these techniques reduce connection overhead, optimize resource consumption, and increase overall throughput while maintaining control over latency.

### LinUCB for Dynamic Optimization
To dynamically optimize key parameters such as **multiplexing sessions (Sp)**, **concurrency levels (Cp)**, and **packet merge timeout (Tp)**, Arcturus uses the **LinUCB (Linear Upper Confidence Bound)** algorithm. This algorithm allows the system to adaptively adjust parameters based on network and load conditions in real-time, ensuring efficient data transport and low-latency performance.

#### Key Parameter Ranges:
- **Sp**: Number of multiplexing sessions (range: 1–10)
- **Cp**: Concurrency levels (range: 50–200)
- **Tp**: Packet merge timeout (range: 1–5 ms)

These parameters are adjusted dynamically to achieve a balance between resource consumption and processing capabilities.

### Bandit-based Decision Making
The decision-making process for parameter optimization uses a **multi-arm bandit** approach, where each parameter set (Sp, Cp, Tp) is considered an "arm" in the bandit model. The system evaluates the performance of each configuration using key metrics such as **Requests Per Second (RPS)** and **Average Request Processing Time (ART)**. These metrics are normalized and combined into a **reward function** to guide the system towards the optimal configuration.

- **Reward Function**: Combines normalized **RQPT (Requests Per Time Unit)** and **ART (Average Request Time)** to guide the system towards low-latency and high-throughput configurations.
  ```text
  Reward = wRQPT × RQPTnorm + wART × (1 − ARTnorm)
  ```

### Bandit Exploration Strategy
To efficiently explore the search space of possible configurations (Sp, Cp, Tp), the system utilizes **stress testing** to define realistic search ranges. Based on this approach, Arcturus can adjust parameters within these predefined ranges, dynamically finding the best configuration for any given network condition.

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
