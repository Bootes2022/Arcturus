# Forwarding System

A distributed multi-node forwarding system designed for high-performance network traffic routing and optimization. The system leverages advanced techniques including TCP connection pooling, stream multiplexing, packet merging, and segment routing to achieve optimal throughput while maintaining low latency.

## Overview

The Forwarding System is a distributed architecture where multiple nodes work collaboratively to forward network traffic efficiently. Each node in the system deploys forwarding functionality and participates in a coordinated network that ensures optimal data transmission across different geographical locations.

## Architecture

The system consists of multiple forwarding nodes that communicate and coordinate through:
- Centralized configuration management via etcd
- Intelligent routing decisions based on real-time network conditions
- Dynamic parameter optimization using machine learning algorithms

## Key Features

### 1. Etcd Cluster Information Synchronization

The system uses etcd as a distributed key-value store for:
- Centralized configuration management
- Node discovery and health monitoring
- Real-time synchronization of cluster state
- Consistent routing table distribution

### 2. Inter-node Monitoring and Reporting

Our monitoring subsystem provides comprehensive visibility into system performance:

- **Node-to-node Probe Collection**: Continuous monitoring of network conditions between all node pairs
- **Self-node Information Reporting**: Each node reports its own health metrics and resource utilization
- **KNN Compression for Probe Data**: Efficient data compression using K-Nearest Neighbors algorithm to reduce bandwidth overhead while maintaining accuracy

### 3. Advanced Data Forwarding

The forwarding functionality integrates three core optimization techniques:

#### TCP Connection Management
- **Connection Pooling**: Reuses existing TCP connections to reduce handshake overhead
- **Stream Multiplexing**: Multiple data streams share single TCP connections
- **Packet Merging**: Combines multiple small packets into larger ones for efficiency

#### Dynamic Parameter Optimization

The system employs the LinUCB algorithm for real-time parameter tuning:

**Key Parameters:**
- **Sp (Sessions)**: Number of multiplexing sessions [1-10]
- **Cp (Concurrency)**: Concurrency levels [50-200] in steps of 10
- **Tp (Timeout)**: Packet merge timeout [1-5ms]

**Contextual Features:**
- CPU utilization
- Requests per second (RPS)
- Requests processed per unit time (RQPT)
- Average request processing time (ART)

#### Segment Routing

The system implements TCP-based segment routing with a custom protocol header:

```
+-------------+--------+----------+------------+
| packet_id   | offset | hop_list | hop_counts |
+-------------+--------+----------+------------+
```

**Header Fields:**
- **packet_id**: Unique identifier for sub-requests within merged requests
- **offset**: Relative position of each sub-request for accurate reconstruction
- **hop_list**: List of intermediate nodes in the routing path
- **hop_counts**: Number of hops for routing control

This standardized header structure enables efficient parsing and forwarding while minimizing computational overhead on proxy nodes.

## System Benefits

1. **Reduced Connection Overhead**: Through connection pooling and multiplexing
2. **Enhanced Throughput**: Via intelligent packet merging and optimization
3. **Controlled Latency**: Dynamic parameter tuning keeps latency within acceptable bounds
4. **Scalability**: Distributed architecture supports horizontal scaling
5. **Reliability**: Real-time monitoring and adaptive routing ensure high availability

## Performance Optimization

The system maintains optimal performance through:
- Continuous monitoring of performance metrics
- Machine learning-based parameter adaptation
- Automatic traffic flow adjustment based on Lyapunov drift calculations
- Real-time response to changing network conditions

## Getting Started

### Prerequisites

- etcd cluster (v3.5+)
- Linux-based operating system
- Network access between all forwarding nodes

### Installation

```bash
# Clone the repository
git clone https://github.com/Bootes2022/Arcturus/tree/main/forwarding

# Install dependencies
cd forwarding
./install.sh

# Configure etcd endpoints
cp config.example.yaml config.yaml
vim config.yaml
```

### Configuration

Edit `config.yaml` to set:
- etcd cluster endpoints
- Node identification
- Initial parameter values
- Monitoring intervals

### Running the System

```bash
# Start the forwarding service
./forwarding-node start

# Check status
./forwarding-node status

# View logs
tail -f logs/forwarding.log
```

## Monitoring

The system provides comprehensive monitoring through:
- Real-time metrics dashboard
- Historical performance data
- Alert configuration for anomaly detection

Access the monitoring dashboard at: `http://localhost:8080/metrics`

## Contributing

We welcome contributions! Please see our [Contributing Guide](CONTRIBUTING.md) for details.

## License

This project is licensed under the Apache License 2.0 - see the [LICENSE](LICENSE) file for details.

## Support

For support and questions:
- Create an issue in the GitHub repository
- Contact the development team at: Arcturus@example.com

## Acknowledgments

This system incorporates advanced research in network optimization and machine learning algorithms for distributed systems.
