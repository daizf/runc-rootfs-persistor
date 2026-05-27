## containerd 配置片段

### 添加 rootfs-persist runtime

在 `/etc/containerd/config.toml` 中添加：

```toml
[plugins."io.containerd.grpc.v1.cri".containerd.runtimes.rootfs-persist]
  runtime_type = "io.containerd.runc.v2"
  pod_annotations = ["eki.rootfs-persist.enabled", "eki.rootfs-persist.volume-mapping"]
  container_annotations = ["eki.rootfs-persist.enabled", "eki.rootfs-persist.volume-mapping"]
  [plugins."io.containerd.grpc.v1.cri".containerd.runtimes.rootfs-persist.options]
    BinaryName = "/usr/local/bin/runc-rootfs-persist"
```

重启 containerd：`systemctl restart containerd`

---

### 创建 RuntimeClass

```yaml
apiVersion: node.k8s.io/v1
kind: RuntimeClass
metadata:
  name: rootfs-persist
handler: rootfs-persist
```

### 用户 Pod 使用

```yaml
metadata:
  annotations:
    eki.rootfs-persist.enabled: "true"
    eki.rootfs-persist.volume-mapping: '[{"containerName":"app","mountPath":"/mnt/pv","subPath":"app-rootfs"}]'
spec:
  runtimeClassName: rootfs-persist
```
