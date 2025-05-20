#!/bin/bash
set -e

echo "=== Starting Environment Setup ==="

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
        
        echo "Setting executable permissions for etcd binaries..."
        sudo chmod +x $ETCD_DIR/etcd
        sudo chmod +x $ETCD_DIR/etcdctl
        sudo chmod +x $ETCD_DIR/etcdutl
        
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

# Main execution flow
echo "Starting environment setup..."
install_go
install_etcd
echo "=== Environment setup completed ==="
