#!/bin/bash
set -e

# OpenCode Setup Script for Copilot Agent
# Sets up OpenCode with Antigravity auth from OPENCODE_AUTH_BUNDLE secret

echo "🔧 OpenCode Setup Script"
echo "========================"
echo ""

# Step 1: Check for OPENCODE_AUTH_BUNDLE environment variable
if [ -z "$OPENCODE_AUTH_BUNDLE" ]; then
    echo "❌ Error: OPENCODE_AUTH_BUNDLE environment variable is not set"
    echo ""
    echo "This script requires the OPENCODE_AUTH_BUNDLE secret to be available."
    echo "The secret should be set in the 'copilot' environment."
    exit 1
fi

echo "✓ OPENCODE_AUTH_BUNDLE found"
echo ""

# Step 2: Install opencode-ai and opencode-antigravity-auth globally
echo "📦 Installing OpenCode packages..."
npm install -g opencode-ai opencode-antigravity-auth@1.2.8 2>&1 | tail -5
if [ $? -ne 0 ]; then
    echo "❌ Failed to install OpenCode packages"
    exit 1
fi
echo "✓ Packages installed"
echo ""

# Step 3: Create necessary directories
echo "📁 Creating directories..."
mkdir -p ~/.config/opencode
mkdir -p ~/.local/share/opencode
mkdir -p /tmp/opencode-restore
echo "✓ Directories created"
echo ""

# Step 4: Decode and extract the auth bundle
echo "🔓 Extracting auth bundle..."
echo "$OPENCODE_AUTH_BUNDLE" | base64 -d | tar -xzf - -C /tmp/opencode-restore 2>&1
if [ $? -ne 0 ]; then
    echo "❌ Failed to extract auth bundle"
    rm -rf /tmp/opencode-restore
    exit 1
fi
echo "✓ Bundle extracted"
echo ""

# Step 5: Verify manifest.json exists
echo "✓ Verifying bundle integrity..."
if [ ! -f /tmp/opencode-restore/manifest.json ]; then
    echo "❌ Error: Invalid bundle - missing manifest.json"
    rm -rf /tmp/opencode-restore
    exit 1
fi
echo "✓ manifest.json found"
echo ""

# Step 6: Copy config files
echo "📋 Copying config files..."
if [ -d /tmp/opencode-restore/config/opencode ]; then
    cp -r /tmp/opencode-restore/config/opencode/* ~/.config/opencode/
    echo "✓ Config files copied"
else
    echo "⚠ Warning: No config files found in bundle"
fi
echo ""

# Step 7: Copy data files
echo "💾 Copying data files..."
if [ -d /tmp/opencode-restore/data/opencode ]; then
    cp -r /tmp/opencode-restore/data/opencode/* ~/.local/share/opencode/
    echo "✓ Data files copied"
else
    echo "⚠ Warning: No data files found in bundle"
fi
echo ""

# Step 7.5: Fix opencode config to remove problematic github plugin
echo "🔧 Fixing OpenCode configuration..."
if [ -f ~/.config/opencode/opencode.jsonc ]; then
    cat > ~/.config/opencode/opencode.jsonc << 'EOF'
{
  "plugin": [
    "opencode-antigravity-auth@1.2.8"
  ],
  "model": "google/antigravity-gemini-3-flash"
}
EOF
    echo "✓ Configuration updated (removed @opencode-ai/github plugin)"
else
    echo "⚠ Warning: opencode.jsonc not found, skipping config fix"
fi
echo ""

# Step 8: Clean up temporary directory
echo "🧹 Cleaning up..."
rm -rf /tmp/opencode-restore
echo "✓ Cleanup complete"
echo ""

# Step 9: Set OPENCODE_HEADLESS environment variable
echo "⚙️  Setting environment variables..."
export OPENCODE_HEADLESS=1
echo "export OPENCODE_HEADLESS=1" >> ~/.bashrc
echo "✓ OPENCODE_HEADLESS=1 set"
echo ""

# Step 10: Verify installation
echo "🔍 Verifying OpenCode installation..."
echo ""

echo "OpenCode version:"
opencode --version
if [ $? -ne 0 ]; then
    echo "❌ OpenCode verification failed"
    exit 1
fi
echo ""

echo "Checking auth status:"
opencode auth list 2>&1 | head -10
echo ""

echo "Available models:"
opencode models 2>&1 | head -20
if [ $? -ne 0 ]; then
    echo "⚠ Warning: Could not list models (may require auth configuration)"
else
    echo ""
    echo "✓ OpenCode is ready!"
    echo ""
    echo "Preferred model for testing: google/antigravity-gemini-3-flash"
fi

echo ""
echo "✅ Setup complete!"
echo ""
echo "You can now use OpenCode with commands like:"
echo "  opencode models"
echo "  opencode run --model google/antigravity-gemini-3-flash \"<prompt>\""
