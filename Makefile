BINARY_NAME = deepthinking-ng
BUILD_FLAGS = -trimpath -ldflags="-buildid="
# Reference Go version for reproducible builds: 1.26.2
GO_VERSION = 1.26.2

.PHONY: build run-stdio run-sse docker-build test clean checksum verify docker-reproducible-build

build:
	@echo "Building binary locally..."
	CGO_ENABLED=0 go build $(BUILD_FLAGS) -o $(BINARY_NAME) .
	@echo "Binary SHA256:"
	@sha256sum $(BINARY_NAME) || shasum -a 256 $(BINARY_NAME)

docker-reproducible-build:
	@echo "Building binary inside Docker (Go $(GO_VERSION))..."
	docker build --target builder -t $(BINARY_NAME)-builder .
	@docker rm -f $(BINARY_NAME)-temp 2>/dev/null || true
	docker create --name $(BINARY_NAME)-temp $(BINARY_NAME)-builder
	docker cp $(BINARY_NAME)-temp:/app/$(BINARY_NAME) ./$(BINARY_NAME)
	docker rm $(BINARY_NAME)-temp
	@echo "Binary SHA256:"
	@sha256sum $(BINARY_NAME) || shasum -a 256 $(BINARY_NAME)

checksum: docker-reproducible-build
	sha256sum $(BINARY_NAME) > $(BINARY_NAME).sha256 || shasum -a 256 $(BINARY_NAME) > $(BINARY_NAME).sha256

verify: docker-reproducible-build
	@echo "Verifying build against reference checksum..."
	@if command -v sha256sum > /dev/null; then \
		sha256sum -c $(BINARY_NAME).sha256; \
	else \
		shasum -a 256 -c $(BINARY_NAME).sha256; \
	fi && echo "Verification SUCCESS: Build matches reference!" || (echo "Verification FAILURE: Build differs from reference!" && exit 1)

test:
	go test -v ./...

run-stdio: build
	./$(BINARY_NAME) --transport=stdio

run-sse: build
	./$(BINARY_NAME) --transport=sse --port=8080

docker-build:
	docker build -t mcp/$(BINARY_NAME) -f Dockerfile .

clean:
	rm -f $(BINARY_NAME) $(BINARY_NAME).sha256
