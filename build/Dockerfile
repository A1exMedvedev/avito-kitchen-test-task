# syntax=docker/dockerfile:1

FROM golang:1.26-alpine AS generator

ARG OAPI_CODEGEN_VERSION=v2.4.1
ARG PROTOC_GEN_GO_VERSION=v1.36.6
ARG PROTOC_GEN_GO_GRPC_VERSION=v1.5.1
ARG EASYP_VERSION=v0.16.6

RUN apk add --no-cache git bash

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go install github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@${OAPI_CODEGEN_VERSION} && \
    go install google.golang.org/protobuf/cmd/protoc-gen-go@${PROTOC_GEN_GO_VERSION} && \
    go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@${PROTOC_GEN_GO_GRPC_VERSION} && \
    go install github.com/easyp-tech/easyp/cmd/easyp@${EASYP_VERSION}

WORKDIR /src

COPY easyp.yaml ./
COPY api/ ./api/

RUN easyp generate && \
    mkdir -p api/gen/openapi/kitchenapi && \
    oapi-codegen -config api/openapi/codegen.yaml api/openapi/kitchen.yaml

FROM golang:1.26-alpine AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY . .
COPY --from=generator /src/api/gen ./api/gen

ARG TARGET=kitchen
ARG VERSION=dev

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build \
        -trimpath \
        -ldflags="-s -w -X main.version=${VERSION}" \
        -o /out/service ./cmd/${TARGET}

FROM alpine:3.21 AS runtime

RUN apk add --no-cache ca-certificates tzdata wget && \
    adduser -D -u 10001 app

COPY --from=builder /out/service /usr/local/bin/service

USER app

ENTRYPOINT ["/usr/local/bin/service"]
