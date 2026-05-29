# runc-rootfs-persist

将容器 rootfs 的可写层持久化到云盘 PV 上，Pod 删除重建后容器内所有文件变更不丢失。

## 原理

OCI Runtime Wrapper 包装真实 runc，在容器创建前将 overlay 的 upperdir（可写层）从本地磁盘重定向到云盘 PV：

```
容器 rootfs = overlay mount
├── lowerdir: 镜像层（只读）
├── upperdir: 云盘 PV 子目录  ← 所有写操作持久化
├── workdir:  云盘 PV 子目录
└── merged:   容器内 /
```

**优点**：不需要特权容器，不需要修改镜像，不依赖 NRI，不侵入 K8s 代码。

## 前置条件

- Kubernetes 集群（任意版本）
- containerd 运行时（任意 1.x 版本）
- CSI 云盘（支持 ReadWriteOnce PVC）
- 节点需加载 overlay 内核模块（`lsmod | grep overlay`）

## 快速开始

### 1. 构建

```bash
git clone <repo>
cd runc-rootfs-persist
make build          # 交叉编译 Linux amd64 二进制
```

产物在 `bin/runc-rootfs-persist`，静态链接，约 2MB。

### 2. 安装到节点

**方式 A：DaemonSet 分发**

```bash
make docker-build
# 将镜像推送到私有仓库
docker tag runc-rootfs-persist:latest <your-registry>/runc-rootfs-persist:latest
docker push <your-registry>/runc-rootfs-persist:latest
# 修改 deploy/daemonset.yaml 中的 image 地址
kubectl apply -f deploy/daemonset.yaml
```

**方式 B：手动安装**

```bash
scp bin/runc-rootfs-persist root@<node>:/usr/local/bin/
ssh root@<node> chmod +x /usr/local/bin/runc-rootfs-persist
```

### 3. 配置 containerd

在 `/etc/containerd/config.toml` 中添加：

```toml
[plugins."io.containerd.grpc.v1.cri".containerd.runtimes.runc-rootfs-persist]
  runtime_type = "io.containerd.runc.v2"
  pod_annotations = ["eki.rootfs-persist.enabled", "eki.rootfs-persist.volume-mapping"]
  container_annotations = ["eki.rootfs-persist.enabled", "eki.rootfs-persist.volume-mapping"]
  [plugins."io.containerd.grpc.v1.cri".containerd.runtimes.runc-rootfs-persist.options]
    BinaryName = "/usr/local/bin/runc-rootfs-persist"
```

> `pod_annotations` / `container_annotations` 是 annotation 白名单，每条均为必须，否则 Pod 自定义 annotation 不会被传递到 OCI spec 中。

重启 containerd：

```bash
systemctl restart containerd
```

### 4. 创建 RuntimeClass

```yaml
apiVersion: node.k8s.io/v1
kind: RuntimeClass
metadata:
  name: runc-rootfs-persist
handler: runc-rootfs-persist
```

### 5. 使用

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: my-app-persistent
  annotations:
    eki.rootfs-persist.enabled: "true"
    eki.rootfs-persist.volume-mapping: |-
      [
        {
          "containerName": "app",
          "mountPath": "/mnt/pv",
          "subPath": "app-rootfs"
        }
      ]
spec:
  runtimeClassName: runc-rootfs-persist
  containers:
  - name: app
    image: nginx:1.25
    volumeMounts:
    - name: root-pv
      mountPath: /mnt/pv
  volumes:
  - name: root-pv
    persistentVolumeClaim:
      claimName: ebs-pvc
```

## Annotation 说明

| Annotation | 必需 | 说明 |
|---|---|---|
| `eki.rootfs-persist.enabled` | 是 | `"true"` 启用 |
| `eki.rootfs-persist.volume-mapping` | 是 | JSON 数组，每个元素定义容器到 PV 的映射 |

`volume-mapping` 字段：

| 字段 | 必需 | 说明 |
|---|---|---|
| `containerName` | 是 | 对应 `spec.containers[].name` |
| `mountPath` | 是 | PV 在容器内的挂载点 |
| `subPath` | 否 | PV 上的存储子目录，默认取容器名 |

## 数据生命周期

```
Pod 创建 → wrapper 创建 overlay（upperdir 在 PV 上）
Pod 删除 → wrapper unmount overlay，PV 数据完整保留
Pod 重建 → wrapper 复用已有 upperdir，容器状态完全继承
PV 删除 → 数据真正清理
```

状态文件位于 `/var/lib/runc-rootfs-persist/<container-id>.json`，用于 delete 时清理 overlay 挂载点。

## 限制

1. 容器 rootfs 为 overlay-on-overlay（二层），IO 密集场景有轻微性能开销
2. 需在 containerd 配置中显式声明 annotation 白名单
3. 每个容器需独立 `subPath` 避免多容器写冲突
4. Pod 漂移到其他节点时，PV 数据跟随云盘移动，但镜像层需重新下载

## 项目结构

```
├── cmd/wrapper/main.go       # OCI runtime wrapper 入口
├── pkg/
│   ├── config/config.go      # annotation 常量与解析
│   ├── overlay/overlay.go    # overlay 挂载/卸载
│   └── state/state.go        # 运行时状态文件管理
├── deploy/
│   ├── daemonset.yaml        # 安装 DaemonSet
│   ├── example-pod.yaml      # 用户 Pod 示例
│   └── containerd-config.md  # containerd 配置说明
├── Dockerfile
├── Makefile
└── go.mod
```
