// 数据源抽屉纯逻辑（17 票，spec §4.3 双通道 / §4.4 独立未保存标记）：抽屉三个
// 文本域（schema / data / flowChain）的基线换算、草稿 JSON 解析、段内独立标记与
// 数据源段载荷组装。与 Vue/DOM 解耦，EditorPage 只做响应式桥接。
// 校验边界：draft-07 与流链语义的校验权威在服务端（spec §4.3「前端不复制校验
// 器」），这里只做「文本能否成 JSON」的机械判定——解析失败本地拒绝，语义校验
// 一律经服务端错误码回显。
import type { DatasetWritePayload } from '../api'

/** 抽屉段基线：文本域载入（或该段保存成功）时的文本快照——段内标记按文本逐字
 *  比较（「文本域 vs 载入基线」），不语义等价折叠：重新缩进/改键序也算改动，
 *  标记如实反映「离开即丢文本」。 */
export interface DrawerDraftBaseline {
    schemaText: string
    dataText: string
    flowChainText: string
}

/** 值 → 文本域展示文本（2 空格 pretty；null/缺省归 null 字面——空链与 null
 *  同义，spec §3.1，抽屉内以 `null` 表达空链/未绑数据源） */
export function prettyJsonText(value: unknown): string {
    return JSON.stringify(value ?? null, null, 2)
}

/** 载入基线：模板记录三字段 → 文本域初值（spec §4.3） */
export function drawerBaselineFromRecord(record: {
    datasetSchema: unknown
    dataset: unknown
    flowChain: unknown
}): DrawerDraftBaseline {
    return {
        schemaText: prettyJsonText(record.datasetSchema),
        dataText: prettyJsonText(record.dataset),
        flowChainText: prettyJsonText(record.flowChain),
    }
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

export type DatasetDraftPayload =
    | { ok: true; payload: DatasetWritePayload }
    | { ok: false; field: 'schema' | 'data'; message: string }

/** 数据源段两文本域 → `PUT /templates/{id}/dataset` 载荷（spec §2.4 #6）：
 *  schema 先判（报错定位优先左字段），任一解析失败即短路返回字段与段内可直显
 *  的完整文案（字段名前缀在此单点拼装），不打服务端。 */
export function datasetDraftPayload(schemaText: string, dataText: string): DatasetDraftPayload {
    const schema = parseJsonDraft(schemaText)
    if (!schema.ok) return { ok: false, field: 'schema', message: `schema JSON 解析失败：${schema.message}` }
    const data = parseJsonDraft(dataText)
    if (!data.ok) return { ok: false, field: 'data', message: `data JSON 解析失败：${data.message}` }
    return { ok: true, payload: { schema: schema.value, data: data.value } }
}
