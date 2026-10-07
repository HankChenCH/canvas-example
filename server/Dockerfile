# server 多阶段构建(spec §1.4):golang CGO_ENABLED=0 纯 Go 构建(modernc.org/sqlite
# 纯 Go 驱动,零 CGO)→ alpine 运行相。运行相 WORKDIR=/app 与 dev 约定(CWD=
# example/server)同构,随镜像带 seed/(含字体,启动播种幂等补齐到 assets/),
# data/ renders/ .cache/ assets/u/ 由启动自建(main.go runtimeDirs)。
#
# canvas 依赖走发布版本:go.mod require module proxy 的 github.com/HankChenCH/go-canvas
# 与 image-renderer v1.0.0,构建期自 proxy 拉取。

FROM golang:1.25-alpine AS build
# 构建机网络到 proxy.golang.org 不可达时用国内 proxy(宿主开发机同款约定);
# proxy 未缓存新版本时 go 回落 direct 直连拉取——alpine 需要 git
ARG GOPROXY=https://goproxy.cn,direct
ENV GOPROXY=${GOPROXY}
RUN apk add --no-cache git
WORKDIR /src
# 依赖清单先行 COPY + download:依赖层缓存不随源码改动失效
COPY server/go.mod server/go.sum /src/example/server/
WORKDIR /src/example/server
RUN go mod download
COPY server/ /src/example/server/
RUN CGO_ENABLED=0 go build -trimpath -o /out/example-server .

FROM alpine:3.22
# ca-certificates:徽标 picsum 走 HTTPS 物化(唯一外网点,spec §5.3);alpine 基础镜像不带 CA 包
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=build /out/example-server /app/example-server
COPY --from=build /src/example/server/seed /app/seed
EXPOSE 8080
ENTRYPOINT ["/app/example-server"]
