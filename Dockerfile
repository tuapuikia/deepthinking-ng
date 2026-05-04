FROM golang:1.26.2-alpine AS builder

WORKDIR /app

# Copy go.mod and go.sum
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build for all platforms and generate checksums
RUN for platform in linux/amd64 linux/arm64 windows/amd64 darwin/amd64 darwin/arm64; do \
        os=$(echo $platform | cut -d/ -f1); \
        arch=$(echo $platform | cut -d/ -f2); \
        ext=""; [ "$os" = "windows" ] && ext=".exe"; \
        echo "Building $os/$arch..."; \
        CGO_ENABLED=0 GOOS=$os GOARCH=$arch go build -trimpath -ldflags="-buildid=" -o deepthinking-ng-$os-$arch$ext .; \
    done && \
    sha256sum deepthinking-ng-* > checksums.txt

# Create a symlink for the default binary used in the final stage
RUN ln -sf deepthinking-ng-linux-amd64 deepthinking-ng

FROM alpine:latest

WORKDIR /app

# Copy the binary from the builder stage
COPY --from=builder /app/deepthinking-ng /app/deepthinking-ng

# Expose port for SSE
EXPOSE 8080

# Set the entrypoint
ENTRYPOINT ["/app/deepthinking-ng"]
