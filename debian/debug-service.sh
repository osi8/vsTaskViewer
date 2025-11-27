#!/bin/bash
# Helper script to debug vsTaskViewer service issues
# Run this script to get detailed error information
#
# SECURITY NOTE: This script requires root privileges and may expose sensitive
# information. Use only for debugging in trusted environments.
# Consider removing or restricting access in production deployments.

set -e

# Check if running as root
if [ "$EUID" -ne 0 ]; then
    echo "ERROR: This script must be run as root"
    exit 1
fi

echo "=== vsTaskViewer Service Debug Helper ==="
echo ""

# Check if service is installed
if [ ! -f /etc/systemd/system/vsTaskViewer.service ]; then
    echo "ERROR: Service file not found at /etc/systemd/system/vsTaskViewer.service"
    exit 1
fi

# Check if binary exists
if [ ! -f /usr/bin/vsTaskViewer ]; then
    echo "ERROR: Binary not found at /usr/bin/vsTaskViewer"
    exit 1
fi

# Check binary permissions
echo "=== Binary Check ==="
ls -la /usr/bin/vsTaskViewer
echo ""

# Check config file
echo "=== Config File Check ==="
if [ -f /etc/vsTaskViewer/vsTaskViewer.toml ]; then
    echo "Config file exists: /etc/vsTaskViewer/vsTaskViewer.toml"
    ls -la /etc/vsTaskViewer/vsTaskViewer.toml
else
    echo "WARNING: Config file not found at /etc/vsTaskViewer/vsTaskViewer.toml"
fi
echo ""

# Check HTML directory
echo "=== HTML Directory Check ==="
if [ -d /etc/vsTaskViewer/html ]; then
    echo "HTML directory exists: /etc/vsTaskViewer/html"
    ls -la /etc/vsTaskViewer/html/
else
    echo "WARNING: HTML directory not found at /etc/vsTaskViewer/html"
fi
echo ""

# Check task directory
echo "=== Task Directory Check ==="
if [ -d /var/vsTaskViewer ]; then
    echo "Task directory exists: /var/vsTaskViewer"
    ls -lad /var/vsTaskViewer
    echo "Permissions: $(stat -c '%a' /var/vsTaskViewer)"
    echo "Owner: $(stat -c '%U:%G' /var/vsTaskViewer)"
else
    echo "WARNING: Task directory not found at /var/vsTaskViewer"
fi
echo ""

# Check www-data user
echo "=== User Check ==="
if id -u www-data >/dev/null 2>&1; then
    echo "www-data user exists:"
    id www-data
else
    echo "ERROR: www-data user does not exist!"
fi
echo ""

# Check service status
echo "=== Service Status ==="
systemctl status vsTaskViewer.service --no-pager -l || true
echo ""

# Show recent logs
echo "=== Recent Journal Logs (last 50 lines) ==="
journalctl -u vsTaskViewer.service -n 50 --no-pager || true
echo ""

# Try to run binary directly (as root, simulating service)
echo "=== Testing Binary Directly (as root) ==="
echo "This will show any immediate errors..."
echo ""
if [ -f /etc/vsTaskViewer/vsTaskViewer.toml ]; then
    timeout 5 /usr/bin/vsTaskViewer \
        -c /etc/vsTaskViewer/vsTaskViewer.toml \
        -t /etc/vsTaskViewer/html \
        -d /var/vsTaskViewer \
        -u www-data 2>&1 || true
else
    echo "Skipping direct test - config file not found"
fi
echo ""

echo "=== Debug Complete ==="
echo ""
echo "To view live logs, run:"
echo "  sudo journalctl -u vsTaskViewer.service -f"
echo ""
echo "To view all logs with timestamps:"
echo "  sudo journalctl -u vsTaskViewer.service --no-pager"

