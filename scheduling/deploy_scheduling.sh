#!/bin/bash
set -e

# Load configuration
CONFIG_FILE="../setup.conf"
if [ -f "$CONFIG_FILE" ]; then
    echo "Loading configuration from $CONFIG_FILE"
    source "$CONFIG_FILE"
else
    echo "Error: Configuration file $CONFIG_FILE not found!"
    exit 1
fi

# --- Set Default Values (if not provided in config file) ---
# MySQL Configuration Defaults
if [ -z "$MYSQL_ROOT_PASSWORD" ]; then
    echo "Warning: MYSQL_ROOT_PASSWORD is not set in $CONFIG_FILE. Using a default (unsafe) or prompting might be needed."
    MYSQL_ROOT_PASSWORD="unsafe_default_root_password_CHANGE_ME" 
fi
MYSQL_DB_NAME="${MYSQL_DB_NAME:-"myapp_db"}"
MYSQL_DB_USER="${MYSQL_DB_USER:-"myapp_user"}"
if [ -z "$MYSQL_DB_PASSWORD" ]; then
    echo "Warning: MYSQL_DB_PASSWORD is not set in $CONFIG_FILE. Using a default (unsafe) or prompting might be needed."
    MYSQL_DB_PASSWORD="unsafe_default_app_password_CHANGE_ME" 
fi
MYSQL_SQL_SCRIPT_PATH="${MYSQL_SQL_SCRIPT_PATH:-"/assets/create_tables.sql"}"

MYSQL_APT_CONFIG_PKG_URL="${MYSQL_APT_CONFIG_PKG_URL:-"https://dev.mysql.com/get/mysql-apt-config_0.8.22-1_all.deb"}"
MYSQL_YUM_REPO_PKG_URL="${MYSQL_YUM_REPO_PKG_URL:-"https://dev.mysql.com/get/mysql80-community-release-el9-1.noarch.rpm"}"
# --- End of Default Values ---

# MySQL configuration parameters are now from $CONFIG_FILE
# DB_NAME, DB_USER, DB_PASSWORD are used for the application user.
# MYSQL_ROOT_PASSWORD is for the MySQL root user.

install_mysql() {
    echo "Starting MySQL installation..."

    if command -v mysql >/dev/null 2>&1; then
        MYSQL_VERSION=$(mysql --version | awk '{print $3}')
        echo "MySQL is already installed (Version: $MYSQL_VERSION)"
    else
        if [ -f /etc/os-release ]; then
            . /etc/os-release
            OS=$NAME
            OS_VERSION=$VERSION_ID
        else
            OS=$(uname -s)
            OS_VERSION=""
        fi
        echo "Detected OS: $OS $OS_VERSION"

        if [[ "$OS" == *"Ubuntu"* ]] || [[ "$OS" == *"Debian"* ]]; then
            echo "Installing MySQL on Debian/Ubuntu..."
            sudo apt-get update
            sudo apt-get install -y wget
            MYSQL_APT_PKG_FILENAME=$(basename "$MYSQL_APT_CONFIG_PKG_URL")
            wget "$MYSQL_APT_CONFIG_PKG_URL"
            sudo dpkg -i "$MYSQL_APT_PKG_FILENAME" # You might need to handle interactive prompts here or pre-configure selections
            sudo apt-get update
            rm "$MYSQL_APT_PKG_FILENAME"

            echo "mysql-community-server mysql-community-server/root-pass password $MYSQL_ROOT_PASSWORD" | sudo debconf-set-selections
            echo "mysql-community-server mysql-community-server/re-root-pass password $MYSQL_ROOT_PASSWORD" | sudo debconf-set-selections
            sudo DEBIAN_FRONTEND=noninteractive apt-get install -y mysql-server

            sudo systemctl start mysql
            sudo systemctl enable mysql
        elif [[ "$OS" == *"CentOS"* ]] || [[ "$OS" == *"Red Hat"* ]]; then
            echo "Installing MySQL on CentOS/RHEL..."
            sudo yum install -y "$MYSQL_YUM_REPO_PKG_URL"
            if [[ "$OS_VERSION" == "8" ]] || [[ "$OS_VERSION" == "9" ]] || [[ "$OS" == *"Rocky Linux"* ]] || [[ "$OS" == *"AlmaLinux"* ]]; then
                 sudo dnf module disable -y mysql # For RHEL 8/9 and derivatives
            fi
            sudo yum install -y mysql-community-server

            sudo systemctl start mysqld
            sudo systemctl enable mysqld

            # For fresh installs on RHEL-based systems, MySQL generates a temporary root password.
            # We need to change it.
            echo "Attempting to set MySQL root password..."
            # Wait a bit for mysqld.log to be populated
            sleep 5
            if sudo grep -q 'temporary password' /var/log/mysqld.log; then
                TEMP_PASSWORD=$(sudo grep 'temporary password' /var/log/mysqld.log | awk '{print $NF}' | tail -n 1)
                if [ -n "$TEMP_PASSWORD" ]; then
                    echo "Temporary root password found. Changing it now."
                    mysql --connect-expired-password -uroot -p"$TEMP_PASSWORD" <<EOF
ALTER USER 'root'@'localhost' IDENTIFIED BY '$MYSQL_ROOT_PASSWORD';
FLUSH PRIVILEGES;
EOF
                    echo "MySQL root password changed."
                else
                    echo "Could not extract temporary password. Manual intervention might be needed or password already set."
                fi
            else
                echo "No temporary password found in logs. Assuming root password is '$MYSQL_ROOT_PASSWORD' or already set."
                # Attempt to login with the configured root password to verify, or just proceed.
                # This part can be tricky if the password was set by other means.
            fi
        else
            echo "Unsupported OS for automatic MySQL installation."
            exit 1
        fi
        echo "MySQL installation completed successfully"
    fi

    echo "Configuring MySQL database and user..."
    if ! mysql -uroot -p"$MYSQL_ROOT_PASSWORD" -e "USE $MYSQL_DB_NAME" 2>/dev/null; then
        echo "Creating database $MYSQL_DB_NAME..."
        mysql -uroot -p"$MYSQL_ROOT_PASSWORD" <<EOF
CREATE DATABASE IF NOT EXISTS $MYSQL_DB_NAME CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
EOF
    else
        echo "Database $MYSQL_DB_NAME already exists"
    fi

    USER_EXISTS=$(mysql -uroot -p"$MYSQL_ROOT_PASSWORD" -sN -e "SELECT COUNT(*) FROM mysql.user WHERE user='$MYSQL_DB_USER'")
    if [ "$USER_EXISTS" -eq 0 ]; then
        echo "Creating MySQL user $MYSQL_DB_USER..."
        mysql -uroot -p"$MYSQL_ROOT_PASSWORD" <<EOF
CREATE USER '$MYSQL_DB_USER'@'%' IDENTIFIED BY '$MYSQL_DB_PASSWORD';
GRANT ALL PRIVILEGES ON $MYSQL_DB_NAME.* TO '$MYSQL_DB_USER'@'%';
FLUSH PRIVILEGES;
EOF
    else
        echo "User $MYSQL_DB_USER already exists. Ensuring privileges..."
        mysql -uroot -p"$MYSQL_ROOT_PASSWORD" <<EOF
GRANT ALL PRIVILEGES ON $MYSQL_DB_NAME.* TO '$MYSQL_DB_USER'@'%';
ALTER USER '$MYSQL_DB_USER'@'%' IDENTIFIED BY '$MYSQL_DB_PASSWORD';
FLUSH PRIVILEGES;
EOF
    fi

    echo "Securing MySQL installation (basic hardening)..."
    mysql -uroot -p"$MYSQL_ROOT_PASSWORD" <<EOF
DELETE FROM mysql.user WHERE User='';
DELETE FROM mysql.user WHERE User='root' AND Host NOT IN ('localhost', '127.0.0.1', '::1');
DROP DATABASE IF EXISTS test;
DELETE FROM mysql.db WHERE Db='test' OR Db='test\\_%';
FLUSH PRIVILEGES;
EOF
    echo "MySQL configuration completed successfully"
}

create_tables() {
    echo "Starting table creation in database $MYSQL_DB_NAME..."
    if [ ! -f "$MYSQL_SQL_SCRIPT_PATH" ]; then
        echo "Error: SQL script file $MYSQL_SQL_SCRIPT_PATH not found!"
        exit 1
    fi

    echo "Executing SQL script $MYSQL_SQL_SCRIPT_PATH..."
    mysql -u"$MYSQL_DB_USER" -p"$MYSQL_DB_PASSWORD" "$MYSQL_DB_NAME" < "$MYSQL_SQL_SCRIPT_PATH"
    if [ $? -eq 0 ]; then
        echo "SQL script executed successfully!"
        echo "Verifying table creation..."
        mysql -u"$MYSQL_DB_USER" -p"$MYSQL_DB_PASSWORD" "$MYSQL_DB_NAME" -e "SHOW TABLES;"
    else
        echo "Error: Failed to execute SQL script!"
        exit 1
    fi
}

echo "=== Starting database deployment ==="
install_mysql

echo "Verifying MySQL connection with application user..."
if ! mysql -u"$MYSQL_DB_USER" -p"$MYSQL_DB_PASSWORD" -e "SHOW DATABASES LIKE '$MYSQL_DB_NAME';" &> /dev/null; then
    echo "Error: MySQL connection failed for user $MYSQL_DB_USER or database $MYSQL_DB_NAME not accessible!"
    # Check if DB exists with root, then if user can access it
    if mysql -uroot -p"$MYSQL_ROOT_PASSWORD" -e "SHOW DATABASES LIKE '$MYSQL_DB_NAME';" | grep "$MYSQL_DB_NAME"; then
        echo "Database $MYSQL_DB_NAME exists, but user $MYSQL_DB_USER might lack privileges or have wrong password."
    else
        echo "Database $MYSQL_DB_NAME might not exist."
    fi
    exit 1
else
    echo "MySQL connection successful for user $MYSQL_DB_USER on database $MYSQL_DB_NAME."
fi

# Database existence check already handled within install_mysql, but can be re-verified
DB_EXISTS=$(mysql -u"$MYSQL_DB_USER" -p"$MYSQL_DB_PASSWORD" -e "SHOW DATABASES LIKE '$MYSQL_DB_NAME'" | grep "$MYSQL_DB_NAME")
if [ -z "$DB_EXISTS" ]; then
    echo "Database $MYSQL_DB_NAME does not exist or not accessible by $MYSQL_DB_USER. This should have been created in install_mysql."
    # Attempt to create it again, though this indicates a prior issue.
    echo "Attempting to create database $MYSQL_DB_NAME again with root..."
    mysql -uroot -p"$MYSQL_ROOT_PASSWORD" -e "CREATE DATABASE IF NOT EXISTS $MYSQL_DB_NAME CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;"
    mysql -uroot -p"$MYSQL_ROOT_PASSWORD" -e "GRANT ALL PRIVILEGES ON $MYSQL_DB_NAME.* TO '$MYSQL_DB_USER'@'%';"
    mysql -uroot -p"$MYSQL_ROOT_PASSWORD" -e "FLUSH PRIVILEGES;"
    echo "Database $MYSQL_DB_NAME creation/permission grant attempted."
else
    echo "Database $MYSQL_DB_NAME already exists and is accessible."
fi

create_tables
echo "=== Database deployment completed successfully ==="
