#!/bin/bash

# ==============================================================================
# control-mate-utils_install.sh
# v.1.0.0
#
# Installs, updates, or uninstalls the control-mate-utils binary as a systemd service.
#
# Usage (Install/Update):
#   curl -sL https://raw.githubusercontent.com/controlx-io/control-mate-utils/refs/heads/main/scripts/install_to_linux.sh | sudo -E bash -
#
# Usage (Uninstall):
#   curl -sL https://raw.githubusercontent.com/controlx-io/control-mate-utils/refs/heads/main/scripts/install_to_linux.sh | sudo -E bash -s -- uninstall
#
# ==============================================================================

# --- Configuration ---
SERVICE_NAME="cm-utils"
INSTALL_USER="cm-utils"
APP_DIR="/var/lib/cm-utils"
BINARY_PATH="${APP_DIR}/cm-utils"
SYMLINK_PATH="/usr/local/bin/cm-utils"
VERSION_FILE="${APP_DIR}/.version"
GITHUB_REPO="controlx-io/control-mate-utils"
SERVICE_FILE="/etc/systemd/system/${SERVICE_NAME}.service"

# --- Color Definitions ---
C_RESET='\033[0m'
C_GREEN='\033[0;32m'
C_RED='\033[0;31m'
C_YELLOW='\033[0;33m'
C_CYAN='\033[0;36m'

# --- Helper Functions ---
info() {
    echo -e "${C_GREEN}[INFO]${C_RESET} $1"
}

warn() {
    echo -e "${C_YELLOW}[WARN]${C_RESET} $1"
}

error() {
    echo -e "${C_RED}[ERROR]${C_RESET} $1"
}

get_arch() {
    case $(uname -m) in
        x86_64) echo "linux64";;
        aarch64) echo "linuxA64";;
        *) error "Unsupported architecture: $(uname -m)." >&2; exit 1;;
    esac
}

get_latest_version_info() {
    local arch=$1
    info "Checking for the latest version..." >&2

    local latest_version
    latest_version=$(curl -sL "https://api.github.com/repos/${GITHUB_REPO}/releases/latest" | grep '"tag_name"' | cut -d'"' -f4 | sed 's/^v//')
    if [ -z "$latest_version" ]; then
        error "Could not determine the latest version." >&2
        exit 1
    fi

    # Map architecture to GitHub release asset naming
    local asset_name
    case $arch in
        linux64) asset_name="cm-utils-linux-amd64" ;;
        linuxA64) asset_name="cm-utils-linux-arm64" ;;
        *) error "Unsupported architecture: $arch" >&2; exit 1 ;;
    esac

    local download_url="https://github.com/${GITHUB_REPO}/releases/download/v${latest_version}/${asset_name}"

    echo "${latest_version}|${download_url}"
}

get_installed_version() {
    if [ -f "${VERSION_FILE}" ]; then
        cat "${VERSION_FILE}" | tr -d '[:space:]'
    else
        echo "not-installed"
    fi
}

uninstall_app() {
    info "Starting ControlMate Utils uninstallation..."

    if systemctl list-units --full -all | grep -Fq "${SERVICE_NAME}.service"; then
        info "Stopping and disabling ${SERVICE_NAME} service..."
        systemctl stop "${SERVICE_NAME}"
        systemctl disable "${SERVICE_NAME}"
    else
        info "Service ${SERVICE_NAME} not found, skipping."
    fi

    if [ -f "${SERVICE_FILE}" ]; then
        info "Removing systemd service file..."
        rm -f "${SERVICE_FILE}"
    fi

    info "Reloading systemd daemon..."
    systemctl daemon-reload

    if [ -L "${SYMLINK_PATH}" ]; then
        info "Removing symlink ${SYMLINK_PATH}..."
        rm -f "${SYMLINK_PATH}"
    fi

    if [ -d "${APP_DIR}" ]; then
        info "Removing application directory ${APP_DIR}..."
        rm -rf "${APP_DIR}"
    fi

    if [ -f "/etc/polkit-1/rules.d/55-allow-full-network-management.rules" ]; then
        info "Removing polkit rule file..."
        rm -f "/etc/polkit-1/rules.d/55-allow-full-network-management.rules"
    fi

    if [ -f "/etc/sudoers.d/99-cm-utils-reboot" ]; then
        info "Removing sudoers rule file..."
        rm -f "/etc/sudoers.d/99-cm-utils-reboot"
    fi

    if id "${INSTALL_USER}" &>/dev/null; then
        warn "The user '${INSTALL_USER}' exists."
        read -p "Do you want to remove this user? (y/N) " -n 1 -r REPLY < /dev/tty
        echo
        if [[ $REPLY =~ ^[Yy]$ ]]; then
            info "Removing user '${INSTALL_USER}'..."
            userdel -r "${INSTALL_USER}"
        else
            info "User '${INSTALL_USER}' was not removed."
        fi
    fi

    echo
    echo -e "${C_GREEN}✔ ControlMate Utils has been uninstalled successfully.${C_RESET}"
    exit 0
}


# --- Main Logic ---

# 1. Check for root privileges
if [ "$(id -u)" -ne 0 ]; then
    error "This installer must be run as root or with sudo."
    exit 1
fi

# Handle uninstall command
if [ "$1" == "uninstall" ]; then
    uninstall_app
fi

# 2. Detect architecture
ARCH=$(get_arch)

# 3. Check for existing installation and handle updates
INSTALLED_VERSION=$(get_installed_version)
VERSION_INFO=$(get_latest_version_info "$ARCH")
if [ -z "$VERSION_INFO" ]; then
    error "Could not retrieve latest version information. Aborting."
    exit 1
fi
IFS='|' read -r LATEST_VERSION DOWNLOAD_URL <<< "$VERSION_INFO"

if [ "$INSTALLED_VERSION" != "not-installed" ]; then
    info "ControlMate Utils is already installed. Version: ${C_CYAN}${INSTALLED_VERSION}${C_RESET}"
    if [ "$INSTALLED_VERSION" == "$LATEST_VERSION" ]; then
        info "You are already running the latest version. Exiting."
        exit 0
    else
        warn "A new version is available: ${C_CYAN}${LATEST_VERSION}${C_RESET}"

        if [ -t 0 ]; then
            read -p "Do you want to upgrade? (y/N) " -n 1 -r REPLY
            echo
            if [[ ! $REPLY =~ ^[Yy]$ ]]; then
                info "Update cancelled."
                exit 0
            fi
        else
            if [ -t 1 ] && [ -r /dev/tty ]; then
                read -p "Do you want to upgrade? (y/N) " -n 1 -r REPLY < /dev/tty
                echo
                if [[ ! $REPLY =~ ^[Yy]$ ]]; then
                    info "Update cancelled."
                    exit 0
                fi
            else
                info "Running in a non-interactive environment. Proceeding with update automatically."
            fi
        fi

        # --- Update Process ---
        info "Starting update process..."

        info "Backing up existing binary to ${BINARY_PATH}.bak-${INSTALLED_VERSION}"
        mv "${BINARY_PATH}" "${BINARY_PATH}.bak-${INSTALLED_VERSION}"

        TMP_FILE=$(mktemp)
        info "Downloading new version from ${DOWNLOAD_URL}..."
        if ! curl -sL --fail "${DOWNLOAD_URL}" -o "${TMP_FILE}"; then
            error "Failed to download the new binary. Restoring from backup."
            mv "${BINARY_PATH}.bak-${INSTALLED_VERSION}" "${BINARY_PATH}"
            systemctl start "${SERVICE_NAME}"
            rm -f "${TMP_FILE}"
            exit 1
        fi

        if [ ! -s "${TMP_FILE}" ]; then
            error "Downloaded file is empty. Restoring from backup."
            mv "${BINARY_PATH}.bak-${INSTALLED_VERSION}" "${BINARY_PATH}"
            systemctl start "${SERVICE_NAME}"
            rm -f "${TMP_FILE}"
            exit 1
        fi

        info "Moving new binary to ${BINARY_PATH}..."
        mv "${TMP_FILE}" "${BINARY_PATH}"

        chmod +x "${BINARY_PATH}"
        echo "${LATEST_VERSION}" > "${VERSION_FILE}"
        chown "${INSTALL_USER}:${INSTALL_USER}" "${BINARY_PATH}" "${VERSION_FILE}"

        info "Restarting ${SERVICE_NAME} service..."
        systemctl restart "${SERVICE_NAME}"

        info "Update complete. Waiting a few seconds to check logs..."
        sleep 2
        echo -e "--- Last 25 log lines from ${C_YELLOW}${SERVICE_NAME}${C_RESET} ---"
        journalctl -u ${SERVICE_NAME} -n 25 --no-pager
        echo "--------------------------------------------------"
        exit 0
    fi
fi

# --- Fresh Installation Process ---
info "Starting new ControlMate Utils installation..."
info "Latest version found: ${C_CYAN}${LATEST_VERSION}${C_RESET}"

info "Creating application directory at ${APP_DIR}..."
mkdir -p "${APP_DIR}"

TMP_FILE=$(mktemp)
info "Downloading ControlMate Utils binary to temporary file..."
if ! curl -sL --fail "${DOWNLOAD_URL}" -o "${TMP_FILE}"; then
    error "Failed to download the binary."
    rm -f "${TMP_FILE}"
    exit 1
fi

if [ ! -s "${TMP_FILE}" ]; then
    error "Downloaded file is empty. Aborting."
    rm -f "${TMP_FILE}"
    exit 1
fi

info "Moving binary to ${BINARY_PATH}..."
mv "${TMP_FILE}" "${BINARY_PATH}"

chmod +x "${BINARY_PATH}"
info "Binary installed."

info "Creating symlink at ${SYMLINK_PATH}..."
ln -sf "${BINARY_PATH}" "${SYMLINK_PATH}"

if id "${INSTALL_USER}" &>/dev/null; then
    info "User '${INSTALL_USER}' already exists."
else
    info "Creating system user '${INSTALL_USER}'..."
    useradd -r -s /bin/false -d "${APP_DIR}" "${INSTALL_USER}"
fi
info "Setting ownership of ${APP_DIR} to ${INSTALL_USER}..."
chown -R "${INSTALL_USER}:${INSTALL_USER}" "${APP_DIR}"

info "Creating systemd service file..."
cat << EOF > "${SERVICE_FILE}"
[Unit]
Description=CM Utils Service
After=network.target

[Service]
ExecStart=${BINARY_PATH}
User=${INSTALL_USER}
Group=${INSTALL_USER}
WorkingDirectory=${APP_DIR}
Restart=always
RestartSec=5
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target
EOF

echo "${LATEST_VERSION}" > "${VERSION_FILE}"
chown "${INSTALL_USER}:${INSTALL_USER}" "${VERSION_FILE}"

info "Creating polkit rule for full network management permissions..."
POLKIT_RULE_FILE="/etc/polkit-1/rules.d/55-allow-full-network-management.rules"
cat << EOF > "${POLKIT_RULE_FILE}"
// Allow user '${INSTALL_USER}' to fully manage system network connections
polkit.addRule(function(action, subject) {
    if ((action.id == "org.freedesktop.NetworkManager.wifi.scan" ||
         action.id == "org.freedesktop.NetworkManager.network-control" ||
         action.id == "org.freedesktop.NetworkManager.settings.modify.system") &&
        subject.user == "${INSTALL_USER}") {
        return polkit.Result.YES;
    }
});
EOF
chmod 644 "${POLKIT_RULE_FILE}"

info "Adding sudo rule for passwordless reboot..."
SUDOERS_FILE="/etc/sudoers.d/99-cm-utils-reboot"
cat << EOF > "${SUDOERS_FILE}"
%admin ALL=NOPASSWD: /sbin/reboot
EOF
chmod 440 "${SUDOERS_FILE}"

info "Reloading systemd, enabling and starting service..."
systemctl daemon-reload
systemctl enable "${SERVICE_NAME}"
systemctl start "${SERVICE_NAME}"

echo
echo -e "${C_GREEN}✔ ControlMate Utils was installed and started successfully!${C_RESET}"
echo
info "You can manage the service with these commands:"
echo -e "  - Check status: ${C_YELLOW}systemctl status ${SERVICE_NAME}${C_RESET}"
echo -e "  - View logs:    ${C_YELLOW}sudo journalctl -u ${SERVICE_NAME} -f${C_RESET}"
echo -e "  - Stop service:   ${C_YELLOW}sudo systemctl stop ${SERVICE_NAME}${C_RESET}"
echo -e "  - Restart service:   ${C_YELLOW}sudo systemctl restart ${SERVICE_NAME}${C_RESET}"
echo
