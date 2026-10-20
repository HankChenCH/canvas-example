// 回滚会话转移纯逻辑测试（template-draft-publish 批 B ③ TDD 缝，spec §5 / 票 04；
// 页模型适配工票 03）：全量模板记录（revert 响应）→ 新页槽序列/dirty 基线的成算。
// 核心断言 = 「dirty 基线重置为新内容」——回滚后立即归 clean（与 loadTemplate/另存为
// 同口径）。纯计算无副作用：任一页解码失败抛错（调用方原会话零变更的前提）；工程数据
// 零特殊处理由宿主接线保证（pageStates 不进本模块 = 不动的结构性声明）。
import { describe, expect, it } from 'vitest'

import { decodeGraph, encodeGraph } from '@hankchen/canvas'
import type { WireGraph, WireLayerNode } from '@hankchen/canvas'

import type { TemplateRecord } from '../api'

import { isDocDirty } from './pages'
import { planRevertSession } from './revert'

/** canonical wire 造数器：单文本层画布（pages.test.ts 同款形态） */
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

function revertRecord(overrides: Partial<TemplateRecord> = {}): TemplateRecord {
    return {
        id: 7,
        name: '证书模板',
        pages: [
            { id: 'pg_a', name: '主页', graph: frameGraph('v2 主页') },
            { id: 'pg_b', name: '续页', graph: frameGraph('v2 续页'), flow: { mode: 'fixed', quota: 3 } },
        ],
        dataSourceId: null,
        editorState: null,
        createdAt: '2026-10-10T08:00:00Z',
        updatedAt: '2026-10-11T09:00:00Z',
        ...overrides,
    }
}

describe('planRevertSession（回滚会话转移，spec §5）', () => {
    it('页槽 = 逐页 canonical 化（loadPageSlots 同缝），页 id/name 保留、flow 随页归一', () => {
        const plan = planRevertSession(revertRecord())
        expect(plan.slots.map((s) => s.id)).toEqual(['pg_a', 'pg_b'])
        expect(plan.slots.map((s) => s.name)).toEqual(['主页', '续页'])
        plan.slots.forEach((slot, i) => {
            expect(slot.graphJson).toBe(JSON.stringify(encodeGraph(decodeGraph(revertRecord().pages[i]!.graph))))
        })
        expect(plan.slots[0]!.flow).toBeNull()
        expect(plan.slots[1]!.flow).toEqual({ mode: 'fixed', quota: 3, omitIfEmpty: false })
    })

    it('flow 非法节点宽容归 null（flowFromWire 归一口径经回滚面传递）', () => {
        // 故意违反页 wire 契约的 flow（宽容读守卫矩阵）：unknown 侧写绕过类型面
        const record = revertRecord({
            pages: [
                { id: 'pg_a', graph: frameGraph('v2 主页'), flow: { mode: 'weird' } },
                { id: 'pg_b', graph: frameGraph('v2 续页') },
            ] as TemplateRecord['pages'],
        })
        const plan = planRevertSession(record)
        expect(plan.slots.map((s) => s.flow)).toEqual([null, null])
    })

    it('dirty 基线重置为新内容：回滚态立即归 clean（核心断言，另存为同口径）', () => {
        const plan = planRevertSession(revertRecord())
        expect(
            isDocDirty({
                baseline: plan.baseline,
                slots: plan.slots,
                activeIndex: 0,
                activeGraphJson: plan.slots[0]!.graphJson,
                templateName: plan.baseline.name,
            }),
        ).toBe(false)
        expect(plan.baseline.name).toBe('证书模板')
    })

    it('基线逐字段 = 回滚记录字节：名 + 各页 id/name/graph/flow canonical 串', () => {
        const plan = planRevertSession(revertRecord())
        expect(plan.baseline.name).toBe('证书模板')
        expect(plan.baseline.pages.map((p) => p.id)).toEqual(['pg_a', 'pg_b'])
        expect(plan.baseline.pages.map((p) => p.name)).toEqual(['主页', '续页'])
        expect(plan.baseline.pages.map((p) => p.graphJson)).toEqual(plan.slots.map((slot) => slot.graphJson))
        expect(plan.baseline.pages[1]!.flow).toBe('{"mode":"fixed","quota":3}')
    })

    it('任一页解码失败抛错并带页序号（1 起，对齐页签序号；会话替换前成算失败 → 调用方原会话零变更）', () => {
        const record = revertRecord({
            pages: [
                { id: 'pg_a', graph: frameGraph('ok') },
                { id: 'pg_b', graph: { canvas: { width: 10, height: 10 }, layers: [{ type: 'NoSuchLayer' }] } },
            ],
        })
        expect(() => planRevertSession(record)).toThrow(/第 2 页/)
    })
})
