#!/bin/bash
set -e

echo "=== Uninstalling env ==="

# Uninstall Go
if [ -d "/usr/local/go" ]; then
    echo "Removing Go..."
    sudo rm -rf /usr/local/go
    sed -i '/\/usr\/local\/go\/bin/d' ~/.profile
    sed -i '/export GOPATH/d' ~/.profile
    rm -rf ~/go
fi

# Uninstall etcd
if systemctl is-active --quiet etcd.service; then
    sudo systemctl stop etcd.service
    sudo systemctl disable etcd.service
fi
sudo rm -rf /usr/local/etcd
sudo rm -f /usr/local/bin/etcd*
sudo rm -rf /var/lib/etcd
sudo rm -f /etc/systemd/system/etcd.service
sudo systemctl daemon-reload

echo "env have been completely uninstalled."
