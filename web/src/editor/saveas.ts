// 另存为纯逻辑（18 票，spec §4.5 两连调用精度锚点③）：数据源复制载荷与两连调用
// 编排——POST /templates（name + canvases + flowChain，不带 dataset）→
// PUT /templates/{newId}/dataset（连带复制数据源）。与 Vue/DOM 解耦，EditorPage
// 只做响应式桥接；HTTP 客户端经调用方注入（测试免 mock fetch）。
// 中途失败阶段归因：create 阶段失败无副本落地；dataset 阶段失败时服务端半副本
// 已存在（无 DELETE 端点可回收），客户端不进入半副本（不重绑、不导航），错误
// 如实回显后由用户决定是否重试（重试为全新两连调用，不在旧副本上续作）。
import { formatApiError } from '../api'
import type { DatasetWritePayload, TemplateRecord, TemplateWritePayload } from '../api'

/** 数据源复制载荷（spec §4.5「连带复制数据源」）：原模板存储态 schema/data 原样
 *  随行——服务端 draft-07 校验 null schema 即 schema_invalid，故未绑数据源
 *  （双 null）返回 null = 跳过第二步，副本 dataset 保持 POST 的 null 即与原件
 *  一致（两列同写同清，存储态不会出现单边 null）。 */
export function datasetCopyPayload(record: { datasetSchema: unknown; dataset: unknown }): DatasetWritePayload | null {
    if (record.datasetSchema === null && record.dataset === null) return null
    return { schema: record.datasetSchema, data: record.dataset }
}

/** 失败归因：create 阶段失败无副本落地；dataset 阶段失败 = 服务端已建副本但
 *  数据源未随行。message 为稳定码在前的可读文案（spec §2.1）。 */
export type SaveAsFailure = { stage: 'create' | 'dataset'; message: string }

export type SaveAsResult = { ok: true; record: TemplateRecord } | { ok: false; failure: SaveAsFailure }

/** 两连调用编排（spec §4.5）：POST 建副本 → PUT dataset 复制数据源，成功返回
 *  PUT 的全量记录（dataset 已随行）；未绑数据源（dataset null）单步即完整副本，
 *  返回 POST 记录。 */
export async function saveAsCopy(params: {
    /** 副本模板载荷（spec §2.4 #3：name + canvases + flowChain，不带 dataset） */
    payload: TemplateWritePayload
    /** 数据源复制载荷；null = 无可复制，跳过第二步 */
    dataset: DatasetWritePayload | null
    createTemplate: (payload: TemplateWritePayload) => Promise<TemplateRecord>
    putDataset: (id: number, payload: DatasetWritePayload) => Promise<TemplateRecord>
}): Promise<SaveAsResult> {
    let created: TemplateRecord
    try {
        created = await params.createTemplate(params.payload)
    } catch (e) {
        return { ok: false, failure: { stage: 'create', message: formatApiError(e) } }
    }
    if (params.dataset === null) return { ok: true, record: created }
    try {
        return { ok: true, record: await params.putDataset(created.id, params.dataset) }
    } catch (e) {
        return { ok: false, failure: { stage: 'dataset', message: formatApiError(e) } }
    }
}
