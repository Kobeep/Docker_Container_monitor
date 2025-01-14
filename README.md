# Docker_Container_monitor🚀

## CI/CD Status 🚀

![GitHub Workflow Status](https://github.com/Kobeep/Docker_Container_monitor/actions/workflows/go-container-monitor.yml/badge.svg)

## Overview

`Docker_Container_monitor` is a lightweight CLI tool written in `Go` that helps monitor running Docker containers and their services. It provides real-time information about container states and checks if the services inside the containers are available. The project includes a `Python` installer script to automate the setup process.

## Features

- ✅ **Automatic container detection** - No need to manually specify container names.
- ✅ **Service health check** - Verifies if the services inside containers are accessible.
- ✅ **Multiple output modes** - Choose between full status, container state, or service availability.
- ✅ **Simple CLI commands** - Use `monitor` to get an instant overview.
- ✅ **Systemd integration** - Runs as a background service to keep monitoring automatically.
- ✅ **Easy installation** - Fully automated setup with `install.py`.

## Installation

### Prerequisites

- 🐳 Docker installed and running
- 🐍 Python3 installed
- 🦫 Go installed (if not, the installer will install it automatically)

### Steps to Install

1. **Clone the repository**:

```sh
git clone https://github.com/yourusername/Docker_Container_monitor.git
cd Docker_Container_monitor
```

2. **Run the installation script:**

```sh
python3 install.py
```

3. **Verify installation:**

```sh
monitor --help
```

## Usage
### Display full container and service status:

```sh
monitor
```

### Display only container states:

```sh
monitor state
```

### Display only service availability:

```sh
monitor service
```

### Check systemd service status:

```sh
systemctl status monitor
```
## How It Works

🚀 **Retrieves a list of running Docker containers** using `docker ps`
🔌 **Gets exposed ports** for each container
🌐 **Attempts an HTTP request** to determine if the service inside the container is responsive
📊 **Displays results** based on the selected mode

## Uninstallation

To remove the tool:
```sh
sudo systemctl stop monitor
sudo systemctl disable monitor
sudo rm /usr/local/bin/monitor
sudo rm /etc/systemd/system/monitor.service
rm -rf ~/Docker_Container_monitor
```
## Contributing

💡 Contributions are welcome! Feel free to submit a pull request or open an issue.

## License

📜 This project is licensed under the `GNU GPL License`.

## Author

👨‍💻 **Author:** Jakub Pospieszny

## GitHub

📌 **GitHub:** [Kobeep](https://github.com/Kobeep)
