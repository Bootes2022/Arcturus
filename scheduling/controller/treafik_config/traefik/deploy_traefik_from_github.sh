#!/bin/bash

# --- Configuration Variables ---
TRAEFIK_VERSION="v3.4.0" # Specify the Traefik version to download, or set to "latest"
TRAEFIK_INSTALL_DIR="/opt/traefik"
CONFIG_DIR="/etc/traefik"
PLUGIN_DIR_NAME="weightedredirector"
SERVICE_USER="traefikuser" # Optional

# GitHub repository information (for downloading config files and plugins)
# Assume the script and config files are in the same repository's relative path
# If your script is downloaded from elsewhere, adjust these paths or let users pass them
SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )" # Get the script's directory
CONFIG_SOURCE_DIR="${SCRIPT_DIR}/config"
PLUGINS_SOURCE_DIR="${SCRIPT_DIR}/plugins"


# --- Functions ---
log_info() {
    echo "[INFO] $1"
}

log_error() {
    echo "[ERROR] $1" >&2
    exit 1
}

check_root() {
    if [ "$(id -u)" -ne 0 ]; then
        log_error "This script requires root privileges. Please use sudo."
    fi
}

# (Optional) Create user function (same as previous script)
create_user_if_not_exists() {
    if ! id "$1" &>/dev/null; then
        log_info "Creating user '$1'..."
        sudo useradd -r -s /bin/false "$1" || log_error "Failed to create user '$1'."
    else
        log_info "User '$1' already exists."
    fi
}

download_traefik() {
    local version="$1"
    local arch
    arch=$(uname -m)
    local os
    os=$(uname -s | tr '[:upper:]' '[:lower:]') # linux, darwin

    local traefik_binary_name="traefik"
    local download_url

    if [ "$arch" == "x86_64" ]; then
        arch="amd64"
    elif [ "$arch" == "aarch64" ]; then
        arch="arm64"
    elif [ "$arch" == "armv7l" ]; then
        arch="armv7"
    else
        log_error "Unsupported architecture: $arch"
    fi

    if [ "$version" == "latest" ]; then
        # Get the latest stable version tag
        # Note: This relies on GitHub API and jq. If 'jq' is not installed, prompt to install or use specific version.
        if ! command -v jq &> /dev/null; then
            log_error "Requires 'jq' to fetch the latest version. Please install jq (e.g., sudo apt install jq) or specify a specific TRAEFIK_VERSION."
        fi
        version_tag=$(curl -s https://api.github.com/repos/traefik/traefik/releases/latest | jq -r .tag_name)
        if [ -z "$version_tag" ] || [ "$version_tag" == "null" ]; then
            log_error "Failed to fetch the latest Traefik version tag from GitHub API."
        fi
        log_info "Fetched latest Traefik version: $version_tag"
    else
        version_tag="$version"
    fi

    download_url="https://github.com/traefik/traefik/releases/download/${version_tag}/traefik_${version_tag}_${os}_${arch}.tar.gz"

    log_info "Downloading Traefik ${version_tag} from $download_url..."
    TEMP_DOWNLOAD_DIR=$(mktemp -d)
    if curl -L "$download_url" -o "$TEMP_DOWNLOAD_DIR/traefik.tar.gz"; then
        log_info "Download completed. Extracting..."
        if tar -xzf "$TEMP_DOWNLOAD_DIR/traefik.tar.gz" -C "$TEMP_DOWNLOAD_DIR"; then
            if [ -f "$TEMP_DOWNLOAD_DIR/$traefik_binary_name" ]; then
                sudo mv "$TEMP_DOWNLOAD_DIR/$traefik_binary_name" "$TRAEFIK_INSTALL_DIR/traefik"
                sudo chmod +x "$TRAEFIK_INSTALL_DIR/traefik"
                log_info "Traefik binary installed to $TRAEFIK_INSTALL_DIR/traefik"
            else
                log_error "Failed to find '$traefik_binary_name' in the extracted package."
            fi
        else
            log_error "Failed to extract Traefik package."
        fi
    else
        log_error "Failed to download Traefik. Please check the version number and network connection."
    fi
    rm -rf "$TEMP_DOWNLOAD_DIR"
}


# --- Main Logic ---
check_root

# 0. (Optional) Create user
# create_user_if_not_exists "$SERVICE_USER"

log_info "Starting Traefik deployment (from GitHub source)..."

# 1. Create installation and config directories
log_info "Creating directories..."
sudo mkdir -p "$TRAEFIK_INSTALL_DIR" || log_error "Failed to create directory '$TRAEFIK_INSTALL_DIR'."
sudo mkdir -p "$CONFIG_DIR/conf.d" || log_error "Failed to create directory '$CONFIG_DIR/conf.d'."
sudo mkdir -p "$CONFIG_DIR/plugins-local/src/$PLUGIN_DIR_NAME" || log_error "Failed to create plugin directory."

# 2. Download and install Traefik binary
download_traefik "$TRAEFIK_VERSION"

# 3. Copy config files and plugins (from script's or specified source)
if [ ! -d "$CONFIG_SOURCE_DIR" ] || [ ! -d "$PLUGINS_SOURCE_DIR" ]; then
    log_error "Config source directory ($CONFIG_SOURCE_DIR) or plugin source directory ($PLUGINS_SOURCE_DIR) not found."
fi

log_info "Copying static config file from $CONFIG_SOURCE_DIR/traefik.yml..."
sudo cp "$CONFIG_SOURCE_DIR/traefik.yml" "$CONFIG_DIR/traefik.yml" || log_error "Failed to copy traefik.yml."

log_info "Copying dynamic config files from $CONFIG_SOURCE_DIR/conf.d/ ..."
sudo cp "$CONFIG_SOURCE_DIR/conf.d/"* "$CONFIG_DIR/conf.d/" || log_error "Failed to copy dynamic config files."

log_info "Copying plugin files from $PLUGINS_SOURCE_DIR/src/$PLUGIN_DIR_NAME/ ..."
sudo cp -r "$PLUGINS_SOURCE_DIR/src/$PLUGIN_DIR_NAME/"* "$CONFIG_DIR/plugins-local/src/$PLUGIN_DIR_NAME/" || log_error "Failed to copy plugin files."


# 4. (Optional) Set file permissions (same as previous)

# 5. Create systemd service file (same as previous script)
SYSTEMD_SERVICE_FILE="/etc/systemd/system/traefik.service"
log_info "Creating systemd service file '$SYSTEMD_SERVICE_FILE'..."
# ... (systemd file content same as previous script) ...
sudo bash -c "cat > $SYSTEMD_SERVICE_FILE" <<EOF
[Unit]
Description=Traefik Ingress Controller
After=network.target

[Service]
ExecStart=$TRAEFIK_INSTALL_DIR/traefik --configFile=$CONFIG_DIR/traefik.yml
WorkingDirectory=$TRAEFIK_INSTALL_DIR
Restart=always
# User=$SERVICE_USER
# Group=$SERVICE_USER
StandardOutput=journal
StandardError=journal
SyslogIdentifier=traefik

[Install]
WantedBy=multi-user.target
EOF

log_info "Reloading systemd and enabling/starting Traefik service..."
sudo systemctl daemon-reload
sudo systemctl enable traefik.service
sudo systemctl restart traefik.service

log_info "Checking Traefik service status:"
sudo systemctl status traefik.service --no-pager -l

log_info "Deployment completed!"
echo "Please check Traefik logs (journalctl -u traefik -f) and Dashboard (if configured)."