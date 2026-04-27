.PHONY: build run-stdio run-sse docker-build test clean

build:
	go build -o deepthinking-ng .

test:
	go test -v ./...

run-stdio: build
	./deepthinking-ng --transport=stdio

run-sse: build
	./deepthinking-ng --transport=sse --port=8080

docker-build:
	docker build -t mcp/deepthinking-ng -f Dockerfile .

clean:
	rm -f deepthinking-ng
