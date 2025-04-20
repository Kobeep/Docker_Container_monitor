#!/usr/bin/env python3
import os
import sys
import time
import shutil
import subprocess
import threading

# 1. Elevate via sudo if needed
if os.geteuid() != 0:
    print("🔒 Root privileges are required. Please enter your password…")
    os.execvp("sudo", ["sudo", sys.executable] + sys.argv)

def spinner(text, stop_evt):
    chars = '|/-\\'
    i = 0
    while not stop_evt.is_set():
        sys.stdout.write(f"\r{text} {chars[i % 4]}")
        sys.stdout.flush()
        i += 1
        time.sleep(0.1)
    sys.stdout.write("\r" + " "*(len(text)+2) + "\r")

def remove_service():
    try:
        out = subprocess.run(
            ["systemctl","list-unit-files","monitor.service"],
            capture_output=True, text=True, check=True
        ).stdout
        if "monitor.service" in out:
            print("🔄 Removing old monitor.service and binary…")
            stop_evt = threading.Event()
            t = threading.Thread(target=spinner, args=("Removing…", stop_evt))
            t.start()
            for cmd in [
                "systemctl stop monitor.service",
                "systemctl disable monitor.service",
                "rm -f /usr/local/bin/monitor",
                "rm -f /etc/systemd/system/monitor.service",
                "rm -f /etc/systemd/system/multi-user.target.wants/monitor.service",
            ]:
                subprocess.run(cmd, shell=True)
                time.sleep(0.3)
            stop_evt.set()
            t.join()
            print("✅ Old service removed.")
        else:
            print("ℹ️  No existing monitor.service to remove.")
    except subprocess.CalledProcessError:
        print("⚠️  Could not query systemd; continuing anyway.")

def run(cmd):
    print("▶️ ", cmd)
    subprocess.run(cmd, shell=True, check=True)

def progress(text):
    for i in range(4):
        bar = ("="*(i+1)).ljust(4)
        sys.stdout.write(f"\r{text} [{bar}]")
        sys.stdout.flush()
        time.sleep(0.3)
    print(f"\r✅ {text} complete!")

# Use cwd as project root
PROJECT = os.getcwd()
BUILD = "/tmp/monitor_build"

# 2. Clean up old
remove_service()

# 3. Ensure Go
progress("Checking Go")
if subprocess.run(["which","go"], capture_output=True).returncode != 0:
    print("🔍 Installing Go…")
    run("dnf install -y golang")

# 4. Validate project layout
if not (os.path.isfile(f"{PROJECT}/go.mod") and os.path.isdir(f"{PROJECT}/cmd/monitor")):
    print("❌ cd into project root (with go.mod & cmd/monitor) and rerun.")
    sys.exit(1)

# 5. Copy to build dir
if os.path.exists(BUILD):
    shutil.rmtree(BUILD)
os.makedirs(BUILD)
progress("Copying files")
for item in ("go.mod","go.sum","cmd","internal","config"):
    src = f"{PROJECT}/{item}"
    dst = f"{BUILD}/{item}"
    if os.path.exists(src):
        if os.path.isdir(src):
            shutil.copytree(src, dst)
        else:
            shutil.copy2(src, BUILD)

# 6. Tidy & build
progress("Initializing modules")
run(f"cd {BUILD} && go mod tidy")
progress("Building binary")
run(f"cd {BUILD} && go build -o monitor ./cmd/monitor")

# 7. Install binary
progress("Installing monitor")
run(f"mv {BUILD}/monitor /usr/local/bin/monitor")
run("chmod +x /usr/local/bin/monitor")

# 8. Write and register systemd unit
progress("Configuring systemd")
unit = """[Unit]
Description=Monitor Docker containers & services
After=network.target docker.service

[Service]
ExecStart=/usr/local/bin/monitor serve --port 9090
Restart=always
User=root
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target
"""
with open("/tmp/monitor.service","w") as f:
    f.write(unit)
run("mv /tmp/monitor.service /etc/systemd/system/monitor.service")
run("systemctl daemon-reload")

# 9. Manually enable + start
run("ln -sf /etc/systemd/system/monitor.service /etc/systemd/system/multi-user.target.wants/monitor.service")

print("\n🎉 Installation finished! `monitor` is now in your PATH, and the service is running.")
