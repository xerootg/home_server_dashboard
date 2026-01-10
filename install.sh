#!/bin/bash
#
# Install script for Home Server Dashboard
# This script compiles the binary, installs it, sets up configuration,
# and installs the systemd service.
#
# Usage: ./install.sh [username]
#   username: The user to run the service as (default: current user)
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

# Service user - override with first argument or use current user
if [[ -n "$1" ]]; then
    SERVICE_USER="$1"
else
    SERVICE_USER="$(whoami)"
fi

# Check if Go is installed
if ! command -v go &> /dev/null; then
    log_error "Go is not installed. Please install Go first."
    exit 1
fi

# Check if user can sudo
if ! sudo -v 2>/dev/null; then
    log_error "This script requires sudo privileges. Please ensure you can run sudo."
    exit 1
fi

log_info "Starting installation of Home Server Dashboard..."

# Step 1: Stop service if running
if systemctl is-active --quiet "${SERVICE_NAME}" 2>/dev/null; then
    log_info "Stopping existing ${SERVICE_NAME}..."
    sudo systemctl stop "${SERVICE_NAME}"
fi

# Also kill any orphan processes (e.g., from manual testing)
if pgrep -f "${BINARY_NAME}" &>/dev/null; then
    log_warn "Found orphan ${BINARY_NAME} process(es), killing..."
    sudo pkill -f "${BINARY_NAME}" || true
    sleep 1
fi

# Step 2: Build JavaScript bundle and compile the binary
log_info "Installing npm dependencies and building JavaScript..."
cd "${SOURCE_DIR}"

# Check if npm is available
if command -v npm &> /dev/null; then
    npm install
    npm run build
else
    log_warn "npm not found - using go generate to build JavaScript"
fi

log_info "Compiling ${BINARY_NAME}..."
go generate ./...
go build -o "${BINARY_NAME}" .

# Step 3: Install the binary
log_info "Installing binary to ${BINARY_PATH}..."
sudo install -m 755 "${BINARY_NAME}" "${BINARY_PATH}"
rm "${BINARY_NAME}"

# Step 4: Verify the service user exists
if ! id "${SERVICE_USER}" &>/dev/null; then
    log_error "User ${SERVICE_USER} does not exist. Please create the user first or specify a different user."
    exit 1
fi
log_info "Service will run as user: ${SERVICE_USER}"

# Step 5: Create config directory and copy sample config
log_info "Setting up configuration directory ${CONFIG_DIR}..."
sudo mkdir -p "${CONFIG_DIR}"
sudo chown "${SERVICE_USER}:${SERVICE_USER}" "${CONFIG_DIR}"
sudo chmod 755 "${CONFIG_DIR}"

if [[ -f "${CONFIG_PATH}" ]]; then
    # Check if source config is newer than installed config
    if [[ -f "${SOURCE_DIR}/services.json" ]] && [[ "${SOURCE_DIR}/services.json" -nt "${CONFIG_PATH}" ]]; then
        log_info "Source services.json is newer than installed version, updating..."
        sudo cp "${SOURCE_DIR}/services.json" "${CONFIG_PATH}"
        sudo chown "${SERVICE_USER}:${SERVICE_USER}" "${CONFIG_PATH}"
        sudo chmod 644 "${CONFIG_PATH}"
    else
        log_info "Configuration file ${CONFIG_PATH} is up to date"
    fi
else
    # Prefer services.json if it exists, otherwise use sample
    if [[ -f "${SOURCE_DIR}/services.json" ]]; then
        log_info "Copying services.json to ${CONFIG_PATH}..."
        sudo cp "${SOURCE_DIR}/services.json" "${CONFIG_PATH}"
        sudo chown "${SERVICE_USER}:${SERVICE_USER}" "${CONFIG_PATH}"
        sudo chmod 644 "${CONFIG_PATH}"
    elif [[ -f "${SOURCE_DIR}/sample.services.json" ]]; then
        log_info "Copying sample.services.json to ${CONFIG_PATH}..."
        sudo cp "${SOURCE_DIR}/sample.services.json" "${CONFIG_PATH}"
        sudo chown "${SERVICE_USER}:${SERVICE_USER}" "${CONFIG_PATH}"
        sudo chmod 644 "${CONFIG_PATH}"
    else
        log_warn "No configuration file found, skipping config copy"
    fi
fi

# Step 6: Install systemd service (with user substitution)
log_info "Installing systemd service to ${SERVICE_PATH}..."
sed "s/@@SERVICE_USER@@/${SERVICE_USER}/g" "${SOURCE_DIR}/${SERVICE_NAME}" | sudo tee "${SERVICE_PATH}" > /dev/null
sudo chmod 644 "${SERVICE_PATH}"

# Step 6.5: Generate and install polkit rules for local systemd service control
POLKIT_RULES_DIR="/etc/polkit-1/rules.d"
POLKIT_RULES_PATH="${POLKIT_RULES_DIR}/50-home-server-dashboard.rules"

if [[ -d "${POLKIT_RULES_DIR}" ]]; then
    log_info "Generating polkit rules for systemd service control..."
    # Use the installed binary to generate polkit rules
    "${BINARY_PATH}" -generate-polkit -user "${SERVICE_USER}" | sudo tee "${POLKIT_RULES_PATH}" > /dev/null
    sudo chmod 644 "${POLKIT_RULES_PATH}"
    log_info "Polkit rules installed to ${POLKIT_RULES_PATH}"
else
    log_warn "Polkit rules directory not found (${POLKIT_RULES_DIR})"
    log_warn "Systemd service control may not work. Install polkit to enable this feature."
fi

# Step 7: Reload systemd and enable service
log_info "Enabling ${SERVICE_NAME}..."
sudo systemctl daemon-reload
sudo systemctl enable "${SERVICE_NAME}"

# Step 8: Start the service
log_info "Starting ${SERVICE_NAME}..."
sudo systemctl start "${SERVICE_NAME}"

# Step 9: Show status
log_info "Installation complete!"
echo ""
systemctl status "${SERVICE_NAME}" --no-pager || true
echo ""
log_info "Dashboard is now running at http://localhost:9001"
log_info "Configuration file: ${CONFIG_PATH}"
log_info "Edit the configuration and restart with: sudo systemctl restart ${SERVICE_NAME}"
