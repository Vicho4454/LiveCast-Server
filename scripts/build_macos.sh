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

# 3. Preparación del paquete (PKG)
echo "-> Preparando estructura del instalador..."
PKG_ROOT="build/pkg_root"
chmod -R u+w "$PKG_ROOT" 2>/dev/null || true
rm -rf "$PKG_ROOT"
mkdir -p "$PKG_ROOT/Applications"
mkdir -p "$PKG_ROOT/Applications/Utilities"
mv "$APP_BUNDLE" "$PKG_ROOT/Applications/"

# Crear desinstalador en la carpeta Utilidades de macOS
cat << 'EOF' > "$PKG_ROOT/Applications/Utilities/Desinstalar LiveCast.command"
#!/bin/bash
echo "=== Desinstalando LiveCast Server ==="
echo "Por favor ingresa tu contraseña de administrador si se te solicita."
sudo rm -rf "/Applications/LiveCast Server.app"
sudo rm -f "/Applications/Utilities/Desinstalar LiveCast.command"
sudo sed -i '' '/com.livecast.server/d' /etc/pf.conf 2>/dev/null
sudo rm -f /etc/pf.anchors/com.livecast.server 2>/dev/null
sudo pfctl -f /etc/pf.conf 2>/dev/null || true
sudo pkgutil --forget "com.livecast.server" 2>/dev/null
echo "Desinstalación completa. Puedes cerrar esta ventana."
EOF
chmod +x "$PKG_ROOT/Applications/Utilities/Desinstalar LiveCast.command"

# 4. Inyección de script post-install (Firewall PF Rules y Quitar Cuarentena)
echo "-> Generando script de configuración de Firewall macOS..."
mkdir -p build/scripts
cat << 'EOF' > build/scripts/postinstall
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

# Eliminar cuarentena de Apple (Soluciona "Desarrollador no identificado")
xattr -cr "/Applications/LiveCast Server.app" || true

# Asegurar permisos de ejecución
chmod -R +x "/Applications/LiveCast Server.app/Contents/MacOS"
chmod +x "/Applications/Utilities/Desinstalar LiveCast.command"
exit 0
EOF
chmod +x build/scripts/postinstall

# 5. Creación del PKG final
echo "-> Generando PKG final..."
pkgbuild --root "$PKG_ROOT" \
         --identifier "com.livecast.server" \
         --version "1.0.2" \
         --scripts "build/scripts" \
         --install-location "/" \
         "build/bin/LiveCast-Server-Mac-ARM64.pkg"

echo "=== Empaquetado finalizado con éxito ==="
