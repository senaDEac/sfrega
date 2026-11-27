pkgname=sfrega
pkgver=1.0.0
pkgrel=1
pkgdesc="Player de áudio estilo Winamp (Gio)"
arch=('x86_64' 'aarch64' 'armv7h')
url="https://github.com/senaDEac/sfrega"
license=('custom:Proprietary')
depends=('glibc')
makedepends=('go')
source=("sfrega-${pkgver}.tar.gz")
sha256sums=('30c46dae7347038609db7697d9f25a9de4e52db6ca6ef3a837b6f975c15e01a6')

build() {
	cd "$srcdir/sfrega-${pkgver}"
	go build -ldflags="-s -w" -trimpath -o sfrega ./cmd/sfrega
}

package() {
	cd "$srcdir/sfrega-${pkgver}"

	install -Dm755 sfrega "$pkgdir/usr/bin/sfrega"

	install -Dm644 /dev/stdin "$pkgdir/usr/share/applications/sfrega.desktop" <<'EOF'
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

	if [ -f assets/icon.png ]; then
		for size in 16 32 48 64 128 256 512; do
			install -Dm644 assets/icon.png \
				"$pkgdir/usr/share/icons/hicolor/${size}x${size}/apps/sfrega.png"
		done
	fi
}
