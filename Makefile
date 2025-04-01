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
build-docker-release:
	$(eval TAG := $(shell git tag --points-at HEAD))
	docker buildx build --platform linux/amd64,linux/arm64 \
		-t xiaozongyang/kube-node-metrics:$(TAG) \
		--push .

.PHONY:
clean:
	rm kube-node-metrics