FROM golang:1.21 AS builder

WORKDIR /internal

COPY go.* .

RUN go mod download

