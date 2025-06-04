# Forwarding

## Overview

The Forwarding System is a distributed architecture where multiple nodes work collaboratively to forward network traffic efficiently. Each node in the system deploys forwarding functionality and participates in a coordinated network that ensures optimal data transmission across different geographical locations.


### Prerequisites

- etcd cluster (v3.5+)
- Linux-based operating system
- Network access between all forwarding nodes

## Custom Settings
If you need to customize the deployment, you can modify the following parameters in the cmd/forwarding_config.toml file:

```bash
# Ensure you are in the root directory of the cloned Arcturus repository
# cd Arcturus
# Navigate to the forwarding directory
cd forwarding

# Install dependencies
./deploy_forwarding.sh

```

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

# Systemd Service (Both nodes) - /etc/systemd/system/etcd.service:
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
#### BPR config
```bash
#!/bin/bash

########################
# Core Algorithm Control
########################
CPU_LOW_THRESHOLD <60>              # CPU low threshold (%). Example: 60. Used for phase differentiation.
CPU_TARGET_THRESHOLD <20>           # Target CPU threshold (%). Example: 20. This is the queue backlog target.
V_WEIGHT <0.001>                    # Latency weighting factor. Example: 0.001 (recommended range: 0.001-0.1).
MAX_BPR_ITERATIONS <3>              # BPR maximum iterations. Example: 3 (recommended: 3-5).

########################
# Dynamic Distribution
########################
REDISTRIB_PROPORTION <0.3-0.7>      # Request redistribution proportion (range: 0.3-0.7).
NON_MAIN_CLUSTER_BOOST <1.0-1.5>    # Non-main cluster score multiplier (range: 1.0-1.5).
GAP_SCORE_BOOST <1.0-2.0>           # Data gap enhancement factor (range: 1.0-2.0).

########################
# Monitoring Thresholds
########################
CPU_ALERT_THRESHOLDS <60,70,80>     # CPU alert thresholds (comma-separated). Example: 60,70,80.
MIN_VARIANCE <0.1>                  # Minimum CPU variance threshold. Example: 0.1.
MAX_LATENCY <500>                   # Maximum latency threshold (ms). Example: 500.
```
#### KNN config
```bash
#!/bin/bash

########################
# Anomaly Detection Core
########################
SIGNIFICANT_GAP_MULTIPLIER <2.5>     # Gap detection sensitivity. Example: 2.5 (higher value reduces detection frequency).
GAP_MAD_FLOOR <0.0001>               # Minimum gap median absolute deviation. Example: 0.0001.
STD_DEV_FACTOR <2.0>                 # Standard deviation threshold multiplier. Example: 2.0.
IQR_COEFFICIENT <1.5>                # IQR range coefficient. Example: 1.5 (default).

########################
# Large Value Handling
########################
LARGE_VALUE_ADJUSTMENT <1.7>         # Large value standard deviation adjustment. Example: 1.7.
ABSOLUTE_LARGE_THRESHOLD <100>       # Absolute large value threshold. Example: 100.
LARGE_RELATIVE_RATIO <1.8>           # Mean relative ratio threshold for large values. Example: 1.8.

########################
# Score Calculation
########################
SMALL_DEVIATION_EXP <1.2>            # Exponent for small anomalies. Example: 1.2 (>1 amplifies score).
LARGE_DEVIATION_EXP <1.3>            # Exponent for large anomalies. Example: 1.3 (>1 amplifies score).
GAP_SCORE_BOOST <1.3>                # Score boost for neighboring gaps. Example: 1.3.
RANGE_OUTLIER_BOOST <1.8>            # Score boost for out-of-cluster range. Example: 1.8.

########################
# Cluster Identification
########################
CLUSTER_SIZE_WEIGHT <0.6>            # Cluster size weight (range: 0-1). Example: 0.6.
CLUSTER_POSITION_WEIGHT <0.25>       # Central position weight (range: 0-1). Example: 0.25.
CLUSTER_DENSITY_WEIGHT <0.15>        # Density score weight (range: 0-1). Example: 0.15.
NON_MAIN_CLUSTER_BOOST <1.2>         # Non-main cluster multiplier. Example: 1.2.

########################
# General Configuration
########################
MINIMUM_SCORE <1.0>                  # Base score for all anomalies. Example: 1.0.
SENSITIVITY <1.0>                    # Global sensitivity multiplier. Example: 1.0 (1.0: neutral, >1: more sensitive, <1: less sensitive).
```


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
