# README [Go To English Version](README.md)

本项目遵守 MIT 协议。

本项目灵感来源于 [kube-state-metrics](https://github.com/kubernetes/kube-state-metrics) 和 [kubectl](https://github.com/kubernetes/kubectl)。

本项目实现了一个暴露 kubernetes 节点资源指标的服务，目前暴露的指标有：
```
kube_node_metrics_cpu_request
kube_node_metrics_cpu_limit
kube_node_metrics_mem_request_bytes
kube_node_metrics_mem_limit_bytes
```

本项目暴露出的资源申请(request)和限制()g(limit)的指标值应该和 `kubectl describe node <node>` 命令的结果相同。

## 快速开始
本项目的 Docker 镜像可以在 [docker hub](URL_ADDRESS.docker.com/r/xiaozongyang/kube-node-metrics/tags) 上找到。
### 在 Minikube 上试用
请确保你已经安装了 [minikube](https://github.com/kubernetes/minikube)。

在你终端中执行下列命令：
```
minikube start
kubectl apply -f deploy.yaml
```

### 在 Kubernetes 上部署
```bash
kubectl apply -f deploy.yaml
```

## 构建
### 手动构建二进制
```bash
go build .
```

### 构建 docker 镜像
```bash
docker build . -t <image>:<tag>
```

## 运维
本项目也暴露了一个 `kube_node_metrics_last_full_sync_ok_time_seconds` ata 指标，用于表示最后一次全量同步成功的时间。 根据这个指标可以配置报警，例如通过 `time() - kube_node_metrics_last_full_sync_ok_time_seconds > 300` 来通知已经 5 分钟没有同步成功的情况。

## TODO
1. 支持暴露节点 k8s 节点标签，范围通过前缀指定