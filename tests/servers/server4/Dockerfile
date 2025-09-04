# Build stage
FROM golang:1.24-alpine AS builder

WORKDIR /app

# Copy Go dependencies first to leverage Docker cache
COPY go.mod go.sum ./

RUN go mod download

# Copy the rest of the application source code
COPY *.go ./

RUN CGO_ENABLED=0 GOOS=linux go build -o /mcp-test-server4

# Runtime stage - intentionally problematic for testing
FROM alpine:latest

RUN apk --no-cache add ca-certificates

WORKDIR /root/

COPY --from=builder /mcp-test-server4 .

# Make the binary executable
RUN chmod +x ./mcp-test-server4

# Default to protocol failure mode (wrong protocol version)
ENV FAILURE_MODE=protocol

# Expose port (though some failure modes won't actually bind to it)
EXPOSE 9090

CMD ["./mcp-test-server4"]
