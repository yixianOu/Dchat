#!/bin/bash
# P2P NAT穿透跨设备测试脚本
#
# 使用方式:
# 1. 在公网服务器上启动信令服务器:
#    go run signal_server.go -port 8080
#
# 2. 在设备A (局域网1) 上运行:
#    ./run_p2p_test.sh device-a Alice
#
# 3. 在设备B (局域网2) 上运行:
#    ./run_p2p_test.sh device-b Bob Alice
#
# 4. 两台设备会自动通过信令服务器交换地址并尝试P2P连接

set -e

# 配置
SIGNAL_SERVER="${SIGNAL_SERVER:-http://121.199.173.116:8080}"
NODE_ID=""
PEER_ID=""
MODE=""

# 颜色输出
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

print_help() {
    echo "P2P NAT穿透跨设备测试"
    echo ""
    echo "用法:"
    echo "  $0 <mode> [options]"
    echo ""
    echo "模式:"
    echo "  server              - 启动信令服务器"
    echo "  device-a <node-id>  - 启动设备A (等待连接)"
    echo "  device-b <node-id> <peer-id> - 启动设备B (连接到A)"
    echo ""
    echo "环境变量:"
    echo "  SIGNAL_SERVER - 信令服务器地址 (默认: $SIGNAL_SERVER)"
    echo ""
    echo "示例:"
    echo "  # 启动信令服务器"
    echo "  $0 server"
    echo ""
    echo "  # 设备A (内网192.168.1.x)"
    echo "  $0 device-a Alice"
    echo ""
    echo "  # 设备B (内网192.168.2.x)"
    echo "  $0 device-b Bob Alice"
}

log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# 检查依赖
check_dependencies() {
    if ! command -v go &> /dev/null; then
        log_error "未找到Go，请先安装Go"
        exit 1
    fi
}

# 启动信令服务器
start_server() {
    log_info "启动信令服务器..."
    log_info "地址: http://0.0.0.0:8080"
    log_info "按 Ctrl+C 停止"
    echo ""
    go run ./cmd/signal_server/main.go -port 8080
}

# 启动设备A
start_device_a() {
    NODE_ID=$1
    if [ -z "$NODE_ID" ]; then
        log_error "必须指定节点ID"
        exit 1
    fi

    log_info "启动设备A: $NODE_ID"
    log_info "信令服务器: $SIGNAL_SERVER"
    log_info ""
    log_info "等待设备B连接..."
    log_info "按 Ctrl+C 停止"
    echo ""

    go run ./cmd/p2p_node/main.go \
        -node-id "$NODE_ID" \
        -listen-port 10001 \
        -signal-server "$SIGNAL_SERVER"
}

# 启动设备B
start_device_b() {
    NODE_ID=$1
    PEER_ID=$2

    if [ -z "$NODE_ID" ] || [ -z "$PEER_ID" ]; then
        log_error "必须指定节点ID和对等节点ID"
        exit 1
    fi

    log_info "启动设备B: $NODE_ID"
    log_info "目标节点: $PEER_ID"
    log_info "信令服务器: $SIGNAL_SERVER"
    log_info ""
    log_info "将自动连接到 $PEER_ID"
    log_info "按 Ctrl+C 停止"
    echo ""

    go run ./cmd/p2p_node/main.go \
        -node-id "$NODE_ID" \
        -listen-port 10002 \
        -peer-id "$PEER_ID" \
        -signal-server "$SIGNAL_SERVER"
}

# 测试信令服务器连接
test_signal_server() {
    log_info "测试信令服务器连接..."
    if curl -s "$SIGNAL_SERVER/status" > /dev/null; then
        log_info "信令服务器连接正常"
    else
        log_warn "无法连接到信令服务器: $SIGNAL_SERVER"
        log_warn "请确保信令服务器已启动"
    fi
}

# 主逻辑
main() {
    if [ $# -lt 1 ]; then
        print_help
        exit 1
    fi

    MODE=$1
    shift

    check_dependencies

    case $MODE in
        server)
            start_server
            ;;
        device-a)
            test_signal_server
            start_device_a "$1"
            ;;
        device-b)
            test_signal_server
            start_device_b "$1" "$2"
            ;;
        help|-h|--help)
            print_help
            ;;
        *)
            log_error "未知模式: $MODE"
            print_help
            exit 1
            ;;
    esac
}

main "$@"
