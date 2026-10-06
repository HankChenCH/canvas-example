# web 多阶段构建(spec §1.4):node:22 + pnpm 构建相 → nginx:alpine 运行相,
# dist 与 nginx.conf 打入镜像。canvas-web 四包经 pnpm file: 引用且在 context 外——
# compose additional_contexts 挂进 canvasweb 上下文,构建内按工作区同构布局摆放
# (/src/example/web + /src/canvas-web),file: 依赖与 style.css @source 的相对路径
# 原样解析,package.json/锁文件零改动。

FROM node:22-alpine AS build
RUN npm install -g pnpm@10
WORKDIR /src
COPY --from=canvasweb /packages/ /src/canvas-web/packages/
# 链接包目录内宿主安装残留的 node_modules(pnpm 符号链接)在镜像内是死链,清掉防干扰
RUN rm -rf /src/canvas-web/packages/*/node_modules
COPY web/ /src/example/web/
WORKDIR /src/example/web
RUN pnpm install --frozen-lockfile
RUN pnpm build

FROM nginx:alpine
COPY --from=build /src/example/web/dist /usr/share/nginx/html
COPY nginx.conf /etc/nginx/conf.d/default.conf
EXPOSE 80
