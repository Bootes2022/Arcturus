# Forwarding System

A distributed multi-node forwarding system designed for high-performance network traffic routing and optimization. The system leverages advanced techniques including TCP connection pooling, stream multiplexing, packet merging, and segment routing to achieve optimal throughput while maintaining low latency.

## Overview

The Forwarding System is a distributed architecture where multiple nodes work collaboratively to forward network traffic efficiently. Each node in the system deploys forwarding functionality and participates in a coordinated network that ensures optimal data transmission across different geographical locations.

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
./deploy_forwarding.sh
```

### Configuration
#### etcd config
```bash

# Member configuration
# Node 1 (192.168.0.1) - /etc/etcd/etcd.conf:
ETCD_NAME="etcd1"
ETCD_DATA_DIR="/var/lib/etcd"
ETCD_LISTEN_PEER_URLS="http://0.0.0.0:2380"        # Listen for peer communication
ETCD_LISTEN_CLIENT_URLS="http://0.0.0.0:2379"      # Listen for client connections

# Cluster configuration
ETCD_INITIAL_ADVERTISE_PEER_URLS="http://192.168.0.1:2380"  # External peer URL
ETCD_ADVERTISE_CLIENT_URLS="http://192.168.0.1:2379"        # External client URL
ETCD_INITIAL_CLUSTER="etcd1=http://192.168.0.1:2380,etcd2=http://192.168.0.2:2380"
ETCD_INITIAL_CLUSTER_TOKEN="etcd-cluster"
ETCD_INITIAL_CLUSTER_STATE="new"

# Member configuration
# Node 2 (192.168.0.2) - /etc/etcd/etcd.conf:
ETCD_NAME="etcd2"
ETCD_DATA_DIR="/var/lib/etcd"
ETCD_LISTEN_PEER_URLS="http://0.0.0.0:2380"
ETCD_LISTEN_CLIENT_URLS="http://0.0.0.0:2379"

# Cluster configuration
ETCD_INITIAL_ADVERTISE_PEER_URLS="http://192.168.0.2:2380"
ETCD_ADVERTISE_CLIENT_URLS="http://192.168.0.2:2379"
ETCD_INITIAL_CLUSTER="etcd1=http://192.168.0.1:2380,etcd2=http://192.168.0.2:2380"
ETCD_INITIAL_CLUSTER_TOKEN="etcd-cluster"
ETCD_INITIAL_CLUSTER_STATE="new"

# Systemd Service (Both nodes) - /etc/systemd/system/etcd.service:​
[Unit]
Description=etcd distributed key-value store
Documentation=https://github.com/etcd-io/etcd
After=network.target

[Service]
Type=notify
EnvironmentFile=/etc/etcd/etcd.conf
ExecStart=/usr/local/bin/etcd
Restart=always
RestartSec=10s
LimitNOFILE=40000

[Install]
WantedBy=multi-user.target
```


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
