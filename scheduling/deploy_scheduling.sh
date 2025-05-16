#!/bin/bash
set -e

echo "=== Starting Scheduling Project Deployment ==="

# Repository information
REPO_OWNER="Bootes2022"
REPO_NAME="Arcturus"
BRANCH="main" 
ARCHIVE_FORMAT="tar.gz"  #  "zip" or "tar.gz"
ARCHIVE_URL="https://github.com/${REPO_OWNER}/${REPO_NAME}/archive/refs/heads/${BRANCH}.${ARCHIVE_FORMAT}"

DEPLOY_DIR="/opt/scheduling"
SCHEDULING_DIR="$DEPLOY_DIR/scheduling"

# Database configuration
DB_NAME="scheduling"
DB_USER="scheduling_user"
DB_PASSWORD="Password" 

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

# Install MySQL
install_mysql() {
    echo "Setting up MySQL environment..."
    
    # Check if MySQL is already installed
    if command -v mysql >/dev/null 2>&1; then
        echo "MySQL is already installed"
    else
        # Install MySQL based on operating system
        if [ -f /etc/os-release ]; then
            . /etc/os-release
            OS=$NAME
        else
            OS=$(uname -s)
        fi
        
        if [[ "$OS" == *"Ubuntu"* ]] || [[ "$OS" == *"Debian"* ]]; then
            echo "Installing MySQL on Debian/Ubuntu..."
            sudo apt-get update
            sudo apt-get install -y mysql-server
            sudo systemctl start mysql
            sudo systemctl enable mysql
        elif [[ "$OS" == *"CentOS"* ]] || [[ "$OS" == *"Red Hat"* ]]; then
            echo "Installing MySQL on CentOS/RHEL..."
            sudo yum install -y mysql-server
            sudo systemctl start mysqld
            sudo systemctl enable mysqld
        else
            echo "Unsupported OS for automatic MySQL installation. Please install MySQL manually."
            exit 1
        fi
        
        echo "MySQL installation completed"
    fi
    
    # Create database and user for the application
    echo "Creating database and user for scheduling application..."
    
    # Check if the database already exists
    DB_EXISTS=$(sudo mysql -e "SHOW DATABASES LIKE '$DB_NAME';" | grep -c $DB_NAME)
    
    if [ "$DB_EXISTS" -eq 0 ]; then
        echo "Creating database $DB_NAME..."
        sudo mysql -e "CREATE DATABASE $DB_NAME CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;"
        
        # Check if the user already exists
        USER_EXISTS=$(sudo mysql -e "SELECT user FROM mysql.user WHERE user='$DB_USER';" | grep -c $DB_USER)
        
        if [ "$USER_EXISTS" -eq 0 ]; then
            echo "Creating MySQL user $DB_USER..."
            sudo mysql -e "CREATE USER '$DB_USER'@'localhost' IDENTIFIED BY '$DB_PASSWORD';"
        fi
        
        # Grant privileges
        echo "Granting privileges to $DB_USER..."
        sudo mysql -e "GRANT ALL PRIVILEGES ON $DB_NAME.* TO '$DB_USER'@'localhost';"
        sudo mysql -e "FLUSH PRIVILEGES;"
    else
        echo "Database $DB_NAME already exists"
    fi
}

# Deploy Scheduling service
deploy_scheduling() {
    echo "Preparing deployment directory..."
    
    # Create deployment directory
    if [ ! -d "$DEPLOY_DIR" ]; then
        sudo mkdir -p $DEPLOY_DIR
        sudo chown $(whoami) $DEPLOY_DIR
    fi
    
    if [ -d "$SCHEDULING_DIR" ]; then
        echo "Deployment directory already exists, removing old version..."
        sudo rm -rf $SCHEDULING_DIR
    fi
    
    echo "Downloading project archive from ${ARCHIVE_URL}..."
    ARCHIVE_FILE="/tmp/${REPO_NAME}-${BRANCH}.${ARCHIVE_FORMAT}"
    
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
    
    wget -O $ARCHIVE_FILE $ARCHIVE_URL
    
    if [ ! -f "$ARCHIVE_FILE" ]; then
        echo "Failed to download archive. Please check the URL and try again."
        exit 1
    fi
    
    echo "Extracting archive..."
    mkdir -p $DEPLOY_DIR
    
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
    
    EXTRACTED_DIR="$DEPLOY_DIR/${REPO_NAME}-${BRANCH}"
    
    if [ ! -d "$EXTRACTED_DIR" ]; then
        echo "Failed to extract the archive. The expected directory does not exist."
        exit 1
    fi
    
    if [ -d "$EXTRACTED_DIR/scheduling" ]; then
        echo "Found scheduling directory in extracted archive"
        sudo mv "$EXTRACTED_DIR/scheduling" "$SCHEDULING_DIR"
    else
        echo "Scheduling directory not found in expected location. "
        echo "The extracted repository contains the following structure:"
        ls -la "$EXTRACTED_DIR"
    
        sudo mv "$EXTRACTED_DIR" "$SCHEDULING_DIR"
    fi
    
    rm $ARCHIVE_FILE
    
    if [ ! -d "$SCHEDULING_DIR" ] || [ -z "$(ls -A $SCHEDULING_DIR)" ]; then
        echo "Error: Scheduling directory not found or empty after extraction"
        exit 1
    fi
    
    cd $SCHEDULING_DIR
    
    echo "Building scheduling service..."
    
    go mod tidy
    go build -o scheduling_service .
    
    if [ ! -f "./scheduling_service" ]; then
        echo "Build failed. Executable not found."
        exit 1
    fi
    
    if [ ! -f "./config.toml" ]; then
        echo "Creating default configuration file..."
        cat > ./config.toml << EOF
# Database Configuration
[database]
driver = "mysql"
dsn = "$DB_USER:$DB_PASSWORD@tcp(localhost:3306)/$DB_NAME?charset=utf8mb4&parseTime=True&loc=Local"

# Server Configuration
[server]
port = 8080
# Add other configuration parameters as needed
EOF
    else
        echo "Configuration file already exists. Using existing configuration."
    fi
    
    echo "Checking for database schema initialization..."
    if [ -f "./db/schema.sql" ]; then
        echo "Initializing database schema..."
        mysql -u$DB_USER -p$DB_PASSWORD $DB_NAME < ./db/schema.sql
    else
        echo "No schema.sql found. Database initialization will be handled by the application if needed."
    fi
    
    echo "Setting up systemd service..."
    SERVICE_FILE="/etc/systemd/system/scheduling.service"
    
    sudo tee $SERVICE_FILE > /dev/null << EOF
[Unit]
Description=Scheduling Service
After=network.target mysql.service

[Service]
Type=simple
User=$(whoami)
WorkingDirectory=$SCHEDULING_DIR
ExecStart=$SCHEDULING_DIR/scheduling_service
Restart=on-failure
RestartSec=10
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target
EOF
    
    # run
    sudo systemctl daemon-reload
    sudo systemctl enable scheduling.service
    sudo systemctl restart scheduling.service
    
    # check
    echo "Service deployment completed. Current status:"
    sudo systemctl status scheduling.service --no-pager
}

echo "Starting scheduling deployment process..."
install_go
install_mysql
deploy_scheduling
echo "=== Scheduling deployment completed ==="
