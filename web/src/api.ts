// HTTP 契约客户面（spec §2.4）：列表摘要 / 模板全量两条读路径 + 模板 PUT（#5）、
// 数据源独立实体 CRUD（#6 数据源段）、绑定（#6 绑定通道）与资源上传（#8）写路径。
// 错误信封统一 {"error":{code,message}}，按 code 判定语义（spec §2.5）。

export interface TemplateSummary {
    id: number
    name: string
    updatedAt: string
}

export interface TemplateRecord {
    id: number
    name: string
    canvases: { name: string; graph: unknown }[]
    flowChain: unknown
    /** 绑定的数据源引用（null = 未绑）——数据源是独立实体，模板只持引用 */
    dataSourceId: number | null
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

/** PUT /api/templates/{id} 载荷（spec §2.4 #5）：整存替换 name/canvases/flowChain，
 *  服务端不触碰 dataSourceId——绑定只经 PUT /templates/{id}/datasource 通道变更 */
export interface TemplateWritePayload {
    name: string
    canvases: { name: string; graph: unknown }[]
    flowChain: unknown
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

/** 渲染记录单图（spec §2.4 #7）：images = 帧序×页序扁平序；url 为前导斜杠形态
 *  （spec §2.2 响应两态），即 /renders 直链 */
export interface RenderImage {
    frame: number
    name: string
    url: string
}

/** RenderRecord 响应（spec §2.3/§2.4 #7）：201 渲染一跳的落库记录 */
export interface RenderRecord {
    id: number
    templateId: number
    createdAt: string
    images: RenderImage[]
}

/** 字体清单条目（GET /api/fonts）：与编辑器内核 FontCatalogEntry 同构——
 *  清单 ≠ 物化，ref 是 https 字体直链（TTF/OTF），预览 FontFace 与服务端
 *  渲染端远程物化各自按引用拉取 */
export interface FontInfo {
    label: string
    ref: string
}

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
    // POST /api/templates/{id}/render（spec §2.4 #7）：无 body——渲染始终以服务端
    // 存储态为准（模板 + 所引数据源），dirty 时结果对应已保存版本（spec §4.6）；30s
    // deadline 由服务端控制，前端不另设超时
    renderTemplate: (id: string | number) =>
        request<RenderRecord>(`/api/templates/${id}/render`, { method: 'POST' }),
}

// 页面错误回显统一格式：稳定 code 在前（spec §2.1 按 code 判定语义），网络层
// 失败（响应非 JSON 等）落 unknown_error 兜底
export function formatApiError(e: unknown): string {
    return e instanceof ApiError ? `${e.code}：${e.message}` : String(e)
}
