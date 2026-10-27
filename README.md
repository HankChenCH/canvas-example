# canvas-example:证书批量生成示例应用

[canvas 三端库](#依赖的库)的**集成参考应用**:以前端编辑器定义证书模板,Go 服务端按
数据集批量渲染——一份 dataset × 文档模板(M 帧 + 流链)→ 每页一张 PNG 或整批
页序列合成一个 PDF 文档。

前端编辑器与后端渲染器**共享同一份 graph JSON 契约**(wire 键级对齐,PHP 端为权威):
浏览器里编辑的就是服务端渲染的,所见即所得。

## 功能

- **模板编辑器**:画布表面(标尺/参考线/吸附)、文本/图片/二维码/表格图层、
  表达式数据绑定(`{{student.name}}` 插值求值)、属性面板与图层面板、快捷键与撤销
- **数据源独立管理**:模板经 `dataSourceId` 引用 schema/data,多模板共享;
  被引用时删除返回 409 `data_source_in_use`,引用不悬空
- **文档级批量渲染**:帧列表 + 流链配当(内容流跨帧续排)→ 每页一张 PNG,
  服务端物化字体/图片/二维码后按五原语绘制
- **PDF 导出**:渲染端点 `{"format":"pdf"}` 单文档轨——同一次编译的整批页序列
  合成一个 PDF 文档(编辑器顶栏「导出 PDF」按钮),字体 TTF 子集内嵌、中文可
  复制提取
- **全站日间/夜间主题切换**(canvas-web v2.2.0 集成):VueUse `useDark` 向 html 写
  `data-cn-theme` 信号,编辑器外观(面板/状态栏/画布桌面)、壳层顶栏、印坊管理
  页与 Teleport 弹层整站跟随;首次访问跟随系统偏好,切换后 localStorage 持久化
  (`src/theme.ts` 单例)。纸面内容色不随主题——渲染产物与导出图不受影响

## 架构

```
web/  编辑器宿主(Vue 3 + vite)
  └─ @hankchen/canvas-editor-vue / -editor / canvas-browser-renderer / canvas   (npm)
        │ graph JSON(wire 契约,POST /api/templates 持久化)
        ▼
server/  Go HTTP 后端(module example/server)
  └─ github.com/HankChenCH/go-canvas 核心 + image-renderer + pdf-renderer 三 module   (module proxy)
        │ 预检(preflight)→ 数据填充(hydrate)→ 文档编译/分页 → 位图渲染(PNG)
        │                                        └→ PDF 文档渲染(format=pdf 单文档轨)
        ▼
  renders/  每页 PNG / document.pdf,GET /renders/... 取产物
```

依赖关系细节(依赖方向、双轨消费)见各库仓库:`js-canvas`(核心契约)、
`canvas-web`(DOM 世界三包)、`go-canvas`(Go 移植)、`php-canvas-next`(PHP 权威端,契约源头)。

## 快速开始(compose,一条命令)

```sh
docker compose up --build    # 构建期从 npm / go module proxy 拉发布依赖
# → http://localhost:8080 (nginx 唯一入口:静态前端 + 反代 /api /assets /renders)
```

对外端口可配置:拷贝 `.env.example` 为 `.env` 改 `CANVAS_EXAMPLE_PORT`(`.env` 不入 git);
改了端口记得让 smoke 跟随:`bash scripts/smoke.sh http://localhost:<端口>`。

功能验收脚本(16 组 curl 断言——1–14 通用组 + 15 版本化轨九断言 + 16 渲染历史,
对 compose/dev 任何轨道通用;版本化轨依赖重置运行库,复跑时首发 publish 幂等
断言变 200 duplicate,重置方法见 `scripts/smoke.sh` 头部注记):

```sh
bash scripts/smoke.sh        # BASE 默认 http://localhost:8080
```

## 开发轨(双终端)

```sh
# 终端 1:Go 服务端(监听 :8080;需要 Go 1.26+)
cd server && go run .

# 终端 2:web 编辑器(vite dev,需要 Node 22 + pnpm 10;/api 等已代理到 :8080)
cd web && pnpm install && pnpm dev    # → http://localhost:5173
```

启动时自动:建运行目录 → `seed/assets/` 幂等补齐到 `assets/` → templates 表空则
插入默认模板 → 字体探针(失败 fail-fast 拒绝启动)。

**服务端进程 CWD = `server/`**:所有相对路径(seed/、assets/、data/、renders/、
.cache/)按此解析;运行时目录不入库(见 .gitignore)。

出网注意:机构徽标是唯一外网依赖(picsum);离线演示可把 dataset 的 `org.logo`
换成自引用 URL(`/assets/u/…`)。

## 布局

- `server/` — Go HTTP 后端:REST API(templates/datasources/assets/renders)、
  SQLite 存储、渲染管线(预检 → 填充 → 编译/分页 → PNG)
- `web/` — 编辑器宿主:vue-router 列表页 + 编辑器页 + 数据源管理页
- `server/seed/` — 默认模板(帧 + 流链)、dataset 与 schema、字体(Noto Sans SC,OFL)
- `scripts/smoke.sh` — 功能断言(契约回归用)
- `compose.yaml` + `nginx.conf` + 两 `Dockerfile` — 单命令演示轨

## License

[MIT](LICENSE)
