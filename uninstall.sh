#!/bin/bash
#
# Uninstall script for Home Server Dashboard
# This script stops the service, removes the binary, config, and systemd unit.
#
# Usage: ./uninstall.sh
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

log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Check if user can sudo
if ! sudo -v 2>/dev/null; then
    log_error "This script requires sudo privileges. Please ensure you can run sudo."
    exit 1
fi

log_info "Starting uninstallation of Home Server Dashboard..."

# Step 1: Stop the service if running
if systemctl is-active --quiet "${SERVICE_NAME}" 2>/dev/null; then
    log_info "Stopping ${SERVICE_NAME}..."
    sudo systemctl stop "${SERVICE_NAME}"
fi

# Step 2: Disable the service
if systemctl is-enabled --quiet "${SERVICE_NAME}" 2>/dev/null; then
    log_info "Disabling ${SERVICE_NAME}..."
    sudo systemctl disable "${SERVICE_NAME}"
fi

# Step 3: Remove the systemd unit file
if [[ -f "${SERVICE_PATH}" ]]; then
    log_info "Removing systemd unit ${SERVICE_PATH}..."
    sudo rm "${SERVICE_PATH}"
    sudo systemctl daemon-reload
else
    log_warn "Systemd unit ${SERVICE_PATH} not found, skipping"
fi

# Step 3.5: Remove polkit rules
POLKIT_RULES_PATH="/etc/polkit-1/rules.d/50-home-server-dashboard.rules"
if [[ -f "${POLKIT_RULES_PATH}" ]]; then
    log_info "Removing polkit rules ${POLKIT_RULES_PATH}..."
    sudo rm "${POLKIT_RULES_PATH}"
else
    log_warn "Polkit rules ${POLKIT_RULES_PATH} not found, skipping"
fi

# Step 4: Remove the binary
if [[ -f "${BINARY_PATH}" ]]; then
    log_info "Removing binary ${BINARY_PATH}..."
    sudo rm "${BINARY_PATH}"
else
    log_warn "Binary ${BINARY_PATH} not found, skipping"
fi

# Step 5: Remove the config (prompt user)
SAMPLE_CONFIG="${CONFIG_DIR}/services.json"
if [[ -f "${SAMPLE_CONFIG}" ]]; then
    read -p "Remove configuration file ${SAMPLE_CONFIG}? [y/N] " -n 1 -r
    echo
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        log_info "Removing ${SAMPLE_CONFIG}..."
        sudo rm "${SAMPLE_CONFIG}"
    else
        log_info "Keeping ${SAMPLE_CONFIG}"
    fi
fi

# Step 6: Remove config directory if empty
if [[ -d "${CONFIG_DIR}" ]]; then
    if [[ -z "$(ls -A "${CONFIG_DIR}")" ]]; then
        log_info "Removing empty config directory ${CONFIG_DIR}..."
        sudo rmdir "${CONFIG_DIR}"
    else
        log_warn "Config directory ${CONFIG_DIR} is not empty, not removing"
        log_info "Remaining files:"
        ls -la "${CONFIG_DIR}"
    fi
fi

log_info "Uninstallation complete!"
