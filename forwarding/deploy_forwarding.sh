#!/bin/bash
set -e

# Configuration parameters (modify as needed)
DOMAIN="yourdomain.com"                  # Your domain name
EMAIL="admin@${DOMAIN}"                  # Admin email (for Let's Encrypt)
TRAEFIK_USER="admin"                     # Dashboard username
TRAEFIK_PASS="StrongPassword123!"        # Dashboard password
ACME_STORAGE="/etc/traefik/acme.json"    # SSL certificate storage location
DASHBOARD_ENABLE=true                    # Enable Dashboard interface

# Detect OS and package manager
if [ -f /etc/os-release ]; then
    . /etc/os-release
    OS=$ID
    OS_VERSION=$VERSION_ID
elif type lsb_release >/dev/null 2>&1; then
    OS=$(lsb_release -si | tr '[:upper:]' '[:lower:]')
    OS_VERSION=$(lsb_release -sr)
elif [ -f /etc/redhat-release ]; then
    OS="rhel"
    OS_VERSION=$(grep -oE '[0-9]+\.[0-9]+' /etc/redhat-release)
else
    OS=$(uname -s)
    OS_VERSION=$(uname -r)
fi

echo "Detected OS: $OS $OS_VERSION"

# Install dependencies
echo "Installing required dependencies..."
case "$OS" in
    debian|ubuntu)
        sudo apt-get update > /dev/null
        sudo apt-get install -y curl wget apt-transport-https apache2-utils
        ;;
    centos|rhel|fedora|amzn)
        sudo yum install -y curl wget which httpd-tools
        ;;
    *)
        echo "Unsupported OS: $OS"
        exit 1
        ;;
esac

# Install Traefik
echo "Installing Traefik..."
case "$OS" in
    debian|ubuntu)
        curl -fsSL https://pkgs.traefik.io/debian/gpg | sudo gpg --dearmor -o /usr/share/keyrings/traefik-archive-keyring.gpg
        echo "deb [signed-by=/usr/share/keyrings/traefik-archive-keyring.gpg] https://pkgs.traefik.io/debian/stable/ $(lsb_release -cs) main" | sudo tee /etc/apt/sources.list.d/traefik.list
        sudo apt-get update > /dev/null
        sudo apt-get install -y traefik
        ;;
    centos|rhel|fedora|amzn)
        sudo tee /etc/yum.repos.d/traefik.repo <<EOF
[traefik]
name=Traefik Repository
baseurl=https://pkgs.traefik.io/rpm/stable/\$basearch/
enabled=1
gpgcheck=1
gpgkey=https://pkgs.traefik.io/rpm/stable/gpg
EOF
        sudo yum install -y traefik
        ;;
esac

# Create configuration directories
echo "Creating configuration directories..."
sudo mkdir -p /etc/traefik/{config,dynamic}
sudo chown -R root:root /etc/traefik
sudo chmod -R 600 /etc/traefik

# Generate password hash (for Basic Auth)
if [ "$DASHBOARD_ENABLE" = true ]; then
    echo "Generating Dashboard credentials..."
    AUTH_CREDS=$(htpasswd -nbB "$TRAEFIK_USER" "$TRAEFIK_PASS" | sed -e 's/\$/\$\$/g')
fi

# Main configuration file
echo "Generating main configuration..."
sudo tee /etc/traefik/traefik.yml > /dev/null <<EOF
global:
  checkNewVersion: true
  sendAnonymousUsage: false

entryPoints:
  http:
    address: ":80"
    http:
      redirections:
        entryPoint:
          to: https
          scheme: https
          permanent: true
  https:
    address: ":443"

providers:
  file:
    directory: /etc/traefik/config
    watch: true
  docker:
    exposedByDefault: false

api:
  dashboard: $DASHBOARD_ENABLE
  insecure: false

certificatesResolvers:
  letsencrypt:
    acme:
      email: $EMAIL
      storage: $ACME_STORAGE
      httpChallenge:
        entryPoint: http
EOF

# Dynamic routing configuration
echo "Generating dynamic routing configuration..."
sudo tee /etc/traefik/config/dynamic.yml > /dev/null <<EOF
http:
  routers:
    # Default HTTPS redirect
    redirect-to-https:
      rule: "HostRegexp(\`{any:.+}\`)"
      entryPoints:
        - "http"
      middlewares:
        - "redirect-to-https"
      service: "noop@internal"

  middlewares:
    redirect-to-https:
      redirectScheme:
        scheme: https
        permanent: true

    # Security headers
    security-headers:
      headers:
        browserXssFilter: true
        contentTypeNosniff: true
        frameDeny: true
        sslRedirect: true
        stsIncludeSubdomains: true
        stsPreload: true
        stsSeconds: 31536000

    # Compression
    compress:
      compress: true
EOF

# Add Dashboard configuration if enabled
if [ "$DASHBOARD_ENABLE" = true ]; then
    sudo tee -a /etc/traefik/config/dynamic.yml > /dev/null <<EOF

    # Dashboard routing
    dashboard:
      rule: "Host(\`traefik.$DOMAIN\`)"
      entryPoints:
        - "https"
      middlewares:
        - "security-headers"
        - "auth"
      service: "api@internal"
      tls:
        certResolver: "letsencrypt"

  middlewares:
    auth:
      basicAuth:
        users:
          - "$AUTH_CREDS"
EOF
fi

# Set permissions for ACME storage
echo "Setting certificate storage permissions..."
sudo touch $ACME_STORAGE
sudo chmod 600 $ACME_STORAGE

# Create systemd service
echo "Creating systemd service..."
sudo tee /etc/systemd/system/traefik.service > /dev/null <<EOF
[Unit]
Description=Traefik Reverse Proxy
After=network.target

[Service]
Type=simple
ExecStart=/usr/bin/traefik --configfile=/etc/traefik/traefik.yml
Restart=on-failure
User=root
Group=root
LimitNOFILE=65536

[Install]
WantedBy=multi-user.target
EOF

# Start the service
echo "Starting Traefik service..."
sudo systemctl daemon-reload
sudo systemctl enable traefik
sudo systemctl restart traefik

# Output status information
echo -e "\nInstallation complete! Status information:"
echo "----------------------------------------"
sudo systemctl status traefik --no-pager
echo "----------------------------------------"

if [ "$DASHBOARD_ENABLE" = true ]; then
    echo -e "\nDashboard access information:"
    echo "URL: https://traefik.$DOMAIN"
    echo "Username: $TRAEFIK_USER"
    echo "Password: $TRAEFIK_PASS"
fi

echo -e "\nPost-installation recommendations:"
echo "1. Point DNS record traefik.$DOMAIN to this server's IP"
echo "2. Add your service routes in /etc/traefik/config/dynamic.yml"
echo "3. View logs with: sudo journalctl -u traefik -f"
