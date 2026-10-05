// HTTP 契约客户面（spec §2.4）：列表摘要 / 模板全量两条读路径 + 模板 PUT（#5）
// 与资源上传（#8）两条写路径。错误信封统一 {"error":{code,message}}，按 code
// 判定语义（spec §2.5）。

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
    datasetSchema: unknown
    dataset: unknown
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
 *  服务端不触碰 dataset/datasetSchema */
export interface TemplateWritePayload {
    name: string
    canvases: { name: string; graph: unknown }[]
    flowChain: unknown
}

/** PUT /api/templates/{id}/dataset 载荷（spec §2.4 #6）：draft-07 校验权威在服务端，
 *  前端只做机械解析（datasource.ts datasetDraftPayload 组装） */
export interface DatasetWritePayload {
    schema: unknown
    data: unknown
}

export const api = {
    listTemplates: () => request<TemplateSummary[]>('/api/templates'),
    getTemplate: (id: string | number) => request<TemplateRecord>(`/api/templates/${id}`),
    // POST /api/templates（spec §2.4 #3）：载荷不含 dataset（04 票锚点③前提，
    // 另存为两连调用补足）；保存即预检，不过 400 打回不落库；201 全量记录
    createTemplate: (payload: TemplateWritePayload) =>
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
    // PUT /api/templates/{id}/dataset（spec §2.4 #6）：{schema, data} draft-07 完整
    // 校验权威在服务端（schema_invalid / dataset_schema_mismatch），通过则整存替换
    // 两列；不触碰 name/canvases/flowChain
    putDataset: (id: string | number, payload: DatasetWritePayload) =>
        request<TemplateRecord>(`/api/templates/${id}/dataset`, {
            method: 'PUT',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(payload),
        }),
    // multipart 上传：字段名 file（spec §2.4 #8）；不手写 Content-Type（boundary
    // 归浏览器）；响应 url 前导斜杠形态，原样写 graph spec.src（spec §2.2）
    uploadAsset: (file: { name: string; mime: string; bytes: Uint8Array }) => {
        const form = new FormData()
        // slice() 落一份独立 ArrayBuffer 拷贝（TS 5.9 BlobPart 收紧为
        // ArrayBufferView<ArrayBuffer>，泛型 ArrayBufferLike 视图不直收）
        form.append('file', new Blob([file.bytes.slice()], { type: file.mime || 'application/octet-stream' }), file.name)
        return request<{ url: string }>('/api/assets', { method: 'POST', body: form })
    },
}

// 页面错误回显统一格式：稳定 code 在前（spec §2.1 按 code 判定语义），网络层
// 失败（响应非 JSON 等）落 unknown_error 兜底
export function formatApiError(e: unknown): string {
    return e instanceof ApiError ? `${e.code}：${e.message}` : String(e)
}
