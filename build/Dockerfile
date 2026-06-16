# syntax=docker/dockerfile:1

# ===== Builder =====
FROM golang:1.26-alpine AS builder

ARG TARGETOS=linux
ARG TARGETARCH=amd64

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# 静态编译,支持跨架构
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags="-s -w" -o /out/relay .

# ===== Runtime =====
FROM alpine:3.20

# ca-certificates: webhook 模式 HTTPS 需要
# tzdata: 日志时区
RUN apk add --no-cache ca-certificates tzdata \
    && adduser -D -H -u 1000 relay

# 内置默认配置(mode=none),生产用 volume 覆盖
RUN mkdir -p /etc/xlip-relay
COPY --from=builder /out/relay /usr/local/bin/relay
COPY config.toml /etc/xlip-relay/config.toml

RUN chown -R relay:relay /etc/xlip-relay

EXPOSE 19090

USER relay

ENTRYPOINT ["relay"]
CMD ["-config", "/etc/xlip-relay/config.toml"]
