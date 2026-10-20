// 页槽位纯逻辑（模板页序列工票 03，spec 页形状契约 / ADR 0025）：宿主内存页序列的
// 载入、dirty 口径与保存载荷组装。与 Vue/DOM 解耦，EditorPage 只做响应式桥接。
// 约定：页槽位的 graphJson 以 canonical encode（decode→encode 往返恒等形态）的
// JSON 字符串持有——dirty 比较与保存载荷直接消费字符串，避免对大 wire 对象反复
// stringify，也保证「切出快照 ↔ 基线」两侧同形不产生伪差异。
//
// 页槽位模型（帧缓冲 + 流链平行数组双模型的继任）：页 = id（前端生成、稳定不随
// 改名/重排变）+ 可选 name（null = 未命名，缺省显示「第 N 页」仅显示层）+ 画布
// wire 数据 + flow（null = 链外页）。flow 附着于页对象，「条目下标 = 帧下标」的
// 平行数组模型退役——加页/删页只动页序列，无第二份数据要同步。
import { decodeGraph, encodeGraph } from '@hankchen/canvas'
import type { Canvas } from '@hankchen/canvas'

import type { PageFlowWire, TemplatePageWire, TemplateWritePayload } from '../api'

/** 页 flow 编辑态（wire flow 的宿主内形）：quota null = 不携带键（容量型按可用区
 *  装）；omitIfEmpty false = 不携带键（wire 缺省 false 同义）。wire 落键（quota/
 *  omitIfEmpty 何时输出）单点在 wireFlowOf。 */
export interface PageFlow {
    mode: 'fixed' | 'paged'
    quota: number | null
    omitIfEmpty: boolean
}

/** 页槽位：id 即 canvases 时代的帧名继任者——工程数据（editorState frames）与页签
 *  key 都引用它；name null = 未命名（显示层渲染「第 N 页」缺省名，spec 定案沿用
 *  帧名惯例）；graphJson 为该页 canonical encode 的 JSON 字符串；flow null = 链外页 */
export interface PageSlot {
    id: string
    name: string | null
    graphJson: string
    flow: PageFlow | null
}

/** 文档级 dirty 基线（spec §4.4 口径的页模型版）：载入（或保存成功）时的模板名 +
 *  各页的载荷字节（id ∪ name ∪ canonical graph ∪ canonical flow）。页 id/name 进
 *  dirty：两者都随保存载荷上行（id 引用工程数据键、name 显示用），变更即字节变更 */
export interface PageBaselineEntry {
    id: string
    name: string | null
    graphJson: string
    flow: string
}

/** 文档级 dirty 基线整体：模板名 + 页序列逐页基线 */
export interface PageBaseline {
    name: string
    pages: PageBaselineEntry[]
}

/** 页 id 前端生成（spec 定案）：`pg_` + uuid v4 形态串。不用 crypto.randomUUID：
 *  它只在 secure context（https 或 localhost）可用，http 内网部署即抛错；
 *  crypto.getRandomValues 无 secure-context 前置（浏览器与 Node 19+ 原生可用，
 *  零第三方依赖）。只活编辑层与工程数据键，随保存载荷上行、编译时服务端丢弃
 *  不出 wire */
export function newPageId(): string {
    const bytes = crypto.getRandomValues(new Uint8Array(16))
    bytes[6] = (bytes[6]! & 0x0f) | 0x40 // version 4
    bytes[8] = (bytes[8]! & 0x3f) | 0x80 // variant 10x
    const hex = Array.from(bytes, (b) => b.toString(16).padStart(2, '0')).join('')
    return `pg_${hex.slice(0, 8)}-${hex.slice(8, 12)}-${hex.slice(12, 16)}-${hex.slice(16, 20)}-${hex.slice(20)}`
}

/** 文档 → canonical JSON 串（切出快照与 dirty 现值共用同一条缝） */
export function encodeGraphJson(doc: Canvas): string {
    return JSON.stringify(encodeGraph(doc))
}

/** canonical JSON 串 → 文档（切页重建会话文档用） */
export function decodeGraphJson(graphJson: string): Canvas {
    return decodeGraph(JSON.parse(graphJson))
}

/** wire flow → 页 flow 编辑态（宽容归一，帧时代 entryFromNode 同口径）：非对象/mode
 *  非 fixed|paged → null（链外页）；quota 仅非负整数落位（其余归 null = 不携带）、
 *  仅 fixed 有意义；omitIfEmpty 仅 true 落位。存储态经服务端保存预检必然合法，归一
 *  只会修复、不丢语义 */
export function flowFromWire(raw: unknown): PageFlow | null {
    if (typeof raw !== 'object' || raw === null) return null
    const node = raw as Record<string, unknown>
    const mode = node['mode']
    if (mode !== 'fixed' && mode !== 'paged') return null
    const quotaRaw = node['quota']
    const quota = typeof quotaRaw === 'number' && Number.isInteger(quotaRaw) && quotaRaw >= 0 ? quotaRaw : null
    return { mode, quota: mode === 'fixed' ? quota : null, omitIfEmpty: node['omitIfEmpty'] === true }
}

/** 页 flow 编辑态 → wire flow（flow null → null）：quota 仅 fixed 且非 null 输出、
 *  omitIfEmpty 仅 true 输出（键只在有意义时落，与引擎链节点键面对齐） */
export function wireFlowOf(flow: PageFlow | null): PageFlowWire | null {
    if (flow === null) return null
    const wire: PageFlowWire = { mode: flow.mode }
    if (flow.mode === 'fixed' && flow.quota !== null) wire.quota = flow.quota
    if (flow.omitIfEmpty) wire.omitIfEmpty = true
    return wire
}

/** 页 flow canonical 串（dirty 基线与 dirty 现值共用同一形态，null 归一 'null'） */
export function canonicalPageFlow(flow: PageFlow | null): string {
    return JSON.stringify(wireFlowOf(flow) ?? null)
}

/** 载入页序列（spec 页形状契约接「decodeGraph 逐页」）：每页解码即验 + canonical 化
 *  ——基线与页槽位同形，切页往返不产生伪 dirty；任一页解码失败抛错（消息带页序号，
 *  1 起，对齐服务端预检消息与前端页签「第 N 页」），由调用方做载入错误面。宽容面：
 *  id 缺失/空（服务端校验非空+唯一，此处只是防御性修复不丢会话）、name 空串、
 *  flow 非法均归一修复。 */
export function loadPageSlots(pages: ReadonlyArray<TemplatePageWire>): PageSlot[] {
    return pages.map((page, index) => {
        let graphJson: string
        try {
            graphJson = encodeGraphJson(decodeGraph(page.graph))
        } catch (e) {
            throw new Error(`第 ${index + 1} 页解码失败：${e instanceof Error ? e.message : String(e)}`)
        }
        return {
            id: typeof page.id === 'string' && page.id !== '' ? page.id : newPageId(),
            name: typeof page.name === 'string' && page.name !== '' ? page.name : null,
            graphJson,
            flow: flowFromWire(page.flow),
        }
    })
}

/** 尾部追加空白页工厂（spec 页形状契约增页）：wire 形 {canvas:{width,height},layers:[]}，
 *  幅面参数化（调用方传追加时活动页画布宽高，文档页幅一致性）。id 前端生成、name
 *  null（显示层渲染「第 N 页」）、flow null（链外页起步）。graphJson 以 canonical 串
 *  入槽，与 loadPageSlots 同缝：decode 顺带验形。 */
export function blankPageSlot(width: number, height: number): PageSlot {
    return {
        id: newPageId(),
        name: null,
        graphJson: encodeGraphJson(decodeGraph({ canvas: { width, height }, layers: [] })),
        flow: null,
    }
}

/** 初始基线：模板名 + 各页取 id/name/canonical 串/canonical flow（载荷字节口径） */
export function baselineFromPages(name: string, slots: readonly PageSlot[]): PageBaseline {
    return {
        name,
        pages: slots.map((slot) => ({
            id: slot.id,
            name: slot.name,
            graphJson: slot.graphJson,
            flow: canonicalPageFlow(slot.flow),
        })),
    }
}

/** 文档级 dirty（spec §4.4，页模型口径）：当前页 ∪ 页序列任一页（id/name/graph/flow）
 *  ∪ 模板名，任一变更即 dirty。当前页以活动文档现值（activeGraphJson）比较，其余页
 *  比槽位；无活动文档时当前页按槽位兜底（无文档则无现场编辑）。页数 ≠ 基线页数即
 *  dirty（尾部增页/删页；显式长度收紧，不依赖逐槽越界比较的隐式行为）。 */
export function isDocDirty(params: {
    baseline: PageBaseline
    slots: readonly PageSlot[]
    activeIndex: number
    activeGraphJson: string | null
    templateName: string
}): boolean {
    const { baseline, slots, activeIndex, activeGraphJson, templateName } = params
    if (templateName !== baseline.name) return true
    if (slots.length !== baseline.pages.length) return true
    return slots.some((slot, i) => {
        const base = baseline.pages[i]
        if (base === undefined) return true
        if (slot.id !== base.id) return true
        if (slot.name !== base.name) return true
        if (canonicalPageFlow(slot.flow) !== base.flow) return true
        const current = i === activeIndex && activeGraphJson !== null ? activeGraphJson : slot.graphJson
        return current !== base.graphJson
    })
}

/** 保存载荷（spec 页形状契约全量 PUT）：name + 页序列逐页（id/name/graph/flow）。
 *  调用方先把当前页快照并入槽位（与切出同一条缝），载荷即「当前页 ∪ 页序列」；页序
 *  = 槽位序（flow 的帧位置 = 页序，中插/重排后续工单解锁）。键条件写：name null 不落
 *  键（可选字段）、flow null 不落键（无 flow 键 = 链外页，spec 定案）。 */
export function buildSavePayload(params: { slots: readonly PageSlot[]; templateName: string }): TemplateWritePayload {
    return {
        name: params.templateName,
        pages: params.slots.map((slot) => ({
            id: slot.id,
            ...(slot.name !== null ? { name: slot.name } : {}),
            graph: JSON.parse(slot.graphJson),
            ...(slot.flow !== null ? { flow: wireFlowOf(slot.flow)! } : {}),
        })),
    }
}
