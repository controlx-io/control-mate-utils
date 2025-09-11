#!/bin/bash

# ==============================================================================
# jaspernode_install.sh
# v.1.0.9
#
# Installs, updates, or uninstalls the jaspernode binary as a user-level systemd service.
#
# Usage (Install/Update):
#   curl -sL https://dl.jasperx.io/jn/linux-install.sh | bash -
#
# Usage (Uninstall):
#   curl -sL https://dl.jasperx.io/jn/linux-install.sh | bash -s -- uninstall
#
# ==============================================================================

# --- Configuration ---
SERVICE_NAME="jaspernode"
APP_DIR="${HOME}/.local/share/jaspernode"
BINARY_PATH="${HOME}/.local/bin/jaspernode"
VERSION_FILE="${APP_DIR}/.version"
BASE_URL="https://dl.jasperx.io/jn"
SERVICE_FILE="${HOME}/.config/systemd/user/${SERVICE_NAME}.service"

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
    latest_version=$(curl -sL "${BASE_URL}/latest" | tr -d '[:space:]')
    if [ -z "$latest_version" ]; then
        error "Could not determine the latest version." >&2
        exit 1
    fi

    local download_url="${BASE_URL}/${arch}/${latest_version}"

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
    info "Starting JasperNode uninstallation..."

    if systemctl --user list-units --full -all | grep -Fq "${SERVICE_NAME}.service"; then
        info "Stopping and disabling ${SERVICE_NAME} service..."
        systemctl --user stop "${SERVICE_NAME}"
        systemctl --user disable "${SERVICE_NAME}"
    else
        info "Service ${SERVICE_NAME} not found, skipping."
    fi

    if [ -f "${SERVICE_FILE}" ]; then
        info "Removing systemd service file..."
        rm -f "${SERVICE_FILE}"
    fi

    info "Reloading systemd daemon..."
    systemctl --user daemon-reload

    if [ -f "${BINARY_PATH}" ]; then
        info "Removing binary ${BINARY_PATH}..."
        rm -f "${BINARY_PATH}"
    fi

    if [ -d "${APP_DIR}" ]; then
        info "Removing application directory ${APP_DIR}..."
        rm -rf "${APP_DIR}"
    fi

    echo
    echo -e "${C_GREEN}✔ JasperNode has been uninstalled successfully.${C_RESET}"
    exit 0
}

# --- Main Logic ---

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
    info "JasperNode is already installed. Version: ${C_CYAN}${INSTALLED_VERSION}${C_RESET}"
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
            systemctl --user start "${SERVICE_NAME}"
            rm -f "${TMP_FILE}"
            exit 1
        fi

        if [ ! -s "${TMP_FILE}" ]; then
            error "Downloaded file is empty. Restoring from backup."
            mv "${BINARY_PATH}.bak-${INSTALLED_VERSION}" "${BINARY_PATH}"
            systemctl --user start "${SERVICE_NAME}"
            rm -f "${TMP_FILE}"
            exit 1
        fi

        info "Moving new binary to ${BINARY_PATH}..."
        mv "${TMP_FILE}" "${BINARY_PATH}"

        chmod +x "${BINARY_PATH}"
        echo "${LATEST_VERSION}" > "${VERSION_FILE}"

        info "Restarting ${SERVICE_NAME} service..."
        systemctl --user restart "${SERVICE_NAME}"

        info "Update complete. Waiting a few seconds to check logs..."
        sleep 2
        echo -e "--- Last 25 log lines from ${C_YELLOW}${SERVICE_NAME}${C_RESET} ---"
        journalctl --user -u ${SERVICE_NAME} -n 25 --no-pager
        echo "--------------------------------------------------"
        exit 0
    fi
fi

# --- Fresh Installation Process ---
info "Starting new JasperNode installation..."
info "Latest version found: ${C_CYAN}${LATEST_VERSION}${C_RESET}"

info "Creating application directory at ${APP_DIR}..."
mkdir -p "${APP_DIR}"

info "Creating binary directory at ${HOME}/.local/bin..."
mkdir -p "${HOME}/.local/bin"

TMP_FILE=$(mktemp)
info "Downloading JasperNode binary to temporary file..."
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

info "Creating systemd user service directory..."
mkdir -p "${HOME}/.config/systemd/user"

info "Creating systemd service file..."
cat << EOF > "${SERVICE_FILE}"
[Unit]
Description=JasperNode Service
After=network.target

[Service]
ExecStart=${BINARY_PATH}
WorkingDirectory=${APP_DIR}
Restart=always
RestartSec=5
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=default.target
EOF

echo "${LATEST_VERSION}" > "${VERSION_FILE}"

info "Reloading systemd, enabling and starting service..."
systemctl --user daemon-reload
systemctl --user enable "${SERVICE_NAME}"
systemctl --user start "${SERVICE_NAME}"

echo
echo -e "${C_GREEN}✔ JasperNode was installed and started successfully!${C_RESET}"
echo
info "You can manage the service with these commands:"
echo -e "  - Check status: ${C_YELLOW}systemctl --user status ${SERVICE_NAME}${C_RESET}"
echo -e "  - View logs:    ${C_YELLOW}journalctl --user -u ${SERVICE_NAME} -f${C_RESET}"
echo -e "  - Stop service:   ${C_YELLOW}systemctl --user stop ${SERVICE_NAME}${C_RESET}"
echo -e "  - Restart service:   ${C_YELLOW}systemctl --user restart ${SERVICE_NAME}${C_RESET}"
echo
info "The binary is available at: ${C_CYAN}${BINARY_PATH}${C_RESET}"
echo