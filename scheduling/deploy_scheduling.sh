#!/bin/bash
set -e

# MySQL configuration parameters (modify as needed)
DB_NAME="myapp_db"
DB_USER="myapp_user"
DB_PASSWORD="StrongPassword123!"

install_mysql() {
    echo "Starting MySQL installation..."
    
    # 1. Check if MySQL is already installed
    if command -v mysql >/dev/null 2>&1; then
        MYSQL_VERSION=$(mysql --version | awk '{print $3}')
        echo "MySQL is already installed (Version: $MYSQL_VERSION)"
        # Continue with configuration even if MySQL is already installed
    else
        # 2. Install MySQL based on OS
        if [ -f /etc/os-release ]; then
            . /etc/os-release
            OS=$NAME
            OS_VERSION=$VERSION_ID
        else
            OS=$(uname -s)
            OS_VERSION=""
        fi

        echo "Detected OS: $OS $OS_VERSION"
        
        # Ubuntu/Debian installation
        if [[ "$OS" == *"Ubuntu"* ]] || [[ "$OS" == *"Debian"* ]]; then
            echo "Installing MySQL on Debian/Ubuntu..."
            
            # Add MySQL APT repository
            sudo apt-get update
            sudo apt-get install -y wget
            wget https://dev.mysql.com/get/mysql-apt-config_0.8.22-1_all.deb
            sudo dpkg -i mysql-apt-config_0.8.22-1_all.deb
            sudo apt-get update
            
            # Install MySQL Server with automatic root password setup
            echo "mysql-community-server mysql-community-server/root-pass password $DB_PASSWORD" | sudo debconf-set-selections
            echo "mysql-community-server mysql-community-server/re-root-pass password $DB_PASSWORD" | sudo debconf-set-selections
            sudo DEBIAN_FRONTEND=noninteractive apt-get install -y mysql-server
            
            # Start service
            sudo systemctl start mysql
            sudo systemctl enable mysql
            
        # CentOS/RHEL installation
        elif [[ "$OS" == *"CentOS"* ]] || [[ "$OS" == *"Red Hat"* ]]; then
            echo "Installing MySQL on CentOS/RHEL..."
            
            # Add MySQL Yum repository
            sudo yum install -y https://dev.mysql.com/get/mysql80-community-release-el9-1.noarch.rpm
            
            # Disable default MySQL module (important for CentOS 8/9)
            sudo dnf module disable -y mysql
            
            # Install MySQL Server
            sudo yum install -y mysql-community-server
            
            # Start service
            sudo systemctl start mysqld
            sudo systemctl enable mysqld
            
            # Get temporary password (only for fresh install)
            if sudo grep -q 'temporary password' /var/log/mysqld.log; then
                TEMP_PASSWORD=$(sudo grep 'temporary password' /var/log/mysqld.log | awk '{print $NF}')
                # Security configuration
                mysql --connect-expired-password -uroot -p"$TEMP_PASSWORD" <<EOF
ALTER USER 'root'@'localhost' IDENTIFIED BY '$DB_PASSWORD';
FLUSH PRIVILEGES;
EOF
            else
                # If no temporary password found, assume password was already set
                echo "No temporary password found, assuming root password is already set"
            fi
        else
            echo "Unsupported OS for automatic MySQL installation."
            exit 1
        fi
        
        echo "MySQL installation completed successfully"
    fi

    # 3. Create application database and user
    echo "Configuring MySQL database and user..."
    
    # Check if database exists
    if ! mysql -uroot -p"$DB_PASSWORD" -e "USE $DB_NAME" 2>/dev/null; then
        echo "Creating database $DB_NAME..."
        mysql -uroot -p"$DB_PASSWORD" <<EOF
CREATE DATABASE $DB_NAME CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
EOF
    else
        echo "Database $DB_NAME already exists"
    fi

    # Check if user exists
    USER_EXISTS=$(mysql -uroot -p"$DB_PASSWORD" -sN -e "SELECT COUNT(*) FROM mysql.user WHERE user='$DB_USER'")
    
    if [ "$USER_EXISTS" -eq 0 ]; then
        echo "Creating MySQL user $DB_USER..."
        mysql -uroot -p"$DB_PASSWORD" <<EOF
CREATE USER '$DB_USER'@'%' IDENTIFIED BY '$DB_PASSWORD';
GRANT ALL PRIVILEGES ON $DB_NAME.* TO '$DB_USER'@'%';
FLUSH PRIVILEGES;
EOF
    else
        echo "User $DB_USER already exists"
    fi

    # 4. Security hardening
    echo "Securing MySQL installation..."
    mysql -uroot -p"$DB_PASSWORD" <<EOF
DELETE FROM mysql.user WHERE User='';
DELETE FROM mysql.user WHERE User='root' AND Host NOT IN ('localhost', '127.0.0.1', '::1');
DROP DATABASE IF EXISTS test;
FLUSH PRIVILEGES;
EOF

    echo "MySQL configuration completed successfully"
}

# Execute installation
install_mysql

# Verify installation
echo "Verifying MySQL installation..."
mysql -u"$DB_USER" -p"$DB_PASSWORD" -e "SHOW DATABASES;"
mysql --version
