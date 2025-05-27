#!/bin/bash

# --- Configuration Variables ---
TRAEFIK_VERSION="v3.4.0"
TRAEFIK_INSTALL_DIR="/opt/traefik"
CONFIG_DIR="/etc/traefik"
PLUGIN_DIR_NAME="weightedredirector"
SERVICE_USER="traefikuser" # Optional

# --- Source Paths (these remain relative to the script) ---
SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )"
STATIC_CONFIG_TEMPLATE_SOURCE="${SCRIPT_DIR}/traefik.yml.template" # MODIFIED: Use a template file
DYNAMIC_CONFIG_SOURCE_DIR="${SCRIPT_DIR}/conf.d"
PLUGINS_REPO_ROOT_SOURCE_DIR="${SCRIPT_DIR}/plugins-local"

# --- Functions ---
# ... (log_info, log_error, check_root, create_user_if_not_exists, download_traefik 函数保持不变) ...
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

# MODIFIED: Get API server IP from command line argument
API_SERVER_IP="$1"
if [ -z "$API_SERVER_IP" ]; then
    log_error "Usage: $0 <api_server_ip_address>"
    # 或者你可以设置一个默认值，但不推荐用于IP地址
    # API_SERVER_IP="127.0.0.1" # Example default
    # log_info "API server IP not provided, using default: $API_SERVER_IP"
fi
log_info "Using API Server IP: $API_SERVER_IP"

# 0. (Optional) Create user
# create_user_if_not_exists "$SERVICE_USER"

log_info "Starting Traefik deployment (from GitHub source)..."

# 1. Create installation and config directories
# ... (目录创建部分保持不变) ...
log_info "Creating directories..."
sudo mkdir -p "$TRAEFIK_INSTALL_DIR" || log_error "Failed to create directory '$TRAEFIK_INSTALL_DIR'."
sudo mkdir -p "$CONFIG_DIR/conf.d" || log_error "Failed to create directory '$CONFIG_DIR/conf.d'."
PLUGIN_DESTINATION_BASE_DIR="$TRAEFIK_INSTALL_DIR/plugins-local"
PLUGIN_DESTINATION_DIR_WITH_SRC="$PLUGIN_DESTINATION_BASE_DIR/src/$PLUGIN_DIR_NAME" # This was the problematic path for Traefik
# Corrected destination for plugin files, directly under plugins-local/PLUGIN_NAME
PLUGIN_CORRECT_DESTINATION_DIR="$TRAEFIK_INSTALL_DIR/plugins-local/$PLUGIN_DIR_NAME"
sudo mkdir -p "$PLUGIN_CORRECT_DESTINATION_DIR" || log_error "Failed to create destination plugin directory: $PLUGIN_CORRECT_DESTINATION_DIR"


# 2. Download and install Traefik binary
download_traefik "$TRAEFIK_VERSION"

# 3. Copy config files and plugins
# ... (调试日志和文件存在性检查保持不变) ...
log_info "--- DEBUG: Path Variables ---"
log_info "SCRIPT_DIR:                   $SCRIPT_DIR"
# log_info "STATIC_CONFIG_TEMPLATE_SOURCE: $STATIC_CONFIG_TEMPLATE_SOURCE" # We will process this
log_info "DYNAMIC_CONFIG_SOURCE_DIR:    $DYNAMIC_CONFIG_SOURCE_DIR"
log_info "PLUGINS_REPO_ROOT_SOURCE_DIR: $PLUGINS_REPO_ROOT_SOURCE_DIR"
log_info "PLUGIN_DIR_NAME:              $PLUGIN_DIR_NAME"
PLUGIN_CODE_SOURCE_DIR="$PLUGINS_REPO_ROOT_SOURCE_DIR/src/$PLUGIN_DIR_NAME"
log_info "PLUGIN_CODE_SOURCE_DIR:       $PLUGIN_CODE_SOURCE_DIR"
log_info "PLUGIN_CORRECT_DESTINATION_DIR: $PLUGIN_CORRECT_DESTINATION_DIR"
log_info "--- END DEBUG: Path Variables ---"

if [ ! -f "$STATIC_CONFIG_TEMPLATE_SOURCE" ]; then # Check for template file
    log_error "Static config template file ($STATIC_CONFIG_TEMPLATE_SOURCE) not found."
fi
if [ ! -d "$DYNAMIC_CONFIG_SOURCE_DIR" ]; then
    log_error "Dynamic config source directory ($DYNAMIC_CONFIG_SOURCE_DIR) not found."
fi
if [ ! -d "$PLUGIN_CODE_SOURCE_DIR" ]; then
    log_error "Plugin source code directory ($PLUGIN_CODE_SOURCE_DIR) not found."
fi


# MODIFIED: Process traefik.yml.template template and copy
log_info "Processing and copying static config template from $STATIC_CONFIG_TEMPLATE_SOURCE to $CONFIG_DIR/traefik.yml..."
# 使用 sed 替换模板中的占位符
# 创建一个临时文件来处理，避免直接修改源模板
TEMP_TRAEFIK_YML=$(mktemp)
# shellcheck disable=SC2002
cat "$STATIC_CONFIG_TEMPLATE_SOURCE" | sed "s|__API_SERVER_IP_PLACEHOLDER__|${API_SERVER_IP}|g" > "$TEMP_TRAEFIK_YML"
sudo cp "$TEMP_TRAEFIK_YML" "$CONFIG_DIR/traefik.yml" || log_error "Failed to copy processed traefik.yml."
rm "$TEMP_TRAEFIK_YML"


log_info "Copying dynamic config files from $DYNAMIC_CONFIG_SOURCE_DIR/ to $CONFIG_DIR/conf.d/ ..."
sudo cp "$DYNAMIC_CONFIG_SOURCE_DIR/"* "$CONFIG_DIR/conf.d/" || log_error "Failed to copy dynamic config files."

log_info "Copying plugin files from $PLUGIN_CODE_SOURCE_DIR/ to $PLUGIN_CORRECT_DESTINATION_DIR/"
shopt -s dotglob
if [ -d "$PLUGIN_CODE_SOURCE_DIR" ] && [ -d "$PLUGIN_CORRECT_DESTINATION_DIR" ]; then
    sudo cp -rT "$PLUGIN_CODE_SOURCE_DIR" "$PLUGIN_CORRECT_DESTINATION_DIR" || log_error "Failed to copy plugin files (including hidden) to $PLUGIN_CORRECT_DESTINATION_DIR."
else
    log_error "Plugin source or destination directory does not exist or is not a directory. Source: $PLUGIN_CODE_SOURCE_DIR, Dest: $PLUGIN_CORRECT_DESTINATION_DIR"
fi
shopt -u dotglob


# 4. (Optional) Set file permissions

# 5. Create systemd service file
# ... (systemd 文件生成部分保持不变，使用 ExecStart=/bin/sh -c 'cd ...') ...
SYSTEMD_SERVICE_FILE="/etc/systemd/system/traefik.service"
log_info "Creating systemd service file '$SYSTEMD_SERVICE_FILE'..."
sudo bash -c "cat > $SYSTEMD_SERVICE_FILE" <<EOF
[Unit]
Description=Traefik Ingress Controller
After=network.target

[Service]
ExecStart=/bin/sh -c 'cd $TRAEFIK_INSTALL_DIR && $TRAEFIK_INSTALL_DIR/traefik --configFile=$CONFIG_DIR/traefik.yml'
Restart=always
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