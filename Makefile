.PHONY:
build:
	go fmt ./...
	go mod tidy
	go vet ./...
	go build .

.PHONY:
build-docker:
	docker buildx build --platform linux/amd64,linux/arm64 \
		-t xiaozongyang/kube-node-metrics:latest \
		--push .

.PHONY:
clean:
	rm kube-node-metrics