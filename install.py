#!/usr/bin/env python3
"""
Docker Container Monitor Installation Script
Installs the monitor tool and optionally sets up systemd service
"""
import os
import time
import sys
import subprocess
import threading
import shutil
import argparse
from pathlib import Path

# Color codes for better output
class Colors:
    HEADER = '\033[95m'
    BLUE = '\033[94m'
    CYAN = '\033[96m'
    GREEN = '\033[92m'
    YELLOW = '\033[93m'
    RED = '\033[91m'
    ENDC = '\033[0m'
    BOLD = '\033[1m'

def print_color(text, color):
    """Print colored text"""
    print(f"{color}{text}{Colors.ENDC}")

def spinner_animation(text, stop_event):
    """Animated spinner for long-running operations"""
    spinner_chars = ['⠋', '⠙', '⠹', '⠸', '⠼', '⠴', '⠦', '⠧', '⠇', '⠏']
    idx = 0
    while not stop_event.is_set():
        sys.stdout.write(f"\r{Colors.CYAN}{text} {spinner_chars[idx % len(spinner_chars)]}{Colors.ENDC}")
        sys.stdout.flush()
        idx += 1
        time.sleep(0.1)
    sys.stdout.write("\r" + " " * (len(text) + 4) + "\r")
    sys.stdout.flush()

def run_with_spinner(text, command_func):
    """Run a command with a spinner animation"""
    stop_spinner = threading.Event()
    spinner_thread = threading.Thread(target=spinner_animation, args=(text, stop_spinner))
    spinner_thread.start()

    try:
        result = command_func()
        stop_spinner.set()
        spinner_thread.join()
        print_color(f"✅ {text}", Colors.GREEN)
        return result
    except Exception as e:
        stop_spinner.set()
        spinner_thread.join()
        print_color(f"❌ {text} failed: {e}", Colors.RED)
        raise

def check_requirements():
    """Check if required tools are installed"""
    print_color("\n🔍 Checking requirements...", Colors.HEADER)

    # Check if running on Linux
    if sys.platform != 'linux':
        print_color(f"⚠️  Warning: This script is designed for Linux. Current OS: {sys.platform}", Colors.YELLOW)

    # Check Docker
    if shutil.which('docker') is None:
        print_color("❌ Docker is not installed. Please install Docker first.", Colors.RED)
        sys.exit(1)
    else:
        print_color("✅ Docker found", Colors.GREEN)

    # Check Python version
    if sys.version_info < (3, 6):
        print_color(f"❌ Python 3.6+ required. Current: {sys.version}", Colors.RED)
        sys.exit(1)

    return True

def check_go_installation():
    """Check if Go is installed, install if necessary"""
    print_color("\n📦 Checking Go installation...", Colors.HEADER)

    if shutil.which('go') is None:
        print_color("⚠️  Go not found. Installing Go...", Colors.YELLOW)

        # Detect package manager
        if shutil.which('dnf'):
            cmd = "sudo dnf install -y golang"
        elif shutil.which('apt'):
            cmd = "sudo apt update && sudo apt install -y golang"
        elif shutil.which('yum'):
            cmd = "sudo yum install -y golang"
        elif shutil.which('pacman'):
            cmd = "sudo pacman -S --noconfirm go"
        else:
            print_color("❌ Unable to detect package manager. Please install Go manually.", Colors.RED)
            sys.exit(1)

        if os.system(cmd) != 0:
            print_color("❌ Failed to install Go", Colors.RED)
            sys.exit(1)

        print_color("✅ Go installed successfully", Colors.GREEN)
    else:
        go_version = subprocess.run(['go', 'version'], capture_output=True, text=True).stdout.strip()
        print_color(f"✅ Go already installed: {go_version}", Colors.GREEN)

    return True
def remove_existing_service():
    """
    Checks if monitor.service exists in systemd and removes it if present
    """
    print_color("\n🔍 Checking for existing installation...", Colors.HEADER)

    try:
        result = subprocess.run(
            ["systemctl", "list-unit-files", "monitor.service"],
            capture_output=True,
            text=True,
            check=True
        )

        if "monitor.service" not in result.stdout:
            print_color("✅ No existing service found", Colors.GREEN)
            return

        print_color("⚠️  Existing monitor.service found. Removing...", Colors.YELLOW)

        def remove_service():
            commands = [
                ("Stopping service", "sudo systemctl stop monitor"),
                ("Disabling service", "sudo systemctl disable monitor"),
                ("Removing binary", "sudo rm -f /usr/local/bin/monitor"),
                ("Removing service file", "sudo rm -f /etc/systemd/system/monitor.service"),
                ("Reloading systemd", "sudo systemctl daemon-reload")
            ]

            for desc, cmd in commands:
                try:
                    subprocess.run(cmd, shell=True, check=False, capture_output=True)
                    print_color(f"  ✓ {desc}", Colors.GREEN)
                except Exception as e:
                    print_color(f"  ⚠️  {desc} (non-fatal): {e}", Colors.YELLOW)

        remove_service()
        print_color("✅ Previous installation removed", Colors.GREEN)

    except subprocess.CalledProcessError as e:
        print_color(f"⚠️  Error checking service (continuing anyway): {e}", Colors.YELLOW)
    except Exception as e:
        print_color(f"⚠️  Unexpected error (continuing anyway): {e}", Colors.YELLOW)

def build_monitor():
    """Build the monitor binary"""
    print_color("\n🔨 Building monitor...", Colors.HEADER)

    # Check if source files exist
    if not os.path.exists("monitor.go"):
        print_color("❌ Error: monitor.go not found in current directory!", Colors.RED)
        sys.exit(1)

    if not os.path.exists("go.mod"):
        print_color("❌ Error: go.mod not found in current directory!", Colors.RED)
        sys.exit(1)

    # Create temporary build directory
    build_dir = "/tmp/monitor_build"
    os.makedirs(build_dir, exist_ok=True)

    def build():
        # Copy source files
        print_color("  📄 Copying source files...", Colors.CYAN)
        shutil.copy2("monitor.go", f"{build_dir}/monitor.go")
        shutil.copy2("go.mod", f"{build_dir}/go.mod")
        if os.path.exists("go.sum"):
            shutil.copy2("go.sum", f"{build_dir}/go.sum")

        # Download dependencies
        print_color("  📦 Downloading dependencies...", Colors.CYAN)
        result = subprocess.run(
            "go mod download",
            shell=True,
            cwd=build_dir,
            capture_output=True,
            text=True
        )
        if result.returncode != 0:
            raise Exception(f"Failed to download dependencies: {result.stderr}")

        # Build
        print_color("  🔧 Compiling...", Colors.CYAN)
        result = subprocess.run(
            "go build -o monitor monitor.go",
            shell=True,
            cwd=build_dir,
            capture_output=True,
            text=True
        )
        if result.returncode != 0:
            raise Exception(f"Compilation failed: {result.stderr}")

        return f"{build_dir}/monitor"

    try:
        binary_path = build()
        print_color("✅ Build successful", Colors.GREEN)
        return binary_path
    except Exception as e:
        print_color(f"❌ Build failed: {e}", Colors.RED)
        sys.exit(1)

def install_binary(binary_path):
    """Install the binary to /usr/local/bin"""
    print_color("\n📥 Installing binary...", Colors.HEADER)

    install_path = "/usr/local/bin/monitor"

    try:
        # Copy binary
        result = subprocess.run(
            f"sudo cp {binary_path} {install_path}",
            shell=True,
            capture_output=True,
            text=True
        )
        if result.returncode != 0:
            raise Exception(f"Failed to copy binary: {result.stderr}")

        # Make executable
        result = subprocess.run(
            f"sudo chmod +x {install_path}",
            shell=True,
            capture_output=True,
            text=True
        )
        if result.returncode != 0:
            raise Exception(f"Failed to set permissions: {result.stderr}")

        # Verify installation
        if not os.path.exists(install_path):
            raise Exception(f"Binary not found at {install_path}")

        print_color(f"✅ Binary installed to {install_path}", Colors.GREEN)
        return True

    except Exception as e:
        print_color(f"❌ Installation failed: {e}", Colors.RED)
        sys.exit(1)

def setup_systemd_service():
    """Set up systemd service (optional)"""
    print_color("\n⚙️  Setting up systemd service...", Colors.HEADER)

    service_config = """[Unit]
Description=Docker Container Monitor
Documentation=https://github.com/Kobeep/Docker_Container_monitor
After=network.target docker.service
Wants=docker.service

[Service]
Type=simple
ExecStart=/usr/local/bin/monitor watch --interval 10
Restart=on-failure
RestartSec=10
User=root
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target
"""

    try:
        # Write service file
        service_path = "/tmp/monitor.service"
        with open(service_path, "w") as f:
            f.write(service_config)

        # Install service
        subprocess.run("sudo cp /tmp/monitor.service /etc/systemd/system/monitor.service",
                      shell=True, check=True, capture_output=True)
        subprocess.run("sudo systemctl daemon-reload",
                      shell=True, check=True, capture_output=True)
        subprocess.run("sudo systemctl enable monitor",
                      shell=True, check=True, capture_output=True)
        subprocess.run("sudo systemctl start monitor",
                      shell=True, check=True, capture_output=True)

        print_color("✅ Systemd service configured and started", Colors.GREEN)
        print_color("  ℹ️  Service runs: monitor watch --interval 10", Colors.CYAN)
        print_color("  ℹ️  Check status: sudo systemctl status monitor", Colors.CYAN)

    except Exception as e:
        print_color(f"❌ Systemd setup failed: {e}", Colors.RED)
        print_color("  ℹ️  You can still use the command-line tool", Colors.YELLOW)

def print_usage():
    """Print usage instructions"""
    print_color("\n" + "="*70, Colors.BLUE)
    print_color("🎉 Installation Complete!", Colors.HEADER + Colors.BOLD)
    print_color("="*70, Colors.BLUE)

    print_color("\n📖 Usage Examples:", Colors.HEADER)
    print_color("  monitor                           # Full status", Colors.CYAN)
    print_color("  monitor state                     # Container states only", Colors.CYAN)
    print_color("  monitor service                   # Service health checks", Colors.CYAN)
    print_color("  monitor stats                     # Resource usage (NEW!)", Colors.CYAN)
    print_color("  monitor watch --interval 5        # Auto-refresh every 5s (NEW!)", Colors.CYAN)
    print_color("  monitor logs <container>          # Stream container logs (NEW!)", Colors.CYAN)
    print_color("  monitor --filter 'name=nginx'     # Filter containers (NEW!)", Colors.CYAN)
    print_color("  monitor remote --host <alias>     # Monitor remote host", Colors.CYAN)
    print_color("  monitor events                    # Watch Docker events", Colors.CYAN)
    print_color("  monitor --help                    # Show all options", Colors.CYAN)

    print_color("\n🔧 System Commands:", Colors.HEADER)
    print_color("  sudo systemctl status monitor     # Check service status", Colors.CYAN)
    print_color("  sudo systemctl stop monitor       # Stop service", Colors.CYAN)
    print_color("  sudo systemctl start monitor      # Start service", Colors.CYAN)
    print_color("  sudo systemctl restart monitor    # Restart service", Colors.CYAN)

    print_color("\n📚 Documentation:", Colors.HEADER)
    print_color("  GitHub: https://github.com/Kobeep/Docker_Container_monitor", Colors.CYAN)
    print_color("="*70 + "\n", Colors.BLUE)

def main():
    """Main installation flow"""
    parser = argparse.ArgumentParser(
        description='Install Docker Container Monitor',
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog='''
Examples:
  python3 install.py                    # Full installation with systemd
  python3 install.py --no-systemd       # Install binary only
  python3 install.py --uninstall        # Uninstall everything
        '''
    )
    parser.add_argument('--no-systemd', action='store_true',
                       help='Skip systemd service installation')
    parser.add_argument('--uninstall', action='store_true',
                       help='Uninstall monitor and service')

    args = parser.parse_args()

    print_color("\n" + "="*70, Colors.BLUE)
    print_color("🐳 Docker Container Monitor - Installation Script", Colors.HEADER + Colors.BOLD)
    print_color("="*70 + "\n", Colors.BLUE)

    if args.uninstall:
        print_color("🗑️  Uninstalling...", Colors.YELLOW)
        remove_existing_service()
        print_color("\n✅ Uninstallation complete!", Colors.GREEN)
        return

    try:
        # Check requirements
        check_requirements()

        # Check/install Go
        check_go_installation()

        # Remove existing installation
        remove_existing_service()

        # Build binary
        binary_path = build_monitor()

        # Install binary
        install_binary(binary_path)

        # Setup systemd (optional)
        if not args.no_systemd:
            response = input(f"\n{Colors.YELLOW}Do you want to set up systemd service? (Y/n): {Colors.ENDC}").strip().lower()
            if response in ['', 'y', 'yes']:
                setup_systemd_service()
            else:
                print_color("⏭️  Skipping systemd service setup", Colors.YELLOW)
        else:
            print_color("⏭️  Systemd service setup skipped (--no-systemd flag)", Colors.YELLOW)

        # Print usage instructions
        print_usage()

        # Cleanup
        print_color("🧹 Cleaning up temporary files...", Colors.CYAN)
        shutil.rmtree("/tmp/monitor_build", ignore_errors=True)

    except KeyboardInterrupt:
        print_color("\n\n⚠️  Installation cancelled by user", Colors.YELLOW)
        sys.exit(1)
    except Exception as e:
        print_color(f"\n\n❌ Installation failed: {e}", Colors.RED)
        sys.exit(1)

if __name__ == "__main__":
    main()
