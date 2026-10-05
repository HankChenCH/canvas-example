// HTTP 契约客户面（spec §2.4）：本票只消费列表摘要与模板全量两条读路径。
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

async function request<T>(url: string): Promise<T> {
    const res = await fetch(url)
    const body: unknown = await res.json().catch(() => null)
    if (!res.ok) {
        const err = (body as { error?: { code?: string; message?: string } } | null)?.error
        throw new ApiError(res.status, err?.code ?? 'unknown_error', err?.message ?? res.statusText)
    }
    return body as T
}

export const api = {
    listTemplates: () => request<TemplateSummary[]>('/api/templates'),
    getTemplate: (id: string | number) => request<TemplateRecord>(`/api/templates/${id}`),
}

// 页面错误回显统一格式：稳定 code 在前（spec §2.1 按 code 判定语义），网络层
// 失败（响应非 JSON 等）落 unknown_error 兜底
export function formatApiError(e: unknown): string {
    return e instanceof ApiError ? `${e.code}：${e.message}` : String(e)
}
