#!/bin/bash

set -e

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

check_root() {
    if [ "$EUID" -ne 0 ]; then 
        log_error "请使用root用户或sudo运行此脚本"
        exit 1
    fi
}

detect_os() {
    if [ -f /etc/os-release ]; then
        . /etc/os-release
        OS=$ID
        VERSION=$VERSION_ID
    else
        log_error "无法检测操作系统类型"
        exit 1
    fi
    log_info "检测到系统: $OS $VERSION"
}

detect_arch() {
    ARCH=$(uname -m)
    case $ARCH in
        x86_64)
            ARCH="amd64"
            ;;
        aarch64|arm64)
            ARCH="arm64"
            ;;
        *)
            log_error "不支持的架构: $ARCH"
            exit 1
            ;;
    esac
    log_info "系统架构: $ARCH"
}

install_postgresql() {
    log_info "正在安装 PostgreSQL..."
    
    case $OS in
        ubuntu|debian)
            apt-get update
            apt-get install -y postgresql postgresql-contrib
            systemctl enable postgresql
            systemctl start postgresql
            ;;
        centos|rhel|rocky|almalinux)
            yum install -y postgresql-server postgresql-contrib
            postgresql-setup --initdb
            systemctl enable postgresql
            systemctl start postgresql
            ;;
        *)
            log_error "不支持的操作系统: $OS"
            exit 1
            ;;
    esac
    
    log_info "PostgreSQL 安装完成"
}

install_redis() {
    log_info "正在安装 Redis..."
    
    case $OS in
        ubuntu|debian)
            apt-get install -y redis-server
            systemctl enable redis-server
            systemctl start redis-server
            ;;
        centos|rhel|rocky|almalinux)
            yum install -y redis
            systemctl enable redis
            systemctl start redis
            ;;
        *)
            log_error "不支持的操作系统: $OS"
            exit 1
            ;;
    esac
    
    log_info "Redis 安装完成"
}

setup_database() {
    log_info "配置数据库..."
    
    read -p "请输入数据库名称 [mailflow]: " DB_NAME
    DB_NAME=${DB_NAME:-mailflow}
    
    read -p "请输入数据库用户名 [mailflow]: " DB_USER
    DB_USER=${DB_USER:-mailflow}
    
    read -sp "请输入数据库密码: " DB_PASSWORD
    echo ""
    
    if [ -z "$DB_PASSWORD" ]; then
        DB_PASSWORD=$(openssl rand -base64 12)
        log_info "生成的随机密码: $DB_PASSWORD"
    fi
    
    sudo -u postgres psql << EOF
CREATE DATABASE $DB_NAME;
CREATE USER $DB_USER WITH PASSWORD '$DB_PASSWORD';
GRANT ALL PRIVILEGES ON DATABASE $DB_NAME TO $DB_USER;
ALTER DATABASE $DB_NAME OWNER TO $DB_USER;
\c $DB_NAME
GRANT ALL ON SCHEMA public TO $DB_USER;
GRANT ALL PRIVILEGES ON ALL TABLES IN SCHEMA public TO $DB_USER;
GRANT ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA public TO $DB_USER;
\q
EOF
    
    log_info "数据库配置完成"
}

configure_app() {
    log_info "配置应用..."
    
    read -p "请输入安装目录 [/opt/mailflow]: " INSTALL_DIR
    INSTALL_DIR=${INSTALL_DIR:-/opt/mailflow}
    
    mkdir -p "$INSTALL_DIR"
    
    read -p "请输入服务端口 [8080]: " SERVER_PORT
    SERVER_PORT=${SERVER_PORT:-8080}
    
    CPU_COUNT=$(nproc)
    read -p "请输入Worker数量 [$CPU_COUNT]: " WORKER_COUNT
    WORKER_COUNT=${WORKER_COUNT:-$CPU_COUNT}
    
    read -p "请输入管理员用户名 [admin]: " ADMIN_USER
    ADMIN_USER=${ADMIN_USER:-admin}
    
    read -sp "请输入管理员密码: " ADMIN_PASSWORD
    echo ""
    
    if [ -z "$ADMIN_PASSWORD" ]; then
        ADMIN_PASSWORD=$(openssl rand -base64 12)
        log_info "生成的管理员密码: $ADMIN_PASSWORD"
    fi
    
    if [ -f "./bin/mailflow-linux-$ARCH" ]; then
        cp "./bin/mailflow-linux-$ARCH" "$INSTALL_DIR/mailflow"
        chmod +x "$INSTALL_DIR/mailflow"
        log_info "已复制可执行文件到 $INSTALL_DIR"
    elif [ -f "./mailflow" ]; then
        cp "./mailflow" "$INSTALL_DIR/mailflow"
        chmod +x "$INSTALL_DIR/mailflow"
        log_info "已复制可执行文件到 $INSTALL_DIR"
    else
        log_error "找不到可执行文件，请先运行 build.sh 编译"
        exit 1
    fi
    
    if [ -d "./web" ]; then
        cp -r ./web "$INSTALL_DIR/"
        log_info "已复制web目录"
    fi
    
    cat > "$INSTALL_DIR/config.yaml" << EOF
server:
  port: $SERVER_PORT

database:
  host: localhost
  port: 5432
  user: $DB_USER
  password: $DB_PASSWORD
  dbname: $DB_NAME
  sslmode: disable

redis:
  addr: localhost:6379
  password: ""
  db: 0

worker:
  count: $WORKER_COUNT

admin:
  username: $ADMIN_USER
  password: $ADMIN_PASSWORD
EOF
    
    log_info "配置文件已生成: $INSTALL_DIR/config.yaml"
}

create_service() {
    log_info "创建systemd服务..."
    
    cat > /etc/systemd/system/mailflow.service << EOF
[Unit]
Description=MailFlow SMTP Load Balancer
After=network.target postgresql.service redis.service
Wants=postgresql.service redis.service

[Service]
Type=simple
User=root
WorkingDirectory=$INSTALL_DIR
ExecStart=$INSTALL_DIR/mailflow
Restart=always
RestartSec=10
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target
EOF
    
    systemctl daemon-reload
    systemctl enable mailflow
    
    log_info "Systemd服务已创建"
}

start_service() {
    log_info "启动服务..."
    
    systemctl start mailflow
    
    if systemctl is-active --quiet mailflow; then
        log_info "MailFlow服务已启动"
    else
        log_error "服务启动失败，请查看日志: journalctl -u mailflow -f"
        exit 1
    fi
}

show_info() {
    local SERVER_IP=$(hostname -I | awk '{print $1}')
    
    echo ""
    echo "======================================"
    echo "  部署完成！"
    echo "======================================"
    echo ""
    echo "安装目录: $INSTALL_DIR"
    echo "配置文件: $INSTALL_DIR/config.yaml"
    echo ""
    echo "服务管理命令:"
    echo "  启动: systemctl start mailflow"
    echo "  停止: systemctl stop mailflow"
    echo "  重启: systemctl restart mailflow"
    echo "  状态: systemctl status mailflow"
    echo "  日志: journalctl -u mailflow -f"
    echo ""
    echo "管理后台访问地址:"
    echo "  http://$SERVER_IP:$SERVER_PORT/admin"
    echo "  http://localhost:$SERVER_PORT/admin"
    echo ""
    echo "管理员账号:"
    echo "  用户名: $ADMIN_USER"
    echo "  密码: $ADMIN_PASSWORD"
    echo ""
    echo "数据库信息:"
    echo "  数据库: $DB_NAME"
    echo "  用户: $DB_USER"
    echo "  密码: $DB_PASSWORD"
    echo ""
    echo "API地址:"
    echo "  http://$SERVER_IP:$SERVER_PORT/api/v1/send"
    echo ""
    echo "======================================"
    echo ""
}

build_app() {
    log_info "开始编译应用..."
    
    if ! command -v go &> /dev/null; then
        log_error "Go编译器未安装"
        log_info "请先安装Go: https://golang.org/dl/"
        exit 1
    fi
    
    detect_arch
    
    if [ ! -f "./cmd/server/main.go" ]; then
        log_error "未找到源码文件，请在项目根目录运行"
        exit 1
    fi
    
    mkdir -p ./bin
    
    log_info "编译 Linux $ARCH 版本..."
    CGO_ENABLED=0 GOOS=linux GOARCH=$ARCH go build \
        -ldflags "-s -w" \
        -o "./bin/mailflow-linux-$ARCH" \
        ./cmd/server
    
    if [ $? -eq 0 ]; then
        log_info "编译成功: ./bin/mailflow-linux-$ARCH"
        cp "./bin/mailflow-linux-$ARCH" "./mailflow"
        log_info "已复制到: ./mailflow"
    else
        log_error "编译失败"
        exit 1
    fi
}

main_menu() {
    echo ""
    echo "======================================"
    echo "  MailFlow 部署脚本"
    echo "======================================"
    echo ""
    echo "请选择操作:"
    echo "1) 完整安装 (编译+安装依赖+配置+部署)"
    echo "2) 仅安装依赖 (PostgreSQL + Redis)"
    echo "3) 仅配置和部署应用 (需要已编译)"
    echo "4) 编译应用"
    echo "5) 更新应用 (自动编译+更新)"
    echo "6) 卸载"
    echo "0) 退出"
    echo ""
    read -p "请输入选项 (0-6): " choice
    
    case $choice in
        1)
            build_app
            full_install
            ;;
        2)
            install_dependencies
            ;;
        3)
            deploy_only
            ;;
        4)
            build_app
            ;;
        5)
            build_app
            update_app
            ;;
        6)
            uninstall
            ;;
        0)
            exit 0
            ;;
        *)
            log_error "无效选项"
            exit 1
            ;;
    esac
}

full_install() {
    check_root
    detect_os
    detect_arch
    
    log_info "开始完整安装..."
    
    install_postgresql
    install_redis
    setup_database
    configure_app
    create_service
    start_service
    show_info
}

install_dependencies() {
    check_root
    detect_os
    
    log_info "开始安装依赖..."
    
    install_postgresql
    install_redis
    
    log_info "依赖安装完成"
}

deploy_only() {
    check_root
    detect_os
    detect_arch
    
    log_info "开始部署应用..."
    
    if ! command -v psql &> /dev/null; then
        log_error "PostgreSQL未安装，请先安装依赖"
        exit 1
    fi
    
    if ! command -v redis-cli &> /dev/null; then
        log_error "Redis未安装，请先安装依赖"
        exit 1
    fi
    
    setup_database
    configure_app
    create_service
    start_service
    show_info
}

update_app() {
    check_root
    detect_arch
    
    log_info "开始更新应用..."
    
    INSTALL_DIR="/opt/mailflow"
    
    if [ ! -d "$INSTALL_DIR" ]; then
        log_error "未找到安装目录: $INSTALL_DIR"
        exit 1
    fi
    
    log_info "停止服务..."
    systemctl stop mailflow
    
    if [ -f "$INSTALL_DIR/config.yaml" ]; then
        cp "$INSTALL_DIR/config.yaml" "$INSTALL_DIR/config.yaml.backup"
        log_info "已备份配置文件"
    fi
    
    if [ -f "$INSTALL_DIR/mailflow" ]; then
        cp "$INSTALL_DIR/mailflow" "$INSTALL_DIR/mailflow.backup"
        log_info "已备份旧版本"
    fi
    
    if [ -f "./bin/mailflow-linux-$ARCH" ]; then
        cp "./bin/mailflow-linux-$ARCH" "$INSTALL_DIR/mailflow"
        chmod +x "$INSTALL_DIR/mailflow"
        log_info "已更新可执行文件"
    elif [ -f "./mailflow" ]; then
        cp "./mailflow" "$INSTALL_DIR/mailflow"
        chmod +x "$INSTALL_DIR/mailflow"
        log_info "已更新可执行文件"
    else
        log_error "找不到可执行文件，请先运行编译"
        if [ -f "$INSTALL_DIR/mailflow.backup" ]; then
            mv "$INSTALL_DIR/mailflow.backup" "$INSTALL_DIR/mailflow"
        fi
        systemctl start mailflow
        exit 1
    fi
    
    if [ -d "./web" ]; then
        if [ -d "$INSTALL_DIR/web" ]; then
            rm -rf "$INSTALL_DIR/web.backup" 2>/dev/null
            mv "$INSTALL_DIR/web" "$INSTALL_DIR/web.backup"
        fi
        cp -r ./web "$INSTALL_DIR/"
        log_info "已更新web目录"
    fi
    
    if [ -f "$INSTALL_DIR/config.yaml.backup" ]; then
        mv "$INSTALL_DIR/config.yaml.backup" "$INSTALL_DIR/config.yaml"
        log_info "已恢复配置文件"
    fi
    
    log_info "启动服务..."
    systemctl start mailflow
    sleep 2
    
    if systemctl is-active --quiet mailflow; then
        log_info "应用更新完成并已重启"
        log_info "备份文件保留在: $INSTALL_DIR/*.backup"
    else
        log_error "服务启动失败，正在恢复备份..."
        systemctl stop mailflow
        
        if [ -f "$INSTALL_DIR/mailflow.backup" ]; then
            mv "$INSTALL_DIR/mailflow.backup" "$INSTALL_DIR/mailflow"
        fi
        
        if [ -d "$INSTALL_DIR/web.backup" ]; then
            rm -rf "$INSTALL_DIR/web"
            mv "$INSTALL_DIR/web.backup" "$INSTALL_DIR/web"
        fi
        
        systemctl start mailflow
        log_error "已恢复备份，请查看日志: journalctl -u mailflow -n 50"
        exit 1
    fi
}

uninstall() {
    check_root
    
    log_warn "此操作将删除MailFlow及其配置，但保留数据库数据"
    read -p "确认卸载? (yes/no): " CONFIRM
    
    if [ "$CONFIRM" != "yes" ]; then
        log_info "已取消"
        exit 0
    fi
    
    INSTALL_DIR="/opt/mailflow"
    
    if systemctl is-active --quiet mailflow; then
        systemctl stop mailflow
    fi
    
    if [ -f /etc/systemd/system/mailflow.service ]; then
        systemctl disable mailflow
        rm /etc/systemd/system/mailflow.service
        systemctl daemon-reload
        log_info "已删除systemd服务"
    fi
    
    if [ -d "$INSTALL_DIR" ]; then
        rm -rf "$INSTALL_DIR"
        log_info "已删除安装目录: $INSTALL_DIR"
    fi
    
    log_info "卸载完成"
    log_warn "数据库和Redis数据未删除，如需删除请手动操作"
}

main_menu

