FROM golang:1.22 AS builder

WORKDIR /internal

COPY go.* .

RUN go mod download

