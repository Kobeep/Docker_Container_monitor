import os
import time
import sys
import subprocess

# Loading animation
def loading_animation(text):
    for _ in range(5):
        sys.stdout.write(f"\r{text} [ {'-' * (_ % 4)} ]")
        sys.stdout.flush()
        time.sleep(0.5)
    print("\r✅ " + text + " complete!")

# Check if Go is installed
loading_animation("Checking Go installation")
if subprocess.run(["which", "go"], capture_output=True).returncode != 0:
    print("🔍 Go is not installed, installing...")
    os.system("sudo dnf install -y golang")

# Ensure monitor.go exists
if not os.path.exists("monitor.go"):
    print("❌ Error: monitor.go not found!")
    sys.exit(1)

# Copy monitor.go and go.mod to /tmp
loading_animation("Copying Go source code")
os.system("mkdir -p /tmp/monitor_build && cp monitor.go go.mod /tmp/monitor_build/")

# Initialize Go modules in /tmp
loading_animation("Initializing Go modules")
os.system("cd /tmp/monitor_build && go mod tidy")

# Install dependencies (Ensure CLI package is installed)
loading_animation("Installing Go dependencies")
os.system("cd /tmp/monitor_build && go get github.com/urfave/cli/v2")

# Compile `monitor.go`
loading_animation("Compiling Go application")
compile_status = os.system("cd /tmp/monitor_build && go build -o monitor monitor.go")
if compile_status != 0:
    print("❌ Error: Failed to compile monitor.go")
    sys.exit(1)

# Move binary to `/usr/local/bin`
loading_animation("Installing monitor command")
os.system("sudo mv /tmp/monitor_build/monitor /usr/local/bin/")
os.system("sudo chmod +x /usr/local/bin/monitor")

# Verify installation
if not os.path.exists("/usr/local/bin/monitor"):
    print("❌ Error: monitor binary not found in /usr/local/bin/")
    sys.exit(1)

# Create systemd service
loading_animation("Setting up systemd service")
service_config = """
[Unit]
Description=Monitor Docker containers and services
After=network.target docker.service

[Service]
ExecStart=/usr/local/bin/monitor
Restart=always
User=root

[Install]
WantedBy=multi-user.target
"""

with open("/tmp/monitor.service", "w") as f:
    f.write(service_config)

os.system("sudo mv /tmp/monitor.service /etc/systemd/system/monitor.service")
os.system("sudo systemctl daemon-reload")
os.system("sudo systemctl enable monitor")
os.system("sudo systemctl start monitor")

# Final message
print("\n🎉 Installation complete! Use:")
print("  ✅ `monitor` → Full container and service status")
print("  ✅ `monitor state` → Displays only container names and states")
print("  ✅ `monitor service` → Displays only service availability")
