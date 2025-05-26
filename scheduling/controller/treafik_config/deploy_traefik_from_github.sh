#!/bin/bash

# --- 配置变量 ---
TRAEFIK_VERSION="v2.10.7" # 指定要下载的 Traefik 版本，或者设为 "latest"
TRAEFIK_INSTALL_DIR="/opt/traefik"
CONFIG_DIR="/etc/traefik"
PLUGIN_DIR_NAME="weightedredirector"
SERVICE_USER="traefikuser" # 可选

# GitHub 仓库信息 (用于下载配置文件和插件)
# 假设脚本和配置文件在同一个仓库的相对路径下
# 如果你的脚本是从别处下载的，你需要调整这些路径或让用户传入
SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )" # 获取脚本所在目录
CONFIG_SOURCE_DIR="${SCRIPT_DIR}/config"
PLUGINS_SOURCE_DIR="${SCRIPT_DIR}/plugins"


# --- 函数 ---
log_info() {
    echo "[INFO] $1"
}

log_error() {
    echo "[ERROR] $1" >&2
    exit 1
}

check_root() {
    if [ "$(id -u)" -ne 0 ]; then
        log_error "此脚本需要 root 权限运行。请使用 sudo。"
    fi
}

# (可选) 创建用户函数 (同前一个脚本)
create_user_if_not_exists() {
    if ! id "$1" &>/dev/null; then
        log_info "创建用户 '$1'..."
        sudo useradd -r -s /bin/false "$1" || log_error "创建用户 '$1' 失败。"
    else
        log_info "用户 '$1' 已存在。"
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
        log_error "不支持的架构: $arch"
    fi

    if [ "$version" == "latest" ]; then
        # 获取最新的稳定版本标签
        # 注意: 这依赖于 GitHub API 和 jq。如果服务器没有 jq，需要提示安装或使用其他方法。
        if ! command -v jq &> /dev/null; then
            log_error "需要 'jq' 来获取最新版本。请安装 jq (例如: sudo apt install jq) 或指定一个具体的 TRAEFIK_VERSION。"
        fi
        version_tag=$(curl -s https://api.github.com/repos/traefik/traefik/releases/latest | jq -r .tag_name)
        if [ -z "$version_tag" ] || [ "$version_tag" == "null" ]; then
            log_error "无法从 GitHub API 获取最新的 Traefik 版本标签。"
        fi
        log_info "获取到的最新 Traefik 版本: $version_tag"
    else
        version_tag="$version"
    fi

    download_url="https://github.com/traefik/traefik/releases/download/${version_tag}/traefik_${version_tag}_${os}_${arch}.tar.gz"

    log_info "正在从 $download_url 下载 Traefik ${version_tag}..."
    TEMP_DOWNLOAD_DIR=$(mktemp -d)
    if curl -L "$download_url" -o "$TEMP_DOWNLOAD_DIR/traefik.tar.gz"; then
        log_info "下载完成。正在解压..."
        if tar -xzf "$TEMP_DOWNLOAD_DIR/traefik.tar.gz" -C "$TEMP_DOWNLOAD_DIR"; then
            if [ -f "$TEMP_DOWNLOAD_DIR/$traefik_binary_name" ]; then
                sudo mv "$TEMP_DOWNLOAD_DIR/$traefik_binary_name" "$TRAEFIK_INSTALL_DIR/traefik"
                sudo chmod +x "$TRAEFIK_INSTALL_DIR/traefik"
                log_info "Traefik 二进制文件已安装到 $TRAEFIK_INSTALL_DIR/traefik"
            else
                log_error "在解压的包中未找到 '$traefik_binary_name'。"
            fi
        else
            log_error "解压 Traefik 包失败。"
        fi
    else
        log_error "下载 Traefik 失败。请检查版本号和网络连接。"
    fi
    rm -rf "$TEMP_DOWNLOAD_DIR"
}


# --- 主逻辑 ---
check_root

# 0. (可选) 创建用户
# create_user_if_not_exists "$SERVICE_USER"

log_info "开始部署 Traefik (从 GitHub 源)..."

# 1. 创建安装目录和配置目录
log_info "创建目录..."
sudo mkdir -p "$TRAEFIK_INSTALL_DIR" || log_error "创建目录 '$TRAEFIK_INSTALL_DIR' 失败。"
sudo mkdir -p "$CONFIG_DIR/conf.d" || log_error "创建目录 '$CONFIG_DIR/conf.d' 失败。"
sudo mkdir -p "$CONFIG_DIR/plugins-local/src/$PLUGIN_DIR_NAME" || log_error "创建插件目录失败。"

# 2. 下载并安装 Traefik 二进制文件
download_traefik "$TRAEFIK_VERSION"

# 3. 复制配置文件和插件 (从脚本同级目录或指定源)
if [ ! -d "$CONFIG_SOURCE_DIR" ] || [ ! -d "$PLUGINS_SOURCE_DIR" ]; then
    log_error "未找到配置文件源目录 ($CONFIG_SOURCE_DIR) 或插件源目录 ($PLUGINS_SOURCE_DIR)。"
fi

log_info "复制静态配置文件从 $CONFIG_SOURCE_DIR/traefik.yml..."
sudo cp "$CONFIG_SOURCE_DIR/traefik.yml" "$CONFIG_DIR/traefik.yml" || log_error "复制 traefik.yml 失败。"

log_info "复制动态配置文件从 $CONFIG_SOURCE_DIR/conf.d/ ..."
sudo cp "$CONFIG_SOURCE_DIR/conf.d/"* "$CONFIG_DIR/conf.d/" || log_error "复制动态配置文件失败。"

log_info "复制插件文件从 $PLUGINS_SOURCE_DIR/src/$PLUGIN_DIR_NAME/ ..."
sudo cp -r "$PLUGINS_SOURCE_DIR/src/$PLUGIN_DIR_NAME/"* "$CONFIG_DIR/plugins-local/src/$PLUGIN_DIR_NAME/" || log_error "复制插件文件失败。"


# 4. (可选) 设置文件权限 (同前)

# 5. 创建 systemd 服务文件 (同前)
SYSTEMD_SERVICE_FILE="/etc/systemd/system/traefik.service"
log_info "创建 systemd 服务文件 '$SYSTEMD_SERVICE_FILE'..."
# ... (systemd 文件内容同前一个脚本) ...
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

log_info "重载 systemd 并启用/启动 Traefik 服务..."
sudo systemctl daemon-reload
sudo systemctl enable traefik.service
sudo systemctl restart traefik.service

log_info "检查 Traefik 服务状态："
sudo systemctl status traefik.service --no-pager -l

log_info "部署完成！"
echo "请检查 Traefik 日志 (journalctl -u traefik -f) 和 Dashboard (如果已配置)。"