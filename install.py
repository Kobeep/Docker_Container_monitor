#!/usr/bin/env python3
"""
Docker Container Monitor - Installation Script
Automates installation with dependency checking, Go setup, and systemd integration
"""

import os
import sys
import subprocess
import shutil
import argparse
from pathlib import Path


class Colors:
    """ANSI color codes for terminal output"""
    HEADER = '\033[95m'
    OKBLUE = '\033[94m'
    OKCYAN = '\033[96m'
    OKGREEN = '\033[92m'
    WARNING = '\033[93m'
    FAIL = '\033[91m'
    ENDC = '\033[0m'
    BOLD = '\033[1m'
    UNDERLINE = '\033[4m'


def print_header(text):
    """Print a formatted header"""
    print(f"\n{Colors.HEADER}{Colors.BOLD}{'='*70}{Colors.ENDC}")
    print(f"{Colors.HEADER}{Colors.BOLD}{text.center(70)}{Colors.ENDC}")
    print(f"{Colors.HEADER}{Colors.BOLD}{'='*70}{Colors.ENDC}\n")


def print_success(text):
    """Print success message"""
    print(f"{Colors.OKGREEN}✅ {text}{Colors.ENDC}")


def print_error(text):
    """Print error message"""
    print(f"{Colors.FAIL}❌ {text}{Colors.ENDC}")


def print_warning(text):
    """Print warning message"""
    print(f"{Colors.WARNING}⚠️  {text}{Colors.ENDC}")


def print_info(text):
    """Print info message"""
    print(f"{Colors.OKCYAN}ℹ️  {text}{Colors.ENDC}")


def run_command(command, check=True, capture_output=False, shell=False):
    """
    Execute a shell command with error handling
    """
    try:
        if capture_output:
            result = subprocess.run(
                command if shell else command.split(),
                capture_output=True,
                text=True,
                check=check,
                shell=shell
            )
            return result.returncode == 0, result.stdout, result.stderr
        else:
            result = subprocess.run(
                command if shell else command.split(),
                check=check,
                shell=shell
            )
            return result.returncode == 0, "", ""
    except subprocess.CalledProcessError as e:
        return False, "", str(e)
    except Exception as e:
        return False, "", str(e)


def check_requirements():
    """Check if Docker and Python are installed"""
    print_info("Checking system requirements...")

    # Check Docker
    success, stdout, _ = run_command("docker --version", capture_output=True)
    if not success:
        print_error("Docker is not installed or not running!")
        print_info("Please install Docker: https://docs.docker.com/get-docker/")
        sys.exit(1)
    print_success(f"Docker found: {stdout.strip()}")

    # Check Python version
    if sys.version_info < (3, 6):
        print_error("Python 3.6 or higher is required!")
        sys.exit(1)
    print_success(f"Python {sys.version_info.major}.{sys.version_info.minor} found")


def detect_distro():
    """Detect Linux distribution"""
    try:
        with open('/etc/os-release', 'r') as f:
            content = f.read().lower()
            if 'fedora' in content or 'rhel' in content or 'centos' in content:
                return 'fedora'
            elif 'ubuntu' in content or 'debian' in content:
                return 'debian'
            elif 'arch' in content:
                return 'arch'
    except FileNotFoundError:
        pass
    return 'unknown'


def check_go_installation():
    """Check if Go is installed, install if missing"""
    print_info("Checking Go installation...")

    success, stdout, _ = run_command("go version", capture_output=True)
    if success:
        print_success(f"Go found: {stdout.strip()}")
        return True

    print_warning("Go is not installed. Attempting to install...")
    distro = detect_distro()

    install_commands = {
        'fedora': 'sudo dnf install -y golang',
        'debian': 'sudo apt update && sudo apt install -y golang-go',
        'arch': 'sudo pacman -S --noconfirm go'
    }

    if distro in install_commands:
        print_info(f"Detected {distro.capitalize()}-based system")
        success, _, stderr = run_command(install_commands[distro], shell=True)
        if success:
            print_success("Go installed successfully!")
            return True
        else:
            print_error(f"Failed to install Go: {stderr}")
            print_info("Please install Go manually: https://golang.org/doc/install")
            sys.exit(1)
    else:
        print_error("Could not detect distribution")
        print_info("Please install Go manually: https://golang.org/doc/install")
        sys.exit(1)


def remove_existing_service():
    """Remove existing monitor service if present"""
    print_info("Checking for existing installation...")

    success, stdout, _ = run_command(
        "systemctl list-unit-files monitor.service",
        capture_output=True
    )

    if success and "monitor.service" in stdout:
        print_warning("Existing installation found. Removing...")

        commands = [
            "sudo systemctl stop monitor",
            "sudo systemctl disable monitor",
            "sudo rm -f /usr/local/bin/monitor",
            "sudo rm -f /etc/systemd/system/monitor.service"
        ]

        for cmd in commands:
            run_command(cmd, check=False, shell=True)

        run_command("sudo systemctl daemon-reload", shell=True)
        print_success("Previous installation removed")
    else:
        print_success("No existing installation found")


def build_monitor():
    """Build the monitor binary"""
    print_header("Building Monitor")

    if not os.path.exists("monitor.go"):
        print_error("monitor.go not found in current directory!")
        sys.exit(1)

    if not os.path.exists("go.mod"):
        print_error("go.mod not found in current directory!")
        sys.exit(1)

    # Create build directory
    build_dir = "/tmp/monitor_build"
    if os.path.exists(build_dir):
        shutil.rmtree(build_dir)
    os.makedirs(build_dir)

    print_info("Copying source files to build directory...")
    shutil.copy("monitor.go", build_dir)
    shutil.copy("go.mod", build_dir)
    if os.path.exists("go.sum"):
        shutil.copy("go.sum", build_dir)

    # Change to build directory
    original_dir = os.getcwd()
    os.chdir(build_dir)

    try:
        print_info("Downloading Go dependencies...")
        success, _, stderr = run_command("go mod download", capture_output=True)
        if not success:
            print_error(f"Failed to download dependencies: {stderr}")
            sys.exit(1)

        print_info("Running go mod tidy...")
        run_command("go mod tidy")

        print_info("Building binary...")
        success, _, stderr = run_command(
            "go build -o monitor monitor.go",
            capture_output=True
        )
        if not success:
            print_error(f"Build failed: {stderr}")
            sys.exit(1)

        if not os.path.exists("monitor"):
            print_error("Binary was not created!")
            sys.exit(1)

        print_success("Monitor binary built successfully!")

    finally:
        os.chdir(original_dir)

    return build_dir


def install_binary(build_dir):
    """Install the monitor binary to /usr/local/bin"""
    print_info("Installing monitor binary...")

    binary_path = os.path.join(build_dir, "monitor")
    success, _, stderr = run_command(
        f"sudo cp {binary_path} /usr/local/bin/monitor",
        shell=True
    )
    if not success:
        print_error(f"Failed to copy binary: {stderr}")
        sys.exit(1)

    run_command("sudo chmod +x /usr/local/bin/monitor", shell=True)

    if not os.path.exists("/usr/local/bin/monitor"):
        print_error("Binary installation verification failed!")
        sys.exit(1)

    print_success("Binary installed to /usr/local/bin/monitor")


def setup_systemd_service():
    """Create and enable systemd service"""
    print_info("Setting up systemd service...")

    service_content = """[Unit]
Description=Docker Container Monitor
Documentation=https://github.com/Kobeep/Docker_Container_monitor
After=network.target docker.service
Wants=docker.service

[Service]
Type=simple
ExecStart=/usr/local/bin/monitor
Restart=on-failure
RestartSec=10
User=root
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target
"""

    service_path = "/tmp/monitor.service"
    with open(service_path, "w") as f:
        f.write(service_content)

    run_command(f"sudo mv {service_path} /etc/systemd/system/monitor.service", shell=True)
    run_command("sudo systemctl daemon-reload", shell=True)
    run_command("sudo systemctl enable monitor", shell=True)
    run_command("sudo systemctl start monitor", shell=True)

    # Verify service status
    success, stdout, _ = run_command(
        "systemctl is-active monitor",
        capture_output=True,
        check=False
    )

    if "active" in stdout:
        print_success("Systemd service configured and started!")
    else:
        print_warning("Service installed but may not be running. Check: systemctl status monitor")


def uninstall():
    """Uninstall monitor completely"""
    print_header("Uninstalling Monitor")

    print_info("Stopping and removing service...")
    commands = [
        "sudo systemctl stop monitor",
        "sudo systemctl disable monitor",
        "sudo rm -f /usr/local/bin/monitor",
        "sudo rm -f /etc/systemd/system/monitor.service"
    ]

    for cmd in commands:
        run_command(cmd, check=False, shell=True)

    run_command("sudo systemctl daemon-reload", shell=True)

    print_success("Monitor uninstalled successfully!")
    print_info("To remove the source code: rm -rf ~/Docker_Container_monitor")


def main():
    """Main installation flow"""
    parser = argparse.ArgumentParser(
        description="Docker Container Monitor Installer",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Examples:
  python3 install.py                 # Full installation with systemd
  python3 install.py --no-systemd    # Install without systemd service
  python3 install.py --uninstall     # Remove installation
"""
    )
    parser.add_argument(
        '--no-systemd',
        action='store_true',
        help='Skip systemd service setup'
    )
    parser.add_argument(
        '--uninstall',
        action='store_true',
        help='Uninstall monitor'
    )

    args = parser.parse_args()

    if args.uninstall:
        uninstall()
        return

    print_header("Docker Container Monitor - Installer v1.1.0")

    # Pre-installation checks
    check_requirements()
    check_go_installation()
    remove_existing_service()

    # Build and install
    build_dir = build_monitor()
    install_binary(build_dir)

    # Optional systemd setup
    if not args.no_systemd:
        setup_systemd_service()
    else:
        print_warning("Skipping systemd service setup (--no-systemd flag)")

    # Clean up
    if os.path.exists(build_dir):
        shutil.rmtree(build_dir)

    # Success message
    print_header("Installation Complete!")
    print_success("Monitor installed successfully!\n")

    print(f"{Colors.BOLD}Available Commands:{Colors.ENDC}")
    print(f"  {Colors.OKCYAN}monitor{Colors.ENDC}                          - Full container status")
    print(f"  {Colors.OKCYAN}monitor state{Colors.ENDC}                    - Container states only")
    print(f"  {Colors.OKCYAN}monitor service{Colors.ENDC}                  - Service availability")
    print(f"  {Colors.OKCYAN}monitor watch{Colors.ENDC}                    - Continuous monitoring")
    print(f"  {Colors.OKCYAN}monitor watch --interval 5{Colors.ENDC}       - Custom refresh interval")
    print(f"  {Colors.OKCYAN}monitor stats{Colors.ENDC}                    - Resource statistics")
    print(f"  {Colors.OKCYAN}monitor logs <container>{Colors.ENDC}         - Stream container logs")
    print(f"  {Colors.OKCYAN}monitor --filter 'name=nginx'{Colors.ENDC}    - Filter containers")
    print(f"  {Colors.OKCYAN}monitor remote --host <alias>{Colors.ENDC}    - Remote monitoring")
    print(f"  {Colors.OKCYAN}monitor events{Colors.ENDC}                   - Docker events")
    print(f"  {Colors.OKCYAN}monitor --version{Colors.ENDC}                - Show version")

    if not args.no_systemd:
        print(f"\n{Colors.BOLD}Systemd Service:{Colors.ENDC}")
        print(f"  {Colors.OKCYAN}systemctl status monitor{Colors.ENDC}        - Check service status")
        print(f"  {Colors.OKCYAN}journalctl -u monitor -f{Colors.ENDC}        - View service logs")

    print(f"\n{Colors.BOLD}Documentation:{Colors.ENDC}")
    print(f"  {Colors.OKCYAN}https://github.com/Kobeep/Docker_Container_monitor{Colors.ENDC}")
    print()


if __name__ == "__main__":
    try:
        main()
    except KeyboardInterrupt:
        print(f"\n{Colors.WARNING}Installation cancelled by user{Colors.ENDC}")
        sys.exit(1)
    except Exception as e:
        print_error(f"Unexpected error: {e}")
        sys.exit(1)
