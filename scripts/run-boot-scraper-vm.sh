#!/bin/bash
#
# Run the boot block scraper on the zelda/link GCE VM.
#
# This script:
# 1. Cross-compiles the scraper CLI from the hop repo
# 2. Stores the GitHub PAT in GCP Secret Manager (if not already there)
# 3. Deploys the binary to the VM
# 4. Runs the scraper with --discover mode for 1000+ users
# 5. Downloads the results
#
# Prerequisites:
# - gcloud CLI configured with ghosttrack-wyvern project
# - Go 1.25+ installed locally
# - GITHUB_TOKEN env var set (or pass with -t flag)
#
# Usage:
#   ./scripts/run-boot-scraper-vm.sh [-t GITHUB_TOKEN] [-c CONCURRENCY] [-n TARGET_USERS]

set -euo pipefail

# Defaults
PROJECT_ID="ghosttrack-wyvern"
ZONE="us-central1-a"
VM_NAME="claude-code-dev-vm"
CONCURRENCY=5
TARGET_USERS=1200
HOP_DIR="${HOME}/gt/hop/mayor/rig"
BINARY_NAME="boot-scraper"
REMOTE_DIR="/workspace/scraper"
RESULTS_DIR="$(pwd)/scraper-results"
TIMESTAMP=$(date +%Y%m%d-%H%M%S)

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

log() { echo -e "${BLUE}[INFO]${NC} $*"; }
success() { echo -e "${GREEN}[OK]${NC} $*"; }
warn() { echo -e "${YELLOW}[WARN]${NC} $*"; }
error() { echo -e "${RED}[ERROR]${NC} $*" >&2; }

# Parse flags
GITHUB_TOKEN="${GITHUB_TOKEN:-}"
while getopts "t:c:n:" opt; do
    case $opt in
        t) GITHUB_TOKEN="$OPTARG" ;;
        c) CONCURRENCY="$OPTARG" ;;
        n) TARGET_USERS="$OPTARG" ;;
        *) echo "Usage: $0 [-t GITHUB_TOKEN] [-c CONCURRENCY] [-n TARGET_USERS]"; exit 1 ;;
    esac
done

# Validate token
if [ -z "$GITHUB_TOKEN" ]; then
    error "GitHub token required. Set GITHUB_TOKEN env var or use -t flag."
    exit 1
fi

# Step 1: Cross-compile
log "Cross-compiling boot-scraper for linux/amd64..."
if [ ! -d "$HOP_DIR" ]; then
    error "Hop directory not found at $HOP_DIR"
    exit 1
fi

TMPBIN=$(mktemp -d)/boot-scraper
(cd "$HOP_DIR" && GOOS=linux GOARCH=amd64 go build -o "$TMPBIN" ./cmd/boot-scraper/)
success "Binary built: $(ls -lh "$TMPBIN" | awk '{print $5}')"

# Step 2: Check VM is running
log "Checking VM status..."
VM_STATUS=$(gcloud compute instances describe "$VM_NAME" \
    --zone="$ZONE" --project="$PROJECT_ID" \
    --format='get(status)' 2>/dev/null || echo "NOT_FOUND")

if [ "$VM_STATUS" = "TERMINATED" ]; then
    log "VM is stopped, starting it..."
    gcloud compute instances start "$VM_NAME" --zone="$ZONE" --project="$PROJECT_ID"
    log "Waiting for VM to boot..."
    sleep 30
elif [ "$VM_STATUS" != "RUNNING" ]; then
    error "VM $VM_NAME is in unexpected state: $VM_STATUS"
    exit 1
fi
success "VM is running"

# Step 3: Store GitHub PAT in Secret Manager
log "Checking for GitHub PAT in Secret Manager..."
if gcloud secrets describe github-pat --project="$PROJECT_ID" &>/dev/null; then
    log "Updating existing github-pat secret..."
    echo -n "$GITHUB_TOKEN" | gcloud secrets versions add github-pat \
        --data-file=- --project="$PROJECT_ID" 2>/dev/null
else
    log "Creating github-pat secret..."
    echo -n "$GITHUB_TOKEN" | gcloud secrets create github-pat \
        --data-file=- --replication-policy="automatic" \
        --project="$PROJECT_ID" 2>/dev/null
fi
success "GitHub PAT stored in Secret Manager"

# Step 4: Deploy binary to VM
log "Deploying binary to VM..."
gcloud compute scp "$TMPBIN" "${VM_NAME}:/tmp/boot-scraper" \
    --zone="$ZONE" --project="$PROJECT_ID"

# Set up remote directory and move binary
gcloud compute ssh "$VM_NAME" --zone="$ZONE" --project="$PROJECT_ID" --command="
    mkdir -p $REMOTE_DIR
    mv /tmp/boot-scraper $REMOTE_DIR/boot-scraper
    chmod +x $REMOTE_DIR/boot-scraper
"
success "Binary deployed to $VM_NAME:$REMOTE_DIR/"

# Step 5: Run the scraper
log "Starting bulk scrape (target: $TARGET_USERS users, concurrency: $CONCURRENCY)..."
log "This will take several hours. Monitor with: gcloud compute ssh $VM_NAME --zone=$ZONE --project=$PROJECT_ID --command='tail -f /workspace/scraper/scraper.log'"

RESULTS_FILE="results-${TIMESTAMP}.json"

# Run in background on the VM via nohup + tmux
gcloud compute ssh "$VM_NAME" --zone="$ZONE" --project="$PROJECT_ID" --command="
    export GITHUB_TOKEN='$GITHUB_TOKEN'
    export PATH=\$PATH:/usr/local/go/bin

    cd $REMOTE_DIR

    # Run scraper in tmux session so it survives SSH disconnect
    tmux new-session -d -s scraper 'GITHUB_TOKEN=\"$GITHUB_TOKEN\" ./boot-scraper \
        -discover \
        -discover-count=$TARGET_USERS \
        -concurrency=$CONCURRENCY \
        -delay=5s \
        -output=$RESULTS_FILE \
        2>&1 | tee scraper.log; echo \"SCRAPE COMPLETE\" >> scraper.log'
"

success "Scraper running in tmux session on VM"
echo ""
echo "═══════════════════════════════════════════════════════════════"
echo "  Boot Block Scraper Deployed and Running"
echo "═══════════════════════════════════════════════════════════════"
echo ""
echo "  VM: $VM_NAME ($ZONE)"
echo "  Target: $TARGET_USERS users"
echo "  Concurrency: $CONCURRENCY workers"
echo "  Output: $REMOTE_DIR/$RESULTS_FILE"
echo ""
echo "  Monitor:"
echo "    gcloud compute ssh $VM_NAME --zone=$ZONE --project=$PROJECT_ID --command='tail -f $REMOTE_DIR/scraper.log'"
echo ""
echo "  Check tmux:"
echo "    gcloud compute ssh $VM_NAME --zone=$ZONE --project=$PROJECT_ID --command='tmux attach -t scraper'"
echo ""
echo "  Download results when complete:"
echo "    gcloud compute scp $VM_NAME:$REMOTE_DIR/$RESULTS_FILE ./ --zone=$ZONE --project=$PROJECT_ID"
echo ""
echo "  Estimated time: ~$(( TARGET_USERS / (CONCURRENCY * 50) )) hours"
echo ""

# Clean up temp binary
rm -f "$TMPBIN"
