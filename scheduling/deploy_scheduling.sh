#!/bin/bash
set -e # Exit immediately if a command exits with a non-zero status

# ==============================================================================
#  Determine script directory and root project directory
#  THIS MUST BE AT THE VERY BEGINNING OF THE SCRIPT, BEFORE ANY 'cd' COMMANDS
# ==============================================================================
SCRIPT_DIR_REALPATH=$(realpath "${BASH_SOURCE[0]}") # Get absolute real path of the script
SCRIPT_DIR="$(dirname "$SCRIPT_DIR_REALPATH")"     # Get the directory of the script

# Assuming this script is in Arturus/scheduling/
# Go one level up from SCRIPT_DIR to get ROOT_PROJECT_DIR
ROOT_PROJECT_DIR="$(realpath "$SCRIPT_DIR/..")"
# ==============================================================================

# Initial DEBUG information
echo "DEBUG (Initial): Current PWD at script start: $(pwd)"
echo "DEBUG (Initial): SCRIPT_DIR_REALPATH: $SCRIPT_DIR_REALPATH"
echo "DEBUG (Initial): SCRIPT_DIR: $SCRIPT_DIR"
echo "DEBUG (Initial): ROOT_PROJECT_DIR: $ROOT_PROJECT_DIR"

# --- Load Configuration ---
CONFIG_FILE_NAME="setup.conf"
CONFIG_FILE_PATH="$ROOT_PROJECT_DIR/$CONFIG_FILE_NAME"

echo "DEBUG: Attempting to load config from: $CONFIG_FILE_PATH"

if [ -f "$CONFIG_FILE_PATH" ]; then
    echo "Loading configuration from $CONFIG_FILE_PATH"
    source "$CONFIG_FILE_PATH"
else
    echo "Error: Configuration file $CONFIG_FILE_PATH not found!"
    echo "Please ensure '$CONFIG_FILE_NAME' exists in '$ROOT_PROJECT_DIR'."
    exit 1
fi

# --- Set Default Values (if not provided in config file) ---
# MySQL Configuration Defaults
if [ -z "$MYSQL_ROOT_PASSWORD" ]; then
    echo "Warning: MYSQL_ROOT_PASSWORD is not set in $CONFIG_FILE. This password will be used for the app user, not directly for root login if auth_socket is used."
    MYSQL_ROOT_PASSWORD="unsafe_default_root_password_CHANGE_ME"
fi
MYSQL_DB_NAME="${MYSQL_DB_NAME:-"myapp_db"}"
MYSQL_DB_USER="${MYSQL_DB_USER:-"myapp_user"}"
if [ -z "$MYSQL_DB_PASSWORD" ]; then
    echo "Warning: MYSQL_DB_PASSWORD is not set in $CONFIG_FILE. Using a default (unsafe)."
    MYSQL_DB_PASSWORD="unsafe_default_app_password_CHANGE_ME"
fi
# MYSQL_SQL_SCRIPT_PATH should be relative to the project root (Arturus) in setup.conf
# e.g., MYSQL_SQL_SCRIPT_PATH="scheduling/assets/create_tables.sql"
DEFAULT_SQL_SCRIPT_PATH="scheduling/assets/create_tables.sql" # Default if not in config
MYSQL_SQL_SCRIPT_PATH_FROM_CONFIG="${MYSQL_SQL_SCRIPT_PATH}" # Store original config value for clarity
MYSQL_SQL_SCRIPT_PATH="${MYSQL_SQL_SCRIPT_PATH:-"$DEFAULT_SQL_SCRIPT_PATH"}"
# Construct the absolute path for the SQL script
ACTUAL_SQL_SCRIPT_PATH="$ROOT_PROJECT_DIR/$MYSQL_SQL_SCRIPT_PATH"

echo "DEBUG: MYSQL_ROOT_PASSWORD (used for app user if root uses auth_socket): [REDACTED]" # Avoid printing actual password
echo "DEBUG: MYSQL_DB_NAME: $MYSQL_DB_NAME"
echo "DEBUG: MYSQL_DB_USER: $MYSQL_DB_USER"
echo "DEBUG: MYSQL_DB_PASSWORD (for app user): [REDACTED]"
echo "DEBUG: MYSQL_SQL_SCRIPT_PATH (from config or default): $MYSQL_SQL_SCRIPT_PATH_FROM_CONFIG"
echo "DEBUG: ACTUAL_SQL_SCRIPT_PATH (to be executed): $ACTUAL_SQL_SCRIPT_PATH"

# --- End of Default Values ---

install_mysql() {
    echo "Starting MySQL installation..."

    if command -v mysql >/dev/null 2>&1; then
        MYSQL_VERSION_OUTPUT=$(mysql --version)
        MYSQL_VERSION=$(echo "$MYSQL_VERSION_OUTPUT" | awk '{print $3}' | sed 's/,//') # Get version like 8.0.42
        echo "MySQL is already installed: $MYSQL_VERSION_OUTPUT"
        # Optionally, add logic here to check if the installed version is sufficient
        # or if configuration (DB/user creation) still needs to be run.
    else
        if [ -f /etc/os-release ]; then
            . /etc/os-release
            OS_NAME=$NAME # Use OS_NAME to avoid conflict with previous OS variable
            OS_VERSION_ID=$VERSION_ID # Use OS_VERSION_ID
        else
            OS_NAME=$(uname -s)
            OS_VERSION_ID=""
        fi
        echo "Detected OS: $OS_NAME $OS_VERSION_ID"

        if [[ "$OS_NAME" == *"Ubuntu"* ]] || [[ "$OS_NAME" == *"Debian"* ]]; then
            echo "Installing MySQL from $OS_NAME's official repositories..."
            sudo apt-get update
            # libaio1 is usually a dependency of mysql-server and will be installed automatically.
            # sudo apt-get install -y libaio1 # If you want to be explicit

            # For Ubuntu's mysql-server, root password setting via debconf might not work as expected
            # due to auth_socket. We will handle root access using 'sudo mysql -u root'.
            # The MYSQL_ROOT_PASSWORD from config will NOT be set for the MySQL root user here.
            # It's better to use it for the application user or for RHEL-based systems.
            echo "Note: MYSQL_ROOT_PASSWORD from config is not directly applied to MySQL root on Ubuntu due to auth_socket."
            echo "Root access will be via 'sudo mysql -u root'."

            sudo DEBIAN_FRONTEND=noninteractive apt-get install -y mysql-server

            echo "Ensuring MySQL service is started and enabled..."
            if sudo systemctl list-unit-files | grep -q mysqld.service; then
                MYSQL_SERVICE_NAME="mysqld"
            else
                MYSQL_SERVICE_NAME="mysql"
            fi
            sudo systemctl start "$MYSQL_SERVICE_NAME"
            sudo systemctl enable "$MYSQL_SERVICE_NAME"
            sudo systemctl status "$MYSQL_SERVICE_NAME" --no-pager
            echo "MySQL installation from $OS_NAME repositories completed."

        elif [[ "$OS_NAME" == *"CentOS"* ]] || [[ "$OS_NAME" == *"Red Hat"* ]]; then
            echo "Installing MySQL on CentOS/RHEL..."
            # This part remains for RHEL-based systems, using MySQL official repo
            sudo yum install -y wget
            MYSQL_YUM_REPO_PKG_URL_DEFAULT="https://dev.mysql.com/get/mysql80-community-release-el$(rpm -E %{rhel})-1.noarch.rpm" # Try to get RHEL version
            ACTUAL_MYSQL_YUM_REPO_PKG_URL="${MYSQL_YUM_REPO_PKG_URL:-$MYSQL_YUM_REPO_PKG_URL_DEFAULT}"

            sudo yum install -y "$ACTUAL_MYSQL_YUM_REPO_PKG_URL"
            if [[ "$OS_VERSION_ID" == "8" ]] || [[ "$OS_VERSION_ID" == "9" ]] || [[ "$OS_NAME" == *"Rocky Linux"* ]] || [[ "$OS_NAME" == *"AlmaLinux"* ]]; then
                 sudo dnf module disable -y mysql # For RHEL 8/9 and derivatives
            fi
            sudo yum install -y mysql-community-server

            sudo systemctl start mysqld
            sudo systemctl enable mysqld

            echo "Attempting to set MySQL root password for RHEL-based system..."
            sleep 5
            if sudo grep -q 'temporary password' /var/log/mysqld.log; then
                TEMP_PASSWORD=$(sudo grep 'temporary password' /var/log/mysqld.log | awk '{print $NF}' | tail -n 1)
                if [ -n "$TEMP_PASSWORD" ]; then
                    echo "Temporary root password found. Changing it now using MYSQL_ROOT_PASSWORD from config."
                    # On RHEL, root user usually has password auth by default after this
                    mysql --connect-expired-password -uroot -p"$TEMP_PASSWORD" <<EOF
ALTER USER 'root'@'localhost' IDENTIFIED BY '$MYSQL_ROOT_PASSWORD';
FLUSH PRIVILEGES;
EOF
                    echo "MySQL root password changed."
                else
                    echo "Could not extract temporary password. Manual intervention might be needed."
                fi
            else
                echo "No temporary password found in logs. Assuming root password is '$MYSQL_ROOT_PASSWORD' or already set."
                # Attempt to ensure the root password is set if possible
                # This might require knowing the current password or specific MySQL versions.
                # For simplicity, we assume if no temp pass, it's either set or needs manual intervention.
            fi
        else
            echo "Unsupported OS ($OS_NAME) for automatic MySQL installation."
            exit 1
        fi
        echo "MySQL server installation process finished."
    fi # End of 'if mysql not installed'

    # Database and User Configuration (Common for both OS types after server is up)
    echo "Configuring MySQL database '$MYSQL_DB_NAME' and user '$MYSQL_DB_USER'..."

    # Determine how to connect as root based on OS
    MYSQL_ROOT_CONNECT_CMD="sudo mysql -u root" # Default for Ubuntu with auth_socket
    if [[ "$OS_NAME" == *"CentOS"* ]] || [[ "$OS_NAME" == *"Red Hat"* ]]; then
        # On RHEL, after setting password, we should be able to connect with it
        MYSQL_ROOT_CONNECT_CMD="mysql -u root -p'$MYSQL_ROOT_PASSWORD'"
    fi
    echo "DEBUG: Using MySQL root connect command: ${MYSQL_ROOT_CONNECT_CMD%%-p*}" # Avoid printing password part

    # Check if database exists
    # The "2>/dev/null" suppresses errors if DB doesn't exist or connection fails temporarily
    if ! $MYSQL_ROOT_CONNECT_CMD -e "USE \`$MYSQL_DB_NAME\`;" 2>/dev/null; then
        echo "Database '$MYSQL_DB_NAME' does not exist or not accessible. Creating it..."
        $MYSQL_ROOT_CONNECT_CMD <<EOF
CREATE DATABASE IF NOT EXISTS \`$MYSQL_DB_NAME\` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
EOF
        echo "Database '$MYSQL_DB_NAME' created."
    else
        echo "Database '$MYSQL_DB_NAME' already exists."
    fi

    # Check if user exists and create/update
    # Note: Using backticks for database name in SQL for safety if it contains special chars
    USER_EXISTS_COUNT=$($MYSQL_ROOT_CONNECT_CMD -sN -e "SELECT COUNT(*) FROM mysql.user WHERE user='$MYSQL_DB_USER' AND host='%';")
    if [ "$USER_EXISTS_COUNT" -eq 0 ]; then
        echo "Creating MySQL user '$MYSQL_DB_USER'..."
        $MYSQL_ROOT_CONNECT_CMD <<EOF
CREATE USER '$MYSQL_DB_USER'@'%' IDENTIFIED WITH mysql_native_password BY '$MYSQL_DB_PASSWORD';
GRANT ALL PRIVILEGES ON \`$MYSQL_DB_NAME\`.* TO '$MYSQL_DB_USER'@'%';
FLUSH PRIVILEGES;
EOF
        echo "User '$MYSQL_DB_USER' created with password authentication."
    else
        echo "User '$MYSQL_DB_USER' already exists. Ensuring privileges and updating password..."
        $MYSQL_ROOT_CONNECT_CMD <<EOF
ALTER USER '$MYSQL_DB_USER'@'%' IDENTIFIED WITH mysql_native_password BY '$MYSQL_DB_PASSWORD';
GRANT ALL PRIVILEGES ON \`$MYSQL_DB_NAME\`.* TO '$MYSQL_DB_USER'@'%';
FLUSH PRIVILEGES;
EOF
        echo "User '$MYSQL_DB_USER' privileges and password updated."
    fi

    # Basic security hardening (optional, adapt as needed)
    # Be cautious with these on an existing system.
    if [[ "$OS_NAME" == *"CentOS"* ]] || [[ "$OS_NAME" == *"Red Hat"* ]]; then
        echo "Securing MySQL installation (basic hardening for RHEL-based)..."
        $MYSQL_ROOT_CONNECT_CMD <<EOF
DELETE FROM mysql.user WHERE User='';
DELETE FROM mysql.user WHERE User='root' AND Host NOT IN ('localhost', '127.0.0.1', '::1');
DROP DATABASE IF EXISTS test;
DELETE FROM mysql.db WHERE Db='test' OR Db='test\\_%';
FLUSH PRIVILEGES;
EOF
    else # Ubuntu/Debian
        echo "Basic security (like removing anonymous users and test DB) is often handled by mysql_secure_installation."
        echo "Consider running 'sudo mysql_secure_installation' manually after this script for more comprehensive security."
    fi
    echo "MySQL configuration completed."
}

create_tables() {
    echo "Starting table creation in database '$MYSQL_DB_NAME' using script '$ACTUAL_SQL_SCRIPT_PATH'..."

    if [ ! -f "$ACTUAL_SQL_SCRIPT_PATH" ]; then
        echo "Error: SQL script file '$ACTUAL_SQL_SCRIPT_PATH' not found!"
        echo "MYSQL_SQL_SCRIPT_PATH (from config/default): '$MYSQL_SQL_SCRIPT_PATH'"
        echo "ROOT_PROJECT_DIR: '$ROOT_PROJECT_DIR'"
        exit 1
    fi

    echo "Executing SQL script '$ACTUAL_SQL_SCRIPT_PATH' as user '$MYSQL_DB_USER'..."
    # Application user should now be able to connect with its password
    if mysql -u"$MYSQL_DB_USER" -p"$MYSQL_DB_PASSWORD" "$MYSQL_DB_NAME" < "$ACTUAL_SQL_SCRIPT_PATH"; then
        echo "SQL script executed successfully!"
        echo "Verifying table creation..."
        mysql -u"$MYSQL_DB_USER" -p"$MYSQL_DB_PASSWORD" "$MYSQL_DB_NAME" -e "SHOW TABLES;"
    else
        echo "Error: Failed to execute SQL script '$ACTUAL_SQL_SCRIPT_PATH'!"
        exit 1
    fi
}

# --- Main Execution Flow ---
echo "=== Starting database deployment ==="

# 1. Install MySQL and configure basic users/DB
install_mysql

# 2. Verify MySQL connection with the application user
echo "Verifying MySQL connection with application user '$MYSQL_DB_USER'..."
# Suppress output for verification, check exit code
if ! mysql -u"$MYSQL_DB_USER" -p"$MYSQL_DB_PASSWORD" -e "USE \`$MYSQL_DB_NAME\`; SELECT 1;" &> /dev/null; then
    echo "Error: MySQL connection failed for user '$MYSQL_DB_USER' on database '$MYSQL_DB_NAME'!"
    echo "Please check MySQL service status, user credentials, and privileges."
    # Further diagnostics (optional)
    echo "Checking root access to the database (this might require sudo password if on Ubuntu)..."
    MYSQL_ROOT_CONNECT_CMD_CHECK="sudo mysql -u root"
    if [[ "$(uname -s)" == *"CentOS"* ]] || [[ "$(uname -s)" == *"Red Hat"* ]]; then
        MYSQL_ROOT_CONNECT_CMD_CHECK="mysql -u root -p'$MYSQL_ROOT_PASSWORD'"
    fi

    if $MYSQL_ROOT_CONNECT_CMD_CHECK -e "SHOW DATABASES LIKE '$MYSQL_DB_NAME';" | grep -q "$MYSQL_DB_NAME"; then
        echo "Database '$MYSQL_DB_NAME' exists and is accessible by MySQL root."
        echo "The issue is likely with '$MYSQL_DB_USER' credentials or privileges on '$MYSQL_DB_NAME'."
    else
        echo "Database '$MYSQL_DB_NAME' might not exist or is not accessible by MySQL root."
    fi
    exit 1
else
    echo "MySQL connection successful for user '$MYSQL_DB_USER' on database '$MYSQL_DB_NAME'."
fi

# Database existence check is implicitly handled by the connection test above.
# If connection works, DB exists and user has access.

# 3. Create tables using the SQL script
create_tables

echo "=== Database deployment completed successfully ==="
