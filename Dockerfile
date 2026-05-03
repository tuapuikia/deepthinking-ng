FROM golang:1.26.2-alpine AS builder

WORKDIR /app

# Copy go.mod and go.sum
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the binary with reproducible flags
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-buildid=" -o deepthinking-ng .

FROM alpine:latest

WORKDIR /app

# Copy the binary from the builder stage
COPY --from=builder /app/deepthinking-ng /app/deepthinking-ng

# Expose port for SSE
EXPOSE 8080

# Set the entrypoint
ENTRYPOINT ["/app/deepthinking-ng"]
