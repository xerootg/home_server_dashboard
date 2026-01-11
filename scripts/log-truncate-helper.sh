#!/bin/bash
# log-truncate-helper - Truncates Docker container log files
#
# This script is installed with setuid root to allow the dashboard
# (which runs with NoNewPrivileges=true) to truncate Docker logs.
#
# Security:
# - Only accepts paths under /var/lib/docker/containers/
# - Only truncates files ending in -json.log
# - Runs as root but only callable by docker group members (mode 4750)

set -e

DOCKER_CONTAINERS_DIR="/var/lib/docker/containers/"
LOG_SUFFIX="-json.log"

if [[ $# -ne 1 ]]; then
    echo "Usage: $0 <container-log-path>" >&2
    exit 1
fi

LOG_PATH="$1"

# Resolve to absolute path and remove any .. components
CLEAN_PATH="$(realpath -m "$LOG_PATH" 2>/dev/null)" || {
    echo "Error: Invalid path" >&2
    exit 1
}

# Validate path is under Docker containers directory
if [[ ! "$CLEAN_PATH" == "${DOCKER_CONTAINERS_DIR}"* ]]; then
    echo "Error: Path must be under ${DOCKER_CONTAINERS_DIR}" >&2
    exit 1
fi

# Validate path ends with the expected log suffix
if [[ ! "$CLEAN_PATH" == *"${LOG_SUFFIX}" ]]; then
    echo "Error: Path must end with ${LOG_SUFFIX}" >&2
    exit 1
fi

# Check that the file exists and is a regular file
if [[ ! -f "$CLEAN_PATH" ]]; then
    echo "Error: Not a regular file or does not exist" >&2
    exit 1
fi

# Truncate the file
truncate -s 0 "$CLEAN_PATH"
echo "Successfully truncated $CLEAN_PATH"
