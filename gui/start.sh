#!/bin/bash
# Gas Town - Unified Startup Script
# Starts both the Go agents and the Node GUI server

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
GT_DIR="$(dirname "$SCRIPT_DIR")"
GT_ROOT="${GT_ROOT:-$HOME/gt}"
PORT="${PORT:-4444}"

# Colors
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${BLUE}╔═══════════════════════════════════════╗${NC}"
echo -e "${BLUE}║       Gas Town - Starting Up          ║${NC}"
echo -e "${BLUE}╚═══════════════════════════════════════╝${NC}"
echo ""

# Check if gt binary exists
if [ ! -f "$GT_DIR/gt" ]; then
    echo -e "${YELLOW}Building gt binary...${NC}"
    cd "$GT_DIR" && go build -o gt ./cmd/gt
fi

# Ensure bd is in PATH
export PATH="$HOME/.local/bin:$PATH"

# Check if bd exists
if ! command -v bd &> /dev/null; then
    echo -e "${YELLOW}Warning: 'bd' (beads) not found in PATH${NC}"
    echo "Some features may not work. Install from: https://github.com/steveyegge/beads"
fi

# Initialize beads if needed
if [ ! -d "$GT_ROOT/.beads" ]; then
    echo -e "${YELLOW}Initializing beads database...${NC}"
    cd "$GT_ROOT" && bd init 2>/dev/null || true
fi

# Start Gas Town agents (Mayor & Deacon)
echo -e "${GREEN}Starting Gas Town agents...${NC}"
cd "$GT_ROOT"
"$GT_DIR/gt" start 2>/dev/null &
GT_PID=$!
sleep 2

# Check if agents started
if kill -0 $GT_PID 2>/dev/null; then
    echo -e "${GREEN}✓ Agents started (PID: $GT_PID)${NC}"
else
    echo -e "${YELLOW}⚠ Agents may have started in background sessions${NC}"
fi

# Start Node GUI server
echo -e "${GREEN}Starting GUI server on port $PORT...${NC}"
cd "$SCRIPT_DIR"
GT_ROOT="$GT_ROOT" PORT="$PORT" node server.js &
NODE_PID=$!
sleep 2

# Check if server started
if curl -s "http://localhost:$PORT/api/status" > /dev/null 2>&1; then
    echo -e "${GREEN}✓ GUI server running at http://localhost:$PORT${NC}"
else
    echo -e "${YELLOW}⚠ GUI server may still be starting...${NC}"
fi

echo ""
echo -e "${BLUE}═══════════════════════════════════════${NC}"
echo -e "${GREEN}Gas Town is running!${NC}"
echo ""
echo "  GUI:    http://localhost:$PORT"
echo "  Agents: Mayor, Deacon"
echo ""
echo "  To stop: $GT_DIR/gt shutdown && kill $NODE_PID"
echo -e "${BLUE}═══════════════════════════════════════${NC}"
echo ""

# Wait for either process to exit
wait $NODE_PID
