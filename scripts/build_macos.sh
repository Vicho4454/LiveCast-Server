#!/bin/bash
# Script de empaquetado para macOS - LiveCast Server

echo "=== Iniciando compilación de LiveCast Server para macOS (ARM64) ==="

# 1. Compilación del binario Wails
echo "-> Compilando aplicación Wails..."
~/go/bin/wails build -platform darwin/arm64

# 2. Empaquetado del NDI SDK (Cumplimiento Legal)
echo "-> Inyectando libndi.dylib para cumplimiento de licencia..."
APP_BUNDLE="build/bin/LiveCast Server.app"
DYLIB_DEST="$APP_BUNDLE/Contents/Frameworks"

mkdir -p "$DYLIB_DEST"
cp "/Library/NDI SDK for Apple/lib/macOS/libndi.dylib" "$DYLIB_DEST/"

# Forzamos rpath en el binario para que busque la librería en Frameworks locales y no dependa del sistema global
install_name_tool -add_rpath "@executable_path/../Frameworks" "$APP_BUNDLE/Contents/MacOS/LiveCast Server"

# Apple Silicon requiere que se vuelva a firmar el binario si se modifica (install_name_tool rompe la firma original)
echo "-> Re-firmando binario modificado..."
codesign --force --deep --sign - "$APP_BUNDLE"

# 3. Inyección de script post-install (Firewall PF Rules)
echo "-> Generando script de configuración de Firewall macOS..."
mkdir -p build/scripts
cat << 'EOF' > build/scripts/postinstall.sh
#!/bin/bash
# Configuración silenciosa de Firewall (PF) en macOS
echo "Configurando puertos para NDI (5960-5980), mDNS (5353), RTMP (1935) y RTSP (8554)..."
cat << 'RUL' > /etc/pf.anchors/com.livecast.server
pass in proto tcp from any to any port { 1935, 8554, 5960:5980 }
pass in proto udp from any to any port { 5353, 5960:5980, 8554 }
RUL
grep -q "anchor \"com.livecast.server\"" /etc/pf.conf || echo "anchor \"com.livecast.server\"" >> /etc/pf.conf
grep -q "load anchor \"com.livecast.server\"" /etc/pf.conf || echo "load anchor \"com.livecast.server\" from \"/etc/pf.anchors/com.livecast.server\"" >> /etc/pf.conf
pfctl -f /etc/pf.conf
pfctl -E
EOF
chmod +x build/scripts/postinstall.sh

# 4. Creación del DMG final
echo "-> Generando DMG final..."
hdiutil create -volname "LiveCast Server" -srcfolder "$APP_BUNDLE" -ov -format UDZO build/bin/LiveCast-Server-Mac-ARM64.dmg

echo "=== Empaquetado finalizado con éxito ==="
