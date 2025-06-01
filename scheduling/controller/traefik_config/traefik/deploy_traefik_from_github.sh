#!/bin/bash
set -e

# --- Configuration Variables ---
TRAEFIK_VERSION="v3.4.0" # Or the version you want to install, or "latest"
CONFIG_DIR="/etc/traefik"
PLUGIN_DIR_NAME="weightedredirector" # Your plugin name
# SERVICE_USER="traefikuser" # Optional, if you need to run as a non-root user

# --- Installation Paths (Strategy 1: Standard Paths) ---
TRAEFIK_BIN_INSTALL_PATH="/usr/local/bin/traefik" # Standard path for Traefik binary

# === Fix: Plugin Path Configuration ===
# Use plugins-local in the root directory to match Traefik's default working directory
PLUGINS_BASE_DIR="/plugins-local"
PLUGINS_SOURCE_STRUCTURE="src" # Traefik requires src subdirectory structure

# --- Source Paths (relative to the script) ---
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
    elif command -v apt-get &> /dev/null; then # Add support for Debian/Ubuntu
        sudo apt-get update && sudo apt-get install -y policycoreutils-python-utils selinux-utils
    else
        log_info "Warning: No common package manager (dnf, yum, apt-get) found. Cannot automatically install 'policycoreutils-python-utils'."
        log_info "If 'semanage' is not available, SELinux context changes will be skipped or may fail."
        return 1
    fi

    if command -v semanage &> /dev/null; then
        log_info "'semanage' installed successfully."
        return 0
    else
        log_error "Failed to install 'policycoreutils-python-utils' or 'semanage' is still not found."
        log_info "SELinux context changes will likely fail or be skipped."
        return 1
    fi
}

download_traefik() {
    local version="$1"
    local install_path="$2" # Pass in the installation path
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
    if curl -sSL "$download_url" -o "$TEMP_DOWNLOAD_DIR/traefik.tar.gz"; then # -sS for silent with errors, -L for redirects
        log_info "Download completed. Extracting..."
        if tar -xzf "$TEMP_DOWNLOAD_DIR/traefik.tar.gz" -C "$TEMP_DOWNLOAD_DIR"; then
            if [ -f "$TEMP_DOWNLOAD_DIR/$traefik_binary_name" ]; then
                # Ensure target directory exists (e.g. /usr/local/bin)
                sudo mkdir -p "$(dirname "$install_path")"
                sudo mv "$TEMP_DOWNLOAD_DIR/$traefik_binary_name" "$install_path"
                sudo chmod +x "$install_path"
                log_info "Traefik binary installed to $install_path"
            else
                log_error "Failed to find '$traefik_binary_name' in the extracted package."
            fi
        else
            log_error "Failed to extract Traefik package."
        fi
    else
        log_error "Failed to download Traefik. Please check the version number and network connection. URL: $download_url"
    fi
    rm -rf "$TEMP_DOWNLOAD_DIR"
}

validate_ip() {
    local ip=$1
    local stat=1
    if [[ $ip =~ ^[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}$ ]]; then
        OIFS=$IFS
        IFS='.'
        ip_array=($ip) # Use a different variable name to avoid conflict
        IFS=$OIFS
        [[ ${ip_array[0]} -le 255 && ${ip_array[1]} -le 255 && ${ip_array[2]} -le 255 && ${ip_array[3]} -le 255 ]]
        stat=$?
    fi
    return $stat
}

# === New: Plugin Installation Function ===
install_plugins() {
    local plugin_name="$1"
    local source_plugin_dir="$2"
    
    log_info "Installing Traefik plugins with correct directory structure..."
    
    # Calculate paths
    local plugin_code_source_dir="${source_plugin_dir}/src/${plugin_name}"
    local plugin_correct_destination_dir="${PLUGINS_BASE_DIR}/${PLUGINS_SOURCE_STRUCTURE}/${plugin_name}"
    
    log_info "--- Plugin Installation Debug ---"
    log_info "PLUGINS_BASE_DIR: $PLUGINS_BASE_DIR"
    log_info "PLUGIN_NAME: $plugin_name"
    log_info "SOURCE_DIR: $plugin_code_source_dir"
    log_info "DESTINATION_DIR: $plugin_correct_destination_dir"
    log_info "--- End Plugin Debug ---"
    
    # Check source directory
    if [ ! -d "$plugin_code_source_dir" ]; then
        log_error "Plugin source code directory ($plugin_code_source_dir) not found."
    fi
    
    # Create target directory structure
    sudo mkdir -p "$plugin_correct_destination_dir" || log_error "Failed to create destination plugin directory: $plugin_correct_destination_dir"
    
    # Copy plugin files to the correct location
    log_info "Copying plugin files from $plugin_code_source_dir/ to $plugin_correct_destination_dir/"
    shopt -s dotglob # Include hidden files
    if [ -d "$plugin_code_source_dir" ] && [ -d "$plugin_correct_destination_dir" ]; then
        sudo cp -rT "$plugin_code_source_dir" "$plugin_correct_destination_dir" || log_error "Failed to copy plugin files to $plugin_correct_destination_dir."
    else
        log_error "Plugin source or destination directory does not exist. Source: $plugin_code_source_dir, Dest: $plugin_correct_destination_dir"
    fi
    shopt -u dotglob
    
    # Ensure plugin manifest file exists and is correctly formatted
    local manifest_file="$plugin_correct_destination_dir/.traefik.yml"
    if [ ! -f "$manifest_file" ]; then
        log_info "Creating plugin manifest file: $manifest_file"
        sudo tee "$manifest_file" > /dev/null <<EOF
# Traefik Plugin Manifest for ${plugin_name}
displayName: "Weighted Redirector"
type: "middleware"
summary: "A plugin for weighted HTTP redirection"

# Import path must match the moduleName in static config
import: "${plugin_name}"

# Test data for plugin configuration
testData:
  redirections:
    - weight: 50
      url: "http://example1.com"
    - weight: 50  
      url: "http://example2.com"
EOF
        log_info "Plugin manifest file created."
    else
        log_info "Plugin manifest file already exists: $manifest_file"
    fi
    
    # Verify key files
    if [ -f "$manifest_file" ] && [ -f "$plugin_correct_destination_dir/${plugin_name}.go" ]; then
        log_info "✅ Plugin installation verified:"
        log_info "   - Manifest: $manifest_file"
        log_info "   - Code: $plugin_correct_destination_dir/${plugin_name}.go"
    else
        log_error "Plugin installation verification failed. Missing files in $plugin_correct_destination_dir"
    fi
    
    # Apply SELinux context to plugin directory
    apply_selinux_context_to_plugins "$PLUGINS_BASE_DIR"
    
    # Set permissions
    log_info "Setting plugin file permissions..."
    sudo chown -R root:root "$PLUGINS_BASE_DIR"
    sudo chmod -R 644 "$PLUGINS_BASE_DIR"
    sudo find "$PLUGINS_BASE_DIR" -type d -exec chmod 755 {} \;
}

# === New: SELinux Context Application Function ===
apply_selinux_context_to_plugins() {
    local plugins_dir="$1"
    
    if command -v semanage &> /dev/null && command -v restorecon &> /dev/null; then
        log_info "Applying SELinux context to Traefik plugins directory ($plugins_dir)..."
        local target_plugin_context_type="httpd_sys_content_t" # Suitable for content read by Traefik
        local target_plugin_path_pattern="${plugins_dir}(/.*)?"
        local escaped_plugin_path_pattern="^${plugins_dir//\//\\/}(\\/.*)?\s+"

        # Check if the context rule exists, then add or modify
        if sudo semanage fcontext -l | grep -q -E "$escaped_plugin_path_pattern"; then
            log_info "Modifying existing SELinux context for plugins $plugins_dir to $target_plugin_context_type."
            sudo semanage fcontext -m -t "$target_plugin_context_type" "$target_plugin_path_pattern" || log_info "Warning: semanage fcontext -m for plugins failed."
        else
            log_info "Adding new SELinux context for plugins $plugins_dir as $target_plugin_context_type."
            sudo semanage fcontext -a -t "$target_plugin_context_type" "$target_plugin_path_pattern" || log_info "Warning: semanage fcontext -a for plugins failed."
        fi

        if sudo restorecon -RvvF "$plugins_dir"; then
            log_info "SELinux context refreshed for $plugins_dir."
        else
            log_info "Warning: SELinux: restorecon command failed for $plugins_dir."
        fi
    else
        log_info "Warning: 'semanage' or 'restorecon' command not found. Skipping SELinux context changes for plugins."
    fi
}

# === Modified: Create systemd Service File Function ===
create_systemd_service() {
    local traefik_bin_path="$1"
    local config_file="$2"
    local systemd_service_file="/etc/systemd/system/traefik.service"
    
    log_info "Creating systemd service file '$systemd_service_file'..."
    sudo bash -c "cat > $systemd_service_file" <<EOF
[Unit]
Description=Traefik Ingress Controller
Documentation=https://doc.traefik.io/traefik
# Wait for network to be truly online
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
# === Fix: Explicitly set working directory to root ===
WorkingDirectory=/
ExecStart=$traefik_bin_path --configFile=$config_file
# Changed from always to on-failure, or keep always if preferred
Restart=on-failure
RestartSec=5
# Consider User= and Group= if SERVICE_USER is defined and created
# User=$SERVICE_USER
# Group=$SERVICE_USER
# AmbientCapabilities=CAP_NET_BIND_SERVICE # Needed if non-root user binds to ports < 1024

# Security Hardening (optional, but good practice if not needing specific capabilities)
# CapabilityBoundingSet=CAP_NET_BIND_SERVICE
# AmbientCapabilities=CAP_NET_BIND_SERVICE
# NoNewPrivileges=true
# ProtectSystem=full
# ProtectHome=true
# PrivateTmp=true
# PrivateDevices=true
# ProtectKernelTunables=true
# ProtectKernelModules=true
# ProtectControlGroups=true
# ReadWritePaths=/var/log/traefik # If Traefik writes logs to a file, make sure this path is writable
# ReadOnlyPaths=/etc/traefik # Config should be read-only by the process

LimitNOFILE=65536
StandardOutput=journal
StandardError=journal
SyslogIdentifier=traefik

[Install]
WantedBy=multi-user.target
EOF

    if ! grep -q "$traefik_bin_path" "$systemd_service_file"; then
        log_error "Traefik executable path not found in systemd service file. Please check."
    fi
    
    log_info "✅ systemd service file created with WorkingDirectory=/"
}

# --- Main Logic ---
check_root
install_selinux_utils # Attempt to install if not present

API_SERVER_IP="$1"
if [ -z "$API_SERVER_IP" ]; then
    log_error "Usage: $0 <api_server_ip_address>"
fi

if ! validate_ip "$API_SERVER_IP"; then
    log_error "Invalid IP address format: $API_SERVER_IP"
fi
log_info "Using API Server IP: $API_SERVER_IP"

log_info "Starting Traefik deployment (Improved with plugin fixes)..."

# 1. Create config directories
log_info "Creating directories..."
sudo mkdir -p "$CONFIG_DIR/conf.d" || log_error "Failed to create directory '$CONFIG_DIR/conf.d'."

# 2. Download and install Traefik binary
download_traefik "$TRAEFIK_VERSION" "$TRAEFIK_BIN_INSTALL_PATH"

# Apply SELinux context to Traefik binary
if command -v restorecon &> /dev/null; then
    log_info "Applying SELinux context to Traefik binary ($TRAEFIK_BIN_INSTALL_PATH)..."
    if sudo restorecon -vF "$TRAEFIK_BIN_INSTALL_PATH"; then
        log_info "SELinux context applied/verified with restorecon for $TRAEFIK_BIN_INSTALL_PATH."
        current_context=$(ls -Z "$TRAEFIK_BIN_INSTALL_PATH" 2>/dev/null | awk '{print $1}')
        log_info "Current SELinux context of Traefik binary: $current_context"
    else
        log_info "Warning: SELinux 'restorecon' command for Traefik binary indicated an issue or no change."
    fi
else
    log_info "Warning: 'restorecon' command not found. Skipping SELinux context changes for Traefik binary."
fi

# 3. Install plugins with correct structure
log_info "=== Installing Plugins ==="
install_plugins "$PLUGIN_DIR_NAME" "$PLUGINS_REPO_ROOT_SOURCE_DIR"

# 4. Copy config files 
log_info "=== Processing Configuration Files ==="
log_info "--- DEBUG: Path Variables ---"
log_info "SCRIPT_DIR:                   $SCRIPT_DIR"
log_info "STATIC_CONFIG_TEMPLATE_SOURCE: $STATIC_CONFIG_TEMPLATE_SOURCE"
log_info "DYNAMIC_CONFIG_SOURCE_DIR:    $DYNAMIC_CONFIG_SOURCE_DIR"
log_info "PLUGINS_REPO_ROOT_SOURCE_DIR: $PLUGINS_REPO_ROOT_SOURCE_DIR"
log_info "PLUGIN_DIR_NAME:              $PLUGIN_DIR_NAME"
PLUGIN_CODE_SOURCE_DIR="$PLUGINS_REPO_ROOT_SOURCE_DIR/src/$PLUGIN_DIR_NAME"
log_info "PLUGIN_CODE_SOURCE_DIR:       $PLUGIN_CODE_SOURCE_DIR"
log_info "PLUGINS_BASE_DIR:             $PLUGINS_BASE_DIR"
log_info "--- END DEBUG: Path Variables ---"

if [ ! -f "$STATIC_CONFIG_TEMPLATE_SOURCE" ]; then
    log_error "Static config template file ($STATIC_CONFIG_TEMPLATE_SOURCE) not found."
fi
if [ ! -d "$DYNAMIC_CONFIG_SOURCE_DIR" ]; then
    log_error "Dynamic config source directory ($DYNAMIC_CONFIG_SOURCE_DIR) not found."
fi

log_info "Processing and copying static config template from $STATIC_CONFIG_TEMPLATE_SOURCE to $CONFIG_DIR/traefik.yml..."
TEMP_TRAEFIK_YML=$(mktemp)
# shellcheck disable=SC2002
cat "$STATIC_CONFIG_TEMPLATE_SOURCE" | sed "s|__API_SERVER_IP_PLACEHOLDER__|${API_SERVER_IP}|g" > "$TEMP_TRAEFIK_YML"
sudo cp "$TEMP_TRAEFIK_YML" "$CONFIG_DIR/traefik.yml" || log_error "Failed to copy processed traefik.yml."
rm "$TEMP_TRAEFIK_YML"

if ! grep -q "$API_SERVER_IP" "$CONFIG_DIR/traefik.yml"; then
    log_error "API_SERVER_IP not found in processed traefik.yml. Please check the template file."
fi

log_info "Copying dynamic config files from $DYNAMIC_CONFIG_SOURCE_DIR/ to $CONFIG_DIR/conf.d/ ..."
sudo cp -r "$DYNAMIC_CONFIG_SOURCE_DIR/"* "$CONFIG_DIR/conf.d/" || log_error "Failed to copy dynamic config files." # -r for directories if any

# 5. Set file permissions
log_info "Setting file permissions..."
sudo chown -R root:root "$CONFIG_DIR"
sudo chmod -R 640 "$CONFIG_DIR" # Config files often contain sensitive data, restrict group/other read
sudo chmod 750 "$CONFIG_DIR/conf.d" # Allow root to rwx, group to rx

# Apply SELinux context to Traefik config directory (/etc/traefik)
if command -v restorecon &> /dev/null; then
    log_info "Applying SELinux context to Traefik config directory ($CONFIG_DIR)..."
    if sudo restorecon -RvvF "$CONFIG_DIR"; then
        log_info "SELinux context refreshed for $CONFIG_DIR."
        current_context_config=$(ls -Z "$CONFIG_DIR/traefik.yml" 2>/dev/null | awk '{print $1}')
        log_info "Current SELinux context of $CONFIG_DIR/traefik.yml: $current_context_config"
    else
        log_info "Warning: SELinux: restorecon command failed for $CONFIG_DIR."
    fi
else
    log_info "Warning: 'restorecon' command not found. Skipping SELinux context refresh for $CONFIG_DIR."
fi

# 6. Create systemd service file with correct WorkingDirectory
create_systemd_service "$TRAEFIK_BIN_INSTALL_PATH" "$CONFIG_DIR/traefik.yml"

log_info "Reloading systemd and enabling/starting Traefik service..."
sudo systemctl daemon-reload
sudo systemctl enable traefik.service

log_info "Starting Traefik service and waiting for it to initialize..."
sudo systemctl restart traefik.service
sleep 5 # Give it a moment to start

# Fixed verification function - replace the verification part in the deployment script

# === Fixed Verification Function ===
verify_deployment() {
    local max_attempts=12  # Max wait 60 seconds (12 * 5 seconds)
    local attempt=1
    
    log_info "=== Deployment Verification ==="
    log_info "Waiting for Traefik service to start..."
    
    while [ $attempt -le $max_attempts ]; do
        log_info "Attempt $attempt/$max_attempts: Checking service status..."
        
        # Check systemd service status
        if sudo systemctl is-active --quiet traefik.service; then
            log_info "✅ systemd service status: active"
            
            # Check port listening
            if netstat -tlnp 2>/dev/null | grep -q ":8080.*traefik"; then
                log_info "✅ Port listening: Port 8080 is normal"
                
                # Check API response
                if curl -s -f "http://localhost:8080/ping" > /dev/null 2>&1; then
                    log_info "✅ API response: Normal"
                    break
                else
                    log_info "⚠️  API not yet responding, continuing to wait..."
                fi
            else
                log_info "⚠️  Port not yet listening, continuing to wait..."
            fi
        else
            log_info "⚠️  Service not yet active, continuing to wait..."
        fi
        
        if [ $attempt -eq $max_attempts ]; then
            log_error "Service startup verification failed. Check detailed logs..."
            echo "=== Service Status ==="
            sudo systemctl status traefik.service --no-pager -l
            echo ""
            echo "=== Recent Logs ==="
            sudo journalctl -u traefik.service --since="2 minutes ago" --no-pager -l
            exit 1
        fi
        
        sleep 5
        ((attempt++))
    done
    
    log_info "✅ Service startup verification successful!"
    
    # Check plugin loading status
    log_info "Checking plugin loading status..."
    sleep 2 # Wait for plugins to load
    
    if sudo journalctl -u traefik.service --since="30 seconds ago" | grep -q "Loading plugins"; then
        log_info "✅ Plugin loading information found"
        if sudo journalctl -u traefik.service --since="30 seconds ago" | grep -q "Plugins loaded"; then
            log_info "✅ Plugins loaded successfully"
        else
            log_info "⚠️  Plugins loading..."
        fi
    else
        log_info "⚠️  Plugin loading information not found"
    fi
    
    # Check for plugin errors
    if sudo journalctl -u traefik.service --since="30 seconds ago" | grep -q "failed to open the plugin manifest"; then
        log_info "❌ Plugin manifest error found:"
        sudo journalctl -u traefik.service --since="30 seconds ago" | grep "plugin" | tail -3
    else
        log_info "✅ No plugin manifest errors"
    fi
    
    # Get server IP
    local server_ip=$(hostname -I | awk '{print $1}')
    
    # Final status report
    log_info "=== Deployment Successful! ==="
    echo ""
    echo "🎯 Service Status:"
    echo "  - Traefik Version: $TRAEFIK_VERSION"
    echo "  - Service Status: $(sudo systemctl is-active traefik.service)"
    echo "  - Process PID: $(pgrep -f traefik | head -1)"
    echo "  - Listening Ports: 80 (HTTP), 8080 (Dashboard/API)"
    echo ""
    echo "🌐 Access Addresses:"
    echo "  - Dashboard: http://$server_ip:8080/dashboard/"
    echo "  - API: http://$server_ip:8080/api/rawdata"
    echo "  - Health: http://$server_ip:8080/ping"
    echo ""
    echo "📋 Management Commands:"
    echo "  - Check status: sudo systemctl status traefik.service"
    echo "  - View logs: sudo journalctl -u traefik.service -f"
    echo "  - Restart service: sudo systemctl restart traefik.service"
    echo ""
    echo "📁 Configuration Files:"
    echo "  - Static Config: $CONFIG_DIR/traefik.yml"
    echo "  - Dynamic Config Directory: $CONFIG_DIR/conf.d/"
    echo "  - Plugin Directory: $PLUGINS_BASE_DIR/src/$PLUGIN_DIR_NAME/"
    echo ""
    echo "🔧 Next Steps:"
    echo "  1. Access Dashboard to confirm plugin is loaded"
    echo "  2. Add route configurations in $CONFIG_DIR/conf.d/ directory"
    echo "  3. If HTTP Provider is needed, resolve API server connection issues"
}

# === Replace the verification part in the original script ===
# Replace the part from "# 7. Verification" to the end of the script with:

log_info "Starting Traefik service and waiting for it to initialize..."
sudo systemctl restart traefik.service

# Call the improved verification function
verify_deployment
# 8. Final status
log_info "=== Deployment Summary ==="
log_info "✅ Deployment completed successfully!"
echo "Traefik is running and configured with API server IP: $API_SERVER_IP"
echo ""
echo "🎯 Status:"
echo "  - Traefik version: $(sudo $TRAEFIK_BIN_INSTALL_PATH version --short 2>/dev/null || echo 'Unable to determine')"
echo "  - Service status: $(sudo systemctl is-active traefik.service)"
echo "  - Plugin directory: $PLUGINS_BASE_DIR"
echo "  - Config directory: $CONFIG_DIR"
echo ""
echo "🌐 Access URLs:"
echo "  - Dashboard: http://$(hostname -I | awk '{print $1}'):8080"
echo "  - API: http://$(hostname -I | awk '{print $1}'):8080/api/rawdata"
echo ""
echo "📋 Useful commands:"
echo "  - Check status: sudo systemctl status traefik.service"
echo "  - Follow logs: sudo journalctl -u traefik.service -f"
echo "  - Plugin verification: ls -la $PLUGINS_BASE_DIR/src/$PLUGIN_DIR_NAME/"
echo ""
echo "📁 Configuration:"
echo "  - Static config: $CONFIG_DIR/traefik.yml"
echo "  - Dynamic configs: $CONFIG_DIR/conf.d/"
echo "  - Add new routes by creating .yml files in $CONFIG_DIR/conf.d/"