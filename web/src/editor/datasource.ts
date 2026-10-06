// 数据源抽屉纯逻辑（23 票重构：数据源为独立实体、模板持引用，spec §4.3 三段式）：
// 绑定段（当前绑定 vs 选中项变更判定）、内容段（绑定的数据源实体 name/schema/data
// 文本基线与载荷组装）、流链段（flowChain 文本基线沿用）。与 Vue/DOM 解耦，
// EditorPage 只做响应式桥接。
// 校验边界：draft-07 与流链语义的校验权威在服务端（spec §4.3「前端不复制校验
// 器」），这里只做「文本能否成 JSON」的机械判定——解析失败本地拒绝，语义校验
// 一律经服务端错误码回显。
import type { DataSourceRecord, DataSourceWritePayload } from '../api'

/** 值 → 文本域展示文本（2 空格 pretty；null/缺省归 null 字面——空链与 null
 *  同义，spec §3.1，抽屉内以 `null` 表达空链） */
export function prettyJsonText(value: unknown): string {
    return JSON.stringify(value ?? null, null, 2)
}

export type JsonDraft = { ok: true; value: unknown } | { ok: false; message: string }

/** 文本域草稿 → JSON 值（仅解析不校验，见文件头校验边界） */
export function parseJsonDraft(text: string): JsonDraft {
    try {
        return { ok: true, value: JSON.parse(text) }
    } catch (e) {
        return { ok: false, message: e instanceof Error ? e.message : String(e) }
    }
}

/** 段内独立未保存标记（spec §4.4：不混全局 saveState 指示灯） */
export function isSegmentDirty(text: string, baselineText: string): boolean {
    return text !== baselineText
}

/** 绑定段判定：选中项与当前绑定是否一致（一致 = 无变更，动作钮禁用） */
export function isBindingChange(
    currentId: number | null,
    selectedId: number | null,
): boolean {
    return currentId !== selectedId
}

// ---- 数据源内容段（编辑的是绑定的数据源实体，spec §4.3）----

/** 内容段载荷：name 先判（报错定位优先左字段），schema/data 仅解析不校验；
 *  未绑数据源时同样的三文本域组装「创建并绑定」载荷 */
export type DataSourceDraftPayload =
    | { ok: true; payload: DataSourceWritePayload }
    | { ok: false; field: 'name' | 'schema' | 'data'; message: string }

export function dataSourceDraftPayload(
    nameText: string,
    schemaText: string,
    dataText: string,
): DataSourceDraftPayload {
    const name = nameText.trim()
    if (!name) return { ok: false, field: 'name', message: '数据源名不能为空' }
    const schema = parseJsonDraft(schemaText)
    if (!schema.ok) return { ok: false, field: 'schema', message: `schema JSON 解析失败：${schema.message}` }
    const data = parseJsonDraft(dataText)
    if (!data.ok) return { ok: false, field: 'data', message: `data JSON 解析失败：${data.message}` }
    return { ok: true, payload: { name, schema: schema.value, data: data.value } }
}

/** 内容段基线：绑定的数据源实体（或未绑空态）→ 文本域初值。
 *  未绑（source = null）：名字空串、schema/data 空——创建并绑定的起笔态。 */
export function sourceDraftBaseline(source: DataSourceRecord | null): {
    nameText: string
    schemaText: string
    dataText: string
} {
    return {
        nameText: source?.name ?? '',
        schemaText: source === null ? '' : prettyJsonText(source.schema),
        dataText: source === null ? '' : prettyJsonText(source.data),
    }
}
