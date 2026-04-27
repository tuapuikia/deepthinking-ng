FROM golang:1.24-alpine AS builder

WORKDIR /app

# Copy go.mod and go.sum
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux go build -o deepthinking-ng .

FROM alpine:latest

WORKDIR /app

# Copy the binary from the builder stage
COPY --from=builder /app/deepthinking-ng /app/deepthinking-ng

# Expose port for SSE
EXPOSE 8080

# Set the entrypoint
ENTRYPOINT ["/app/deepthinking-ng"]
