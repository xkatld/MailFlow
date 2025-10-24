#!/bin/bash

set -e

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

log_info() { echo -e "${GREEN}[INFO]${NC} $1"; }
log_warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }
log_error() { echo -e "${RED}[ERROR]${NC} $1"; }

if [ "$EUID" -ne 0 ]; then 
    log_error "请使用root用户或sudo运行"
    exit 1
fi

INSTALL_DIR="/opt/mailflow"

if [ ! -d "$INSTALL_DIR" ]; then
    log_error "未找到安装目录: $INSTALL_DIR"
    log_error "请先部署应用"
    exit 1
fi

log_info "开始升级 MailFlow..."

ARCH=$(uname -m)
case $ARCH in
    x86_64) ARCH="amd64" ;;
    aarch64|arm64) ARCH="arm64" ;;
    *) log_error "不支持的架构: $ARCH"; exit 1 ;;
esac

log_info "编译新版本..."
CGO_ENABLED=0 GOOS=linux GOARCH=$ARCH go build -ldflags "-s -w" -o ./mailflow ./cmd/server

if [ $? -ne 0 ]; then
    log_error "编译失败"
    exit 1
fi

log_info "停止服务..."
systemctl stop mailflow

log_info "备份当前版本..."
cp "$INSTALL_DIR/mailflow" "$INSTALL_DIR/mailflow.backup.$(date +%Y%m%d_%H%M%S)"
cp "$INSTALL_DIR/config.yaml" "$INSTALL_DIR/config.yaml.backup.$(date +%Y%m%d_%H%M%S)"

log_info "更新文件..."
cp ./mailflow "$INSTALL_DIR/mailflow"
chmod +x "$INSTALL_DIR/mailflow"

rm -rf "$INSTALL_DIR/web.backup.$(date +%Y%m%d_%H%M%S)" 2>/dev/null || true
if [ -d "$INSTALL_DIR/web" ]; then
    mv "$INSTALL_DIR/web" "$INSTALL_DIR/web.backup.$(date +%Y%m%d_%H%M%S)"
fi
cp -r ./web "$INSTALL_DIR/"

log_info "启动服务..."
systemctl start mailflow
sleep 2

if systemctl is-active --quiet mailflow; then
    log_info "升级成功！"
    log_info "备份保留在: $INSTALL_DIR/*.backup.*"
else
    log_error "服务启动失败"
    log_error "查看日志: journalctl -u mailflow -n 50"
    exit 1
fi

