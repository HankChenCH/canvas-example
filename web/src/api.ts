// HTTP 契约客户面（spec §2.4）：列表摘要 / 模板全量两条读路径 + 模板 PUT（#5）、
// 数据源独立实体 CRUD（#6 数据源段）、绑定（#6 绑定通道）、资源上传（#8）与
// 编辑器状态（工程数据通道，project-data spec §4）写路径。
// 错误信封统一 {"error":{code,message}}，按 code 判定语义（spec §2.5）。
import type { CanvasEditorState, EditorPrefsState } from '@hankchen/canvas-editor'

export interface TemplateSummary {
    id: number
    name: string
    updatedAt: string
}

/** 页 flow wire 形（模板页序列工票，spec 页形状契约，与工票 02 服务端共享）：镜像
 *  引擎链节点键面去掉 frame（帧位置 = 页序）。quota 仅 fixed 携带（缺省 = 不携带
 *  键 = 容量型按可用区装）；omitIfEmpty 仅 true 携带（wire 缺省 false 同义） */
export interface PageFlowWire {
    mode: 'fixed' | 'paged'
    quota?: number
    omitIfEmpty?: boolean
}

/** 页 wire 形（模板页序列工票，spec 页形状契约）：一页恰承载一帧。id 前端生成、
 *  页内唯一（服务端校验非空 + 唯一），只活编辑层与工程数据键、编译时丢弃不出 wire；
 *  name 可选（显示用，服务端不查重，缺省名「第 N 页」由前端渲染）；无 flow 键（或
 *  null）= 链外页（不吃行、照常按页序进产物） */
export interface TemplatePageWire {
    id: string
    name?: string
    graph: unknown
    flow?: PageFlowWire | null
}

export interface TemplateRecord {
    id: number
    name: string
    /** 页序列（spec 页形状契约）：模板内容 = pages，双数组 canvases+flowChain 已退役 */
    pages: TemplatePageWire[]
    /** 绑定的数据源引用（null = 未绑）——数据源是独立实体，模板只持引用 */
    dataSourceId: number | null
    /** 工程数据快照（project-data spec §4 读合并）：字节面服务端零校验原样透传，
     *  null = 从未保存 = 全默认态；宽容解析权威在宿主 editorState.ts + 内核恢复端 */
    editorState: unknown
    createdAt: string
    updatedAt: string
}

export interface DataSourceSummary {
    id: number
    name: string
    /** 引用该数据源的模板数（共享影响面注记用） */
    templateCount: number
    updatedAt: string
}

export interface DataSourceRecord {
    id: number
    name: string
    schema: unknown
    data: unknown
    createdAt: string
    updatedAt: string
}

export class ApiError extends Error {
    constructor(
        public readonly status: number,
        public readonly code: string,
        message: string,
    ) {
        super(message)
    }
}

async function request<T>(url: string, init?: RequestInit): Promise<T> {
    const res = await fetch(url, init)
    const body: unknown = await res.json().catch(() => null)
    if (!res.ok) {
        const err = (body as { error?: { code?: string; message?: string } } | null)?.error
        throw new ApiError(res.status, err?.code ?? 'unknown_error', err?.message ?? res.statusText)
    }
    return body as T
}

/** PUT /api/templates/{id} 载荷（spec 页形状契约）：整存替换 name/pages，
 *  服务端不触碰 dataSourceId——绑定只经 PUT /templates/{id}/datasource 通道变更 */
export interface TemplateWritePayload {
    name: string
    pages: TemplatePageWire[]
}

/** POST /api/templates 载荷（spec §2.4 #3）：文档内容 + 可选 dataSourceId 随建
 *  随绑（另存为单调用携带引用）；缺省 null = 未绑 */
export interface TemplateCreatePayload extends TemplateWritePayload {
    dataSourceId?: number | null
}

/** POST/PUT /api/datasources 载荷（spec §2.4 数据源段）：draft-07 校验权威在
 *  服务端，前端只做机械解析（datasource.ts dataSourceDraftPayload 组装） */
export interface DataSourceWritePayload {
    name: string
    schema: unknown
    data: unknown
}

/** 宿主聚合快照（project-data spec §6）：PUT /api/templates/{id}/editor-state
 *  载荷即此形状——schemaVersion/切片/常量自内核与宿主 editorState.ts 导入，宿主只
 *  拼装不解释。frames 键 = **页 id**（模板页序列工票 03 起，页 id 稳定不随改名/重排
 *  变），prefs 为编辑器级偏好。服务端零校验整存替换，宽容读权威在宿主 editorState.ts
 *  + 内核恢复端 */
export interface TemplateEditorStateSnapshot {
    schemaVersion: number
    frames: Record<string, CanvasEditorState>
    prefs: EditorPrefsState
}

/** 渲染记录单图（spec §2.4 #7）：images = 页序扁平序（frame 字段不改名，语义由
 *  帧下标自然延续为页下标，ADR 0025）；url 为前导斜杠形态（spec §2.2 响应两态），
 *  即 /renders 直链 */
export interface RenderImage {
    frame: number
    name: string
    url: string
}

/** RenderRecord 响应（spec §2.3/§2.4 #7）：201 渲染一跳的落库记录。version =
 *  渲染所用发布版本（tdp spec §4 渲染选版）；null = "draft" 哨兵渲染或版本化
 *  前的旧行（诚实语义，不造假数据） */
export interface RenderRecord {
    id: number
    templateId: number
    version: number | null
    createdAt: string
    images: RenderImage[]
}

/** 版本摘要条目（tdp spec §3，GET /api/templates/{id}/versions）：轻量列表不吐
 *  pages 载荷，version 降序（最新在前）；**空数组 = 未发布**——发布态永远是
 *  推导态非存储态（徽标/下拉据此推导） */
export interface TemplateVersionSummary {
    version: number
    createdAt: string
}

/** POST …/publish 响应（tdp spec §3）：201 新版本 / 200 duplicate:true（草稿列
 *  与最新版本字节相等，幂等跳过不产生新版本行）——两种形态都算发布成功 */
export interface PublishResult {
    version: number
    createdAt: string
    /** true = 内容与最新版本一致，跳过重复发布 */
    duplicate?: boolean
}

/** GET …/versions/{version} 单版本全量（tdp spec §3，回滚与未来预览消费）：快照
 *  粒度 = 文档整体（pages 序列，页 id 在内）；data_source_id 是绑定不是内容，不进
 *  快照（渲染时取当前绑定） */
export interface TemplateVersionRecord {
    version: number
    pages: TemplatePageWire[]
    createdAt: string
}

/** 字体清单条目（GET /api/fonts）：与编辑器内核 FontCatalogEntry 同构——
 *  清单 ≠ 物化，ref 是 https 字体直链（TTF/OTF），预览 FontFace 与服务端
 *  渲染端远程物化各自按引用拉取 */
export interface FontInfo {
    label: string
    ref: string
}

/** 工程数据写分道端点（project-data spec §4）：普通通道与页签关闭 keepalive 通道共用 */
const editorStateUrl = (id: string | number): string => `/api/templates/${id}/editor-state`

export const api = {
    listTemplates: () => request<TemplateSummary[]>('/api/templates'),
    // GET /api/fonts：字体清单白名单（seed/fonts.json 只读下发）；失败由调用方
    // 容忍（清单不可得时编辑器字体字段退化手输）
    listFonts: () => request<FontInfo[]>('/api/fonts'),
    getTemplate: (id: string | number) => request<TemplateRecord>(`/api/templates/${id}`),
    // POST /api/templates（spec §2.4 #3）：保存即预检，不过 400 打回不落库；
    // 201 全量记录（含随建绑定）
    createTemplate: (payload: TemplateCreatePayload) =>
        request<TemplateRecord>('/api/templates', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(payload),
        }),
    updateTemplate: (id: string | number, payload: TemplateWritePayload) =>
        request<TemplateRecord>(`/api/templates/${id}`, {
            method: 'PUT',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(payload),
        }),
    // PUT /api/templates/{id}/editor-state（project-data spec §4 写分道）：快照原样
    // 整存替换，服务端零校验；成功回全量记录；不触碰 name/pages/dataSourceId/
    // updatedAt（拖参考线不污染「最近编辑」排序）
    updateTemplateEditorState: (id: string | number, snapshot: TemplateEditorStateSnapshot) =>
        request<TemplateRecord>(editorStateUrl(id), {
            method: 'PUT',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(snapshot),
        }),
    // 同端点页签关闭通道（project-data spec §6 写入时点三，工单 05）：unload 期经
    // fetch keepalive 发出——sendBeacon 只能 POST，PUT 写分道语义保不住；快照载荷
    // 自足（同步组装同步发出，不依赖页面存活）。fire-and-forget：不解析响应（unload
    // 期无从消费）、失败静默 console.warn（spec §6 失败面，调用方无 catch 可挂）
    updateTemplateEditorStateKeepalive: (id: string | number, snapshot: TemplateEditorStateSnapshot): void => {
        void fetch(editorStateUrl(id), {
            method: 'PUT',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(snapshot),
            keepalive: true,
        }).catch((e) => {
            console.warn('[editor-state] 页签关闭 flush 失败（静默降级）', e)
        })
    },
    // PUT /api/templates/{id}/datasource（spec §2.4 #6 绑定通道）：{dataSourceId}
    // 整存替换引用列，null = 解绑；引用存在性由服务端先验（404 data_source_not_found）
    bindTemplateDataSource: (id: string | number, dataSourceId: number | null) =>
        request<TemplateRecord>(`/api/templates/${id}/datasource`, {
            method: 'PUT',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ dataSourceId }),
        }),
    // DELETE /api/templates/{id}（spec §2.4 #10，卡片操作修订）：204 无响应体——
    // 模板行与其渲染记录行级联清除；已落盘产物 PNG 保留（keep-all 快照直链仍可
    // 用）。数据源是独立实体不受影响（引用随模板行整行消失，不悬空）
    deleteTemplate: (id: string | number) =>
        request<null>(`/api/templates/${id}`, { method: 'DELETE' }),
    // 数据源独立实体 CRUD（spec §2.4 数据源段）：列表摘要含 templateCount
    // （共享影响面）；写路径 draft-07 完整校验（schema_invalid /
    // dataset_schema_mismatch），通过则整存替换
    listDataSources: () => request<DataSourceSummary[]>('/api/datasources'),
    getDataSource: (id: string | number) => request<DataSourceRecord>(`/api/datasources/${id}`),
    createDataSource: (payload: DataSourceWritePayload) =>
        request<DataSourceRecord>('/api/datasources', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(payload),
        }),
    updateDataSource: (id: string | number, payload: DataSourceWritePayload) =>
        request<DataSourceRecord>(`/api/datasources/${id}`, {
            method: 'PUT',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(payload),
        }),
    // DELETE /api/datasources/{id}（spec §2.4 #6e，28 票）：204 无响应体——被模板
    // 引用时 409 data_source_in_use 拒绝（引用不悬空，先解绑或删模板再删）
    deleteDataSource: (id: string | number) =>
        request<null>(`/api/datasources/${id}`, { method: 'DELETE' }),
    // multipart 上传：字段名 file（spec §2.4 #8）；不手写 Content-Type（boundary
    // 归浏览器）；响应 url 前导斜杠形态，原样写 graph spec.src（spec §2.2）
    uploadAsset: (file: { name: string; mime: string; bytes: Uint8Array }) => {
        const form = new FormData()
        // slice() 落一份独立 ArrayBuffer 拷贝（TS 5.9 BlobPart 收紧为
        // ArrayBufferView<ArrayBuffer>，泛型 ArrayBufferLike 视图不直收）
        form.append('file', new Blob([file.bytes.slice()], { type: file.mime || 'application/octet-stream' }), file.name)
        return request<{ url: string }>('/api/assets', { method: 'POST', body: form })
    },
    // POST /api/templates/{id}/render（spec §2.4 #7 / tdp spec §4 渲染选版）：可选
    // body {version}——缺省 = 最新已发布版本（从未发布 409 template_unpublished）；
    // 正整数 = 指定历史版本（不存在 404 version_not_found）；哨兵 "draft" = 渲染
    // 草稿列现值（编辑器预览专用，票 03 契约修订）。body 另可携带 {format} 选
    // 产物轨（PDF 集成共识：单轨语义，与选版正交）——缺省 "png" 位图页序列
    // （现状）；"pdf" = 整批页序列合成单文档 document.pdf。无 opts 保持无 body
    // POST（列表页批量渲染零改路径）。30s deadline 由服务端控制，前端不另设超时
    renderTemplate: (id: string | number, opts?: { version?: number | 'draft'; format?: 'png' | 'pdf' }) =>
        request<RenderRecord>(
            `/api/templates/${id}/render`,
            opts?.version === undefined && opts?.format === undefined
                ? { method: 'POST' }
                : {
                      method: 'POST',
                      headers: { 'Content-Type': 'application/json' },
                      body: JSON.stringify({ version: opts.version, format: opts.format }),
                  },
        ),
    // GET /api/templates/{id}/renders（渲染历史对比）：模板维度渲染记录，id 降序
    // （最新在前），响应形状与 renderTemplate 同形（version null = draft 渲染行）；
    // 服务端固定档位截最新 20 条（keep-all 语义下防无界返回），无查询参数
    listRenders: (id: string | number) => request<RenderRecord[]>(`/api/templates/${id}/renders`),
    // POST /api/templates/{id}/publish（tdp spec §3 发布）：无 body，把草稿列固化
    // 为新版本（201）；草稿与最新版本字节相等 → 200 duplicate:true 幂等跳过；图
    // 病态 422 invalid_graph（preflight 守门，不产生版本行）；模板不存在 404
    publishTemplate: (id: string | number) =>
        request<PublishResult>(`/api/templates/${id}/publish`, { method: 'POST' }),
    // GET /api/templates/{id}/versions（tdp spec §3 版本读家族）：轻量列表（version
    // 降序，最新在前），空数组 = 未发布；调用方容忍失败（徽标/下拉退化，不阻塞主流程）
    listTemplateVersions: (id: string | number) =>
        request<TemplateVersionSummary[]>(`/api/templates/${id}/versions`),
    // GET /api/templates/{id}/versions/{version}（tdp spec §3）：单版本全量快照
    // （回滚消费；预览/对比归后续工作）；404 version_not_found
    getTemplateVersion: (id: string | number, version: number) =>
        request<TemplateVersionRecord>(`/api/templates/${id}/versions/${version}`),
    // POST /api/templates/{id}/revert（tdp spec §5 回滚）：body {version}，服务端
    // 单事务原子复制版本内容回草稿列；200 回全量模板记录（与 GET /templates/{id}
    // 同形，含 editorState）——编辑器拿到即会话替换，无需二次 GET；404
    // version_not_found。版本线线性：回滚永不产生新版本
    revertTemplate: (id: string | number, version: number) =>
        request<TemplateRecord>(`/api/templates/${id}/revert`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ version }),
        }),
}

// 页面错误回显统一格式：稳定 code 在前（spec §2.1 按 code 判定语义），网络层
// 失败（响应非 JSON 等）落 unknown_error 兜底
export function formatApiError(e: unknown): string {
    return e instanceof ApiError ? `${e.code}：${e.message}` : String(e)
}
