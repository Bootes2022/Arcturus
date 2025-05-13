#!/bin/bash
set -e

echo "=== Starting Forwarding Project Deployment ==="

# Repository information
REPO_URL="https://github.com/Bootes2022/Arcturus.git"
DEPLOY_DIR="/opt/forwarding"
FORWARDING_DIR="$DEPLOY_DIR/forwarding"

# Install Go environment
install_go() {
    echo "Installing Go environment..."
    
    # Check if Go is already installed
    if command -v go >/dev/null 2>&1; then
        GO_VERSION=$(go version | awk '{print $3}')
        echo "Go is already installed: $GO_VERSION"
        
        # Verify Go version meets requirements
        if [[ "$GO_VERSION" < "go1.23" ]]; then
            echo "Warning: This project requires Go 1.23 or later. Current version: $GO_VERSION"
        fi
    else
        # Install Go based on operating system
        if [ -f /etc/os-release ]; then
            . /etc/os-release
            OS=$NAME
        else
            OS=$(uname -s)
        fi
        
        echo "Detected OS: $OS"
        
        if [[ "$OS" == *"Ubuntu"* ]] || [[ "$OS" == *"Debian"* ]]; then
            echo "Installing Go on Debian/Ubuntu..."
            sudo apt-get update
            sudo apt-get install -y wget
            wget https://go.dev/dl/go1.23.7.linux-amd64.tar.gz
            sudo rm -rf /usr/local/go
            sudo tar -C /usr/local -xzf go1.23.7.linux-amd64.tar.gz
            echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.profile
            source ~/.profile
            rm go1.23.7.linux-amd64.tar.gz
        elif [[ "$OS" == *"CentOS"* ]] || [[ "$OS" == *"Red Hat"* ]]; then
            echo "Installing Go on CentOS/RHEL..."
            sudo yum install -y wget
            wget https://go.dev/dl/go1.23.7.linux-amd64.tar.gz
            sudo rm -rf /usr/local/go
            sudo tar -C /usr/local -xzf go1.23.7.linux-amd64.tar.gz
            echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.profile
            source ~/.profile
            rm go1.23.7.linux-amd64.tar.gz
        else
            echo "Unsupported OS for automatic Go installation. Please install Go 1.23.7 manually."
            exit 1
        fi
        
        echo "Go installed successfully: $(go version)"
    fi
    
    # Set up GOPATH if not already configured
    if [ -z "$GOPATH" ]; then
        echo 'export GOPATH=$HOME/go' >> ~/.profile
        echo 'export PATH=$PATH:$GOPATH/bin' >> ~/.profile
        source ~/.profile
        echo "GOPATH configured: $GOPATH"
    fi
}

# Deploy Forwarding service
deploy_forwarding() {
    echo "Preparing deployment directory..."
    
    # Create deployment directory
    if [ ! -d "$DEPLOY_DIR" ]; then
        sudo mkdir -p $DEPLOY_DIR
        sudo chown $(whoami) $DEPLOY_DIR
    fi
    
    # Clone or update the repository
    if [ -d "$FORWARDING_DIR" ]; then
        echo "Updating existing repository..."
        cd $FORWARDING_DIR
        git pull
    else
        echo "Cloning repository..."
        git clone $REPO_URL $DEPLOY_DIR
        if [ ! -d "$FORWARDING_DIR" ]; then
            echo "Error: Forwarding directory not found in the repository"
            exit 1
        fi
        cd $FORWARDING_DIR
    fi
    
    echo "Building forwarding service..."
    # Fetch dependencies and build
    go mod tidy
    go build -o forwarding_service .
    
    if [ ! -f "./forwarding_service" ]; then
        echo "Build failed. Executable not found."
        exit 1
    fi
    
    # Create systemd service file
    echo "Setting up systemd service..."
    SERVICE_FILE="/etc/systemd/system/forwarding.service"
    
    sudo tee $SERVICE_FILE > /dev/null << EOF
[Unit]
Description=Forwarding Service
After=network.target

[Service]
Type=simple
User=$(whoami)
WorkingDirectory=$FORWARDING_DIR
ExecStart=$FORWARDING_DIR/forwarding_service
Restart=on-failure
RestartSec=10
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target
EOF
    
    # Enable and start the service
    sudo systemctl daemon-reload
    sudo systemctl enable forwarding.service
    sudo systemctl restart forwarding.service
    
    # Check service status
    echo "Service deployment completed. Current status:"
    sudo systemctl status forwarding.service --no-pager
}

# Main execution
echo "Starting forwarding deployment process..."
install_go
deploy_forwarding
echo "=== Forwarding deployment completed ==="
