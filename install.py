import os
import time
import sys
import subprocess
import threading

def spinner_animation(text, stop_event):
    spinner_chars = ['|', '/', '-', '\\']
    idx = 0
    while not stop_event.is_set():
        sys.stdout.write(f"\r{text} {spinner_chars[idx % len(spinner_chars)]}")
        sys.stdout.flush()
        idx += 1
        time.sleep(0.1)
    sys.stdout.write("\r" + " " * (len(text) + 4) + "\r")
    sys.stdout.flush()

def remove_existing_service():
    """
    Checks if monitor.service exists in systemd and, if it does,
    stops, disables, and removes the service along with its binary,
    displaying an animated spinner during the process.
    """
    try:
        result = subprocess.run(
            ["systemctl", "list-unit-files", "monitor.service"],
            capture_output=True,
            text=True,
            check=True
        )
        if "monitor.service" in result.stdout:
            print("monitor.service found. Removing service and related files...")

            stop_spinner = threading.Event()
            spinner_thread = threading.Thread(target=spinner_animation, args=("Removing monitor service...", stop_spinner))
            spinner_thread.start()

            commands = [
                "sudo systemctl stop monitor",
                "sudo systemctl disable monitor",
                "sudo rm /usr/local/bin/monitor",
                "sudo rm /etc/systemd/system/monitor.service"
            ]
            for cmd in commands:
                subprocess.run(cmd, shell=True, check=True)
                time.sleep(0.5)

            stop_spinner.set()
            spinner_thread.join()
            print("Monitor service removal complete!")
        else:
            print("monitor.service not found. Nothing to do.")
    except subprocess.CalledProcessError as e:
        print(f"Error checking for monitor.service: {e}")

remove_existing_service()

def loading_animation(text):
    for _ in range(5):
        sys.stdout.write(f"\r{text} [ {'-' * (_ % 4)} ]")
        sys.stdout.flush()
        time.sleep(0.5)
    print("\r✅ " + text + " complete!")

loading_animation("Checking Go installation")
if subprocess.run(["which", "go"], capture_output=True).returncode != 0:
    print("🔍 Go is not installed, installing...")
    os.system("sudo dnf install -y golang")

if not os.path.exists("monitor.go"):
    print("❌ Error: monitor.go not found!")
    sys.exit(1)

loading_animation("Copying Go source code")
os.system("mkdir -p /tmp/monitor_build && cp monitor.go go.mod /tmp/monitor_build/")

loading_animation("Initializing Go modules")
os.system("cd /tmp/monitor_build && go mod tidy")

loading_animation("Installing Go dependencies")
os.system("cd /tmp/monitor_build && go get github.com/urfave/cli/v2")

loading_animation("Compiling Go application")
compile_status = os.system("cd /tmp/monitor_build && go build -o monitor monitor.go")
if compile_status != 0:
    print("❌ Error: Failed to compile monitor.go")
    sys.exit(1)

loading_animation("Installing monitor command")
os.system("sudo mv /tmp/monitor_build/monitor /usr/local/bin/")
os.system("sudo chmod +x /usr/local/bin/monitor")

if not os.path.exists("/usr/local/bin/monitor"):
    print("❌ Error: monitor binary not found in /usr/local/bin/")
    sys.exit(1)

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

print("\n🎉 Installation complete! Use:")
print("  ✅ `monitor` → Full container and service status")
print("  ✅ `monitor state` → Displays only container names and states")
print("  ✅ `monitor service` → Displays only service availability")
print("  ✅ `monitor remote` → Displays container and service status on remote hosts")
