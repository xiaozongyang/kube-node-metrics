FROM golang:1.24.0 as builder

WORKDIR /workspace
ENV GOPROXY=https://goproxy.cn,direct

COPY . .

#### Build
RUN CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build .

CMD ["/workspace/kube-node-metrics"]