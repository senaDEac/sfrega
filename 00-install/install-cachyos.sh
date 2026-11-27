#!/usr/bin/env bash
# Instala o sfrega no sistema (CachyOS/Arch): binário em /usr/bin,
# atalho em /usr/share/applications e ícone em /usr/share/icons/hicolor.
# Uso: sudo ./00-install/install-cachyos.sh
set -euo pipefail

if [ "$(id -u)" -ne 0 ]; then
	echo "ERRO: rode com sudo (instala em /usr)." >&2
	exit 1
fi

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

echo "Compilando sfrega..."
go build -ldflags="-s -w" -o sfrega ./cmd/sfrega

echo "Instalando binário..."
install -m 755 sfrega /usr/bin/sfrega

echo "Instalando atalho..."
cat > /usr/share/applications/sfrega.desktop <<'EOF'
[Desktop Entry]
Type=Application
Name=sfrega
Comment=Player de áudio estilo Winamp
Exec=sfrega %U
Icon=sfrega
Categories=AudioVideo;Audio;Player;
MimeType=audio/mpeg;audio/x-wav;audio/flac;audio/ogg;audio/mp4;audio/aac;audio/x-ms-wma;
Terminal=false
StartupNotify=true
Version=1.0
EOF

if [ -f "$ROOT/assets/icon.png" ]; then
	echo "Instalando ícone..."
	for size in 16 32 48 64 128 256 512; do
		dir="/usr/share/icons/hicolor/${size}x${size}/apps"
		mkdir -p "$dir"
		install -m 644 "$ROOT/assets/icon.png" "$dir/sfrega.png"
	done
	gtk-update-icon-cache -f -t /usr/share/icons/hicolor 2>/dev/null || true
else
	echo "AVISO: assets/icon.png não encontrado; .desktop usa Icon=sfrega sem imagem."
fi

update-desktop-database /usr/share/applications 2>/dev/null || true

echo "Instalado: sfrega → /usr/bin/sfrega"
echo "Uso: sfrega <arquivo|pasta>   |   sfrega --web"
