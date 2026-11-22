#!/usr/bin/env bash
# Instala o sfrega localmente: binário em ~/.local/bin + atalho no menu de apps.
# Uso: ./00-install/install-local.sh
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

echo "Compilando sfrega..."
go build -ldflags="-s -w" -o sfrega ./cmd/sfrega

mkdir -p "$HOME/.local/bin" "$HOME/.local/share/applications"
install -m 755 sfrega "$HOME/.local/bin/sfrega"

DESKTOP_FILE="$HOME/.local/share/applications/sfrega.desktop"
if [ -w "$(dirname "$DESKTOP_FILE")" ]; then
cat > "$DESKTOP_FILE" <<'EOF'
[Desktop Entry]
Type=Application
Name=sfrega
Comment=Player de áudio estilo Winamp
Exec=sfrega %U
Icon=multimedia-audio-player
Categories=AudioVideo;Audio;Player;
MimeType=audio/mpeg;audio/x-wav;audio/flac;audio/ogg;audio/mp4;audio/aac;audio/x-ms-wma;
Terminal=false
EOF
	command -v update-desktop-database >/dev/null && update-desktop-database "$HOME/.local/share/applications" || true
else
	echo "AVISO: ~/.local/share/applications não é gravável (dono root)."
	echo "Para o atalho no menu, rode: sudo chown $USER:$USER ~/.local/share/applications"
	echo "e execute este script novamente."
fi

echo "Instalado:"
echo "  binário  → ~/.local/bin/sfrega"
echo ""
echo "Uso: sfrega <arquivo|pasta>   |   sfrega --web <pasta>"
command -v update-desktop-database >/dev/null && update-desktop-database "$HOME/.local/share/applications" || true
