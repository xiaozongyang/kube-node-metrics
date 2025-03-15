FROM --platform=$BUILDPLATFORM golang:1.24.0 as builder

WORKDIR /app
ENV GOPROXY=https://goproxy.cn,direct

COPY go.mod go.sum ./
RUN go mod download

COPY . .


ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=linux GOARCH=$TARGETARCH go build -o kube-node-metrics .

FROM scratch
WORKDIR /
COPY --from=builder /app/kube-node-metrics .

EXPOSE 19191

CMD ["/kube-node-metrics"]