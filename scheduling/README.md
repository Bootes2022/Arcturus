# Arcturus Scheduling

**Arcturus** is a cloud-native global accelerator framework designed to optimize networking for real-time applications. This module focuses on **Last-Mile** and **Middle-Mile Scheduling** with cutting-edge optimization techniques to ensure high performance and low latency.

## Table of Contents
- [Overview](#overview)
- [Last-Mile Scheduling](#last-mile-scheduling)
  - [Lyapunov Drift and Optimization Modeling](#lyapunov-drift-and-optimization-modeling)
  - [BPR Solutions for Lyapunov Optimization](#bpr-solutions-for-lyapunov-optimization)
- [Middle-Mile Scheduling](#middle-mile-scheduling)
  - [MFPC Modeling](#mfpc-modeling)
  - [Carousel Greedy Approach for MFPC](#carousel-greedy-approach-for-mfpc)

## Overview
Arcturus employs a **two-plane architecture** for scheduling, which includes the **Forwarding Plane** (focused on proxies) and the **Scheduling Plane** (dedicated to optimization). The **Scheduling Plane** addresses both **Last-Mile Scheduling** and **Middle-Mile Scheduling**, ensuring that the system balances CPU utilization, network latency, and stability. This scheduling mechanism enables Arcturus to handle **multi-cloud environments** efficiently while minimizing resource overhead and maintaining high system stability.

## Last-Mile Scheduling
### Lyapunov Drift and Optimization Modeling
In Last-Mile Scheduling, regional user requests are mapped to proxy nodes. These proxy nodes are modeled as a system of **N queues**. The queue backlog, denoted by `Q(t)`, represents the cumulative deviation of the average CPU load from a predefined threshold (θ), typically set to 60%. This queue update equation ensures system stability while optimizing CPU load distribution:

```text
Qk(t + 1) = max(Qk(t) + cpu¯t,onset_k + δcpu¯t,in_k − θ, 0)
