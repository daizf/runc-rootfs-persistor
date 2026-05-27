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

---

## NVIDIA GPU 支持

runc-rootfs-persist 通过环境变量 `RUNC_BINARY` 支持链式调用下游 runtime，
可无缝配合 nvidia-container-runtime 使用。

### 调用链

```
containerd → runc-rootfs-persist → nvidia-container-runtime → runc
               ↑ 修改 Root.Path        ↑ 注入 GPU prestart hook   ↑ 执行 hook
```

### containerd 配置

```toml
[plugins."io.containerd.grpc.v1.cri".containerd.runtimes.rootfs-persist]
  runtime_type = "io.containerd.runc.v2"
  pod_annotations = ["eki.rootfs-persist.enabled", "eki.rootfs-persist.volume-mapping"]
  container_annotations = ["eki.rootfs-persist.enabled", "eki.rootfs-persist.volume-mapping"]
  [plugins."io.containerd.grpc.v1.cri".containerd.runtimes.rootfs-persist.options]
    BinaryName = "/usr/local/bin/runc-rootfs-persist"
```

### 注入环境变量

使用 systemd drop-in 注入 `RUNC_BINARY` 环境变量：

```bash
mkdir -p /etc/systemd/system/containerd.service.d
cat > /etc/systemd/system/containerd.service.d/runc-binary.conf <<'EOF'
[Service]
Environment="RUNC_BINARY=/usr/bin/nvidia-container-runtime"
EOF

systemctl daemon-reload
systemctl restart containerd
```

不设置 `RUNC_BINARY` 时默认回退到调用 `runc`，不影响非 GPU 场景。
