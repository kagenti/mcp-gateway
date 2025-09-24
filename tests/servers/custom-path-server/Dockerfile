FROM golang:1.23-alpine AS builder
RUN apk --no-cache add git

WORKDIR /app
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN go build -o custom-path-server .

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/
COPY --from=builder /app/custom-path-server .
ENTRYPOINT ["./custom-path-server", "-http", ":8080"]
