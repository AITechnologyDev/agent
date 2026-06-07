#!/data/data/com.termux/files/usr/bin/bash

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

echo -e "${GREEN}► Installing TermAgent for Termux (ARM64)${NC}"

# Update packages and install pre-compiled Go binaries (to avoid hours of compilation on ARM)
echo -e "${YELLOW}● Installing dependencies (golang-bin, git)...${NC}"
pkg update -y && pkg install -y golang git

if [ $? -ne 0 ]; then
    echo -e "${RED}✖ Dependency installation failed.${NC}"
    exit 1
fi

# Create configuration directory (XDG standard)
CONFIG_DIR="$HOME/.config/agent"
mkdir -p "$CONFIG_DIR"

# Create default config if it doesn't exist
if [ ! -f "$CONFIG_DIR/config.json" ]; then
    echo -e "${YELLOW}● Creating default config...${NC}"
    cat << 'EOF' > "$CONFIG_DIR/config.json"
{
  "api_url": "http://localhost:1234/v1/chat/completions",
  "model_name": "qwen2.5-coder-7b-instruct",
  "api_key": ""
}
EOF
    echo -e "${GREEN}✓ Config created: $CONFIG_DIR/config.json${NC}"
else
    echo -e "${GREEN}✓ Config already exists, skipping.${NC}"
fi

# Compilation
echo -e "${YELLOW}● Compiling agent (native ARM64)...${NC}"
# The -ldflags="-s -w" flag strips debug information. The binary will be 2x smaller!
go build -ldflags="-s -w" -o "$PREFIX/bin/agent"

if [ $? -eq 0 ]; then
    echo -e "${GREEN}✓ Successfully compiled!${NC}"
    echo -e "${GREEN}► Run in terminal: termagent${NC}"
else
    echo -e "${RED}✖ Compilation failed.${NC}"
    exit 1
fi