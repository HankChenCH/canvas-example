# example:证书批量生成示例应用

Go 服务端(go-canvas + image-renderer)+ canvas-web 前端,以「结业证书批量打印页」
演示**文档级渲染**:一份 dataset × 文档(M 帧)→ 流链配当 → 每页一张 PNG。

契约与验收唯一输入:`.scratch/example-app/spec.md`(工作区)。

## 布局

- `server/` — Go HTTP 后端(module `example/server`,go.mod 双 replace 指向 `../go-canvas` 两个本地 module)
- `web/` — canvas-web 宿主编辑器(14 票起)
- `scripts/smoke.sh` — spec §6.2 curl 冒烟断言

## 运行约定

**服务端进程 CWD = `example/server`**,所有相对路径(seed/、assets/、data/、renders/、.cache/)按此解析。

```sh
# dev:server(终端 1)
cd example/server && go run .        # 监听 :8080

# dev:web(终端 2,14 票起)
export PATH="$(brew --prefix)/opt/node@22/bin:$PATH"
cd example/web && pnpm install && pnpm dev

# 冒烟(断言随票数推进追加)
example/scripts/smoke.sh             # BASE 默认 http://localhost:8080
```

启动时自动:建运行目录 → `seed/assets/` 幂等补齐到 `assets/` → templates 表空则插入
默认模板(05 票 fixture)→ 字体启动探针(失败 fail-fast 拒绝启动)。
