# web 多阶段构建(spec §1.4):node:22 + pnpm 构建相 → nginx:alpine 运行相,
# dist 与 nginx.conf 打入镜像。canvas 四包依赖走发布版本(npm @hankchen/canvas*),
# pnpm install 自 registry 拉取;editor-vue 发布包内的工具类由宿主 tailwind 扫描
# 编译(style.css @source 指向 node_modules 内包 dist,见 src/style.css)。

FROM node:22-alpine AS build
RUN npm install -g pnpm@10
WORKDIR /src
COPY web/package.json web/pnpm-lock.yaml /src/example/web/
WORKDIR /src/example/web
RUN pnpm install --frozen-lockfile
COPY web/ /src/example/web/
RUN pnpm build

FROM nginx:alpine
COPY --from=build /src/example/web/dist /usr/share/nginx/html
COPY nginx.conf /etc/nginx/conf.d/default.conf
EXPOSE 80
