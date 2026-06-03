BINARY_NAME = deepthinking-ng
BUILD_FLAGS = -trimpath -ldflags="-buildid="
# Reference Go version for reproducible builds: 1.26.2
GO_VERSION = 1.26.2

.PHONY: build run-stdio run-sse docker-build test clean checksum verify docker-reproducible-build \
	build-windows build-darwin build-darwin-amd64 build-darwin-arm64 build-linux build-linux-amd64 build-linux-arm64 build-all

build:
	@echo "Building static binary locally..."
	CGO_ENABLED=0 GOEXPERIMENT=boringcrypto go build $(BUILD_FLAGS) -o $(BINARY_NAME) .
	@echo "Binary SHA256:"
	@sha256sum $(BINARY_NAME) || shasum -a 256 $(BINARY_NAME)

build-windows:
	@echo "Building for Windows (amd64)..."
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build $(BUILD_FLAGS) -o $(BINARY_NAME)-windows-amd64.exe .

build-darwin: build-darwin-amd64 build-darwin-arm64

build-darwin-amd64:
	@echo "Building for macOS (amd64)..."
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build $(BUILD_FLAGS) -o $(BINARY_NAME)-darwin-amd64 .

build-darwin-arm64:
	@echo "Building for macOS (arm64)..."
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build $(BUILD_FLAGS) -o $(BINARY_NAME)-darwin-arm64 .

build-linux: build-linux-amd64 build-linux-arm64

build-linux-amd64:
	@echo "Building for Linux (amd64)..."
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build $(BUILD_FLAGS) -o $(BINARY_NAME)-linux-amd64 .

build-linux-arm64:
	@echo "Building for Linux (arm64)..."
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build $(BUILD_FLAGS) -o $(BINARY_NAME)-linux-arm64 .

build-all: build-windows build-darwin build-linux

docker-reproducible-build:
	@echo "Building all binaries inside Docker (Go $(GO_VERSION))..."
	docker build --target builder -t $(BINARY_NAME)-builder .
	@docker rm -f $(BINARY_NAME)-temp 2>/dev/null || true
	docker create --name $(BINARY_NAME)-temp $(BINARY_NAME)-builder
	@mkdir -p dist
	docker cp $(BINARY_NAME)-temp:/app/checksums.txt ./dist/checksums.txt
	docker cp $(BINARY_NAME)-temp:/app/deepthinking-ng-linux-amd64 ./dist/
	docker cp $(BINARY_NAME)-temp:/app/deepthinking-ng-linux-arm64 ./dist/
	docker cp $(BINARY_NAME)-temp:/app/deepthinking-ng-windows-amd64.exe ./dist/
	docker cp $(BINARY_NAME)-temp:/app/deepthinking-ng-darwin-amd64 ./dist/
	docker cp $(BINARY_NAME)-temp:/app/deepthinking-ng-darwin-arm64 ./dist/
	# Also copy Linux binary to root for compatibility with other targets
	docker cp $(BINARY_NAME)-temp:/app/deepthinking-ng-linux-amd64 ./$(BINARY_NAME)
	docker rm $(BINARY_NAME)-temp
	@echo "Build complete. Binaries and checksums are in the 'dist' directory."
	@cat ./dist/checksums.txt

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
	rm -rf $(BINARY_NAME) $(BINARY_NAME)-* $(BINARY_NAME).sha256 dist
