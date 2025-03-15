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
TODO: add docker image

### build manually
```bash
go build 
```

# TODO
1. integrate github CI
1. release with docker image
1. support custom k8s api server address
1. support mutiple arch docker iamges