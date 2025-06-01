#!/bin/bash

# --- Configuration Variables ---
TRAEFIK_VERSION="v3.4.0"
TRAEFIK_INSTALL_DIR="/opt/traefik"
CONFIG_DIR="/etc/traefik"
PLUGIN_DIR_NAME="weightedredirector"
SERVICE_USER="traefikuser" # Optional

# --- Source Paths (these remain relative to the script) ---
SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )"
STATIC_CONFIG_TEMPLATE_SOURCE="${SCRIPT_DIR}/traefik.yml.template"
DYNAMIC_CONFIG_SOURCE_DIR="${SCRIPT_DIR}/conf.d"
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

install_selinux_utils() {
    log_info "Checking for SELinux utilities (semanage)..."
    if command -v semanage &> /dev/null; then
        log_info "'semanage' is already available."
        return 0
    fi

    log_info "Attempting to install policycoreutils-python-utils (provides semanage)..."
    if command -v dnf &> /dev/null; then
        sudo dnf install -y policycoreutils-python-utils
    elif command -v yum &> /dev/null; then
        sudo yum install -y policycoreutils-python-utils
    else
        log_info "Warning: Neither 'dnf' nor 'yum' found. Cannot automatically install 'policycoreutils-python-utils'."
        log_info "If 'semanage' is not available, SELinux context changes will be skipped."
        return 1 # Indicate that semanage might not be available
    fi

    if command -v semanage &> /dev/null; then
        log_info "'semanage' installed successfully."
        return 0
    else
        log_error "Failed to install 'policycoreutils-python-utils' or 'semanage' is still not found."
        log_info "SELinux context changes will likely fail or be skipped."
        return 1 # Indicate failure
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

# Validate IP address format
validate_ip() {
    local ip=$1
    local stat=1

    if [[ $ip =~ ^[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}$ ]]; then
        OIFS=$IFS
        IFS='.'
        ip=($ip)
        IFS=$OIFS
        [[ ${ip[0]} -le 255 && ${ip[1]} -le 255 && ${ip[2]} -le 255 && ${ip[3]} -le 255 ]]
        stat=$?
    fi
    return $stat
}

# --- Main Logic ---
check_root
install_selinux_utils

# Validate API_SERVER_IP parameter
API_SERVER_IP="$1"
if [ -z "$API_SERVER_IP" ]; then
    log_error "Usage: $0 <api_server_ip_address>"
fi

if ! validate_ip "$API_SERVER_IP"; then
    log_error "Invalid IP address format: $API_SERVER_IP"
fi
log_info "Using API Server IP: $API_SERVER_IP"

# 0. (Optional) Create user
# create_user_if_not_exists "$SERVICE_USER"

log_info "Starting Traefik deployment (from GitHub source)..."

# 1. Create installation and config directories
log_info "Creating directories..."
sudo mkdir -p "$TRAEFIK_INSTALL_DIR" || log_error "Failed to create directory '$TRAEFIK_INSTALL_DIR'."
sudo mkdir -p "$CONFIG_DIR/conf.d" || log_error "Failed to create directory '$CONFIG_DIR/conf.d'."
PLUGIN_DESTINATION_BASE_DIR="$TRAEFIK_INSTALL_DIR/plugins-local"
PLUGIN_DESTINATION_DIR_WITH_SRC="$PLUGIN_DESTINATION_BASE_DIR/src/$PLUGIN_DIR_NAME"
PLUGIN_CORRECT_DESTINATION_DIR="$TRAEFIK_INSTALL_DIR/plugins-local/$PLUGIN_DIR_NAME"
sudo mkdir -p "$PLUGIN_CORRECT_DESTINATION_DIR" || log_error "Failed to create destination plugin directory: $PLUGIN_CORRECT_DESTINATION_DIR"

# 2. Download and install Traefik binary
download_traefik "$TRAEFIK_VERSION"

# Apply SELinux context to Traefik binary
if command -v semanage &> /dev/null && command -v restorecon &> /dev/null; then
    log_info "Applying SELinux context to Traefik binary ($TRAEFIK_INSTALL_DIR/traefik)..."
    # Target type httpd_sys_script_exec_t is common for web service executables
    TARGET_CONTEXT_TYPE="httpd_sys_script_exec_t"
    TARGET_BINARY_PATH="$TRAEFIK_INSTALL_DIR/traefik"

    # Check if a context is already defined for this exact path pattern
    if sudo semanage fcontext -l | grep -q -E "^${TARGET_BINARY_PATH//\//\\/}\s+"; then # Escape slashes for regex
        log_info "Modifying existing SELinux context for $TARGET_BINARY_PATH to $TARGET_CONTEXT_TYPE."
        sudo semanage fcontext -m -t "$TARGET_CONTEXT_TYPE" "$TARGET_BINARY_PATH" || log_info "Warning: semanage fcontext -m command failed. Continuing."
    else
        log_info "Adding new SELinux context for $TARGET_BINARY_PATH as $TARGET_CONTEXT_TYPE."
        sudo semanage fcontext -a -t "$TARGET_CONTEXT_TYPE" "$TARGET_BINARY_PATH" || log_info "Warning: semanage fcontext -a command failed. Continuing."
    fi

    if sudo restorecon -vF "$TARGET_BINARY_PATH"; then
        log_info "SELinux context applied with restorecon for $TARGET_BINARY_PATH."
        current_context=$(ls -Z "$TARGET_BINARY_PATH" 2>/dev/null | awk '{print $1}')
        log_info "Current SELinux context of Traefik binary: $current_context"
        if ! echo "$current_context" | grep -q "$TARGET_CONTEXT_TYPE"; then
            log_info "Warning: SELinux context for Traefik binary might not be $TARGET_CONTEXT_TYPE. Current: $current_context. Manual check might be needed."
        fi
    else
        log_error "SELinux: restorecon command failed for Traefik binary. The service might not start if SELinux is enforcing."
    fi
else
    log_info "Warning: 'semanage' or 'restorecon' command not found. Skipping SELinux context changes for Traefik binary."
fi

# 3. Copy config files and plugins
log_info "--- DEBUG: Path Variables ---"
log_info "SCRIPT_DIR:                   $SCRIPT_DIR"
log_info "STATIC_CONFIG_TEMPLATE_SOURCE: $STATIC_CONFIG_TEMPLATE_SOURCE"
log_info "DYNAMIC_CONFIG_SOURCE_DIR:    $DYNAMIC_CONFIG_SOURCE_DIR"
log_info "PLUGINS_REPO_ROOT_SOURCE_DIR: $PLUGINS_REPO_ROOT_SOURCE_DIR"
log_info "PLUGIN_DIR_NAME:              $PLUGIN_DIR_NAME"
PLUGIN_CODE_SOURCE_DIR="$PLUGINS_REPO_ROOT_SOURCE_DIR/src/$PLUGIN_DIR_NAME"
log_info "PLUGIN_CODE_SOURCE_DIR:       $PLUGIN_CODE_SOURCE_DIR"
log_info "PLUGIN_CORRECT_DESTINATION_DIR: $PLUGIN_CORRECT_DESTINATION_DIR"
log_info "--- END DEBUG: Path Variables ---"

if [ ! -f "$STATIC_CONFIG_TEMPLATE_SOURCE" ]; then
    log_error "Static config template file ($STATIC_CONFIG_TEMPLATE_SOURCE) not found."
fi
if [ ! -d "$DYNAMIC_CONFIG_SOURCE_DIR" ]; then
    log_error "Dynamic config source directory ($DYNAMIC_CONFIG_SOURCE_DIR) not found."
fi
if [ ! -d "$PLUGIN_CODE_SOURCE_DIR" ]; then
    log_error "Plugin source code directory ($PLUGIN_CODE_SOURCE_DIR) not found."
fi

# Process traefik.yml.template template and copy
log_info "Processing and copying static config template from $STATIC_CONFIG_TEMPLATE_SOURCE to $CONFIG_DIR/traefik.yml..."
TEMP_TRAEFIK_YML=$(mktemp)
# shellcheck disable=SC2002
cat "$STATIC_CONFIG_TEMPLATE_SOURCE" | sed "s|__API_SERVER_IP_PLACEHOLDER__|${API_SERVER_IP}|g" > "$TEMP_TRAEFIK_YML"
sudo cp "$TEMP_TRAEFIK_YML" "$CONFIG_DIR/traefik.yml" || log_error "Failed to copy processed traefik.yml."
rm "$TEMP_TRAEFIK_YML"

# Verify the processed config file contains the correct IP
if ! grep -q "$API_SERVER_IP" "$CONFIG_DIR/traefik.yml"; then
    log_error "API_SERVER_IP not found in processed traefik.yml. Please check the template file."
fi

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

# Apply SELinux context to Traefik plugins directory
if command -v semanage &> /dev/null && command -v restorecon &> /dev/null; then
    log_info "Applying SELinux context to Traefik plugins directory ($PLUGIN_CORRECT_DESTINATION_DIR)..."
    # Target type httpd_sys_content_t is common for web content read by services
    TARGET_PLUGIN_CONTEXT_TYPE="httpd_sys_content_t"
    TARGET_PLUGIN_PATH_PATTERN="${PLUGIN_CORRECT_DESTINATION_DIR}(/.*)?"

    # Escape path for grep regex
    ESCAPED_PLUGIN_PATH_PATTERN="^${PLUGIN_CORRECT_DESTINATION_DIR//\//\\/}(\\/.*)?\s+"

    if sudo semanage fcontext -l | grep -q -E "$ESCAPED_PLUGIN_PATH_PATTERN"; then
        log_info "Modifying existing SELinux context for plugins $PLUGIN_CORRECT_DESTINATION_DIR to $TARGET_PLUGIN_CONTEXT_TYPE."
        sudo semanage fcontext -m -t "$TARGET_PLUGIN_CONTEXT_TYPE" "$TARGET_PLUGIN_PATH_PATTERN" || log_info "Warning: semanage fcontext -m for plugins failed."
    else
        log_info "Adding new SELinux context for plugins $PLUGIN_CORRECT_DESTINATION_DIR as $TARGET_PLUGIN_CONTEXT_TYPE."
        sudo semanage fcontext -a -t "$TARGET_PLUGIN_CONTEXT_TYPE" "$TARGET_PLUGIN_PATH_PATTERN" || log_info "Warning: semanage fcontext -a for plugins failed."
    fi

    if sudo restorecon -RvvF "$PLUGIN_CORRECT_DESTINATION_DIR"; then
        log_info "SELinux context refreshed for $PLUGIN_CORRECT_DESTINATION_DIR."
    else
        log_info "Warning: SELinux: restorecon command failed for $PLUGIN_CORRECT_DESTINATION_DIR."
    fi
else
    log_info "Warning: 'semanage' or 'restorecon' command not found. Skipping SELinux context changes for plugins."
fi

# 4. Set file permissions
log_info "Setting file permissions..."
sudo chown -R root:root "$CONFIG_DIR"
sudo chmod -R 644 "$CONFIG_DIR"/*
sudo chmod -R 755 "$CONFIG_DIR/conf.d"

# Apply SELinux context to Traefik config directory
if command -v restorecon &> /dev/null; then
    log_info "Applying SELinux context to Traefik config directory ($CONFIG_DIR)..."
    # Default context for /etc files is usually etc_t. restorecon should enforce this.
    if sudo restorecon -RvvF "$CONFIG_DIR"; then
        log_info "SELinux context refreshed for $CONFIG_DIR."
        current_context_config=$(ls -Z "$CONFIG_DIR/traefik.yml" 2>/dev/null | awk '{print $1}')
        log_info "Current SELinux context of $CONFIG_DIR/traefik.yml: $current_context_config"
        if ! echo "$current_context_config" | grep -q "etc_t"; then
            log_info "Warning: SELinux context for $CONFIG_DIR/traefik.yml might not be etc_t (or similar like user_home_etc_t). Current: $current_context_config."
        fi
    else
        log_info "Warning: SELinux: restorecon command failed for $CONFIG_DIR. Default contexts might be incorrect."
    fi
else
    log_info "Warning: 'restorecon' command not found. Skipping SELinux context refresh for $CONFIG_DIR."
fi

# 5. Create systemd service file
SYSTEMD_SERVICE_FILE="/etc/systemd/system/traefik.service"
log_info "Creating systemd service file '$SYSTEMD_SERVICE_FILE'..."
sudo bash -c "cat > $SYSTEMD_SERVICE_FILE" <<EOF
[Unit]
Description=Traefik Ingress Controller
After=network.target

[Service]
Type=simple
ExecStart=$TRAEFIK_INSTALL_DIR/traefik --configFile=$CONFIG_DIR/traefik.yml
Restart=always
RestartSec=5
LimitNOFILE=65536
StandardOutput=journal
StandardError=journal
SyslogIdentifier=traefik

[Install]
WantedBy=multi-user.target
EOF

# Verify systemd service file
if ! grep -q "$TRAEFIK_INSTALL_DIR/traefik" "$SYSTEMD_SERVICE_FILE"; then
    log_error "Traefik executable path not found in systemd service file. Please check."
fi

log_info "Reloading systemd and enabling/starting Traefik service..."
sudo systemctl daemon-reload
sudo systemctl enable traefik.service

# Wait for service initialization before checking status
log_info "Starting Traefik service and waiting for it to initialize..."
sudo systemctl restart traefik.service
sleep 5

# Check service status
log_info "Checking Traefik service status:"
if ! sudo systemctl is-active --quiet traefik.service; then
    log_error "Traefik service is not active. Checking logs:"
    journalctl -u traefik.service -n 20 --no-pager
fi

# Check if Traefik process is running as expected
if ! pgrep -f "traefik --configFile=$CONFIG_DIR/traefik.yml" > /dev/null; then
    log_error "Traefik process is not running as expected."
fi

log_info "Deployment completed successfully!"
echo "Traefik is running and configured with API server IP: $API_SERVER_IP"
echo "Check Traefik logs: journalctl -u traefik -f"