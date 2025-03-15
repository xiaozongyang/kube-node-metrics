# Kube Node Metrics
[中文版](README-zh.md)

This project is inspired by [kube-state-metrics](https://github.com/kubernetes/kube-state-metrics) and [kubectl](https://github.com/kubernetes/kubectl).

This project is exposted kubernetes node resource metrics to prometheus. Following Metrics will be exposed.
```
kube_node_metrics_cpu_request
kube_node_metrics_cpu_limit
kube_node_metrics_mem_request_bytes
kube_node_metrics_mem_limit_bytes
```

These samples values should be equal to the result of `kubectl describe node <node>`.

## Getting started

### Try in Minikube
Please ensure you have [minikube](https://github.com/kubernetes/minikube) installed.

Run with following commands in your terminal:
```
minikube start
kubectl apply -f deploy.yaml
```

### Run in Kubernetes
```bash
kubectl apply -f deploy.yaml
```

## Build
### build binary manually
```bash
go build .
```

### build docker image
```bash
docker build . -t <image>:<tag>
```

## Operating
This exporter also exposed a metric `kube_node_metrics_last_full_sync_ok_time_seconds` which indicates the last full sync timestamp in seconds. You can create alert rule to monitor the exporter health with the expression `time() - kube_node_metrics_last_full_sync_ok_time_seconds > 300` and you will receive alert if the exporter is not sync sucessfully for 300 seconds.


## TODO
1. support exposed node labels match given pattern