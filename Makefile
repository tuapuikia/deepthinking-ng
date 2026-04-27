.PHONY: build run-stdio run-sse docker-build

build:
	go build -o deepthinking-ng .

run-stdio: build
	./deepthinking-ng --transport=stdio

run-sse: build
	./deepthinking-ng --transport=sse --port=8080

docker-build:
	docker build -t mcp/deepthinking-ng -f Dockerfile .
