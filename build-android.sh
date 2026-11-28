#!/bin/bash
# Build APK Android (arm64) do sfrega — c-shared + empacotamento manual.
set -e
cd "$(dirname "$0")"

SDK=/mnt/Dados/android-sdk
NDK=$SDK/ndk/27.2.12479018
API=24
BT=$SDK/build-tools/36.0.0
AJ=$SDK/platforms/android-36/android.jar
TOOLCHAIN=$NDK/toolchains/llvm/prebuilt/linux-x86_64/bin
WORK=$(mktemp -d)
trap 'rm -rf "$WORK"' EXIT

echo "==> compilando libgio.so (arm64)"
GOOS=android GOARCH=arm64 CGO_ENABLED=1 \
  CC=$TOOLCHAIN/aarch64-linux-android$API-clang \
  go build -buildmode=c-shared -o "$WORK/libgio.so" ./cmd/sfrega

echo "==> compilando Java (org.gioui)"
mkdir -p "$WORK/src/org/gioui" "$WORK/classes" "$WORK/dex"
GIO=$(go env GOMODCACHE)/gioui.org@v0.10.3/app
cp "$GIO/Gio.java" "$GIO/GioActivity.java" "$GIO/GioView.java" "$WORK/src/org/gioui/"
cp android/OverlayService.java "$WORK/src/org/gioui/"
# SurfaceView translúcida: permite ver o wallpaper através da janela
sed -i 's|setBackgroundColor(Color.argb(0, 0, 0, 0));|setBackgroundColor(Color.argb(0, 0, 0, 0));\n\t\tgetHolder().setFormat(android.graphics.PixelFormat.TRANSLUCENT);|' "$WORK/src/org/gioui/GioView.java"
# GioView dentro de Service (overlay): os casts para Activity quebrariam — guarda instanceof
sed -i 's|((Activity) this.getContext()).setRequestedOrientation(id);|if (getContext() instanceof Activity) { ((Activity) getContext()).setRequestedOrientation(id); }|' "$WORK/src/org/gioui/GioView.java"
sed -i 's|Window window = ((Activity) this.getContext()).getWindow();|if (!(getContext() instanceof Activity)) { return; } Window window = ((Activity) this.getContext()).getWindow();|' "$WORK/src/org/gioui/GioView.java"
sed -i 's|Window window = ((Activity) context).getWindow();|if (!(context instanceof Activity)) { return; } Window window = ((Activity) context).getWindow();|' "$WORK/src/org/gioui/GioView.java"
# setHighRefreshRate: getDisplay() exige Context visual (Activity) — Service não tem
python3 - "$WORK/src/org/gioui/GioView.java" <<'PY'
import sys
p = sys.argv[1]
s = open(p).read()
old = "\t\tContext context = getContext();\n\t\tDisplay display = context.getDisplay();"
new = "\t\tContext context = getContext();\n\t\tif (!(context instanceof Activity)) { return; }\n\t\tDisplay display = context.getDisplay();"
assert old in s, "padrao nao achado"
open(p, "w").write(s.replace(old, new, 1))
PY
# pede permissão de mídia em runtime (Android 13+ / ≤12) + vigia ovl.flag p/ overlay
sed -i 's|setContentView(layer);|setContentView(layer);\n\t\tgetWindow().setBackgroundDrawable(new android.graphics.drawable.ColorDrawable(0)); layer.setBackgroundColor(0); view.setBackgroundColor(0);|' "$WORK/src/org/gioui/GioActivity.java"
sed -i 's|super.onCreate(state);|super.onCreate(state);\n\t\tif (android.os.Build.VERSION.SDK_INT >= 33) { requestPermissions(new String[]{"android.permission.READ_MEDIA_AUDIO", "android.permission.POST_NOTIFICATIONS"}, 1); } else { requestPermissions(new String[]{"android.permission.READ_EXTERNAL_STORAGE"}, 1); }\n\t\tnew android.os.Handler(android.os.Looper.getMainLooper()).post(new java.lang.Runnable() { public void run() { org.gioui.OverlayService.maybeStart(GioActivity.this); new android.os.Handler(android.os.Looper.getMainLooper()).postDelayed(this, 500); } });|' "$WORK/src/org/gioui/GioActivity.java"
javac --release 8 -nowarn -cp "$AJ" -d "$WORK/classes" "$WORK"/src/org/gioui/*.java
"$BT/d8" --lib "$AJ" --output "$WORK/dex" "$WORK"/classes/org/gioui/*.class

echo "==> manifest + resources"
cat > "$WORK/AndroidManifest.xml" <<'MANIFEST'
<?xml version="1.0" encoding="utf-8"?>
<manifest xmlns:android="http://schemas.android.com/apk/res/android"
    package="org.gioui.sfrega">
    <uses-sdk android:minSdkVersion="24" android:targetSdkVersion="36"/>
    <uses-permission android:name="android.permission.READ_EXTERNAL_STORAGE" android:maxSdkVersion="32"/>
    <uses-permission android:name="android.permission.READ_MEDIA_AUDIO"/>
    <uses-permission android:name="android.permission.POST_NOTIFICATIONS"/>
    <uses-permission android:name="android.permission.SYSTEM_ALERT_WINDOW"/>
    <uses-permission android:name="android.permission.FOREGROUND_SERVICE"/>
    <uses-permission android:name="android.permission.FOREGROUND_SERVICE_MEDIA_PLAYBACK"/>
    <application android:label="sfrega" android:icon="@mipmap/ic_launcher"
        android:theme="@style/SfregaTheme" android:debuggable="true">
        <activity android:name="org.gioui.GioActivity"
            android:exported="true"
            android:configChanges="orientation|screenSize|keyboardHidden|screenLayout|uiMode|smallestScreenSize|density|fontScale|locale|layoutDirection"
            android:launchMode="singleTask">
            <intent-filter>
                <action android:name="android.intent.action.MAIN"/>
                <category android:name="android.intent.category.LAUNCHER"/>
            </intent-filter>
        </activity>
        <service android:name="org.gioui.OverlayService" android:exported="false"
            android:foregroundServiceType="mediaPlayback"/>
    </application>
</manifest>
MANIFEST
for spec in "mipmap-mdpi 48" "mipmap-hdpi 72" "mipmap-xhdpi 96" "mipmap-xxhdpi 144" "mipmap-xxxhdpi 192"; do
  set -- $spec
  mkdir -p "$WORK/res/$1"
  magick assets/icon.png -resize ${2}x${2} "$WORK/res/$1/ic_launcher.png"
done
mkdir -p "$WORK/res/values"
cat > "$WORK/res/values/styles.xml" <<'STYLES'
<?xml version="1.0" encoding="utf-8"?>
<resources>
    <style name="SfregaTheme" parent="android:Theme.Translucent.NoTitleBar.Fullscreen">
        <item name="android:windowBackground">@null</item>
        <item name="android:colorBackground">@android:color/transparent</item>
        <item name="android:windowContentOverlay">@null</item>
        <item name="android:windowIsTranslucent">true</item>
        <item name="android:windowNoTitle">true</item>
        <item name="android:windowFullscreen">true</item>
        <item name="android:windowShowWallpaper">true</item>
    </style>
</resources>
STYLES
"$BT/aapt2" compile --dir "$WORK/res" -o "$WORK/res.zip"
"$BT/aapt2" link -o "$WORK/base.apk" -I "$AJ" --manifest "$WORK/AndroidManifest.xml" \
  "$WORK/res.zip"

echo "==> montando APK"
cp "$WORK/base.apk" sfrega.apk
python3 - "$WORK" <<'PY'
import sys, zipfile, os
w = sys.argv[1]
with zipfile.ZipFile('sfrega.apk', 'a', compression=zipfile.ZIP_STORED) as z:
    z.write(os.path.join(w, 'dex/classes.dex'), 'classes.dex')
    z.write(os.path.join(w, 'libgio.so'), 'lib/arm64-v8a/libgio.so')
PY

echo "==> alinhando e assinando"
"$BT/zipalign" -f 4 sfrega.apk sfrega-aligned.apk
KS=${ANDROID_KEYSTORE:-/tmp/debug.keystore}
if [ ! -f "$KS" ]; then
  keytool -genkeypair -keystore "$KS" -alias androiddebugkey -keyalg RSA \
    -keysize 2048 -validity 10000 -storepass android -keypass android \
    -dname "CN=Android Debug,O=Android,C=US" >/dev/null 2>&1
fi
"$BT/apksigner" sign --ks "$KS" --ks-pass pass:android --key-pass pass:android \
  --out sfrega.apk sfrega-aligned.apk
rm -f sfrega-aligned.apk
echo "OK: $(pwd)/sfrega.apk"
