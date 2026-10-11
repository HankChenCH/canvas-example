// 回滚会话转移纯逻辑测试（template-draft-publish 批 B ③ TDD 缝，spec §5 / 票 04）：
// 全量模板记录（revert 响应）→ 新帧槽/新流链草稿/新 dirty 基线的成算。核心断言 =
// 「dirty 基线重置为新内容」——回滚后立即归 clean（与 loadTemplate/另存为同口径）。
// 纯计算无副作用：任一帧解码失败抛错（调用方原会话零变更的前提）；工程数据零特殊
// 处理由宿主接线保证（frameStates 不进本模块 = 不动的结构性声明）。
import { describe, expect, it } from 'vitest'

import { decodeGraph, encodeGraph } from '@hankchen/canvas'
import type { WireGraph, WireLayerNode } from '@hankchen/canvas'

import type { TemplateRecord } from '../api'

import { isDocDirty } from './frames'
import { planRevertSession } from './revert'

/** canonical wire 造数器：单文本层画布（frames.test.ts 同款形态） */
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
        canvases: [
            { name: '主页', graph: frameGraph('v2 主页') },
            { name: '续页', graph: frameGraph('v2 续页') },
        ],
        flowChain: [{ frame: 0, mode: 'fixed', quota: 3 }],
        dataSourceId: null,
        editorState: null,
        createdAt: '2026-10-10T08:00:00Z',
        updatedAt: '2026-10-11T09:00:00Z',
        ...overrides,
    }
}

describe('planRevertSession（回滚会话转移，spec §5）', () => {
    it('帧槽 = 逐帧 canonical 化（loadFrameSlots 同缝），帧名保留', () => {
        const plan = planRevertSession(revertRecord())
        expect(plan.slots.map((s) => s.name)).toEqual(['主页', '续页'])
        plan.slots.forEach((slot, i) => {
            expect(slot.graphJson).toBe(JSON.stringify(encodeGraph(decodeGraph(revertRecord().canvases[i]!.graph))))
        })
    })

    it('流链草稿 = 结构化归一（fixed 条目随行），基线链 = 草稿派生链', () => {
        const plan = planRevertSession(revertRecord())
        expect(plan.flowChainDraft).toHaveLength(2)
        expect(plan.flowChainDraft[0]).toEqual({ role: 'fixed', quota: 3, omitIfEmpty: false })
        expect(plan.flowChainDraft[1]).toEqual({ role: 'off', quota: null, omitIfEmpty: false })
        expect(plan.normalizedChain).toEqual([{ frame: 0, mode: 'fixed', quota: 3 }])
    })

    it('flowChain null → 空链（全 off 条目 + 派生链 null，spec §3.1 键缺省同义）', () => {
        const plan = planRevertSession(revertRecord({ flowChain: null }))
        expect(plan.flowChainDraft.every((entry) => entry.role === 'off')).toBe(true)
        expect(plan.normalizedChain).toBeNull()
    })

    it('越界/非法链节点宽容丢弃（draftFromFlowChain 归一口径经回滚面传递）', () => {
        const record = revertRecord({ flowChain: [{ frame: 5, mode: 'fixed' }, { frame: 0, mode: 'paged' }] })
        const plan = planRevertSession(record)
        expect(plan.flowChainDraft[0]!.role).toBe('paged')
        expect(plan.flowChainDraft[1]!.role).toBe('off')
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
                flowChain: plan.normalizedChain,
            }),
        ).toBe(false)
        expect(plan.baseline.name).toBe('证书模板')
    })

    it('基线逐字段 = 回滚记录：名 + 各帧 canonical 串 + 归一链 canonical 串', () => {
        const plan = planRevertSession(revertRecord())
        expect(plan.baseline.name).toBe('证书模板')
        expect(plan.baseline.names).toEqual(['主页', '续页'])
        expect(plan.baseline.frames).toEqual(plan.slots.map((slot) => slot.graphJson))
        expect(plan.baseline.flowChain).toBe(JSON.stringify([{ frame: 0, mode: 'fixed', quota: 3 }]))
    })

    it('任一帧解码失败抛错并带帧下标（会话替换前成算失败 → 调用方原会话零变更）', () => {
        const record = revertRecord({
            canvases: [{ name: '主页', graph: frameGraph('ok') }, { name: '坏帧', graph: { canvas: { width: 10, height: 10 }, layers: [{ type: 'NoSuchLayer' }] } }],
        })
        expect(() => planRevertSession(record)).toThrow(/第 1 帧/)
    })
})
