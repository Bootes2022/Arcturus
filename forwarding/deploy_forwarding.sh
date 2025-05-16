#!/bin/bash
set -e

echo "=== Starting Forwarding Project Deployment ==="

# Repository information
REPO_OWNER="Bootes2022"
REPO_NAME="Arcturus"
BRANCH="main"  # Default branch, can be changed as needed
ARCHIVE_FORMAT="tar.gz"  # Options: "zip" or "tar.gz"
ARCHIVE_URL="https://github.com/${REPO_OWNER}/${REPO_NAME}/archive/refs/heads/${BRANCH}.${ARCHIVE_FORMAT}"

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

# Install etcd
install_etcd() {
    echo "Installing etcd..."
    
    # Check if etcd is already installed
    if command -v etcd >/dev/null 2>&1; then
        ETCD_VERSION=$(etcd --version | grep "etcd Version" | awk '{print $3}')
        echo "etcd is already installed: $ETCD_VERSION"
        return
    fi
    
    # Determine OS type
    if [ -f /etc/os-release ]; then
        . /etc/os-release
        OS=$NAME
    else
        OS=$(uname -s)
    fi
    
    echo "Detected OS: $OS"
    
    # Set etcd version and installation directories
    ETCD_VER="v3.5.9"
    ETCD_DIR="/usr/local/etcd"
    DOWNLOAD_DIR="/tmp/etcd-download"
    
    # Create download directory
    mkdir -p $DOWNLOAD_DIR
    
    if [[ "$OS" == *"Ubuntu"* ]] || [[ "$OS" == *"Debian"* ]] || [[ "$OS" == *"CentOS"* ]] || [[ "$OS" == *"Red Hat"* ]]; then
        echo "Installing etcd on Linux..."
        
        # Download etcd
        DOWNLOAD_URL="https://github.com/etcd-io/etcd/releases/download/${ETCD_VER}/etcd-${ETCD_VER}-linux-amd64.tar.gz"
        wget -q -O $DOWNLOAD_DIR/etcd.tar.gz $DOWNLOAD_URL
        
        # Extract etcd
        sudo mkdir -p $ETCD_DIR
        sudo tar -xzf $DOWNLOAD_DIR/etcd.tar.gz -C $DOWNLOAD_DIR
        sudo mv $DOWNLOAD_DIR/etcd-${ETCD_VER}-linux-amd64/etcd* $ETCD_DIR/
        
        # Create symbolic links
        sudo ln -sf $ETCD_DIR/etcd /usr/local/bin/etcd
        sudo ln -sf $ETCD_DIR/etcdctl /usr/local/bin/etcdctl
        sudo ln -sf $ETCD_DIR/etcdutl /usr/local/bin/etcdutl
        
        # Create etcd data directory
        sudo mkdir -p /var/lib/etcd
        
        # Create systemd service file
        echo "Creating etcd systemd service..."
        ETCD_SERVICE_FILE="/etc/systemd/system/etcd.service"
        
        sudo tee $ETCD_SERVICE_FILE > /dev/null << EOF
[Unit]
Description=etcd distributed key-value store
Documentation=https://github.com/etcd-io/etcd
After=network.target

[Service]
Type=notify
ExecStart=/usr/local/bin/etcd \\
  --data-dir=/var/lib/etcd \\
  --name=default \\
  --initial-advertise-peer-urls=http://localhost:2380 \\
  --listen-peer-urls=http://localhost:2380 \\
  --advertise-client-urls=http://localhost:2379 \\
  --listen-client-urls=http://localhost:2379 \\
  --initial-cluster=default=http://localhost:2380
Restart=on-failure
RestartSec=10
LimitNOFILE=65536

[Install]
WantedBy=multi-user.target
EOF
        
        # Enable and start etcd service
        sudo systemctl daemon-reload
        sudo systemctl enable etcd.service
        sudo systemctl start etcd.service
        
        # Verify etcd is running
        echo "Checking etcd service status..."
        sudo systemctl status etcd.service --no-pager
        
        echo "etcd installation completed"
    else
        echo "Unsupported OS for automatic etcd installation. Please install etcd manually."
        exit 1
    fi
    
    # Clean up
    rm -rf $DOWNLOAD_DIR
}

# Deploy Forwarding service
deploy_forwarding() {
    echo "Preparing deployment directory..."
    
    # Create deployment directory
    if [ ! -d "$DEPLOY_DIR" ]; then
        sudo mkdir -p $DEPLOY_DIR
        sudo chown $(whoami) $DEPLOY_DIR
    fi
    
    # Check if old version exists and remove it
    if [ -d "$FORWARDING_DIR" ]; then
        echo "Deployment directory already exists, removing old version..."
        sudo rm -rf $FORWARDING_DIR
    fi
    
    echo "Downloading project archive from ${ARCHIVE_URL}..."
    ARCHIVE_FILE="/tmp/${REPO_NAME}-${BRANCH}.${ARCHIVE_FORMAT}"
    
    # Install wget if not already installed
    if ! command -v wget >/dev/null 2>&1; then
        if [ -f /etc/os-release ]; then
            . /etc/os-release
            OS=$NAME
        else
            OS=$(uname -s)
        fi
        
        if [[ "$OS" == *"Ubuntu"* ]] || [[ "$OS" == *"Debian"* ]]; then
            sudo apt-get update
            sudo apt-get install -y wget
        elif [[ "$OS" == *"CentOS"* ]] || [[ "$OS" == *"Red Hat"* ]]; then
            sudo yum install -y wget
        else
            echo "Please install wget manually"
            exit 1
        fi
    fi
    
    # Download the archive
    wget -O $ARCHIVE_FILE $ARCHIVE_URL
    
    if [ ! -f "$ARCHIVE_FILE" ]; then
        echo "Failed to download archive. Please check the URL and try again."
        exit 1
    fi
    
    echo "Extracting archive..."
    # Ensure deployment directory exists
    mkdir -p $DEPLOY_DIR
    
    # Extract based on archive format
    if [[ "$ARCHIVE_FORMAT" == "zip" ]]; then
        if ! command -v unzip >/dev/null 2>&1; then
            if [ -f /etc/os-release ]; then
                . /etc/os-release
                OS=$NAME
            else
                OS=$(uname -s)
            fi
            
            if [[ "$OS" == *"Ubuntu"* ]] || [[ "$OS" == *"Debian"* ]]; then
                sudo apt-get update
                sudo apt-get install -y unzip
            elif [[ "$OS" == *"CentOS"* ]] || [[ "$OS" == *"Red Hat"* ]]; then
                sudo yum install -y unzip
            else
                echo "Please install unzip manually"
                exit 1
            fi
        fi
        unzip -q $ARCHIVE_FILE -d $DEPLOY_DIR
    elif [[ "$ARCHIVE_FORMAT" == "tar.gz" ]]; then
        tar -xzf $ARCHIVE_FILE -C $DEPLOY_DIR
    else
        echo "Unsupported archive format: $ARCHIVE_FORMAT"
        exit 1
    fi
    
    # Extracted folder name is typically "repo-branch"
    EXTRACTED_DIR="$DEPLOY_DIR/${REPO_NAME}-${BRANCH}"
    
    if [ ! -d "$EXTRACTED_DIR" ]; then
        echo "Failed to extract the archive. The expected directory does not exist."
        exit 1
    fi
    
    # Check the extracted directory structure to find the forwarding directory
    if [ -d "$EXTRACTED_DIR/forwarding" ]; then
        echo "Found forwarding directory in extracted archive"
        sudo mv "$EXTRACTED_DIR/forwarding" "$FORWARDING_DIR"
    else
        echo "Forwarding directory not found in expected location. "
        echo "The extracted repository contains the following structure:"
        ls -la "$EXTRACTED_DIR"
        
        # Assume the extracted directory is the project root
        sudo mv "$EXTRACTED_DIR" "$FORWARDING_DIR"
    fi
    
    # Clean up the downloaded archive file
    rm $ARCHIVE_FILE
    
    # Exit with error if the forwarding directory doesn't exist or is empty
    if [ ! -d "$FORWARDING_DIR" ] || [ -z "$(ls -A $FORWARDING_DIR)" ]; then
        echo "Error: Forwarding directory not found or empty after extraction"
        exit 1
    fi
    
    cd $FORWARDING_DIR
    
    echo "Building forwarding service..."
    # Fetch dependencies and build
    go mod tidy
    go build -o forwarding_service .
    
    if [ ! -f "./forwarding_service" ]; then
        echo "Build failed. Executable not found."
        exit 1
    fi
    
    # Create config file to connect to etcd if it doesn't exist
    if [ ! -f "./config.toml" ]; then
        echo "Creating default configuration file with etcd settings..."
        cat > ./config.toml << EOF
# Server Configuration
[server]
port = 8081

# etcd Configuration
[etcd]
endpoints = ["localhost:2379"]
dial_timeout = 5
request_timeout = 5

# Add other configuration parameters as needed
EOF
    else
        echo "Configuration file already exists. Using existing configuration."
    fi
    
    # Create systemd service file
    echo "Setting up systemd service..."
    SERVICE_FILE="/etc/systemd/system/forwarding.service"
    
    sudo tee $SERVICE_FILE > /dev/null << EOF
[Unit]
Description=Forwarding Service
After=network.target etcd.service
Requires=etcd.service

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

# Main execution flow
echo "Starting forwarding deployment process..."
install_go
install_etcd
deploy_forwarding
echo "=== Forwarding deployment completed ==="
