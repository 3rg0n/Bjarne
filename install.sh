#!/bin/bash
# bjarne installer for macOS and Linux
# Usage: curl -fsSL https://raw.githubusercontent.com/3rg0n/bjarne/master/install.sh | bash

set -e

REPO="3rg0n/bjarne"
INSTALL_DIR="${BJARNE_INSTALL_DIR:-$HOME/.bjarne/bin}"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

info() { echo -e "${GREEN}[INFO]${NC} $1"; }
warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }
error() { echo -e "${RED}[ERROR]${NC} $1"; exit 1; }

# Get latest release version from GitHub
get_latest_version() {
    local version
    version=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name"' | sed -E 's/.*"([^"]+)".*/\1/')
    if [ -z "$version" ]; then
        error "Failed to get latest version from GitHub"
    fi
    echo "$version"
}

# Download and install bjarne
install_bjarne() {
    local os arch version version_num tarball download_url tmp_dir

    info "Detecting platform..."
    case "$(uname -s)" in
        Linux*)  os="linux" ;;
        Darwin*) os="darwin" ;;
        *)       error "Unsupported operating system: $(uname -s)" ;;
    esac
    case "$(uname -m)" in
        x86_64|amd64)  arch="amd64" ;;
        arm64|aarch64) arch="arm64" ;;
        *)             error "Unsupported architecture: $(uname -m)" ;;
    esac
    info "Platform: ${os}-${arch}"

    info "Fetching latest version..."
    version=$(get_latest_version)
    info "Latest version: $version"

    # goreleaser creates versioned tar.gz archives: bjarne_0.1.5_linux_amd64.tar.gz
    version_num="${version#v}"  # Remove 'v' prefix
    tarball="bjarne_${version_num}_${os}_${arch}.tar.gz"
    download_url="https://github.com/${REPO}/releases/download/${version}/${tarball}"
    info "Downloading: $tarball"

    # Create install directory
    mkdir -p "$INSTALL_DIR"

    # Download and extract
    tmp_dir=$(mktemp -d)
    if ! curl -fsSL "$download_url" -o "${tmp_dir}/${tarball}"; then
        rm -rf "$tmp_dir"
        error "Failed to download bjarne"
    fi

    # Extract and install
    tar -xzf "${tmp_dir}/${tarball}" -C "$tmp_dir"
    mv "${tmp_dir}/bjarne" "${INSTALL_DIR}/bjarne"
    chmod +x "${INSTALL_DIR}/bjarne"

    # Cleanup
    rm -rf "$tmp_dir"

    info "Installed bjarne to ${INSTALL_DIR}/bjarne"
}

# Add to PATH if needed
setup_path() {
    local shell_rc profile_updated=false

    # Check if already in PATH
    if command -v bjarne &> /dev/null; then
        info "bjarne is already in PATH"
        return
    fi

    # Check if install dir is in PATH
    if [[ ":$PATH:" == *":${INSTALL_DIR}:"* ]]; then
        info "${INSTALL_DIR} is already in PATH"
        return
    fi

    # Detect shell and config file
    case "$SHELL" in
        */zsh)  shell_rc="$HOME/.zshrc" ;;
        */bash)
            if [ -f "$HOME/.bash_profile" ]; then
                shell_rc="$HOME/.bash_profile"
            else
                shell_rc="$HOME/.bashrc"
            fi
            ;;
        */fish) shell_rc="$HOME/.config/fish/config.fish" ;;
        *)      shell_rc="" ;;
    esac

    if [ -n "$shell_rc" ] && [ -f "$shell_rc" ]; then
        # Check if already added
        if ! grep -q "\.bjarne/bin" "$shell_rc" 2>/dev/null; then
            echo "" >> "$shell_rc"
            echo "# bjarne" >> "$shell_rc"
            if [[ "$SHELL" == */fish ]]; then
                echo "set -gx PATH \$HOME/.bjarne/bin \$PATH" >> "$shell_rc"
            else
                echo 'export PATH="$HOME/.bjarne/bin:$PATH"' >> "$shell_rc"
            fi
            profile_updated=true
            info "Added ${INSTALL_DIR} to PATH in $shell_rc"
        fi
    fi

    echo ""
    if [ "$profile_updated" = true ]; then
        warn "Restart your terminal or run: source $shell_rc"
    else
        warn "Add ${INSTALL_DIR} to your PATH:"
        echo '  export PATH="$HOME/.bjarne/bin:$PATH"'
    fi
}

# Check for container runtime
check_container_runtime() {
    echo ""
    if command -v podman &> /dev/null; then
        info "Podman detected: $(podman --version)"
    elif command -v docker &> /dev/null; then
        info "Docker detected: $(docker --version)"
    else
        warn "No container runtime found!"
        echo ""
        echo "bjarne requires Podman or Docker for validation."
        echo ""
        case "$(uname -s)" in
            Linux*)
                echo "Install Podman:"
                echo "  Ubuntu/Debian: sudo apt-get install podman"
                echo "  Fedora/RHEL:   sudo dnf install podman"
                echo "  Arch:          sudo pacman -S podman"
                ;;
            Darwin*)
                echo "Install Podman:"
                echo "  brew install podman"
                echo "  podman machine init && podman machine start"
                ;;
        esac
    fi
}

# Main
main() {
    echo ""
    echo "  bjarne installer"
    echo "  AI-assisted C/C++ code generation with mandatory validation"
    echo ""

    install_bjarne
    setup_path
    check_container_runtime

    echo ""
    info "Installation complete!"
    echo ""
    echo "  Run 'bjarne' to start"
    echo "  Run 'bjarne --help' for options"
    echo ""
}

main
