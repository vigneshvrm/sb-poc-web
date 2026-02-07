#!/bin/bash
# ============================================
# StackBill Deployer - Quick Start Script
# ============================================
# Downloads and runs the StackBill Deployer container.
# After deployment completes, cleans up automatically.
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/vigneshvrm/sb-poc-web/main/scripts/deploy.sh | bash
#
# ============================================

set -e

CONTAINER_NAME="stackbill-deployer"
IMAGE="vickyinfra/sb-poc:latest"
PORT=9876

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m'

info()  { echo -e "${GREEN}[INFO]${NC}  $1"; }
warn()  { echo -e "${YELLOW}[WARN]${NC}  $1"; }
error() { echo -e "${RED}[ERROR]${NC} $1"; }

echo ""
echo -e "${CYAN}╔═══════════════════════════════════════════════════════════════╗${NC}"
echo -e "${CYAN}║              STACKBILL DEPLOYER - Quick Start               ║${NC}"
echo -e "${CYAN}╚═══════════════════════════════════════════════════════════════╝${NC}"
echo ""

# --- Detect container runtime ---
RUNTIME=""
if command -v docker &>/dev/null; then
    RUNTIME="docker"
elif command -v podman &>/dev/null; then
    RUNTIME="podman"
fi

# --- Install podman-docker if no runtime found ---
if [ -z "$RUNTIME" ]; then
    info "No container runtime found. Installing podman-docker..."

    if [ "$(id -u)" -ne 0 ]; then
        error "Root privileges required to install podman. Run with sudo:"
        echo "  curl -fsSL <url> | sudo bash"
        exit 1
    fi

    export DEBIAN_FRONTEND=noninteractive
    apt-get update -qq
    apt-get install -y -qq podman-docker

    if ! command -v podman &>/dev/null; then
        error "Failed to install podman-docker."
        exit 1
    fi

    RUNTIME="podman"
    info "Podman installed successfully."
fi

info "Using container runtime: $RUNTIME"

# --- Configure podman to pull from Docker Hub ---
if [ "$RUNTIME" = "podman" ]; then
    REGISTRIES_CONF="/etc/containers/registries.conf"
    if [ -f "$REGISTRIES_CONF" ]; then
        if ! grep -q 'unqualified-search-registries.*docker.io' "$REGISTRIES_CONF" 2>/dev/null; then
            info "Configuring podman to search Docker Hub..."
            echo 'unqualified-search-registries = ["docker.io"]' >> "$REGISTRIES_CONF"
        fi
    else
        mkdir -p /etc/containers
        echo 'unqualified-search-registries = ["docker.io"]' > "$REGISTRIES_CONF"
    fi
fi

# --- Stop any existing deployer container ---
if $RUNTIME ps -a --format '{{.Names}}' 2>/dev/null | grep -q "^${CONTAINER_NAME}$"; then
    warn "Removing existing ${CONTAINER_NAME} container..."
    $RUNTIME stop "$CONTAINER_NAME" 2>/dev/null || true
    $RUNTIME rm "$CONTAINER_NAME" 2>/dev/null || true
fi

# --- Pull latest image ---
info "Pulling ${IMAGE}..."
$RUNTIME pull "$IMAGE"

# --- Start container ---
info "Starting StackBill Deployer on port ${PORT}..."
$RUNTIME run -d --name "$CONTAINER_NAME" -p "${PORT}:${PORT}" "$IMAGE"

# --- Wait for container to start and extract token ---
sleep 2

TOKEN=""
for i in $(seq 1 10); do
    LOGS=$($RUNTIME logs "$CONTAINER_NAME" 2>&1)
    TOKEN=$(echo "$LOGS" | grep "Auth Token:" | awk '{print $NF}' | tail -1)
    if [ -n "$TOKEN" ]; then
        break
    fi
    sleep 1
done

if [ -z "$TOKEN" ]; then
    error "Could not extract auth token. Check container logs:"
    echo "  $RUNTIME logs $CONTAINER_NAME"
    exit 1
fi

# --- Detect server IP ---
SERVER_IP=$(hostname -I 2>/dev/null | awk '{print $1}')
if [ -z "$SERVER_IP" ]; then
    SERVER_IP="localhost"
fi

echo ""
echo -e "${GREEN}═══════════════════════════════════════════════════════════════${NC}"
echo -e "${GREEN}  StackBill Deployer is ready!${NC}"
echo -e "${GREEN}═══════════════════════════════════════════════════════════════${NC}"
echo ""
echo -e "  URL:   ${CYAN}http://${SERVER_IP}:${PORT}${NC}"
echo -e "  Token: ${CYAN}${TOKEN}${NC}"
echo ""
echo -e "  1. Open the URL in your browser"
echo -e "  2. Enter the token above to authenticate"
echo -e "  3. Fill in your server details and deploy"
echo ""
echo -e "${GREEN}═══════════════════════════════════════════════════════════════${NC}"
echo ""

# --- Monitor for deployment completion ---
info "Monitoring deployment status... (Ctrl+C to exit without cleanup)"

cleanup() {
    echo ""
    info "Cleaning up..."
    $RUNTIME stop "$CONTAINER_NAME" 2>/dev/null || true
    $RUNTIME rm "$CONTAINER_NAME" 2>/dev/null || true
    $RUNTIME rmi "$IMAGE" 2>/dev/null || true
    info "Container and image removed. Goodbye!"
}

# If user hits Ctrl+C, ask about cleanup
trap 'echo ""; read -r -p "Clean up container and image? [y/N] " ans; if [[ "$ans" =~ ^[Yy] ]]; then cleanup; fi; exit 0' INT

API_URL="http://127.0.0.1:${PORT}/api/deployments"

while true; do
    sleep 5

    # Check if container is still running
    if ! $RUNTIME ps --format '{{.Names}}' 2>/dev/null | grep -q "^${CONTAINER_NAME}$"; then
        warn "Container stopped unexpectedly."
        break
    fi

    # Poll API for any completed deployment
    RESPONSE=$(curl -sf -H "Authorization: Bearer ${TOKEN}" "${API_URL}" 2>/dev/null) || continue

    # Check if any deployment has completed
    HAS_SUCCESS=$(echo "$RESPONSE" | grep -o '"status":"success"' | head -1) || true
    HAS_FAILED=$(echo "$RESPONSE" | grep -o '"status":"failed"' | head -1) || true
    HAS_RUNNING=$(echo "$RESPONSE" | grep -o '"status":"running"' | head -1) || true
    HAS_PENDING=$(echo "$RESPONSE" | grep -o '"status":"pending"' | head -1) || true

    if [ -n "$HAS_SUCCESS" ] && [ -z "$HAS_RUNNING" ] && [ -z "$HAS_PENDING" ]; then
        echo ""
        echo -e "${GREEN}═══════════════════════════════════════════════════════════════${NC}"
        echo -e "${GREEN}  Deployment completed successfully!${NC}"
        echo -e "${GREEN}═══════════════════════════════════════════════════════════════${NC}"
        echo ""
        cleanup
        exit 0
    fi

    if [ -n "$HAS_FAILED" ] && [ -z "$HAS_RUNNING" ] && [ -z "$HAS_PENDING" ]; then
        echo ""
        echo -e "${RED}═══════════════════════════════════════════════════════════════${NC}"
        echo -e "${RED}  Deployment failed. You can retry from the browser.${NC}"
        echo -e "${RED}═══════════════════════════════════════════════════════════════${NC}"
        echo ""
        info "Waiting for retry or exit... (Ctrl+C to stop)"
        # Don't exit on failure - user might retry from the UI
    fi
done

echo ""
read -r -p "Clean up container and image? [y/N] " ans
if [[ "$ans" =~ ^[Yy] ]]; then
    cleanup
fi
