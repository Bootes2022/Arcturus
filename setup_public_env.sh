#!/bin/bash
set -e

# Load configuration
CONFIG_FILE="setup.conf"
if [ -f "$CONFIG_FILE" ]; then
    echo "Loading configuration from $CONFIG_FILE"
    source "$CONFIG_FILE"
else
    echo "Error: Configuration file $CONFIG_FILE not found!"
    exit 1
fi

echo "=== Starting Environment Setup ==="

# Install Go environment
install_go() {
    echo "Installing Go environment..."

    if command -v go >/dev/null 2>&1; then
        CURRENT_GO_VERSION_FULL=$(go version)
        CURRENT_GO_VERSION=$(echo "$CURRENT_GO_VERSION_FULL" | awk '{print $3}')
        echo "Go is already installed: $CURRENT_GO_VERSION_FULL"

        # Verify Go version meets requirements (comparing goX.Y with goA.B)
        # Simple string comparison works if format is consistent (goX.Y.Z)
        if [[ "$CURRENT_GO_VERSION" < "$REQUIRED_GO_VERSION" ]]; then
            echo "Warning: This project requires Go $REQUIRED_GO_VERSION or later. Current version: $CURRENT_GO_VERSION"
        fi
    else
        if [ -f /etc/os-release ]; then
            . /etc/os-release
            OS=$NAME
        else
            OS=$(uname -s)
        fi
        echo "Detected OS: $OS"

        GO_TARBALL="go${GO_INSTALL_VERSION}.${GO_ARCH}.tar.gz"
        GO_DOWNLOAD_URL="${GO_DOWNLOAD_BASE_URL}${GO_TARBALL}"

        if [[ "$OS" == *"Ubuntu"* ]] || [[ "$OS" == *"Debian"* ]]; then
            echo "Installing Go on Debian/Ubuntu..."
            sudo apt-get update
            sudo apt-get install -y wget
            wget "$GO_DOWNLOAD_URL"
            sudo rm -rf "$GO_INSTALL_PATH"
            sudo tar -C "$GO_EXTRACT_DIR" -xzf "$GO_TARBALL"
            echo "export PATH=\$PATH:${GO_INSTALL_PATH}/bin" >> ~/.profile
            # source ~/.profile # Avoid sourcing in script, user should do it or re-login
            rm "$GO_TARBALL"
        elif [[ "$OS" == *"CentOS"* ]] || [[ "$OS" == *"Red Hat"* ]]; then
            echo "Installing Go on CentOS/RHEL..."
            sudo yum install -y wget
            wget "$GO_DOWNLOAD_URL"
            sudo rm -rf "$GO_INSTALL_PATH"
            sudo tar -C "$GO_EXTRACT_DIR" -xzf "$GO_TARBALL"
            echo "export PATH=\$PATH:${GO_INSTALL_PATH}/bin" >> ~/.profile
            # source ~/.profile
            rm "$GO_TARBALL"
        else
            echo "Unsupported OS for automatic Go installation. Please install Go $GO_INSTALL_VERSION manually."
            exit 1
        fi
        echo "Go installed successfully. Please run 'source ~/.profile' or re-login to update your PATH."
        echo "Expected version after re-login: $( "$GO_INSTALL_PATH/bin/go" version)"
    fi

    if [ -z "$GOPATH" ]; then
        echo "export GOPATH=${GOPATH_DIR}" >> ~/.profile
        echo "export PATH=\$PATH:\$GOPATH/bin" >> ~/.profile
        # source ~/.profile
        echo "GOPATH configured to ${GOPATH_DIR}. Please run 'source ~/.profile' or re-login."
    fi
}

install_etcd() {
    echo "Installing etcd..."

    if command -v etcd >/dev/null 2>&1; then
        ETCD_CURRENT_VERSION=$(etcd --version | grep "etcd Version" | awk '{print $3}')
        echo "etcd is already installed: $ETCD_CURRENT_VERSION"
        # Optionally, add version check here if needed
        return
    fi

    if [ -f /etc/os-release ]; then
        . /etc/os-release
        OS=$NAME
    else
        OS=$(uname -s)
    fi
    echo "Detected OS: $OS"

    ETCD_TARBALL="etcd-${ETCD_INSTALL_VERSION}-${ETCD_ARCH}.tar.gz"
    ETCD_DOWNLOAD_URL="https://github.com/etcd-io/etcd/releases/download/${ETCD_INSTALL_VERSION}/${ETCD_TARBALL}"

    mkdir -p "$ETCD_DOWNLOAD_TEMP_DIR"

    if [[ "$OS" == *"Ubuntu"* ]] || [[ "$OS" == *"Debian"* ]] || [[ "$OS" == *"CentOS"* ]] || [[ "$OS" == *"Red Hat"* ]]; then
        echo "Installing etcd on Linux..."
        wget -q -O "$ETCD_DOWNLOAD_TEMP_DIR/etcd.tar.gz" "$ETCD_DOWNLOAD_URL"

        sudo mkdir -p "$ETCD_INSTALL_DIR"
        sudo tar -xzf "$ETCD_DOWNLOAD_TEMP_DIR/etcd.tar.gz" -C "$ETCD_DOWNLOAD_TEMP_DIR" --strip-components=1 "etcd-${ETCD_INSTALL_VERSION}-${ETCD_ARCH}/etcd" "etcd-${ETCD_INSTALL_VERSION}-${ETCD_ARCH}/etcdctl" "etcd-${ETCD_INSTALL_VERSION}-${ETCD_ARCH}/etcdutl"
        sudo mv "$ETCD_DOWNLOAD_TEMP_DIR/etcd" "$ETCD_INSTALL_DIR/"
        sudo mv "$ETCD_DOWNLOAD_TEMP_DIR/etcdctl" "$ETCD_INSTALL_DIR/"
        sudo mv "$ETCD_DOWNLOAD_TEMP_DIR/etcdutl" "$ETCD_INSTALL_DIR/"


        echo "Setting executable permissions for etcd binaries..."
        sudo chmod +x "${ETCD_INSTALL_DIR}/etcd"
        sudo chmod +x "${ETCD_INSTALL_DIR}/etcdctl"
        sudo chmod +x "${ETCD_INSTALL_DIR}/etcdutl"

        sudo ln -sf "${ETCD_INSTALL_DIR}/etcd" "${BIN_DIR}/etcd"
        sudo ln -sf "${ETCD_INSTALL_DIR}/etcdctl" "${BIN_DIR}/etcdctl"
        sudo ln -sf "${ETCD_INSTALL_DIR}/etcdutl" "${BIN_DIR}/etcdutl"

        sudo mkdir -p "$ETCD_DATA_DIR"

        echo "Creating etcd systemd service..."
        sudo tee "$ETCD_SERVICE_FILE_PATH" > /dev/null << EOF
[Unit]
Description=etcd distributed key-value store
Documentation=https://github.com/etcd-io/etcd
After=network.target

[Service]
Type=notify
ExecStart=${BIN_DIR}/etcd \\
  --data-dir=${ETCD_DATA_DIR} \\
  --name=${ETCD_NODE_NAME} \\
  --initial-advertise-peer-urls=${ETCD_INITIAL_ADVERTISE_PEER_URLS} \\
  --listen-peer-urls=${ETCD_LISTEN_PEER_URLS} \\
  --advertise-client-urls=${ETCD_ADVERTISE_CLIENT_URLS} \\
  --listen-client-urls=${ETCD_LISTEN_CLIENT_URLS} \\
  --initial-cluster=${ETCD_INITIAL_CLUSTER}
Restart=on-failure
RestartSec=10
LimitNOFILE=65536

[Install]
WantedBy=multi-user.target
EOF
        sudo systemctl daemon-reload
        sudo systemctl enable etcd.service
        sudo systemctl start etcd.service
        echo "Checking etcd service status..."
        sudo systemctl status etcd.service --no-pager
        echo "etcd installation completed"
    else
        echo "Unsupported OS for automatic etcd installation. Please install etcd manually."
        exit 1
    fi
    rm -rf "$ETCD_DOWNLOAD_TEMP_DIR"
}

echo "Starting environment setup..."
install_go
install_etcd
echo "=== Environment setup completed ==="
echo "IMPORTANT: You might need to run 'source ~/.profile' or re-login for PATH changes to take effect."
