

# Arcturus 🌌  
*A Cloud-Native Global Accelerator Framework*



## 📌 Overview  
**Arcturus** redefines **Global Acceleration (GA)** by dynamically orchestrating low-cost, multi-cloud resources to deliver **high-performance, low-latency, and highly stable networking** — all without the constraints of vendor lock-in.

Unlike traditional cloud-tied GA solutions, Arcturus achieves **1.7× faster acceleration at 71% lower cost** on the forwarding plane, while sustaining over **80% resource efficiency**, significantly boosting both performance and stability at the data plane level.

On the scheduling plane, Arcturus *analyzes the heterogeneity of hybrid-cloud infrastructure and the dynamic behavior of acceleration workloads*, and based on these insights, *designs specialized scheduling strategies*. It combines **last-mile scheduling based on Lyapunov optimization** with **mid-mile scheduling formulated as a constrained maximum flow problem**, jointly optimizing acceleration performance and system stability to achieve **Pareto-optimal efficiency** across diverse network conditions.




**Aim to**:  
- Real-time interactive applications  
- Cost-sensitive large-scale deployments  
- Multi-cloud or hybrid-cloud environments  


## ✨ Key Features  
| **Feature**               | **Advantage**                                                                 |
|---------------------------|-------------------------------------------------------------------------------|
| **Multi-Cloud Adaptive**  | Leverages heterogeneous resources across providers (AWS, GCP, etc.)    |
| **Two-Plane Architecture**| Forwarding plane (adaptive proxies) + Scheduling plane (lightweight optimization) |
| **High Performance**  | 1.7× faster acceleration​​ via streaming multiplexing, CMAB tuning, and segment routing |
| **​​Stability Under Load​**| Lyapunov-based load balancing (last-mile) + Carousel Greedy algorithm for path diversity (mid-mile) |

## 🏆 Comparative Metrics
### Public Internet vs Arcturus Latency Comparison (Washington / Osaka / London Server Deployment)

| Route                      | Direct (ms) | Arcturus (ms) | Improvement |
|----------------------------|-------------|---------------|-------------|
| Mexico → Osaka             | 150         | 74            | 50.67%     |
| Santiago → Washington      | 143         | 77            | 46.15%     |
| Bangalore → Osaka         | 112         | 78            | 30.36%     |
| Querétaro → London        | 123         | 74            | 39.84%     |
| Singapore → London        | 171         | 99            | 42.11%     |
| Tel Aviv → Washington     | 133         | 75            | 43.61%     |
| Frankfurt → Osaka         | 190         | 104           | 45.26%     |
| **Average Latency**          | **146.0**   | **83.3**      | **42.54%** |

## Global Latency Comparison: GCP vs. AWS vs. Arcturus

| Route                  | Provider   | Baseline Latency (ms) | Arcturus Latency (ms) | Improvement (%) |
|------------------------|------------|----------------------|----------------------|-----------------|
| Bangalore → New York   | GCP        | 267.0                | 142.0                | 46.8%          |
| Johannesburg → NY      | GCP        | 176.0                | 128.0                | 27.3%          |
| London → NY            | GCP        | 74.0                 | 61.0                 | 17.6%          |
| London → Los Angeles   | AWS GA     | 135.0                | 91.0                 | 32.6%          |
| Virginia → LA          | AWS GA     | 73.0                 | 43.0                 | 41.1%          |
| Singapore → LA         | AWS GA     | 177.0                | 138.0                | 22.0%          |
| **Average**           | —          | **150.3**            | **100.5**            | **31.2%**   |

## 🏗️ Architecture 
![Scheduling Architecture](scheduling/assets/Arcturus.svg)

## 🚀 Quick Start Guide

### 📋 Prerequisites
| Requirement       | Version  | Verification Command       |
|-------------------|----------|----------------------------|
| Kubernetes        | ≥1.23    | `kubectl version --short`  |
| Terraform         | ≥1.4     | `terraform --version`       |
| Helm              | ≥3.11    | `helm version --short`      |

## 🛠️ Installation
### Method : Helm 
```bash
# Add Arcturus repo
helm repo add arcturus https://charts.arcturus.io/stable

# Install with production profile
helm install arcturus arcturus/arcturus \
  --namespace arcturus-system \
  --create-namespace \
  --values https://raw.githubusercontent.com/your-repo/arcturus/main/config/production.yaml
```

## License Agreement

## 📑 Apache 2.0 Summary
Permits:
- ✅ Commercial use  
- ✅ Modification  
- ✅ Patent use  
- ✅ Private use  

Requirements:
- ℹ️ License and copyright notice preservation  
- ℹ️ State changes  

