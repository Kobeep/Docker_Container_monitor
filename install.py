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

# Copy `monitor.go` to temp directory
loading_animation("Copying Go source code")
os.system("cp monitor.go /tmp/monitor.go")

#Compile `monitor.go`
loading_animation("Compiling Go application")
os.system("cd /tmp && go build -o monitor monitor.go")

# Move binary to `/usr/local/bin`
loading_animation("Installing monitor command")
os.system("sudo mv /tmp/monitor /usr/local/bin/")

#  Create systemd service
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
