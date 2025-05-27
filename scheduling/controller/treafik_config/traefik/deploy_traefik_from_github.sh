#!/bin/bash

# --- Configuration Variables ---
TRAEFIK_VERSION="v3.0.0" # 或者你之前使用的 v3.4.0，请确保这是你想要的版本
TRAEFIK_INSTALL_DIR="/opt/traefik" # Traefik 二进制安装目录，也将是工作目录
CONFIG_DIR="/etc/traefik"          # 存放 traefik.yml 和 conf.d/ 的目录
PLUGIN_DIR_NAME="weightedredirector" # 你的插件目录名 (也是 moduleName)
SERVICE_USER="traefikuser" # Optional

# --- Source Paths based on your repository structure ---
SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )" # Get the script's directory

STATIC_CONFIG_FILE_SOURCE="${SCRIPT_DIR}/traefik.yml"
DYNAMIC_CONFIG_SOURCE_DIR="${SCRIPT_DIR}/conf.d"
# 指向你仓库中 plugins-local 目录，实际插件代码在 plugins-local/src/plugin_name 下
PLUGINS_REPO_ROOT_SOURCE_DIR="${SCRIPT_DIR}/plugins-local"

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
    local version_tag

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

# MODIFIED: Create plugin directory under TRAEFIK_INSTALL_DIR (Traefik's working directory)
# Traefik looks for plugins-local/MODULE_NAME/ relative to its working directory
sudo mkdir -p "$TRAEFIK_INSTALL_DIR/plugins-local/$PLUGIN_DIR_NAME" || log_error "Failed to create destination plugin directory in Traefik working directory."

# 2. Download and install Traefik binary
download_traefik "$TRAEFIK_VERSION"

# 3. Copy config files and plugins
if [ ! -f "$STATIC_CONFIG_FILE_SOURCE" ]; then
    log_error "Static config file ($STATIC_CONFIG_FILE_SOURCE) not found."
fi
if [ ! -d "$DYNAMIC_CONFIG_SOURCE_DIR" ]; then
    log_error "Dynamic config source directory ($DYNAMIC_CONFIG_SOURCE_DIR) not found."
fi
# Source for plugin files is REPO_ROOT/plugins-local/src/PLUGIN_NAME/
PLUGIN_CODE_SOURCE_DIR="$PLUGINS_REPO_ROOT_SOURCE_DIR/src/$PLUGIN_DIR_NAME"
if [ ! -d "$PLUGIN_CODE_SOURCE_DIR" ]; then
    log_error "Plugin source code directory ($PLUGIN_CODE_SOURCE_DIR) not found."
fi

log_info "Copying static config file from $STATIC_CONFIG_FILE_SOURCE to $CONFIG_DIR/traefik.yml..."
sudo cp "$STATIC_CONFIG_FILE_SOURCE" "$CONFIG_DIR/traefik.yml" || log_error "Failed to copy traefik.yml."

log_info "Copying dynamic config files from $DYNAMIC_CONFIG_SOURCE_DIR/ to $CONFIG_DIR/conf.d/ ..."
sudo cp "$DYNAMIC_CONFIG_SOURCE_DIR/"* "$CONFIG_DIR/conf.d/" || log_error "Failed to copy dynamic config files."

# MODIFIED: Copy plugin files to TRAEFIK_INSTALL_DIR/plugins-local/PLUGIN_DIR_NAME/
PLUGIN_DESTINATION_DIR="$TRAEFIK_INSTALL_DIR/plugins-local/$PLUGIN_DIR_NAME"
log_info "Copying plugin files from $PLUGIN_CODE_SOURCE_DIR/ to $PLUGIN_DESTINATION_DIR/"
sudo cp -r "$PLUGIN_CODE_SOURCE_DIR/"* "$PLUGIN_DESTINATION_DIR/" || log_error "Failed to copy plugin files to Traefik working directory."


# 4. (Optional) Set file permissions
# If you created a SERVICE_USER, you might want to set ownership for $TRAEFIK_INSTALL_DIR and $CONFIG_DIR
# sudo chown -R "$SERVICE_USER":"$SERVICE_USER" "$TRAEFIK_INSTALL_DIR"
# sudo chown -R "$SERVICE_USER":"$SERVICE_USER" "$CONFIG_DIR"

# 5. Create systemd service file
SYSTEMD_SERVICE_FILE="/etc/systemd/system/traefik.service"
log_info "Creating systemd service file '$SYSTEMD_SERVICE_FILE'..."
sudo bash -c "cat > $SYSTEMD_SERVICE_FILE" <<EOF
[Unit]
Description=Traefik Ingress Controller
After=network.target

[Service]
ExecStart=$TRAEFIK_INSTALL_DIR/traefik --configFile=$CONFIG_DIR/traefik.yml
WorkingDirectory=$TRAEFIK_INSTALL_DIR # Traefik's working directory
Restart=always
# User=$SERVICE_USER
# Group=$SERVICE_USER
StandardOutput=journal
StandardError=journal
SyslogIdentifier=traefik
# Environment="TRAEFIK_LOG_LEVEL=DEBUG" # Example: Set log level via environment

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