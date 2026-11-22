#!/usr/bin/env bash
# Instala ferramentas de desenvolvimento (Linux/CachyOS).
# Uso: ./install-ferramentas.sh [--full]
set -euo pipefail

FULL=0
[ "${1:-}" = "--full" ] && FULL=1

# Pacotes base (Arch/pacman). Ajuste para sua distro se necessário.
pkgs=(git make base-devel)
if ! command -v go >/dev/null; then
  echo "Go não encontrado. Instale via: sudo pacman -S go"
  exit 1
fi

command -v git >/dev/null || sudo pacman -S --needed "${pkgs[@]}"

if [ "$FULL" -eq 1 ]; then
  # Android NDK (build de .aar/.apk Go) e gomobile
  go install golang.org/x/mobile/cmd/gomobile@latest
  go install golang.org/x/mobile/cmd/gobind@latest
  echo "Android: instale o Android SDK/NDK e exporte ANDROID_NDK_HOME."
  echo "iOS: requer macOS + Xcode (não compilável neste Linux)."
fi

echo "Ferramentas OK. go $(go version)"
