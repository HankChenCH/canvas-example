// 帧缓冲纯逻辑（16 票，spec §4.2 多帧 / §4.4 文档级 dirty）：宿主内存帧缓冲的
// 载入、dirty 口径与保存载荷组装。与 Vue/DOM 解耦，EditorPage 只做响应式桥接。
// 约定：帧缓冲条目的 graphJson 以 canonical encode（decode→encode 往返恒等形态）
// 的 JSON 字符串持有——dirty 比较与保存载荷直接消费字符串，避免对大 wire 对象
// 反复 stringify，也保证「切出快照 ↔ 基线」两侧同形不产生伪差异。
import { decodeGraph, encodeGraph } from '@hankchen/canvas'
import type { Canvas } from '@hankchen/canvas'

import type { TemplateWritePayload } from '../api'

/** 帧缓冲条目：name 即 canvases[i].name（tab 显示与双击重命名对象），graphJson
 *  为该帧 canonical encode 的 JSON 字符串 */
export interface FrameSlot {
    name: string
    graphJson: string
}

/** 文档级 dirty 基线（spec §4.4）：载入（或保存成功）时的模板名 + 各帧名（tab
 *  重命名进 dirty，票面口径）+ 各帧 canonical 串 + flowChain canonical 串 */
export interface FrameBaseline {
    name: string
    names: string[]
    frames: string[]
    flowChain: string
}

/** 文档 → canonical JSON 串（切出快照与 dirty 现值共用同一条缝） */
export function encodeGraphJson(doc: Canvas): string {
    return JSON.stringify(encodeGraph(doc))
}

/** canonical JSON 串 → 文档（切帧重建会话文档用） */
export function decodeGraphJson(graphJson: string): Canvas {
    return decodeGraph(JSON.parse(graphJson))
}

/** 载入帧缓冲（spec §4.2 接线序「decodeGraph 逐帧」）：每帧解码即验 + canonical 化
 *  ——基线与帧缓冲同形，切帧往返不产生伪 dirty；任一帧解码失败抛错（消息带帧
 *  下标，0 起，对齐服务端预检消息序号），由调用方做载入错误面 */
export function loadFrameSlots(canvases: ReadonlyArray<{ name: string; graph: unknown }>): FrameSlot[] {
    return canvases.map((canvas, index) => {
        let graphJson: string
        try {
            graphJson = encodeGraphJson(decodeGraph(canvas.graph))
        } catch (e) {
            throw new Error(`第 ${index} 帧解码失败：${e instanceof Error ? e.message : String(e)}`)
        }
        return { name: canvas.name, graphJson }
    })
}

/** flowChain 归一（键缺省 = null = 空链，spec §3.1）：dirty 基线、dirty 现值与
 *  保存基线三处共用同一 canonical 形态，归一规则单点维护 */
export function canonicalFlowChain(flowChain: unknown): string {
    return JSON.stringify(flowChain ?? null)
}

/** 尾部追加空白帧工厂（26 票，spec §4.2 增帧）：wire 形
 *  {canvas:{width,height},layers:[]}，幅面参数化（调用方传追加时活动帧画布宽高，
 *  文档页幅一致性）；帧名「第 N 帧」（N = 下标 + 1，与 22 票「第 1 帧」同系确定性
 *  命名；尾部追加时下标 = 追加时帧数，不查重——帧名是显示用非唯一键）。graphJson
 *  以 canonical 串入槽，与 loadFrameSlots 同缝：decode 顺带验形。 */
export function blankFrameSlot(width: number, height: number, index: number): FrameSlot {
    return {
        name: `第 ${index + 1} 帧`,
        graphJson: encodeGraphJson(decodeGraph({ canvas: { width, height }, layers: [] })),
    }
}

/** 初始基线：各帧取槽位名与 canonical 串，flowChain 归一 canonicalFlowChain */
export function baselineFromSlots(name: string, slots: readonly FrameSlot[], flowChain: unknown): FrameBaseline {
    return {
        name,
        names: slots.map((slot) => slot.name),
        frames: slots.map((slot) => slot.graphJson),
        flowChain: canonicalFlowChain(flowChain),
    }
}

/** 文档级 dirty（spec §4.4）：当前帧 ∪ 帧缓冲任一帧（graph 或帧名）∪ flowChain ∪
 *  模板名，任一变更即 dirty。当前帧以活动文档现值（activeGraphJson）比较，其余帧
 *  比槽位；无活动文档时当前帧按槽位兜底（无文档则无现场编辑）。槽位数 ≠ 基线帧数
 *  即 dirty（26 票尾部增帧；显式长度收紧，不依赖逐槽越界比较的隐式行为）。 */
export function isDocDirty(params: {
    baseline: FrameBaseline
    slots: readonly FrameSlot[]
    activeIndex: number
    activeGraphJson: string | null
    templateName: string
    flowChain: unknown
}): boolean {
    const { baseline, slots, activeIndex, activeGraphJson, templateName, flowChain } = params
    if (templateName !== baseline.name) return true
    if (canonicalFlowChain(flowChain) !== baseline.flowChain) return true
    if (slots.length !== baseline.frames.length || slots.length !== baseline.names.length) return true
    return slots.some((slot, i) => {
        if (slot.name !== baseline.names[i]) return true
        const current = i === activeIndex && activeGraphJson !== null ? activeGraphJson : slot.graphJson
        return current !== baseline.frames[i]
    })
}

/** 保存载荷（spec §4.4 全量 PUT）：name + 帧缓冲各帧 + flowChain 原样透传。调用方
 *  先把当前帧快照并入槽位（与切出同一条缝），载荷即「当前帧 ∪ 帧缓冲」；帧序 =
 *  槽位序（服务端 flowChain 按帧下标引用，序不可动）。 */
export function buildSavePayload(params: {
    slots: readonly FrameSlot[]
    templateName: string
    flowChain: unknown
}): TemplateWritePayload {
    return {
        name: params.templateName,
        canvases: params.slots.map((slot) => ({ name: slot.name, graph: JSON.parse(slot.graphJson) })),
        flowChain: params.flowChain,
    }
}
