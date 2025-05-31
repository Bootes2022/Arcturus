#!/bin/bash
set -e # Exit immediately if a command exits with a non-zero status

# ==============================================================================
#  Determine script directory and root project directory
# ==============================================================================
SCRIPT_DIR_REALPATH=$(realpath "${BASH_SOURCE[0]}")
SCRIPT_DIR="$(dirname "$SCRIPT_DIR_REALPATH")"
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
if [ -z "$MYSQL_ROOT_PASSWORD" ]; then
    echo "Warning: MYSQL_ROOT_PASSWORD is not set in $CONFIG_FILE. This password will be used for the app user, not directly for root login if auth_socket is used on Debian/Ubuntu."
    MYSQL_ROOT_PASSWORD="unsafe_default_root_password_CHANGE_ME"
fi
MYSQL_DB_NAME="${MYSQL_DB_NAME:-"myapp_db"}"
MYSQL_DB_USER="${MYSQL_DB_USER:-"myapp_user"}"
if [ -z "$MYSQL_DB_PASSWORD" ]; then
    echo "Warning: MYSQL_DB_PASSWORD is not set in $CONFIG_FILE. Using a default (unsafe)."
    MYSQL_DB_PASSWORD="unsafe_default_app_password_CHANGE_ME"
fi
MYSQL_APP_USER_HOST="${MYSQL_APP_USER_HOST:-"localhost"}" # Default to localhost for app user, can be '%'

DEFAULT_SQL_SCRIPT_PATH="scheduling/assets/create_tables.sql"
MYSQL_SQL_SCRIPT_PATH_FROM_CONFIG="${MYSQL_SQL_SCRIPT_PATH}"
MYSQL_SQL_SCRIPT_PATH="${MYSQL_SQL_SCRIPT_PATH:-"$DEFAULT_SQL_SCRIPT_PATH"}"
ACTUAL_SQL_SCRIPT_PATH="$ROOT_PROJECT_DIR/$MYSQL_SQL_SCRIPT_PATH"

echo "DEBUG: MYSQL_ROOT_PASSWORD (used for app user if root uses auth_socket): [REDACTED]"
echo "DEBUG: MYSQL_DB_NAME: $MYSQL_DB_NAME"
echo "DEBUG: MYSQL_DB_USER: $MYSQL_DB_USER"
echo "DEBUG: MYSQL_DB_PASSWORD (for app user): [REDACTED]"
echo "DEBUG: MYSQL_APP_USER_HOST: $MYSQL_APP_USER_HOST"
echo "DEBUG: MYSQL_SQL_SCRIPT_PATH (from config or default): $MYSQL_SQL_SCRIPT_PATH_FROM_CONFIG"
echo "DEBUG: ACTUAL_SQL_SCRIPT_PATH (to be executed): $ACTUAL_SQL_SCRIPT_PATH"
# --- End of Default Values ---

# Function to check if a command exists
command_exists() {
    command -v "$1" >/dev/null 2>&1
}

# Function to detect OS
detect_os() {
    if [ -f /etc/os-release ]; then
        . /etc/os-release
        OS_FAMILY_DETECTED=$ID_LIKE # e.g., "debian" or "rhel fedora"
        OS_NAME_DETECTED=$ID      # e.g., "ubuntu", "almalinux", "centos"
        OS_VERSION_ID_DETECTED=$VERSION_ID
    elif command_exists lsb_release; then
        OS_NAME_DETECTED=$(lsb_release -si | tr '[:upper:]' '[:lower:]')
        OS_VERSION_ID_DETECTED=$(lsb_release -sr)
        if [[ "$OS_NAME_DETECTED" == "ubuntu" ]] || [[ "$OS_NAME_DETECTED" == "debian" ]]; then
            OS_FAMILY_DETECTED="debian"
        elif [[ "$OS_NAME_DETECTED" == "centos" ]] || [[ "$OS_NAME_DETECTED" == "almalinux" ]] || [[ "$OS_NAME_DETECTED" == "rocky" ]] || [[ "$OS_NAME_DETECTED" == "rhel" ]]; then
            OS_FAMILY_DETECTED="rhel"
        fi
    else
        OS_NAME_DETECTED=$(uname -s | tr '[:upper:]' '[:lower:]')
        OS_VERSION_ID_DETECTED=$(uname -r)
        # Basic fallback for family
        if [[ "$OS_NAME_DETECTED" == "linux" ]]; then # Too generic, but a fallback
            if grep -qi "debian" /etc/*release*; then OS_FAMILY_DETECTED="debian"; fi
            if grep -qi "red hat\|centos\|alma\|rocky" /etc/*release*; then OS_FAMILY_DETECTED="rhel"; fi
        fi
    fi
    echo "Detected OS Name: $OS_NAME_DETECTED, Version: $OS_VERSION_ID_DETECTED, Family: $OS_FAMILY_DETECTED"
}


install_mysql() {
    echo "Starting MySQL installation..."
    detect_os # Call OS detection

    if command_exists mysql; then
        MYSQL_VERSION_OUTPUT=$(mysql --version)
        echo "MySQL (or compatible) is already installed: $MYSQL_VERSION_OUTPUT"
    else
        echo "MySQL not found. Proceeding with installation for $OS_NAME_DETECTED..."

        if [[ "$OS_FAMILY_DETECTED" == *"debian"* ]]; then
            echo "Installing MySQL on Debian-based system ($OS_NAME_DETECTED)..."
            sudo apt-get update -qq
            echo "Note: MYSQL_ROOT_PASSWORD from config is not directly applied to MySQL root on Debian/Ubuntu due to auth_socket."
            echo "Root access will be via 'sudo mysql -u root'."
            sudo DEBIAN_FRONTEND=noninteractive apt-get install -y mysql-server
            MYSQL_SERVICE_NAME="mysql" # Usually 'mysql' on Debian/Ubuntu
            if ! sudo systemctl list-unit-files | grep -q "^${MYSQL_SERVICE_NAME}.service"; then
                 # Some systems might use mysqld even if Debian based, though less common for default repo
                if sudo systemctl list-unit-files | grep -q "^mysqld.service"; then
                    MYSQL_SERVICE_NAME="mysqld"
                fi
            fi

        elif [[ "$OS_FAMILY_DETECTED" == *"rhel"* ]] || [[ "$OS_FAMILY_DETECTED" == *"fedora"* ]]; then # Broader check for RHEL-like
            echo "Installing MySQL on RHEL-based system ($OS_NAME_DETECTED)..."
            if ! command_exists wget; then sudo yum install -y wget; fi

            # Determine RHEL major version for repo URL
            RHEL_MAJOR_VERSION=$(echo "$OS_VERSION_ID_DETECTED" | cut -d. -f1)
            if [[ -z "$RHEL_MAJOR_VERSION" ]] && command_exists rpm; then # Fallback if VERSION_ID is not just a number
                RHEL_MAJOR_VERSION=$(rpm -E %{rhel})
            fi
            if [[ -z "$RHEL_MAJOR_VERSION" ]]; then # If still not found, default or error
                echo "Warning: Could not determine RHEL major version. Defaulting to 8 for MySQL repo."
                RHEL_MAJOR_VERSION="8" # Or choose to exit
            fi

            echo "Attempting to remove potentially outdated MySQL YUM repository release packages..."
            # Adding || echo to prevent script exit if packages are not found or removal fails for other non-critical reasons
            sudo yum remove -y "mysql80-community-release-el${RHEL_MAJOR_VERSION}*" || echo "INFO: No specific mysql80-community-release-el${RHEL_MAJOR_VERSION}* package found/removed. This is often normal."
            sudo yum remove -y 'mysql*-community-release*' || echo "INFO: No generic mysql*-community-release* package found/removed. This is often normal."

            MYSQL_YUM_REPO_PKG_URL_DEFAULT="https://dev.mysql.com/get/mysql80-community-release-el${RHEL_MAJOR_VERSION}.rpm" # UPDATED URL
            ACTUAL_MYSQL_YUM_REPO_PKG_URL="${MYSQL_YUM_REPO_PKG_URL:-$MYSQL_YUM_REPO_PKG_URL_DEFAULT}"

            echo "Installing/Updating MySQL YUM repository from: $ACTUAL_MYSQL_YUM_REPO_PKG_URL"
            sudo yum install -y "$ACTUAL_MYSQL_YUM_REPO_PKG_URL"

            # Disable default mysql module on RHEL 8/9 and derivatives if DNF is used
            if command_exists dnf && ( [[ "$OS_NAME_DETECTED" == "almalinux" ]] || [[ "$OS_NAME_DETECTED" == "rocky" ]] || [[ "$OS_NAME_DETECTED" == "centos" && "$RHEL_MAJOR_VERSION" -ge 8 ]] || [[ "$OS_NAME_DETECTED" == "rhel" && "$RHEL_MAJOR_VERSION" -ge 8 ]] ); then
                 echo "Disabling default mysql module for DNF..."
                 sudo dnf module reset -y mysql # Reset first to avoid conflicts
                 sudo dnf module disable -y mysql
            fi
            sudo yum install -y mysql-community-server
            MYSQL_SERVICE_NAME="mysqld" # Usually 'mysqld' for Oracle MySQL on RHEL

        else
            echo "Unsupported OS Family ($OS_FAMILY_DETECTED / $OS_NAME_DETECTED) for automatic MySQL installation."
            exit 1
        fi

        echo "Ensuring MySQL service ($MYSQL_SERVICE_NAME) is started and enabled..."
        sudo systemctl start "$MYSQL_SERVICE_NAME"
        sudo systemctl enable "$MYSQL_SERVICE_NAME"
        sudo systemctl status "$MYSQL_SERVICE_NAME" --no-pager || echo "Warning: systemctl status failed for $MYSQL_SERVICE_NAME"
        echo "MySQL server installation process finished for $OS_NAME_DETECTED."

        # RHEL-specific root password setup
        if [[ "$OS_FAMILY_DETECTED" == *"rhel"* ]] || [[ "$OS_FAMILY_DETECTED" == *"fedora"* ]]; then
            echo "Attempting to set MySQL root password for RHEL-based system..."
            echo "Waiting for MySQL service to be fully ready..."
            # Simple wait, can be improved with a loop checking mysqladmin ping
            for i in {1..10}; do
                if sudo mysqladmin ping -u root --silent &>/dev/null || sudo mysqladmin ping --silent &>/dev/null ; then # Try with and without -u root
                    echo "MySQL service is responsive."
                    break
                fi
                echo "Waiting... ($i/10)"
                sleep 3
            done

            if sudo grep -q 'temporary password' /var/log/mysqld.log; then
                TEMP_PASSWORD=$(sudo grep 'temporary password' /var/log/mysqld.log | awk '{print $NF}' | tail -n 1)
                if [ -n "$TEMP_PASSWORD" ]; then
                    echo "Temporary root password found. Changing it now using MYSQL_ROOT_PASSWORD from config."
                    # Retry logic for password change, as it can sometimes fail if MySQL is not fully ready
                    for attempt in {1..3}; do
                        if mysql --connect-expired-password -uroot -p"$TEMP_PASSWORD" <<EOF
ALTER USER 'root'@'localhost' IDENTIFIED BY '$MYSQL_ROOT_PASSWORD';
FLUSH PRIVILEGES;
EOF
                        then
                            echo "MySQL root password changed successfully on attempt $attempt."
                            break
                        else
                            echo "Attempt $attempt to change MySQL root password failed. Retrying in 5 seconds..."
                            sleep 5
                        fi
                        if [ "$attempt" -eq 3 ]; then
                            echo "Failed to change MySQL root password after multiple attempts. Manual intervention might be needed."
                        fi
                    done
                else
                    echo "Could not extract temporary password. Manual intervention might be needed."
                fi
            else
                echo "No temporary password found in logs. Assuming root password is '$MYSQL_ROOT_PASSWORD' or already set."
                # Attempt to login with configured password to verify
                if ! mysql -u root -p"$MYSQL_ROOT_PASSWORD" -e "SELECT 1;" &>/dev/null; then
                    echo "Warning: Could not login as root with MYSQL_ROOT_PASSWORD. Manual password setup might be required."
                else
                    echo "Successfully logged in as root with MYSQL_ROOT_PASSWORD."
                fi
            fi
        fi
    fi # End of 'if mysql not installed'

    # Database and User Configuration
    echo "Configuring MySQL database '$MYSQL_DB_NAME' and user '$MYSQL_DB_USER'@'$MYSQL_APP_USER_HOST'..."

    # Determine the root connection command based on OS family
    local -a MYSQL_ROOT_EXEC_PARTS # Declare as array
    local MYSQL_ROOT_USES_PASSWORD=false

    if [[ "$OS_FAMILY_DETECTED" == *"rhel"* ]] || [[ "$OS_FAMILY_DETECTED" == *"fedora"* ]]; then
        echo "DEBUG: RHEL-like OS detected. Determining MySQL root connection method."
        # First, try to connect using the password from the configuration file.
        if mysql -u root -p"$MYSQL_ROOT_PASSWORD" -e "SELECT 1;" &>/dev/null; then
            MYSQL_ROOT_EXEC_PARTS=(mysql -u root "-p$MYSQL_ROOT_PASSWORD")
            MYSQL_ROOT_USES_PASSWORD=true
            echo "INFO: RHEL-like: Successfully tested connection for 'root' using MYSQL_ROOT_PASSWORD."
        elif [ "$(id -u)" -eq 0 ] && mysql -u root -e "SELECT 1;" &>/dev/null; then
            MYSQL_ROOT_EXEC_PARTS=(mysql -u root)
            echo "INFO: RHEL-like: Connection with MYSQL_ROOT_PASSWORD failed. Successfully tested 'mysql -u root' (as OS root, likely auth_socket)."
        else
            # BOTH RHEL root connection attempts failed.
            echo "ERROR: Unable to establish MySQL root connection on this RHEL-like system." >&2
            echo "       Attempted to connect using MYSQL_ROOT_PASSWORD from '$CONFIG_FILE_PATH' - FAILED." >&2
            if [ "$(id -u)" -eq 0 ]; then
                echo "       Attempted to connect as 'mysql -u root' (passwordless, as OS root) - FAILED." >&2
                echo "Possible reasons for failure:" >&2
                echo "  1. The MYSQL_ROOT_PASSWORD in '$CONFIG_FILE_PATH' is incorrect for your MySQL 'root'@'localhost' user." >&2
                echo "  2. Your MySQL 'root'@'localhost' user is not configured to allow passwordless login for the OS 'root' user (e.g., via the auth_socket plugin) and requires a password." >&2
                echo "Action: Please verify the MYSQL_ROOT_PASSWORD in '$CONFIG_FILE_PATH' is the correct current password for MySQL's 'root'@'localhost' user." >&2
                echo "        Alternatively, ensure you can log in to MySQL as root from the command line (e.g., by typing 'mysql -u root' if auth_socket is expected, or 'mysql -u root -p' and then entering the correct password)." >&2
            else # Script not run as OS root
                echo "       The script is not running as the OS root user, so passwordless MySQL root login (e.g. auth_socket) could not be fully tested/utilized as a fallback." >&2
                echo "Possible reason for failure:" >&2
                echo "  1. The MYSQL_ROOT_PASSWORD in '$CONFIG_FILE_PATH' is likely incorrect for your MySQL 'root'@'localhost' user." >&2
                echo "Action: Please verify the MYSQL_ROOT_PASSWORD in '$CONFIG_FILE_PATH' is the correct current password for MySQL's 'root'@'localhost' user, or run this script as the OS root user if passwordless MySQL access is intended." >&2
            fi
            exit 1
        fi
    elif [[ "$OS_FAMILY_DETECTED" == *"debian"* ]]; then
        # For Debian-based systems, 'sudo mysql -u root' is typically used due to auth_socket.
        MYSQL_ROOT_EXEC_PARTS=(sudo mysql -u root)
        echo "INFO: Debian-like: Using 'sudo mysql -u root' for MySQL root operations."
    else
        # Fallback for unknown OS families. This is a guess and might need user configuration.
        echo "WARN: Unknown OS family ($OS_FAMILY_DETECTED). Attempting password-based root connection for MySQL."
        MYSQL_ROOT_EXEC_PARTS=(mysql -u root "-p$MYSQL_ROOT_PASSWORD")
        MYSQL_ROOT_USES_PASSWORD=true
    fi

    local printable_connect_cmd_array=("${MYSQL_ROOT_EXEC_PARTS[@]}")
    if $MYSQL_ROOT_USES_PASSWORD; then
        for i in "${!printable_connect_cmd_array[@]}"; do
            if [[ "${printable_connect_cmd_array[i]}" == -p* ]]; then
                printable_connect_cmd_array[i]="-p[REDACTED]"
                break 
            fi
        done
    fi
    echo "DEBUG: Using MySQL root connect command array: ${printable_connect_cmd_array[*]}"

    # Create database
    echo "Checking/Creating database '$MYSQL_DB_NAME'..."
    "${MYSQL_ROOT_EXEC_PARTS[@]}" <<EOF
CREATE DATABASE IF NOT EXISTS \`$MYSQL_DB_NAME\` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
EOF
    echo "Database '$MYSQL_DB_NAME' check/creation complete."

    # Create/Update user
    echo "Checking/Creating user '$MYSQL_DB_USER'@'$MYSQL_APP_USER_HOST'..."
    # Check if user exists. Note: mysql.user structure can vary. This is a common way.
    # Need to be careful with the connect command if it fails.
    USER_EXISTS_SQL="SELECT COUNT(*) FROM mysql.user WHERE user='$MYSQL_DB_USER' AND host='$MYSQL_APP_USER_HOST';"
    USER_EXISTS_COUNT=$("${MYSQL_ROOT_EXEC_PARTS[@]}" -sN -e "$USER_EXISTS_SQL" 2>/dev/null || echo "0") # Default to 0 on error to attempt creation

    if [ "$USER_EXISTS_COUNT" -eq 0 ]; then
        echo "Creating MySQL user '$MYSQL_DB_USER'@'$MYSQL_APP_USER_HOST'..."
        "${MYSQL_ROOT_EXEC_PARTS[@]}" <<EOF
CREATE USER '$MYSQL_DB_USER'@'$MYSQL_APP_USER_HOST' IDENTIFIED WITH mysql_native_password BY '$MYSQL_DB_PASSWORD';
GRANT ALL PRIVILEGES ON \`$MYSQL_DB_NAME\`.* TO '$MYSQL_DB_USER'@'$MYSQL_APP_USER_HOST';
FLUSH PRIVILEGES;
EOF
        echo "User '$MYSQL_DB_USER'@'$MYSQL_APP_USER_HOST' created."
    else
        echo "User '$MYSQL_DB_USER'@'$MYSQL_APP_USER_HOST' already exists. Ensuring privileges and updating password..."
        "${MYSQL_ROOT_EXEC_PARTS[@]}" <<EOF
ALTER USER '$MYSQL_DB_USER'@'$MYSQL_APP_USER_HOST' IDENTIFIED WITH mysql_native_password BY '$MYSQL_DB_PASSWORD';
GRANT ALL PRIVILEGES ON \`$MYSQL_DB_NAME\`.* TO '$MYSQL_DB_USER'@'$MYSQL_APP_USER_HOST';
FLUSH PRIVILEGES;
EOF
        echo "User '$MYSQL_DB_USER'@'$MYSQL_APP_USER_HOST' privileges and password updated."
    fi

    # Basic security hardening for RHEL-based systems
    if [[ "$OS_FAMILY_DETECTED" == *"rhel"* ]] || [[ "$OS_FAMILY_DETECTED" == *"fedora"* ]]; then
        echo "Securing MySQL installation (basic hardening for RHEL-based)..."
        "${MYSQL_ROOT_EXEC_PARTS[@]}" <<EOF
DELETE FROM mysql.user WHERE User='' AND Host IN ('localhost', '127.0.0.1', '::1'); -- More specific delete
DELETE FROM mysql.user WHERE User='root' AND Host NOT IN ('localhost', '127.0.0.1', '::1');
DROP DATABASE IF EXISTS test;
DELETE FROM mysql.db WHERE Db='test' OR Db='test\\_%';
FLUSH PRIVILEGES;
EOF
    else
        echo "For Debian/Ubuntu, consider running 'sudo mysql_secure_installation' manually for comprehensive security."
    fi
    echo "MySQL configuration completed."
}

create_tables() {
    echo "Starting table creation in database '$MYSQL_DB_NAME' using script '$ACTUAL_SQL_SCRIPT_PATH'..."

    if [ ! -f "$ACTUAL_SQL_SCRIPT_PATH" ]; then
        echo "Error: SQL script file '$ACTUAL_SQL_SCRIPT_PATH' not found!"
        exit 1
    fi

    echo "Executing SQL script '$ACTUAL_SQL_SCRIPT_PATH' as user '$MYSQL_DB_USER'..."
    if mysql -u"$MYSQL_DB_USER" -p"$MYSQL_DB_PASSWORD" -h"$( [[ "$MYSQL_APP_USER_HOST" == "%" ]] && echo "127.0.0.1" || echo "$MYSQL_APP_USER_HOST" )" "$MYSQL_DB_NAME" < "$ACTUAL_SQL_SCRIPT_PATH"; then
        echo "SQL script executed successfully!"
        echo "Verifying table creation (listing tables):"
        mysql -u"$MYSQL_DB_USER" -p"$MYSQL_DB_PASSWORD" -h"$( [[ "$MYSQL_APP_USER_HOST" == "%" ]] && echo "127.0.0.1" || echo "$MYSQL_APP_USER_HOST" )" "$MYSQL_DB_NAME" -e "SHOW TABLES;"
    else
        echo "Error: Failed to execute SQL script '$ACTUAL_SQL_SCRIPT_PATH'!"
        exit 1
    fi
}

# --- Main Execution Flow ---
echo "=== Starting database deployment ==="

install_mysql

echo "Verifying MySQL connection with application user '$MYSQL_DB_USER'..."
APP_USER_CONNECT_HOST_PARAM=""
if [[ "$MYSQL_APP_USER_HOST" != "localhost" ]] && [[ "$MYSQL_APP_USER_HOST" != "127.0.0.1" ]]; then
    # If host is '%' or a remote IP, we need to connect to 127.0.0.1 for local verification
    # or ensure MySQL is listening on all interfaces if connecting from outside.
    # For this script, assume we are verifying a local connection.
    APP_USER_CONNECT_HOST_PARAM="-h127.0.0.1"
    if [[ "$MYSQL_APP_USER_HOST" == "localhost" ]] || [[ "$MYSQL_APP_USER_HOST" == "127.0.0.1" ]]; then # if config was localhost
        APP_USER_CONNECT_HOST_PARAM="-h$MYSQL_APP_USER_HOST"
    fi
else
    APP_USER_CONNECT_HOST_PARAM="-h$MYSQL_APP_USER_HOST"
fi


if ! mysql -u"$MYSQL_DB_USER" -p"$MYSQL_DB_PASSWORD" $APP_USER_CONNECT_HOST_PARAM -e "USE \`$MYSQL_DB_NAME\`; SELECT 1;" &> /dev/null; then
    echo "Error: MySQL connection failed for user '$MYSQL_DB_USER' on database '$MYSQL_DB_NAME' (Host: ${APP_USER_CONNECT_HOST_PARAM:-localhost})!"
    echo "Please check MySQL service status, user credentials, and privileges."
    exit 1
else
    echo "MySQL connection successful for user '$MYSQL_DB_USER' on database '$MYSQL_DB_NAME' (Host: ${APP_USER_CONNECT_HOST_PARAM:-localhost})."
fi

create_tables

echo "=== Database deployment completed successfully ==="