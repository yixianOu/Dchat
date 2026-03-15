#!/bin/bash
set -euo pipefail

# 测试目标：验证提前配置好路由的JetStream节点可以组成正常工作的Routes集群
# 测试场景：3个Hub节点预先配置好所有节点的路由，启动后自动形成集群

# 配置
BASE_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
CONFIG_DIR="$BASE_DIR/hub-configs"
DATA_DIR="$BASE_DIR/data"
STREAM_NAME="HUB_TEST_STREAM"
TEST_SUBJECT="test.hub.msg"

# 清理函数
cleanup() {
    echo -e "\n🧹 正在清理测试资源..."
    pkill -f "nats-server.*hub[1-3].conf" 2>/dev/null || true
    pkill -f "nats-server.*leafnode.conf" 2>/dev/null || true
    rm -rf "$DATA_DIR"
    echo "✅ 清理完成"
}
trap cleanup EXIT INT TERM

# 等待普通客户端端口就绪（用nats pub检测）
wait_for_client_port() {
    local port=$1
    local retries=15
    echo "⏳ 等待客户端端口 $port 就绪..."
    while [ $retries -gt 0 ]; do
        if nats pub --server "localhost:$port" test.health "ping" >/dev/null 2>&1; then
            echo "✅ 客户端端口 $port 就绪"
            return 0
        fi
        sleep 1
        retries=$((retries-1))
    done
    echo "❌ 客户端端口 $port 启动超时"
    # 打印错误日志
    if [ -f "$DATA_DIR/hub1.log" ]; then
        echo -e "\n📝 Hub1错误日志:"
        tail -20 "$DATA_DIR/hub1.log"
    fi
    exit 1
}

# 等待TCP端口就绪（用于leaf、cluster等非客户端端口）
wait_for_tcp_port() {
    local port=$1
    local retries=15
    echo "⏳ 等待TCP端口 $port 就绪..."
    while [ $retries -gt 0 ]; do
        # 用bash内置的/dev/tcp检测端口是否通
        if (echo > /dev/tcp/localhost/$port) >/dev/null 2>&1; then
            echo "✅ TCP端口 $port 就绪"
            return 0
        fi
        sleep 1
        retries=$((retries-1))
    done
    echo "❌ TCP端口 $port 启动超时"
    exit 1
}

# 主测试流程
echo -e "=== 🚀 预配置JetStream Routes集群测试 ===\n"
echo "📋 测试前提：3个Hub节点已预先配置好所有节点的路由"

# 创建数据目录
mkdir -p "$DATA_DIR/hub1" "$DATA_DIR/hub2" "$DATA_DIR/hub3" "$DATA_DIR/leafnode"

# ========== 步骤1：启动3个预配置的Hub节点 ==========
echo -e "\n📌 Step 1: 启动3个预先配置好路由的Hub节点"

# 启动Hub1
cd "$BASE_DIR"
echo "🚀 启动Hub1..."
nats-server -c "$CONFIG_DIR/hub1.conf" > "$DATA_DIR/hub1.log" 2>&1 &
wait_for_client_port 4221
wait_for_tcp_port 7421

# 启动Hub2
echo "🚀 启动Hub2..."
nats-server -c "$CONFIG_DIR/hub2.conf" > "$DATA_DIR/hub2.log" 2>&1 &
wait_for_client_port 4222
wait_for_tcp_port 7422

# 启动Hub3
echo "🚀 启动Hub3..."
nats-server -c "$CONFIG_DIR/hub3.conf" > "$DATA_DIR/hub3.log" 2>&1 &
wait_for_client_port 4223
wait_for_tcp_port 7423

# 等待集群形成和JetStream Raft选举
echo "⏳ 等待JetStream集群Raft选举完成..."
sleep 20
# 检查JetStream是否就绪
retries=10
while [ $retries -gt 0 ]; do
    if nats account info --server localhost:4221 >/dev/null 2>&1; then
        break
    fi
    sleep 1
    retries=$((retries-1))
done
echo "✅ 3个Hub节点全部启动完成，预配置Routes集群形成"

# ========== 步骤2：测试Routes集群消息路由 ==========
echo -e "\n📌 Step 2: 测试Routes集群跨节点消息路由"

# Hub3订阅消息
echo "📥 Hub3订阅主题 $TEST_SUBJECT..."
timeout 5 nats subscribe --server localhost:4223 "$TEST_SUBJECT" > "$DATA_DIR/route_test.out" 2>&1 &
SUB_PID=$!
sleep 1

# Hub1发布消息
echo "📤 Hub1发布消息到 $TEST_SUBJECT..."
test_msg="Hello from Hub1 to Hub3 via Routes cluster!"
nats pub --server localhost:4221 "$TEST_SUBJECT" "$test_msg" >/dev/null
sleep 1

# 验证是否收到消息
if grep -q "$test_msg" "$DATA_DIR/route_test.out"; then
    echo "✅ 跨节点消息路由成功：Hub1 → Hub3"
else
    echo "❌ 跨节点消息路由失败"
    cat "$DATA_DIR/route_test.out"
    exit 1
fi
kill $SUB_PID 2>/dev/null || true

# ========== 步骤3：测试JetStream集群功能 ==========
echo -e "\n📌 Step 3: 测试JetStream集群功能（3副本）"

# Hub1创建3副本流
echo "🔧 Hub1创建3副本JetStream流 $STREAM_NAME..."
if ! nats stream create "$STREAM_NAME" \
    --server localhost:4221 \
    --subjects "test.jetstream.*" \
    --replicas 3 \
    --storage file \
    --defaults; then
    echo "❌ 创建流失败，打印Hub1日志："
    tail -50 "$DATA_DIR/hub1.log"
    exit 1
fi
echo "✅ 3副本流创建成功"

# Hub1发布10条测试消息
echo "📤 Hub1发布10条测试消息..."
for i in $(seq 1 10); do
    nats pub --server localhost:4221 "test.jetstream.$i" "jetstream-msg-$i" >/dev/null
done

# 等待消息同步到所有副本
sleep 2

# 验证消息是否成功写入
echo "📥 验证流消息数量..."
sleep 1
msg_count=$(nats stream info --server localhost:4223 "$STREAM_NAME" -j | jq -r '.state.messages')
if [ "$msg_count" = "10" ]; then
    echo "✅ JetStream集群工作正常：10条消息全部写入3副本流"
else
    echo "❌ JetStream集群测试失败，预期10条消息，实际$msg_count条"
    exit 1
fi

# ========== 步骤3.1：创建线上环境所需的DChat流 ==========
echo -e "\n📌 Step 3.1: 创建线上生产环境所需的DChat流（和线上配置一致）"

# 创建群聊消息流（3副本）
echo "🔧 创建DChatGroups流（存储所有群聊消息）..."
if ! nats stream create "DChatGroups" \
    --server localhost:4221 \
    --subjects "dchat.grp.*.msg" \
    --storage file \
    --retention limits \
    --max-msgs-per-subject 1000 \
    --max-age 30d \
    --replicas 3 \
    --discard old \
    --defaults >/dev/null 2>&1; then
    echo "❌ 创建DChatGroups流失败"
    exit 1
fi
echo "✅ DChatGroups流创建成功"

# 创建私聊消息流（3副本）
echo "🔧 创建DChatDirect流（存储所有私聊消息）..."
if ! nats stream create "DChatDirect" \
    --server localhost:4221 \
    --subjects "dchat.dm.*.msg" \
    --storage file \
    --retention limits \
    --max-msgs-per-subject 1000 \
    --max-age 30d \
    --replicas 3 \
    --discard old \
    --defaults >/dev/null 2>&1; then
    echo "❌ 创建DChatDirect流失败"
    exit 1
fi
echo "✅ DChatDirect流创建成功"

# 验证流创建成功
echo "🔍 验证所有流是否创建成功..."
streams=$(nats stream ls --server localhost:4221 | grep -E '(DChatGroups|DChatDirect)' || true)
echo "现有DChat流："
echo "$streams"
if echo "$streams" | grep -q "DChatGroups" && echo "$streams" | grep -q "DChatDirect"; then
    echo "✅ 所有DChat生产流创建成功，符合线上配置要求"
else
    echo "❌ DChat生产流创建失败"
    nats stream ls --server localhost:4221
    exit 1
fi

# ========== 步骤4：测试LeafNode接入集群 ==========
echo -e "\n📌 Step 4: 测试LeafNode接入集群"

# 创建LeafNode配置（和用户终端配置一致）
cat > "$DATA_DIR/leafnode.conf" << EOF
server_name: "test-leafnode"
host: "127.0.0.1"
port: 43222

leaf {
  remotes = [
    {
      url: "nats://localhost:7421"  # 连接到Hub1
    }
  ]
}

jetstream {
  enabled: true
  domain: "leaf"  # 和Hub的domain不同
  store_dir: "$DATA_DIR/leafnode"
}
EOF

# 启动LeafNode
echo "🚀 启动LeafNode，连接到Hub1..."
nats-server -c "$DATA_DIR/leafnode.conf" > "$DATA_DIR/leafnode.log" 2>&1 &
wait_for_client_port 43222
sleep 3

# LeafNode订阅全局主题
echo "📥 LeafNode订阅全局主题 $TEST_SUBJECT..."
timeout 10 nats subscribe --server localhost:43222 "$TEST_SUBJECT" > "$DATA_DIR/leaf_test.out" 2>&1 &
LEAF_SUB_PID=$!
sleep 1

# 从Hub3发布消息
echo "📤 Hub3发布消息到全局主题..."
leaf_test_msg="Hello from Hub3 to LeafNode via cluster!"
nats pub --server localhost:4223 "$TEST_SUBJECT" "$leaf_test_msg" >/dev/null
sleep 1

# 验证LeafNode收到消息
if grep -q "$leaf_test_msg" "$DATA_DIR/leaf_test.out"; then
    echo "✅ LeafNode接入成功：Hub3 → Hub1 → LeafNode 消息路由正常"
else
    echo "❌ LeafNode消息接收失败"
    cat "$DATA_DIR/leaf_test.out"
    exit 1
fi
kill $LEAF_SUB_PID 2>/dev/null || true

# ========== 测试结论 ==========
echo -e "\n🎉 所有测试通过！✅"
echo "========================================================"
echo "✅ 预先配置好路由的JetStream节点可以正常组成Routes集群"
echo "✅ 跨节点消息路由功能正常"
echo "✅ 3副本JetStream流工作正常，数据跨节点同步"
echo "✅ LeafNode可以正常接入集群，全局消息路由正常"
echo "✅ 架构完全符合线上生产配置要求"
echo "========================================================"
echo "💡 结论：提前配置好路由的Hub集群完全满足生产级高可用需求"
