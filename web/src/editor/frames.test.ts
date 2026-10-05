// 帧缓冲纯逻辑测试（16 票 TDD 缝）：载入逐帧 canonical 化、文档级 dirty 口径
// （spec §4.4：当前帧 ∪ 帧缓冲任一帧 ∪ flowChain ∪ 模板名）、保存载荷组装。
// 全部 Node 无 DOM 环境，graph 造数对齐 canvas-next roundtrip 测试的 canonical 形态。
import { describe, expect, it } from 'vitest'

import { decodeGraph, encodeGraph } from '@hankchen/canvas-next'
import type { WireGraph, WireLayerNode } from '@hankchen/canvas-next'

import {
    baselineFromSlots,
    buildSavePayload,
    decodeGraphJson,
    encodeGraphJson,
    isDocDirty,
    loadFrameSlots,
    type FrameSlot,
} from './frames'

/** canonical wire 造数器：单文本层画布，text 区分两帧（键级对齐 php graph() 输出） */
function frameGraph(text: string): WireGraph {
    const node: WireLayerNode = {
        type: 'TextLayer',
        priority: 0,
        spec: {
            shape: {
                width: 100,
                height: 30,
                autoWidth: false,
                autoHeight: false,
                lineHeight: 1,
                padding: { top: 0, bottom: 0, left: 0, right: 0 },
                border: { top: null, bottom: null, left: null, right: null },
                backgroundColor: null,
            },
            align: { horizontal: 'left', vertical: 'top' },
            position: { x: 0, y: 0, position: 'top-left' },
            fontFamily: { font: '', fontSize: 12, fontColor: '#000000', angle: 0, autowrap: false },
        },
        data: { valueType: 'StaticValue', expression: '', value: text },
    }
    return { canvas: { width: 100, height: 100 }, layers: [node] }
}

/** 载入形态两帧（主页/续页）+ flowChain，直接取 wire 对象为 canvases[i].graph */
function seedCanvases(): { name: string; graph: unknown }[] {
    return [
        { name: '主页', graph: frameGraph('主页帧') },
        { name: '续页', graph: frameGraph('续页帧') },
    ]
}

describe('loadFrameSlots（载入逐帧 decode→canonical encode）', () => {
    it('两帧名保留、graphJson 与逐帧 decode-encode 往返一致', () => {
        const canvases = seedCanvases()
        const slots = loadFrameSlots(canvases)
        expect(slots.map((s) => s.name)).toEqual(['主页', '续页'])
        slots.forEach((slot, i) => {
            const canonical = JSON.stringify(encodeGraph(decodeGraph(canvases[i]!.graph)))
            expect(slot.graphJson).toBe(canonical)
            // 字符串持有形态可 decode 回同构文档
            expect(decodeGraphJson(slot.graphJson)).toEqual(decodeGraph(canvases[i]!.graph))
        })
    })

    it('canonical 输入往返恒等：load 后立即以同帧重建文档再 encode 不产生伪差异', () => {
        const slots = loadFrameSlots(seedCanvases())
        for (const slot of slots) {
            expect(encodeGraphJson(decodeGraphJson(slot.graphJson))).toBe(slot.graphJson)
        }
    })

    it('任一帧解码失败抛错并带帧下标（0 起，对齐服务端预检消息序号）', () => {
        const canvases = [
            { name: '主页', graph: frameGraph('ok') },
            { name: '坏帧', graph: { canvas: { width: 10, height: 10 }, layers: [{ type: 'NoSuchLayer' }] } },
        ]
        expect(() => loadFrameSlots(canvases)).toThrow(/第 1 帧/)
    })
})

describe('baselineFromSlots / isDocDirty（spec §4.4 文档级 dirty 口径）', () => {
    const slots: FrameSlot[] = [
        { name: '主页', graphJson: JSON.stringify(frameGraph('主页帧')) },
        { name: '续页', graphJson: JSON.stringify(frameGraph('续页帧')) },
    ]
    const flowChain = [{ frame: 0, mode: 'fixed' }]
    const baseline = baselineFromSlots('证书模板', slots, flowChain)

    it('基线 = 名 + 各帧 canonical 串 + flowChain canonical 串', () => {
        expect(baseline.name).toBe('证书模板')
        expect(baseline.frames).toEqual(slots.map((s) => s.graphJson))
        expect(baseline.flowChain).toBe(JSON.stringify(flowChain))
    })

    it('全 clean：当前帧现值=基线、各槽=基线、名/链一致 → false', () => {
        expect(
            isDocDirty({
                baseline,
                slots,
                activeIndex: 0,
                activeGraphJson: slots[0]!.graphJson,
                templateName: '证书模板',
                flowChain,
            }),
        ).toBe(false)
    })

    it('模板名变更 → true', () => {
        expect(
            isDocDirty({
                baseline,
                slots,
                activeIndex: 0,
                activeGraphJson: slots[0]!.graphJson,
                templateName: '改名了',
                flowChain,
            }),
        ).toBe(true)
    })

    it('flowChain 变更 → true', () => {
        expect(
            isDocDirty({
                baseline,
                slots,
                activeIndex: 0,
                activeGraphJson: slots[0]!.graphJson,
                templateName: '证书模板',
                flowChain: [{ frame: 0, mode: 'paged' }],
            }),
        ).toBe(true)
    })

    it('当前帧编辑（activeGraphJson ≠ 基线）→ true；切回未编辑帧恢复 false', () => {
        const edited = JSON.stringify(frameGraph('主页帧改'))
        expect(
            isDocDirty({ baseline, slots, activeIndex: 0, activeGraphJson: edited, templateName: '证书模板', flowChain }),
        ).toBe(true)
        expect(
            isDocDirty({
                baseline,
                slots,
                activeIndex: 1,
                activeGraphJson: slots[1]!.graphJson,
                templateName: '证书模板',
                flowChain,
            }),
        ).toBe(false)
    })

    it('非当前帧槽位被快照改写 → true（数据不串帧，各帧独立计入）', () => {
        const editedSlots: FrameSlot[] = [slots[0]!, { ...slots[1]!, graphJson: JSON.stringify(frameGraph('续页帧改')) }]
        expect(
            isDocDirty({
                baseline,
                slots: editedSlots,
                activeIndex: 0,
                activeGraphJson: slots[0]!.graphJson,
                templateName: '证书模板',
                flowChain,
            }),
        ).toBe(true)
    })

    it('仅帧名重命名（graph 不动）→ true（票面：tab 双击重命名进 dirty）', () => {
        const renamedSlots: FrameSlot[] = [slots[0]!, { ...slots[1]!, name: '续页 · 副页' }]
        expect(
            isDocDirty({
                baseline,
                slots: renamedSlots,
                activeIndex: 1,
                activeGraphJson: slots[1]!.graphJson,
                templateName: '证书模板',
                flowChain,
            }),
        ).toBe(true)
        // 保存归 clean：基线同步新帧名后恢复 false
        const savedBaseline = baselineFromSlots('证书模板', renamedSlots, flowChain)
        expect(
            isDocDirty({
                baseline: savedBaseline,
                slots: renamedSlots,
                activeIndex: 1,
                activeGraphJson: renamedSlots[1]!.graphJson,
                templateName: '证书模板',
                flowChain,
            }),
        ).toBe(false)
    })

    it('canonical 恒等不误报：活动文档 decode→encode 往返后与基线相等 → false', () => {
        const doc = decodeGraphJson(baseline.frames[1]!)
        expect(
            isDocDirty({
                baseline,
                slots,
                activeIndex: 1,
                activeGraphJson: encodeGraphJson(doc),
                templateName: '证书模板',
                flowChain,
            }),
        ).toBe(false)
    })

    it('无活动文档（activeGraphJson null）按槽位值兜底比较 → 匹配则 false', () => {
        expect(
            isDocDirty({ baseline, slots, activeIndex: 0, activeGraphJson: null, templateName: '证书模板', flowChain }),
        ).toBe(false)
    })

    it('flowChain 为 null/缺省时归一 null 比较（键缺省 = 空链，spec §3.1）', () => {
        const nullBaseline = baselineFromSlots('证书模板', slots, null)
        expect(
            isDocDirty({
                baseline: nullBaseline,
                slots,
                activeIndex: 0,
                activeGraphJson: slots[0]!.graphJson,
                templateName: '证书模板',
                flowChain: null,
            }),
        ).toBe(false)
        expect(
            isDocDirty({
                baseline: nullBaseline,
                slots,
                activeIndex: 0,
                activeGraphJson: slots[0]!.graphJson,
                templateName: '证书模板',
                flowChain: undefined,
            }),
        ).toBe(false)
    })
})

describe('buildSavePayload（保存 = 全量 PUT：name + 帧缓冲各帧 + flowChain 原样）', () => {
    it('canvases 从槽位组装、graph 解回 wire 对象、name/flowChain 透传', () => {
        const slots = loadFrameSlots(seedCanvases())
        const flowChain = [{ frame: 0, mode: 'fixed' }, { frame: 1, mode: 'paged', omitIfEmpty: true }]
        const payload = buildSavePayload({ slots, templateName: '结业证书', flowChain })
        expect(payload.name).toBe('结业证书')
        expect(payload.flowChain).toEqual(flowChain)
        payload.canvases.forEach((c, i) => {
            expect(c.name).toBe(seedCanvases()[i]!.name)
            // graphJson 解析回 wire 对象，decode 同构（载荷即帧缓冲字节）
            expect(decodeGraph(c.graph)).toEqual(decodeGraph(seedCanvases()[i]!.graph))
        })
    })

    it('保存期间编辑中的活动文档不影响载荷——载荷只消费已快照的槽位', () => {
        const slots = loadFrameSlots(seedCanvases())
        const payload = buildSavePayload({ slots, templateName: 'n', flowChain: null })
        expect(decodeGraph(payload.canvases[0]!.graph)).toEqual(decodeGraphJson(slots[0]!.graphJson))
    })
})
