#!/bin/bash
#
# Install script for Home Server Dashboard
# This script compiles the binary, installs it, sets up configuration,
# and installs the systemd service.
#
# Usage: sudo ./install.sh [username]
#   username: The user to run the service as (default: current user via SUDO_USER)
#

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Paths
BINARY_NAME="nas-dashboard"
BINARY_PATH="/usr/local/bin/${BINARY_NAME}"
SERVICE_NAME="nas-dashboard.service"
SERVICE_PATH="/etc/systemd/system/${SERVICE_NAME}"
CONFIG_DIR="/etc/nas_dashboard"
CONFIG_PATH="${CONFIG_DIR}/services.json"
SOURCE_DIR="$(cd "$(dirname "$0")" && pwd)"

log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Service user - override with first argument or use SUDO_USER
if [[ -n "$1" ]]; then
    SERVICE_USER="$1"
elif [[ -n "$SUDO_USER" ]]; then
    SERVICE_USER="$SUDO_USER"
else
    log_error "Could not determine non-root user. Please run with: sudo ./install.sh <username>"
    exit 1
fi

# Check for root privileges
if [[ $EUID -ne 0 ]]; then
    log_error "This script must be run as root (use sudo)"
    exit 1
fi

# Check if Go is installed
if ! command -v go &> /dev/null; then
    log_error "Go is not installed. Please install Go first."
    exit 1
fi

log_info "Starting installation of Home Server Dashboard..."

# Step 1: Stop service if running
if systemctl is-active --quiet "${SERVICE_NAME}" 2>/dev/null; then
    log_info "Stopping existing ${SERVICE_NAME}..."
    systemctl stop "${SERVICE_NAME}"
fi

# Also kill any orphan processes (e.g., from manual testing)
if pgrep -f "${BINARY_NAME}" &>/dev/null; then
    log_warn "Found orphan ${BINARY_NAME} process(es), killing..."
    pkill -f "${BINARY_NAME}" || true
    sleep 1
fi

# Step 2: Compile the binary
log_info "Compiling ${BINARY_NAME}..."
cd "${SOURCE_DIR}"
# Build as the user who invoked sudo to avoid permission issues with go cache
SUDO_USER_HOME=$(getent passwd "${SUDO_USER:-root}" | cut -d: -f6)
sudo -u "${SUDO_USER:-root}" HOME="${SUDO_USER_HOME}" go build -o "${BINARY_NAME}" .

# Step 3: Install the binary
log_info "Installing binary to ${BINARY_PATH}..."
install -m 755 "${BINARY_NAME}" "${BINARY_PATH}"
rm "${BINARY_NAME}"

# Step 4: Verify the service user exists
if ! id "${SERVICE_USER}" &>/dev/null; then
    log_error "User ${SERVICE_USER} does not exist. Please create the user first or specify a different user."
    exit 1
fi
log_info "Service will run as user: ${SERVICE_USER}"

# Step 5: Create config directory and copy sample config
log_info "Setting up configuration directory ${CONFIG_DIR}..."
mkdir -p "${CONFIG_DIR}"
chown "${SERVICE_USER}:${SERVICE_USER}" "${CONFIG_DIR}"
chmod 755 "${CONFIG_DIR}"

if [[ -f "${CONFIG_PATH}" ]]; then
    log_warn "Configuration file ${CONFIG_PATH} already exists, not overwriting"
else
    # Prefer services.json if it exists, otherwise use sample
    if [[ -f "${SOURCE_DIR}/services.json" ]]; then
        log_info "Copying services.json to ${CONFIG_PATH}..."
        cp "${SOURCE_DIR}/services.json" "${CONFIG_PATH}"
        chown "${SERVICE_USER}:${SERVICE_USER}" "${CONFIG_PATH}"
        chmod 644 "${CONFIG_PATH}"
    elif [[ -f "${SOURCE_DIR}/sample.services.json" ]]; then
        log_info "Copying sample.services.json to ${CONFIG_PATH}..."
        cp "${SOURCE_DIR}/sample.services.json" "${CONFIG_PATH}"
        chown "${SERVICE_USER}:${SERVICE_USER}" "${CONFIG_PATH}"
        chmod 644 "${CONFIG_PATH}"
    else
        log_warn "No configuration file found, skipping config copy"
    fi
fi

# Step 6: Install systemd service (with user substitution)
log_info "Installing systemd service to ${SERVICE_PATH}..."
sed "s/@@SERVICE_USER@@/${SERVICE_USER}/g" "${SOURCE_DIR}/${SERVICE_NAME}" > "${SERVICE_PATH}"
chmod 644 "${SERVICE_PATH}"

# Step 7: Reload systemd and enable service
log_info "Enabling ${SERVICE_NAME}..."
systemctl daemon-reload
systemctl enable "${SERVICE_NAME}"

# Step 8: Start the service
log_info "Starting ${SERVICE_NAME}..."
systemctl start "${SERVICE_NAME}"

# Step 9: Show status
log_info "Installation complete!"
echo ""
systemctl status "${SERVICE_NAME}" --no-pager || true
echo ""
log_info "Dashboard is now running at http://localhost:9001"
log_info "Configuration file: ${CONFIG_PATH}"
log_info "Edit the configuration and restart with: sudo systemctl restart ${SERVICE_NAME}"
