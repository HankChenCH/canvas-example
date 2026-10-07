# example:证书批量生成示例应用

Go 服务端(go-canvas + image-renderer)+ canvas-web 前端,以「结业证书批量打印页」
演示**文档级渲染**:一份 dataset × 文档(M 帧)→ 流链配当 → 每页一张 PNG。

**数据源是独立资源**(`datasources` 表 + `/api/datasources` CRUD):模板经
`dataSourceId` 引用,多模板共享同一份 schema/data——新建模板从数据源库直接绑定,
另存为单调用引用随行,不再逐模板重录(23 票契约修订);web 侧有独立管理页
`/datasources`(列表/新建/编辑,25 票;卡片删除,28 票——被模板引用时 409
`data_source_in_use` 拒绝,引用不悬空)。

契约与验收唯一输入:`.scratch/example-app/spec.md`(工作区)。

## 布局

- `server/` — Go HTTP 后端(module `example/server`,依赖走 module proxy:`github.com/HankChenCH/go-canvas` + `image-renderer` v1.0.0)
- `web/` — canvas-web 宿主编辑器(14 票起)
- `scripts/smoke.sh` — spec §6.2 curl 冒烟断言(双轨通用)
- `compose.yaml` + `nginx.conf` + 两 `Dockerfile` — 验收/演示轨(20 票起)

## 运行约定

**服务端进程 CWD = `example/server`**,所有相对路径(seed/、assets/、data/、renders/、.cache/)按此解析。

```sh
# dev:server(终端 1)
cd example/server && go run .        # 监听 :8080

# dev:web(终端 2,14 票起)
export PATH="$(brew --prefix)/opt/node@22/bin:$PATH"
cd example/web && pnpm install && pnpm dev

# 冒烟(双轨通用,断言随票数推进追加)
example/scripts/smoke.sh             # BASE 默认 http://localhost:8080
```

启动时自动:建运行目录 → `seed/assets/` 幂等补齐到 `assets/` → templates 表空则插入
默认模板(05 票 fixture)→ 字体启动探针(失败 fail-fast 拒绝启动)。

### compose 验收/演示轨(20 票)

```sh
cd example && docker compose up --build   # nginx 发布 :8080,唯一入口
example/scripts/smoke.sh                  # 对 compose 轨跑 spec §6.2 全 12 断言
```

- 两服务:`server`(CGO_ENABLED=0 纯 Go 构建 → alpine,WORKDIR=/app 随镜像带 seed/ 与字体)
  + `nginx`(node:22+pnpm 构建 dist → nginx:alpine,静态 + SPA fallback,反代 /api /assets /renders)。
- canvas 依赖全部走发布版本(module proxy `go-canvas` v1.0.0 + npm `@hankchen/canvas*`),
  镜像构建期自 registry/proxy 拉取,无本地依赖仓接线。
- 出网注意:机构徽标是唯一外网依赖(picsum);离线演示可把 dataset 的 `org.logo`
  换成自引用 URL(`/assets/u/…`)。
