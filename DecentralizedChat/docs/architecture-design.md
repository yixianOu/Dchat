#### 3.1 Hub集群高可用
- 3+节点组成NATS Routes全互联集群，JetStream流配置3副本冗余，单节点故障无数据丢失
- **动态扩容**：支持在线添加新节点，零停机，Raft自动同步数据，业务完全无感知
- **容错能力**：3节点集群可容忍1台节点故障，5节点可容忍2台故障

#### 3.2 终端高可用
- LeafNode配置多个Hub地址，自动健康检查，故障秒切到其他可用Hub
- **配置示例**：
  ```json
  "leafnode": {
    "hub_urls": [
      "nats://hub1.example.com:7422",
      "nats://hub2.example.com:7422",
      "nats://hub3.example.com:7422"
    ]
  }
  ```

还需要测试：NSC Seed导入，hub routes集群