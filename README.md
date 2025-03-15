# Kube Node Metrics

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
```
minikube start
kubectl apply -f deploy.yaml
```

### Run in Kubernetes
```bash
kubectl apply -f deploy.yaml
```

### build manually
```bash
go build .
```

### build docker image
```bash
docker build . -t <image>:<tag>
```

# TODO
1. support custom k8s api server address
1. support exposed node labels match given pattern