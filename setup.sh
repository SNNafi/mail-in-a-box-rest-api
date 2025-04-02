#!/bin/bash
set -e  # Exit on error

# Configuration variables
SERVICE_NAME="mail-api"
GO_BINARY_PATH="/usr/local/bin/mail-api"
SERVICE_PORT=1111
WORKING_DIR="/home"
DOMAIN_NAME=domain.com
GO_API_PORT=1112

echo "=== Starting Mail API setup ==="

# Allow port 1111 through UFW
echo "Opening port $SERVICE_PORT in firewall..."
sudo ufw allow $SERVICE_PORT/tcp

# Create systemd service
echo "Creating systemd service..."
cat <<EOF | sudo tee /etc/systemd/system/$SERVICE_NAME.service
[Unit]
Description=Mail API Service
After=network.target

[Service]
ExecStart=$GO_BINARY_PATH
Restart=always
WorkingDirectory=$WORKING_DIR

[Install]
WantedBy=multi-user.target
EOF

# Copy binary to correct location (assuming it's in the current directory)
echo "Installing binary to $GO_BINARY_PATH..."
sudo cp ./mail-api $GO_BINARY_PATH
sudo chmod +x $GO_BINARY_PATH

# Enable and start the service
echo "Enabling and starting service..."
sudo systemctl daemon-reload
sudo systemctl enable --now $SERVICE_NAME

# Configure Nginx to proxy to our service
echo "Configuring Nginx..."
cat <<EOF | sudo tee /etc/nginx/sites-available/mail-api
server {
    listen $SERVICE_PORT ssl;
    server_name $DOMAIN_NAME;

    ssl_certificate /etc/letsencrypt/live/$DOMAIN_NAME/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/$DOMAIN_NAME/privkey.pem;

    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_prefer_server_ciphers on;
    ssl_ciphers ECDHE-ECDSA-AES128-GCM-SHA256:ECDHE-RSA-AES128-GCM-SHA256:ECDHE-ECDSA-AES256-GCM-SHA384:ECDHE-RSA-AES256-GCM-SHA384:ECDHE-ECDSA-CHACHA20-POLY1305:ECDHE-RSA-CHACHA20-POLY1305:DHE-RSA-AES128-GCM-SHA256:DHE-RSA-AES256-GCM-SHA384;

    location / {
        proxy_pass http://127.0.0.1:$GO_API_PORT;
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$remote_addr;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
    }
}
EOF

# Enable the Nginx site
sudo ln -sf /etc/nginx/sites-available/mail-api /etc/nginx/sites-enabled/mail-api

# Test Nginx configuration
echo "Testing Nginx configuration..."
sudo nginx -t

# Restart Nginx to apply changes
echo "Restarting Nginx..."
sudo systemctl restart nginx

echo "Setup complete. Mail API is now available at $DOMAIN_NAME:$SERVICE_PORT"
echo "You can test it with: curl -X POST -H 'Authorization: Basic <base64-encoded-credentials>' -H 'Content-Type: application/json' -d '{\"to\":[\"recipient@example.com\"],\"subject\":\"Test\",\"content\":\"Test message\",\"title\":\"Title\"}' https://$DOMAIN_NAME:$SERVICE_PORT/mail/send"