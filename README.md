# 🐳 Docker Container Monitor

[![CI/CD Status](https://github.com/Kobeep/Docker_Container_monitor/actions/workflows/CICD.yml/badge.svg)](https://github.com/Kobeep/Docker_Container_monitor/actions)
[![Go Version](https://img.shields.io/badge/Go-1.22+-00ADD8?style=flat&logo=go)](https://golang.org/)
[![License](https://img.shields.io/badge/License-GPL-blue.svg)](LICENSE)

> A powerful, production-ready CLI tool for monitoring Docker containers, tracking resource usage, and managing containerized applications across local and remote hosts.

**Docker Container Monitor** is designed for DevOps engineers and developers who need comprehensive insights into their containerized infrastructure. With features like continuous monitoring, resource statistics, service health checks, and remote SSH support, it's an essential tool for container management.

---

## ✨ Features

| Feature | Description |
|---------|-------------|
| ⏰ **Watch Mode** | Continuous auto-refresh monitoring - no more manual re-runs! |
| 📊 **Resource Statistics** | Real-time CPU, memory, and network usage with color-coded warnings |
| 🔍 **Container Filtering** | Focus on specific containers by name, status, or labels |
| 🏥 **Service Health Checks** | Automatic HTTP endpoint probing for service availability |
| 📜 **Log Streaming** | Built-in container log viewer with follow mode |
| 🌐 **Remote Monitoring** | Monitor Docker hosts over SSH using config aliases |
| 📡 **Docker Events** | Subscribe to real-time Docker lifecycle events |
| 🔧 **Multiple Output Formats** | Human-readable or JSON for easy scripting |
| ⚙️ **Systemd Integration** | Optional background service for continuous monitoring |
| 🎨 **Color-Coded Output** | Visual indicators for quick status recognition |

---

## 📋 Table of Contents

- [Installation](#-installation)
- [Quick Start](#-quick-start)
- [Usage Examples](#-usage-examples)
- [Features in Detail](#-features-in-detail)
- [Configuration](#-configuration)
- [Systemd Service](#-systemd-service)
- [Uninstallation](#-uninstallation)
- [Development](#-development)
- [Contributing](#-contributing)
- [License](#-license)

---

## 🚀 Installation

### Prerequisites

- **Docker** (20.10+) - Required for container monitoring
- **Python 3.6+** - For the installation script
- **Go 1.22+** - Automatically installed if missing
- **SSH** (optional) - For remote monitoring

### Automated Installation

```bash
# Clone the repository
git clone https://github.com/Kobeep/Docker_Container_monitor.git
cd Docker_Container_monitor

# Run the installer (automatically handles dependencies)
python3 install.py

# Verify installation
monitor --version
```

The installer will:
- ✅ Check and install Go if needed (Fedora/Ubuntu/Debian/RHEL/Arch supported)
- ✅ Verify Docker installation and version
- ✅ Build the monitor binary
- ✅ Install to `/usr/local/bin`
- ✅ Optionally set up systemd service

**Installation Options:**

```bash
python3 install.py --help           # Show all options
python3 install.py --no-systemd     # Skip systemd service setup
python3 install.py --uninstall      # Remove installation
```

---

## 🎯 Quick Start

Once installed, start monitoring immediately:

```bash
# Basic container status
monitor

# Watch mode with 3-second refresh
monitor watch --interval 3

# Show resource usage statistics
monitor stats

# Filter specific containers
monitor --filter "name=nginx"

# Stream container logs
monitor logs <container-name> --follow
```

---

## 📖 Usage Examples

### Basic Monitoring

**Full status overview:**

```bash
monitor
```

Shows containers, their states, exposed ports, and service health.

![Full Status](./readme/monitor.png)

**Container states only:**

```bash
monitor state
```

![Container States](./readme/monitor-state.png)

**Service availability check:**

```bash
monitor service
```

![Service Check](./readme/monitor-service.png)

### Watch Mode (New!)

Continuous monitoring with auto-refresh:

```bash
# Refresh every 2 seconds (default)
monitor watch

# Custom interval
monitor watch --interval 5

# Exit with Ctrl+C
```

### Resource Statistics (New!)

View CPU, memory, and network usage:

```bash
monitor stats
```

**Output includes:**
- 🔴 Red: High resource usage (>80%)
- 🟡 Yellow: Medium usage (50-80%)
- 🟢 Green: Normal usage (<50%)

### Container Filtering (New!)

Focus on specific containers:

```bash
# Filter by name
monitor --filter "name=nginx"

# Filter by status
monitor --filter "status=running"

# Filter by label
monitor --filter "label=env=production"
```

### Log Streaming (New!)

Built-in log viewer:

```bash
# View last 100 lines
monitor logs <container-name>

# Follow mode (like docker logs -f)
monitor logs <container-name> --follow

# Limit lines
monitor logs <container-name> --tail 50
```

### Remote Monitoring

Monitor Docker on remote hosts via SSH:

```bash
# Using SSH config alias
monitor remote --host production-server

# The tool uses your ~/.ssh/config for authentication
```

### Docker Events

Real-time event monitoring:

```bash
# Human-readable format
monitor events

# JSON output for scripting
monitor events --json
```

---

## 🔧 Features in Detail

### How It Works

1. **Container Discovery**: Uses Docker API to list all running containers
2. **Port Detection**: Identifies exposed ports and their protocols
3. **Service Health**: Sends HTTP requests to verify service availability
4. **Resource Tracking**: Collects CPU, memory, and network metrics via Docker stats
5. **Event Subscription**: Listens to Docker daemon events for real-time updates

### Architecture

```
┌─────────────────┐
│   CLI Layer     │  (urfave/cli)
├─────────────────┤
│ Monitor Logic   │  (Go routines for concurrent checks)
├─────────────────┤
│  Docker Client  │  (Docker API v25.0+)
├─────────────────┤
│ SSH Transport   │  (Optional remote monitoring)
└─────────────────┘
```

### Output Modes

| Mode | Command | Description |
|------|---------|-------------|
| Full | `monitor` | All information (default) |
| State | `monitor state` | Container states only |
| Service | `monitor service` | Service availability only |
| Watch | `monitor watch` | Continuous monitoring |
| Stats | `monitor stats` | Resource usage |
| Events | `monitor events` | Real-time Docker events |
| Logs | `monitor logs` | Container log streaming |

---

## ⚙️ Configuration

### SSH Configuration

For remote monitoring, set up `~/.ssh/config`:

```ssh-config
Host production
    HostName prod.example.com
    User admin
    IdentityFile ~/.ssh/prod_key
    Port 22
```

Then use:

```bash
monitor remote --host production
```

### Environment Variables

| Variable | Purpose | Default |
|----------|---------|---------|
| `DOCKER_HOST` | Docker daemon socket | `unix:///var/run/docker.sock` |
| `DOCKER_API_VERSION` | API version | Auto-detected |

---

## 🔄 Systemd Service

### Setup Service

During installation, you can enable systemd integration:

```bash
python3 install.py  # Will prompt for systemd setup
```

Or manually:

```bash
sudo systemctl enable monitor
sudo systemctl start monitor
```

### Check Service Status

```bash
systemctl status monitor

# View logs
journalctl -u monitor -f
```

### Service Configuration

The service runs in background mode and logs to systemd journal. Edit the service file:

```bash
sudo systemctl edit monitor
```

---

## 🗑️ Uninstallation

### Using Installer

```bash
python3 install.py --uninstall
```

### Manual Removal

```bash
# Stop and disable service
sudo systemctl stop monitor
sudo systemctl disable monitor

# Remove files
sudo rm /usr/local/bin/monitor
sudo rm /etc/systemd/system/monitor.service
sudo systemctl daemon-reload

# Remove repository
rm -rf ~/Docker_Container_monitor
```

---

## 🛠️ Development

### Building from Source

```bash
# Install dependencies
go mod download

# Build binary
go build -o monitor monitor.go

# Run tests
go test ./...

# Install locally
sudo cp monitor /usr/local/bin/
```

### Project Structure

```
Docker_Container_monitor/
├── monitor.go              # Main application
├── install.py              # Installation script
├── go.mod                  # Go dependencies
├── go.sum                  # Dependency checksums
├── README.md               # This file
├── LICENSE                 # GPL License
└── readme/                 # Screenshots
    ├── monitor.png
    ├── monitor-state.png
    └── monitor-service.png
```

### Dependencies

- `github.com/docker/docker` - Docker Engine API
- `github.com/urfave/cli/v2` - CLI framework
- `github.com/fatih/color` - Colored output
- `golang.org/x/crypto/ssh` - SSH client
- `github.com/kevinburke/ssh_config` - SSH config parsing

---

## 🤝 Contributing

Contributions are welcome! Here's how you can help:

1. **Fork the repository**
2. **Create a feature branch**: `git checkout -b feature/amazing-feature`
3. **Commit your changes**: `git commit -m 'Add amazing feature'`
4. **Push to the branch**: `git push origin feature/amazing-feature`
5. **Open a Pull Request**

### Guidelines

- Follow Go best practices and conventions
- Add tests for new features
- Update documentation as needed
- Keep commits atomic and well-described

---

## 📄 License

This project is licensed under the **GNU General Public License v3.0** - see the [LICENSE](LICENSE) file for details.

---

## � Author

**Jakub Pospieszny**

- GitHub: [@Kobeep](https://github.com/Kobeep)
- Project: [Docker Container Monitor](https://github.com/Kobeep/Docker_Container_monitor)

---

## 🌟 Acknowledgments

Built with:
- [Go](https://golang.org/) - Programming language
- [Docker Engine API](https://docs.docker.com/engine/api/) - Container management
- [urfave/cli](https://github.com/urfave/cli) - CLI framework

---

<div align="center">

**⭐ Star this repository if you find it helpful!**

</div>
