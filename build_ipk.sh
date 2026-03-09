#!/usr/bin/env sh
set -eu

PKG_NAME="luci-app-homenet-sentinel"
PKG_VERSION="${PKG_VERSION:-1.0.0}"
PKG_RELEASE="${PKG_RELEASE:-1}"
PKG_ARCH="${PKG_ARCH:-all}"
PKG_FULL_VERSION="${PKG_VERSION}-${PKG_RELEASE}"

ROOT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
PKG_SRC_DIR="$ROOT_DIR/package/luci-app-homenet-sentinel"
FILES_DIR="$PKG_SRC_DIR/files"
PREBUILT_BIN="$PKG_SRC_DIR/prebuilt/HomeNet-Sentinel"
IPKG_BUILD_BIN="ipkg-build"

# 修复：默认输出目录改为当前目录的 ipk 文件夹（和 Action 对齐）
BUILD_ROOT="$ROOT_DIR/.ipkbuild"
PKG_DIR="$BUILD_ROOT/${PKG_NAME}"
CONTROL_DIR="$PKG_DIR/CONTROL"
OUT_DIR="$ROOT_DIR/ipk"  # 关键：默认和 Action 传入的路径一致

# 用 sh 兼容的参数解析
while [ "$#" -gt 0 ]; do
  case "$1" in
    --ipkg-build)
      if [ "$#" -ge 2 ]; then
        IPKG_BUILD_BIN="$2"
        shift 2
      else
        echo "ERROR: --ipkg-build requires a path argument" >&2
        exit 1
      fi
      ;;
    --output|-o)
      if [ "$#" -ge 2 ]; then
        # 修复：将传入的路径转为绝对路径
        OUT_DIR="$(cd "$2" && pwd)"
        shift 2
      else
        echo "ERROR: --output requires a directory argument" >&2
        exit 1
      fi
      ;;
    --bin)
      if [ "$#" -ge 2 ]; then
        PREBUILT_BIN="$2"
        shift 2
      else
        echo "ERROR: --bin requires a binary path argument" >&2
        exit 1
      fi
      ;;
    --arch)
      if [ "$#" -ge 2 ]; then
        PKG_ARCH="$2"
        shift 2
      else
        echo "ERROR: --arch requires an architecture argument" >&2
        exit 1
      fi
      ;;
    --version)
      if [ "$#" -ge 2 ]; then
        PKG_FULL_VERSION="$2"
        shift 2
      else
        echo "ERROR: --version requires a version string" >&2
        exit 1
      fi
      ;;
    --help|-h)
      cat <<'EOF'
Usage: ./build_ipk.sh [options]

Options:
  --bin <path>          Use specific HomeNet-Sentinel binary
  --arch <arch>         Package architecture (e.g. x86_64)
  --version <ver-rel>   Package version-release (e.g. 1.0.3-0)
  --ipkg-build <path>   Use specific ipkg-build binary/script
  --output, -o <dir>    Output directory for .ipk
  --help, -h            Show this help

Environment overrides:
  PKG_VERSION, PKG_RELEASE, PKG_ARCH
EOF
      exit 0
      ;;
    *)
      echo "ERROR: Unknown argument: $1" >&2
      echo "Try: ./build_ipk.sh --help" >&2
      exit 1
      ;;
  esac
done

# 验证 ipkg-build
if [ "$IPKG_BUILD_BIN" = "ipkg-build" ]; then
  if ! command -v ipkg-build >/dev/null 2>&1; then
    echo "ERROR: ipkg-build not found in PATH" >&2
    exit 1
  fi
else
  if [ ! -f "$IPKG_BUILD_BIN" ]; then
    echo "ERROR: ipkg-build not found: $IPKG_BUILD_BIN" >&2
    exit 1
  fi
fi

# 验证二进制文件
if [ ! -f "$PREBUILT_BIN" ]; then
  echo "ERROR: Missing binary: $PREBUILT_BIN" >&2
  ls -lah "$(dirname "$PREBUILT_BIN")" || true
  exit 1
fi

# 清理旧构建目录
rm -rf "$PKG_DIR"
# 确保输出目录存在（关键修复）
mkdir -p "$CONTROL_DIR" "$OUT_DIR"

# 复制 LuCI 文件
if [ -d "$FILES_DIR" ]; then
  cp -a "$FILES_DIR/." "$PKG_DIR/"
else
  echo "WARNING: No files directory found at $FILES_DIR" >&2
fi

# 安装二进制文件
mkdir -p "$PKG_DIR/usr/bin"
cp "$PREBUILT_BIN" "$PKG_DIR/usr/bin/HomeNet-Sentinel"
chmod 0755 "$PKG_DIR/usr/bin/HomeNet-Sentinel"

# 确保 init 脚本可执行
if [ -f "$PKG_DIR/etc/init.d/homenet-sentinel" ]; then
  chmod 0755 "$PKG_DIR/etc/init.d/homenet-sentinel"
fi

# 生成 CONTROL 文件
cat > "$CONTROL_DIR/control" <<EOF
Package: $PKG_NAME
Version: $PKG_FULL_VERSION
Architecture: $PKG_ARCH
Maintainer: HomeNet Sentinel
Section: net
Priority: optional
Depends: libc, libpthread, luci-base
Description: HomeNet Sentinel service with LuCI UI for MQTT presence and WAN telemetry.
EOF

# 生成 postinst
cat > "$CONTROL_DIR/postinst" <<'EOF'
#!/bin/sh
exit 0
EOF
chmod 0755 "$CONTROL_DIR/postinst"

# 生成 prerm
cat > "$CONTROL_DIR/prerm" <<'EOF'
#!/bin/sh
if [ -x /etc/init.d/homenet-sentinel ]; then
    /etc/init.d/homenet-sentinel stop >/dev/null 2>&1 || true
    /etc/init.d/homenet-sentinel disable >/dev/null 2>&1 || true
fi
exit 0
EOF
chmod 0755 "$CONTROL_DIR/prerm"

# 生成 conffiles（如果存在）
if [ -f "$PKG_DIR/etc/config/homenet-sentinel" ]; then
  cat > "$CONTROL_DIR/conffiles" <<'EOF'
/etc/config/homenet-sentinel
EOF
fi

# 构建 IPK（关键：确保 OUT_DIR 是绝对路径）
echo "Building IPK to $OUT_DIR..."
if [ "$IPKG_BUILD_BIN" = "ipkg-build" ]; then
  ipkg-build "$PKG_DIR" "$OUT_DIR"
elif [ -x "$IPKG_BUILD_BIN" ]; then
  "$IPKG_BUILD_BIN" "$PKG_DIR" "$OUT_DIR"
else
  bash "$IPKG_BUILD_BIN" "$PKG_DIR" "$OUT_DIR"
fi

echo
echo "Done. Output:"
ls -lh "$OUT_DIR"/*.ipk || true
