.PHONY: build export clean

DIST_DIR := dist

# 编译所有二进制文件并构建Docker镜像
build:
	@echo "创建dist目录..."
	@mkdir -p $(DIST_DIR)
	@echo "编译信令服务器（静态链接）..."
	CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o $(DIST_DIR)/signal_server ./cmd/signal_server/main.go
	@echo "编译P2P节点客户端（静态链接）..."
	CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o $(DIST_DIR)/p2p_node ./cmd/p2p_node/main.go
	@echo "构建信令服务器Docker镜像..."
	docker build -f Dockerfile.server -t p2p-signal-server:latest .
	@echo "构建P2P节点Docker镜像..."
	docker build -f Dockerfile.client -t p2p-node:latest .
	@echo "✓ 构建完成"

# 导出Docker镜像为tar包
export:
	@echo "创建dist目录..."
	@mkdir -p $(DIST_DIR)
	@echo "导出信令服务器镜像..."
	docker save p2p-signal-server:latest -o $(DIST_DIR)/p2p-signal-server.tar
	@echo "导出P2P节点镜像..."
	docker save p2p-node:latest -o $(DIST_DIR)/p2p-node.tar
	@echo "✓ 导出完成: $(DIST_DIR)/p2p-signal-server.tar, $(DIST_DIR)/p2p-node.tar"

# 清理编译产物
clean:
	@echo "清理编译产物..."
	rm -rf $(DIST_DIR)
	@echo "✓ 清理完成"
