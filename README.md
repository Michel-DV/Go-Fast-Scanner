# Go-Fast-Scanner
High-performance TCP Port Scanner written in Go. Utilizes Goroutines and Channels to achieve 100x speed compared to sequential scanning. Tooling development showcase.
# High-Performance Concurrent Port Scanner (Go)

![Language](https://img.shields.io/badge/Language-Go-00ADD8?logo=go&logoColor=white)
![Performance](https://img.shields.io/badge/Performance-High-brightgreen)
![Concurrency](https://img.shields.io/badge/Pattern-Worker%20Pool-orange)

## ⚡ Overview
This repository contains a multi-threaded TCP Port Scanner developed in **Golang**. 
Unlike traditional sequential scanners (which test one port at a time), this tool leverages Go's native concurrency primitives (**Goroutines** and **Channels**) to scan hundreds of ports simultaneously.

This project demonstrates the **Worker Pool** design pattern, essential for developing high-performance security tools that need to process large datasets (like network ranges) in seconds.

## 🏗️ Architecture: The Worker Pool

The scanner creates a pool of 100 concurrent workers. Ports are dispatched via a buffered channel, and results are collected asynchronously.

🛠️ Key Concepts Implemented
Goroutines: Lightweight threads managed by the Go runtime for massive concurrency.

Channels: Thread-safe communication pipes to prevent race conditions without complex locking (Mutex).

WaitGroup: Synchronization mechanism to ensure all scans complete before program exit.

net Package: Low-level networking interactions.

🚀 Usage
Build the Binary
---

Bash: go build fast_scan.go

Run the Scanner
Bash:
# Windows
fast_scan.exe
---
# Linux/Mac
./fast_scan
---
Default target is set to scanme.nmap.org for demonstration purposes.

---
⚠️ Disclaimer
EDUCATIONAL USE ONLY. Port scanning without permission is illegal in many jurisdictions. This tool is intended for use on networks you own or have explicit authorization to audit.

Project developed as part of the Advanced Cybersecurity Portfolio.

```mermaid
graph LR
    A[Main Process] -->|Feeds Ports| B(Jobs Channel)
    B --> C{Worker Pool}
    C -->|Worker 1| D[Scan Port]
    C -->|Worker 2| D
    C -->|Worker N| D
    D -->|Results| E(Results Channel)
    E --> F[Result Aggregator]
