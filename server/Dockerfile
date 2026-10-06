# server 多阶段构建(spec §1.4):golang CGO_ENABLED=0 纯 Go 构建(modernc.org/sqlite
# 纯 Go 驱动,零 CGO)→ alpine 运行相。运行相 WORKDIR=/app 与 dev 约定(CWD=
# example/server)同构,随镜像带 seed/(含字体,启动播种幂等补齐到 assets/),
# data/ renders/ .cache/ assets/u/ 由启动自建(main.go runtimeDirs)。
#
# go.mod 双 replace 指向 ../../go-canvas——compose additional_contexts 把同层 go-canvas
# 挂进 gocanvas 上下文,构建内按工作区同构布局摆放(/src/example/server + /src/go-canvas),
# replace 相对路径原样解析,go.mod/go.sum 零改动。

FROM golang:1.25-alpine AS build
WORKDIR /src
COPY --from=gocanvas / /src/go-canvas
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
