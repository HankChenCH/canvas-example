// 页槽位纯逻辑测试（模板页序列工票 03 TDD 缝，spec 页形状契约）：载入逐页 canonical
// 化 + flow 宽容归一、文档级 dirty 口径（spec §4.4 页模型版：当前页 ∪ 页序列任一页
// id/name/graph/flow ∪ 模板名）、保存载荷组装。全部 Node 无 DOM 环境，graph 造数对齐
// @hankchen/canvas roundtrip 测试的 canonical 形态。
import { describe, expect, it } from 'vitest'

import { decodeGraph, encodeGraph } from '@hankchen/canvas'
import type { WireGraph, WireLayerNode } from '@hankchen/canvas'

import type { TemplatePageWire } from '../api'

import {
    baselineFromPages,
    blankPageSlot,
    buildSavePayload,
    canonicalPageFlow,
    decodeGraphJson,
    encodeGraphJson,
    flowFromWire,
    isDocDirty,
    loadPageSlots,
    wireFlowOf,
    type PageSlot,
} from './pages'

/** canonical wire 造数器：单文本层画布，text 区分两页（键级对齐 php graph() 输出） */
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

/** 载入形态两页（主页/续页），直接取 wire 对象为 pages[i].graph */
function seedPages(): TemplatePageWire[] {
    return [
        { id: 'pg_a', name: '主页', graph: frameGraph('主页画布') },
        { id: 'pg_b', name: '续页', graph: frameGraph('续页画布'), flow: { mode: 'fixed', quota: 3 } },
    ]
}

describe('loadPageSlots（载入逐页 decode→canonical encode + 宽容归一）', () => {
    it('id/name 保留、graphJson 与逐页 decode-encode 往返一致、flow 随页归一', () => {
        const pages = seedPages()
        const slots = loadPageSlots(pages)
        expect(slots.map((s) => s.id)).toEqual(['pg_a', 'pg_b'])
        expect(slots.map((s) => s.name)).toEqual(['主页', '续页'])
        slots.forEach((slot, i) => {
            const canonical = JSON.stringify(encodeGraph(decodeGraph(pages[i]!.graph)))
            expect(slot.graphJson).toBe(canonical)
            // 字符串持有形态可 decode 回同构文档
            expect(decodeGraphJson(slot.graphJson)).toEqual(decodeGraph(pages[i]!.graph))
        })
        expect(slots[0]!.flow).toBeNull()
        expect(slots[1]!.flow).toEqual({ mode: 'fixed', quota: 3, omitIfEmpty: false })
    })

    it('canonical 输入往返恒等：load 后立即以同页重建文档再 encode 不产生伪差异', () => {
        const slots = loadPageSlots(seedPages())
        for (const slot of slots) {
            expect(encodeGraphJson(decodeGraphJson(slot.graphJson))).toBe(slot.graphJson)
        }
    })

    it('任一页解码失败抛错并带页序号（1 起，对齐服务端预检消息与前端页签「第 N 页」）', () => {
        const pages = [
            { id: 'pg_a', graph: frameGraph('ok') },
            { id: 'pg_b', graph: { canvas: { width: 10, height: 10 }, layers: [{ type: 'NoSuchLayer' }] } },
        ]
        expect(() => loadPageSlots(pages)).toThrow(/第 2 页/)
    })

    it('宽容归一：name 缺省/空串归 null（显示层渲染「第 N 页」）；id 缺失防御性补号；flow 非法归 null', () => {
        // 故意违反页 wire 契约的输入（宽容读的守卫矩阵）：unknown 侧写绕过类型面
        const pages: TemplatePageWire[] = [
            { id: 'pg_a', graph: frameGraph('p1') },
            { id: 'pg_b', name: '', graph: frameGraph('p2'), flow: { mode: 'weird' } },
            { id: '', name: '无 id 页', graph: frameGraph('p3'), flow: 'broken' },
        ] as unknown as TemplatePageWire[]
        const slots = loadPageSlots(pages)
        expect(slots[0]!.name).toBeNull()
        expect(slots[1]!.name).toBeNull()
        expect(slots[1]!.flow).toBeNull()
        expect(slots[2]!.id).not.toBe('')
        expect(slots[2]!.id).toMatch(/^pg_/)
        expect(slots[2]!.name).toBe('无 id 页')
        expect(slots[2]!.flow).toBeNull()
    })

    it('flow 宽容归一（flowFromWire）：未知键忽略、paged 携带 quota 归 null、显式 omitIfEmpty:false 归 false', () => {
        expect(flowFromWire({ mode: 'fixed', quota: 3, extra: 'x' })).toEqual({
            mode: 'fixed',
            quota: 3,
            omitIfEmpty: false,
        })
        expect(flowFromWire({ mode: 'paged', quota: 2 })).toEqual({ mode: 'paged', quota: null, omitIfEmpty: false })
        expect(flowFromWire({ mode: 'fixed', omitIfEmpty: false })).toEqual({
            mode: 'fixed',
            quota: null,
            omitIfEmpty: false,
        })
        // 非法面：非对象 / mode 非法 / quota 形态非法（负数、小数、非数）
        expect(flowFromWire(null)).toBeNull()
        expect(flowFromWire(undefined)).toBeNull()
        expect(flowFromWire('fixed')).toBeNull()
        expect(flowFromWire({ mode: 'off' })).toBeNull()
        expect(flowFromWire({ mode: 'fixed', quota: -1 })).toEqual({ mode: 'fixed', quota: null, omitIfEmpty: false })
        expect(flowFromWire({ mode: 'fixed', quota: 1.5 })).toEqual({ mode: 'fixed', quota: null, omitIfEmpty: false })
        expect(flowFromWire({ mode: 'fixed', quota: '3' })).toEqual({ mode: 'fixed', quota: null, omitIfEmpty: false })
    })

    it('wireFlowOf 条件写键：quota 仅 fixed 且非 null 输出、omitIfEmpty 仅 true 输出；null → null', () => {
        expect(wireFlowOf(null)).toBeNull()
        expect(wireFlowOf({ mode: 'fixed', quota: 3, omitIfEmpty: false })).toEqual({ mode: 'fixed', quota: 3 })
        expect(wireFlowOf({ mode: 'fixed', quota: null, omitIfEmpty: true })).toEqual({ mode: 'fixed', omitIfEmpty: true })
        expect(wireFlowOf({ mode: 'paged', quota: null, omitIfEmpty: true })).toEqual({ mode: 'paged', omitIfEmpty: true })
        // canonical 串与 wire 落键同缝：flow null 归一 'null'（空链与 null 同义）
        expect(canonicalPageFlow(null)).toBe('null')
        expect(canonicalPageFlow({ mode: 'fixed', quota: 3, omitIfEmpty: false })).toBe('{"mode":"fixed","quota":3}')
    })
})

describe('baselineFromPages / isDocDirty（spec §4.4 页模型 dirty 口径）', () => {
    const slots: PageSlot[] = [
        { id: 'pg_a', name: '主页', graphJson: JSON.stringify(frameGraph('主页画布')), flow: null },
        {
            id: 'pg_b',
            name: '续页',
            graphJson: JSON.stringify(frameGraph('续页画布')),
            flow: { mode: 'fixed', quota: 3, omitIfEmpty: false },
        },
    ]
    const baseline = baselineFromPages('证书模板', slots)

    it('基线 = 名 + 各页载荷字节（id/name/graph/flow canonical 串）', () => {
        expect(baseline.name).toBe('证书模板')
        expect(baseline.pages.map((p) => p.id)).toEqual(['pg_a', 'pg_b'])
        expect(baseline.pages.map((p) => p.name)).toEqual(['主页', '续页'])
        expect(baseline.pages.map((p) => p.graphJson)).toEqual(slots.map((s) => s.graphJson))
        expect(baseline.pages.map((p) => p.flow)).toEqual(slots.map((s) => canonicalPageFlow(s.flow)))
    })

    it('全 clean：当前页现值=基线、各槽=基线、名一致 → false', () => {
        expect(
            isDocDirty({
                baseline,
                slots,
                activeIndex: 0,
                activeGraphJson: slots[0]!.graphJson,
                templateName: '证书模板',
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
            }),
        ).toBe(true)
    })

    it('页 flow 变更 → true；改回后归 false（页模型下 flow 进 dirty）', () => {
        const flowed: PageSlot[] = [slots[0]!, { ...slots[1]!, flow: { mode: 'paged', quota: null, omitIfEmpty: true } }]
        expect(
            isDocDirty({
                baseline,
                slots: flowed,
                activeIndex: 0,
                activeGraphJson: slots[0]!.graphJson,
                templateName: '证书模板',
            }),
        ).toBe(true)
        expect(
            isDocDirty({
                baseline,
                slots,
                activeIndex: 0,
                activeGraphJson: slots[0]!.graphJson,
                templateName: '证书模板',
            }),
        ).toBe(false)
    })

    it('当前页编辑（activeGraphJson ≠ 基线）→ true；切回未编辑页恢复 false', () => {
        const edited = JSON.stringify(frameGraph('主页画布改'))
        expect(
            isDocDirty({ baseline, slots, activeIndex: 0, activeGraphJson: edited, templateName: '证书模板' }),
        ).toBe(true)
        expect(
            isDocDirty({
                baseline,
                slots,
                activeIndex: 1,
                activeGraphJson: slots[1]!.graphJson,
                templateName: '证书模板',
            }),
        ).toBe(false)
    })

    it('非当前页槽位被快照改写 → true（数据不串页，各页独立计入）', () => {
        const editedSlots: PageSlot[] = [
            slots[0]!,
            { ...slots[1]!, graphJson: JSON.stringify(frameGraph('续页画布改')) },
        ]
        expect(
            isDocDirty({
                baseline,
                slots: editedSlots,
                activeIndex: 0,
                activeGraphJson: slots[0]!.graphJson,
                templateName: '证书模板',
            }),
        ).toBe(true)
    })

    it('尾部追加新页 → true：页数超基线即 dirty（26 票口径，显式长度收紧）', () => {
        const appended: PageSlot[] = [...slots, blankPageSlot(100, 100)]
        expect(
            isDocDirty({
                baseline,
                slots: appended,
                activeIndex: 0,
                activeGraphJson: slots[0]!.graphJson,
                templateName: '证书模板',
            }),
        ).toBe(true)
    })

    it('追加后保存基线同步（baselineFromPages 以当页槽位重建）→ 恢复 false', () => {
        const appended: PageSlot[] = [...slots, blankPageSlot(100, 100)]
        const savedBaseline = baselineFromPages('证书模板', appended)
        expect(
            isDocDirty({
                baseline: savedBaseline,
                slots: appended,
                activeIndex: appended.length - 1,
                activeGraphJson: appended[appended.length - 1]!.graphJson,
                templateName: '证书模板',
            }),
        ).toBe(false)
    })

    it('仅页名重命名（graph/flow/id 不动）→ true（票面：tab 双击重命名进 dirty）；保存归 clean', () => {
        const renamedSlots: PageSlot[] = [slots[0]!, { ...slots[1]!, name: '续页 · 副页' }]
        expect(
            isDocDirty({
                baseline,
                slots: renamedSlots,
                activeIndex: 1,
                activeGraphJson: slots[1]!.graphJson,
                templateName: '证书模板',
            }),
        ).toBe(true)
        const savedBaseline = baselineFromPages('证书模板', renamedSlots)
        expect(
            isDocDirty({
                baseline: savedBaseline,
                slots: renamedSlots,
                activeIndex: 1,
                activeGraphJson: renamedSlots[1]!.graphJson,
                templateName: '证书模板',
            }),
        ).toBe(false)
    })

    it('页 id 变更 → true（id 随载荷上行，替换页即字节变更；适用域 = 本实现钉死的 dirty 口径）', () => {
        const swapped: PageSlot[] = [slots[0]!, { ...slots[1]!, id: 'pg_other' }]
        expect(
            isDocDirty({
                baseline,
                slots: swapped,
                activeIndex: 1,
                activeGraphJson: slots[1]!.graphJson,
                templateName: '证书模板',
            }),
        ).toBe(true)
    })

    it('canonical 恒等不误报：活动文档 decode→encode 往返后与基线相等 → false', () => {
        const doc = decodeGraphJson(baseline.pages[1]!.graphJson)
        expect(
            isDocDirty({
                baseline,
                slots,
                activeIndex: 1,
                activeGraphJson: encodeGraphJson(doc),
                templateName: '证书模板',
            }),
        ).toBe(false)
    })

    it('无活动文档（activeGraphJson null）按槽位值兜底比较 → 匹配则 false', () => {
        expect(
            isDocDirty({ baseline, slots, activeIndex: 0, activeGraphJson: null, templateName: '证书模板' }),
        ).toBe(false)
    })
})

describe('blankPageSlot（26 票口径尾部追加空白页工厂，页模型版）', () => {
    it('id 前端生成（pg_ 前缀）、name null（显示层渲染「第 N 页」）、flow null 链外页起步', () => {
        const slot = blankPageSlot(794, 1123)
        expect(slot.id).toMatch(/^pg_/)
        expect(slot.name).toBeNull()
        expect(slot.flow).toBeNull()
        expect(JSON.parse(slot.graphJson)).toEqual({ canvas: { width: 794, height: 1123 }, layers: [] })
    })

    it('graphJson 为 canonical 串：decode 顺带验形，往返恒等（与 loadPageSlots 同缝）', () => {
        const slot = blankPageSlot(794, 1123)
        expect(encodeGraphJson(decodeGraphJson(slot.graphJson))).toBe(slot.graphJson)
    })

    it('幅面参数化：同工厂出任意宽高空页文档；两次调用 id 页内唯一', () => {
        const a = blankPageSlot(640, 480)
        const b = blankPageSlot(640, 480)
        const doc = decodeGraphJson(a.graphJson)
        expect(doc.width).toBe(640)
        expect(doc.height).toBe(480)
        expect(doc.layers).toEqual([])
        expect(a.id).not.toBe(b.id)
    })
})

describe('buildSavePayload（保存 = 全量 PUT：name + 页序列各页，spec 页形状契约）', () => {
    it('pages 从槽位组装：id/name 条件落键、graph 解回 wire 对象、flow 条件落键', () => {
        const slots = loadPageSlots(seedPages())
        const payload = buildSavePayload({ slots, templateName: '结业证书' })
        expect(payload.name).toBe('结业证书')
        expect(payload.pages).toHaveLength(2)
        expect(payload.pages[0]).toEqual({ id: 'pg_a', name: '主页', graph: JSON.parse(slots[0]!.graphJson) })
        expect(payload.pages[1]).toEqual({
            id: 'pg_b',
            name: '续页',
            graph: JSON.parse(slots[1]!.graphJson),
            flow: { mode: 'fixed', quota: 3 },
        })
        payload.pages.forEach((p, i) => {
            // graphJson 解析回 wire 对象，decode 同构（载荷即页槽字节）
            expect(decodeGraph(p.graph)).toEqual(decodeGraph(seedPages()[i]!.graph))
        })
    })

    it('保存期间编辑中的活动文档不影响载荷——载荷只消费已快照的槽位', () => {
        const slots = loadPageSlots(seedPages())
        const payload = buildSavePayload({ slots, templateName: 'n' })
        expect(decodeGraph(payload.pages[0]!.graph)).toEqual(decodeGraphJson(slots[0]!.graphJson))
    })

    it('追加新页后载荷 pages 长 N+1 且含空白新页（无 name/flow 键），原有页原样（26 票口径）', () => {
        const slots = loadPageSlots(seedPages())
        const appended: PageSlot[] = [...slots, blankPageSlot(794, 1123)]
        const payload = buildSavePayload({ slots: appended, templateName: '证书模板' })
        expect(payload.pages).toHaveLength(3)
        expect(payload.pages[0]!.id).toBe('pg_a')
        expect(payload.pages[1]!.id).toBe('pg_b')
        expect(payload.pages[2]!.id).toMatch(/^pg_/)
        expect('name' in payload.pages[2]!).toBe(false)
        expect('flow' in payload.pages[2]!).toBe(false)
        expect(payload.pages[2]!.graph).toEqual({ canvas: { width: 794, height: 1123 }, layers: [] })
    })

    it('载荷为保存往返恒等缝的一半：build 载荷 → loadPageSlots 回读同构槽位（flow 容忍 null 同义）', () => {
        const slots = loadPageSlots(seedPages())
        const payload = buildSavePayload({ slots, templateName: '结业证书' })
        const reloaded = loadPageSlots(payload.pages)
        expect(reloaded.map((s) => ({ id: s.id, name: s.name, flow: s.flow }))).toEqual(
            slots.map((s) => ({ id: s.id, name: s.name, flow: s.flow })),
        )
        expect(reloaded.map((s) => s.graphJson)).toEqual(slots.map((s) => s.graphJson))
    })
})
