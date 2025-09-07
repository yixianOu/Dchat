#!/bin/bash

# DChat 桌面应用启动脚本 - 本地版本
# 尝试使用现有的系统库启动桌面应用

set -e

echo "🖥️  DChat 桌面应用启动（本地模式）"
echo "=========================="

cd /home/orician/workspace/learn/nats/Dchat/DecentralizedChat

# 检查当前目录
if [ ! -f "wails.json" ]; then
    echo "❌ 错误：不在 Wails 项目目录中"
    exit 1
fi

# 设置环境变量绕过依赖检查
# export CGO_ENABLED=1
# export PKG_CONFIG_PATH="/usr/lib/x86_64-linux-gnu/pkgconfig:/usr/share/pkgconfig"

echo "📦 当前 PKG_CONFIG_PATH: $PKG_CONFIG_PATH"

# 检查可用的包
echo "🔍 检查可用的库..."
pkg-config --list-all | grep -E "(gtk|glib|webkit)" | head -5

# 方案1: 尝试直接构建
echo "🚀 尝试直接构建桌面应用..."

# 临时修改 wails.json 减少依赖
# cp wails.json wails.json.backup

# 生成绑定
echo "📁 生成 TypeScript 绑定..."
wails generate bindings

# 尝试编译
echo "🔨 使用 Wails 构建桌面应用..."
wails build

if [ $? -eq 0 ]; then
    echo "✅ Wails 桌面应用构建成功！"
    
    # 启动应用
    echo "🎯 启动桌面应用..."
    ./build/bin/DecentralizedChat
    
else
    echo "⚠️  Wails 构建失败，尝试开发模式..."
    
    # 回退到开发模式但指定参数
    echo "🌐 启动开发服务器（桌面模式）..."
    wails dev --loglevel Info --devserver "http://localhost:5173" --frontend "http://localhost:5173"
fi

# 恢复备份
# mv wails.json.backup wails.json
