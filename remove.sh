#!/bin/bash
set -e  # Exit on error

# Configuration variables (must match those used in the setup)
SERVICE_NAME="mail-api"
GO_BINARY_PATH="/usr/local/bin/mail-api"
SERVICE_PORT=1111

echo "=== Starting Mail API cleanup ==="

# Stop and disable the service
echo "Stopping and disabling service..."
sudo systemctl stop $SERVICE_NAME || echo "Service was not running"
sudo systemctl disable $SERVICE_NAME || echo "Service was not enabled"
sudo rm -f /etc/systemd/system/$SERVICE_NAME.service
sudo systemctl daemon-reload

# Remove the binary
echo "Removing binary..."
sudo rm -f $GO_BINARY_PATH

# Remove Nginx configuration
echo "Removing Nginx configuration..."
sudo rm -f /etc/nginx/sites-enabled/mail-api
sudo rm -f /etc/nginx/sites-available/mail-api
sudo systemctl restart nginx

# Close firewall port
echo "Closing firewall port..."
sudo ufw delete allow $SERVICE_PORT/tcp || echo "Firewall rule not found"

echo "Cleanup complete. All Mail API components have been removed."