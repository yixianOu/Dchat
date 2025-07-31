# 去中心化聊天室需求与开发计划

## 设计

操作记录：

- 2025-07-22：修改cmd/LeafJWT/main.go，依次执行4条nsc命令并打印输出。
- 2025-07-21：修改cmd/LeafJWT/main.go，捕获并打印nsc命令(nsc push -a APP)的stdout输出。
- 2025-07-06：添加项目需求与开发计划

## Option

leafNode cluster无法热更新,考虑mcp server
中心化的frp(暴露cluster端口),发现靠tls公钥(携带公网cluster端口)
问题1:重启后映射的端口会变,需要重新发现(可以尝试frp的api查询和通配符公钥)
去中心化,但是各个集群内部可以广播自己的位置(群聊只需加入一个节点并订阅节点即可)

## TODO
1. 阅读leafnode源码
2. 先尝试frp的端口映射规则