#!/bin/bash

# Usage:
#   curl -sfL https://raw.githubusercontent.com/BoringDystopiaDevelopment/tmuxai/main/install.sh | bash
#   curl -sfL https://.../install.sh | bash -s -- -b /usr/local/bin # Install to custom bin dir
#   curl -sfL https://.../install.sh | bash -s v1.0.0               # Install specific version (Tag name)
#
set -euo pipefail


GH_REPO="BoringDystopiaDevelopment/tmuxai"
PROJECT_NAME="tmuxai"
DEFAULT_INSTALL_DIR="/usr/local/bin"

CONFIG_DIR="$HOME/.config/tmuxai"
CONFIG_FILE="$CONFIG_DIR/config.example.yaml"
EXAMPLE_CONFIG_URL="https://raw.githubusercontent.com/BoringDystopiaDevelopment/tmuxai/main/config.example.yaml"

tmp_dir=""

err() {
  echo "[ERROR] $*" >&2
  exit 1
}

info() {
  echo "$*"
}

# Checks if a command exists
command_exists() {
  command -v "$1" >/dev/null 2>&1
}

# --- Main Script Logic ---

main() {
  local version="" # Keep version local to main
  local install_dir="$DEFAULT_INSTALL_DIR" # Keep install_dir local to main

  while [ $# -gt 0 ]; do
    case $1 in
      -b | --bin-dir)
        install_dir="$2"
        shift 2
        ;;
      -V | --version)
        version="$2"
        shift 2
        ;;
      # Allow specifying version directly as the first argument (e.g., bash -s v1.0.0)
      v*)
        if [ -z "$version" ]; then # Only if version not already set by -V
            version="$1"
        fi
        shift
        ;;
      *)
        echo "Unknown argument: $1"
        # You could add a usage function here
        exit 1
        ;;
    esac
  done

  # Ensure the target installation directory exists
  mkdir -p "$install_dir" || err "Failed to create installation directory: $install_dir"

  # --- Check for tmux ---
  if ! command_exists tmux; then
    info "-----------------------------------------------------------"
    info "'tmux' command not found."
    info "tmuxai requires tmux to function."
    info "Please install tmux:"
    info "  On Debian/Ubuntu: sudo apt update && sudo apt install tmux"
    info "  On macOS (Homebrew): brew install tmux"
    info "  On Fedora: sudo dnf install tmux"
    info "  On Arch Linux: sudo pacman -S tmux"
    info "-----------------------------------------------------------"
    exit 1
  fi

  # --- Dependency Checks ---
  command_exists curl || err "'curl' is required but not installed."
  command_exists grep || err "'grep' is required but not installed."
  command_exists cut || err "'cut' is required but not installed."
  command_exists tar || err "'tar' is required but not installed."
  command_exists mktemp || err "'mktemp' is required but not installed."


  # --- Platform Detection ---
  # Keep these local as they are only used within main
  local os_raw os_lower arch archive_ext
  os_raw=$(uname -s)
  os_lower=$(echo "$os_raw" | tr '[:upper:]' '[:lower:]')
  arch=$(uname -m)

  case "$os_lower" in
    linux)
      archive_ext="tar.gz"
      ;;
    darwin)
      archive_ext="tar.gz"
      ;;
    mingw* | cygwin* | msys*)
      os_raw="Windows"
      archive_ext="zip"
      command_exists unzip || err "'unzip' is required for Windows assets."
      ;;
    *)
      err "Unsupported operating system: $os_raw"
      ;;
  esac

  case "$arch" in
    x86_64 | amd64)
      arch="amd64"
      ;;
    arm64 | aarch64)
      arch="arm64"
      ;;
    armv*)
      if [[ "$arch" == "armv7"* ]]; then
          arch="armv7"
      elif [[ "$arch" == "armv6"* ]]; then
          arch="armv6"
      else
          arch="arm"
      fi
      ;;
    *)
      err "Unsupported architecture: $arch"
      ;;
  esac

  local api_url release_data download_url asset_filename tag_name
  if [ -z "$version" ]; then
    api_url="https://api.github.com/repos/${GH_REPO}/releases/latest"
    release_data=$(curl -sfL "$api_url") || err "Failed to fetch latest release info from GitHub API."
    tag_name=$(echo "$release_data" | grep '"tag_name":' | cut -d'"' -f4)
    if [ -z "$tag_name" ]; then
      err "Could not determine latest release version tag."
    fi
    info "Latest version tag is: $tag_name"
  else
    tag_name="$version"
    api_url="https://api.github.com/repos/${GH_REPO}/releases/tags/${tag_name}"
    release_data=$(curl -sfL "$api_url") || err "Failed to fetch release info for tag $tag_name from GitHub API."
    if ! echo "$release_data" | grep -q "\"tag_name\": *\"$tag_name\""; then
       err "Release tag '$tag_name' not found or API error."
    fi
     info "Using specified version tag: $tag_name"
  fi

  asset_filename="${PROJECT_NAME}_${os_raw}_${arch}.${archive_ext}"

  download_url=$(echo "$release_data" | grep '"browser_download_url":' | grep -ioE "https://[^\"]+/${asset_filename}\"" | head -n 1 | sed 's/"$//')

  if [ -z "$download_url" ]; then
    err "Could not find download URL for asset: $asset_filename"
    info "Check the release assets at: https://github.com/${GH_REPO}/releases/tag/${tag_name}"
    info "Available assets detected:"
    echo "$release_data" | grep '"browser_download_url":' | cut -d'"' -f4
    exit 1
  fi

  # --- Download and Install ---
  tmp_dir=$(mktemp -d -t ${PROJECT_NAME}_install_XXXXXX)

  info "Downloading $asset_filename to $tmp_dir..."
  curl -sfLo "$tmp_dir/$asset_filename" "$download_url" || err "Download failed."

  pushd "$tmp_dir" > /dev/null
  case "$archive_ext" in
    tar.gz)
      tar -xzf "$asset_filename" || err "Failed to extract tar.gz archive."
      ;;
    zip)
      unzip -q "$asset_filename" || err "Failed to extract zip archive."
      ;;
    *)
      popd > /dev/null
      err "Unsupported archive extension: $archive_ext"
      ;;
  esac
  popd > /dev/null

  # Keep binary_path local
  local binary_path="$tmp_dir/$PROJECT_NAME"
  if [ ! -f "$binary_path" ]; then
     info "Binary not found at top level, searching subdirectories..."
     local found_binary=$(find "$tmp_dir" -maxdepth 2 -type f -name "$PROJECT_NAME" -print -quit)
     if [ -z "$found_binary" ]; then
        err "Could not find executable '$PROJECT_NAME' in the extracted archive."
     fi
     binary_path="$found_binary"
     info "Found executable in subdirectory: $binary_path"
  fi

  # --- Installation ---
  local target_path="$install_dir/$PROJECT_NAME"
  local sudo_cmd=""

  if [ -d "$install_dir" ] && [ ! -w "$install_dir" ] || { [ ! -e "$install_dir" ] && [ ! -w "$(dirname "$install_dir")" ]; }; then
      info "Write permission required for $install_dir or its parent. Using sudo."
      command_exists sudo || err "'sudo' command not found, but required to write to $install_dir. Please install sudo or choose a writable directory with -b option."
      sudo_cmd="sudo"
  fi

  if command_exists install; then
    $sudo_cmd install -m 755 "$binary_path" "$target_path" || err "Installation failed using 'install' command."
  else
    info "Command 'install' not found, using 'mv' and 'chmod'."
    $sudo_cmd mv "$binary_path" "$target_path" || err "Failed to move binary to $target_path."
    $sudo_cmd chmod 755 "$target_path" || err "Failed to make binary executable."
  fi

  # --- Verification and Completion ---
  info "Installed binary: $target_path"

  # Keep installed_version_output local
  local installed_version_output="N/A"
  if "$target_path" --version > /dev/null 2>&1; then
      installed_version_output=$("$target_path" --version)
  elif "$target_path" version > /dev/null 2>&1; then
      installed_version_output=$("$target_path" version)
  else
      installed_version_output="(version command failed or not supported by '$PROJECT_NAME')"
  fi
  info ""
  info "$installed_version_output"
  info ""

  # --- Install Configuration File ---
  mkdir -p "$CONFIG_DIR" || err "Failed to create configuration directory: $CONFIG_DIR"
  if curl -sfLo "$CONFIG_FILE" "$EXAMPLE_CONFIG_URL"; then
    info "Example configuration added to $CONFIG_FILE"
  fi

  case ":$PATH:" in
      *":$install_dir:"*)
          ;;
      *)
          info "Warning: '$install_dir' is not in your PATH."
          info "You may need to add it to your shell configuration (e.g., ~/.bashrc, ~/.zshrc):"
          info "  export PATH=\"\$PATH:$install_dir\""
          info "Then, restart your shell or run: source ~/.your_shell_rc"
          ;;
  esac
  info ""
  info "Post-installation setup:"
  info "  1. Create ${CONFIG_DIR}/config.yaml"
  info "  2. Add a minimal configuration like:"
  info "       models:"
  info "         primary:"
  info "           provider: openrouter  # openrouter, openai or azure"
  info "           model: anthropic/claude-haiku-4.5"
  info "           api_key: sk-your-api-key"
  info "  3. Launch tmuxai with: tmuxai"
  info ""
  info "See README.md for more details on configuring tmuxai."
}

main "$@"
