#!/bin/bash

set -e

# 解析参数
while [[ $# -gt 0 ]]; do
    case $1 in
        --bin)
            BIN_PATH="$2"
            shift 2
            ;;
        --arch)
            ARCH="$2"
            shift 2
            ;;
        --output)
            OUTPUT_DIR="$2"
            shift 2
            ;;
        *)
            echo "Unknown parameter: $1"
            exit 1
            ;;
    esac
done

# 检查必要参数
if [[ -z "$BIN_PATH" || -z "$ARCH" || -z "$OUTPUT_DIR" ]]; then
    echo "Usage: $0 --bin <binary_path> --arch <architecture> --output <output_dir>"
    exit 1
fi

# 创建IPK构建目录
IPK_DIR=$(mktemp -d)

# 创建必要的目录结构
mkdir -p "$IPK_DIR/usr/bin"
mkdir -p "$IPK_DIR/DEBIAN"

# 复制二进制文件
cp "$BIN_PATH" "$IPK_DIR/usr/bin/"

# 创建控制文件
cat > "$IPK_DIR/DEBIAN/control" << EOF
Package: homenetsentinel
Version: 1.0.0
Section: utils
Priority: optional
Architecture: $ARCH
Maintainer: HomeNetSentinel
Description: Home Network Sentinel - MQTT status monitor
EOF

# 构建IPK
mkdir -p "$OUTPUT_DIR"
ipkg-build -o "$OUTPUT_DIR" "$IPK_DIR"

# 清理临时目录
rm -rf "$IPK_DIR"

echo "IPK built successfully: $OUTPUT_DIR/homenetsentinel_1.0.0_${ARCH}.ipk"
