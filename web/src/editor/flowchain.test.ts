// 结构化流链草稿纯逻辑测试（29 票 TDD 缝，spec §4.2 流链编辑器）：结构化帧条目
// ⇄ wire FlowChainNode[] 双向转换、增/删帧的草稿同步、本地校验（UI 拦截面——
// 校验权威仍在服务端保存预检）、帧 wire 顶层表信息解析（rowsPath 提示用）。
// 全部 Node 无 DOM 环境。
import { describe, expect, it } from 'vitest'

import {
    appendFrameToDraft,
    deleteFrameFromDraft,
    draftFromFlowChain,
    flowChainFromDraft,
    frameTableInfo,
    validateFlowChainDraft,
    type FrameChainEntry,
    type FrameChainFrameInfo,
} from './flowchain'

/** seed 默认模板流链（example/server/seed/flow-chain.json 同形） */
const seedChain = [{ frame: 0, mode: 'fixed' }, { frame: 1, mode: 'paged', omitIfEmpty: true }]

/** 全 off 条目断言捷径 */
function offEntry(): FrameChainEntry {
    return { role: 'off', quota: null, omitIfEmpty: false }
}

/** 链上帧信息造数器 */
function frameInfo(name: string, table: { tableCount: number; rowsPath: string | null; hasTemplate: boolean } | null): FrameChainFrameInfo {
    return { name, table }
}

const okTable = { tableCount: 1, rowsPath: 'certificates', hasTemplate: true }

describe('draftFromFlowChain（wire 链 → 结构化帧条目，宽容归一）', () => {
    it('null/undefined/空数组 → frameCount 条全 off（空链与 null 同义，spec §3.1）', () => {
        const expected = [offEntry(), offEntry()]
        expect(draftFromFlowChain(null, 2)).toEqual(expected)
        expect(draftFromFlowChain(undefined, 2)).toEqual(expected)
        expect(draftFromFlowChain([], 2)).toEqual(expected)
    })

    it('seed 双节点链 → 帧 0 fixed（无 quota）、帧 1 paged + omitIfEmpty', () => {
        expect(draftFromFlowChain(seedChain, 2)).toEqual([
            { role: 'fixed', quota: null, omitIfEmpty: false },
            { role: 'paged', quota: null, omitIfEmpty: true },
        ])
    })

    it('fixed 携带 quota 落位；quota 仅 fixed 有意义（paged 携带归 null）', () => {
        const draft = draftFromFlowChain(
            [{ frame: 0, mode: 'fixed', quota: 3 }, { frame: 1, mode: 'paged', quota: 2 }],
            2,
        )
        expect(draft[0]).toEqual({ role: 'fixed', quota: 3, omitIfEmpty: false })
        expect(draft[1]).toEqual({ role: 'paged', quota: null, omitIfEmpty: false })
    })

    it('显式 omitIfEmpty:false 与缺省同归 false', () => {
        const draft = draftFromFlowChain([{ frame: 0, mode: 'fixed', omitIfEmpty: false }], 1)
        expect(draft[0]).toEqual({ role: 'fixed', quota: null, omitIfEmpty: false })
    })

    it('宽容归一：未知键忽略（存储态经服务端预检不可能出现，归一即修复）', () => {
        const draft = draftFromFlowChain([{ frame: 0, mode: 'fixed', extra: 'x' }], 1)
        expect(draft[0]).toEqual({ role: 'fixed', quota: null, omitIfEmpty: false })
    })

    it('越界/非法节点丢弃（frame 非整数或超出帧数、mode 非 fixed|paged、节点非对象）', () => {
        expect(draftFromFlowChain([{ frame: 5, mode: 'fixed' }], 2)).toEqual([offEntry(), offEntry()])
        expect(draftFromFlowChain([{ frame: -1, mode: 'fixed' }], 2)).toEqual([offEntry(), offEntry()])
        expect(draftFromFlowChain([{ frame: 0.5, mode: 'fixed' }], 2)).toEqual([offEntry(), offEntry()])
        expect(draftFromFlowChain([{ frame: 'x', mode: 'fixed' }], 2)).toEqual([offEntry(), offEntry()])
        expect(draftFromFlowChain([{ frame: 0, mode: 'weird' }], 2)).toEqual([offEntry(), offEntry()])
        expect(draftFromFlowChain([null, 42], 2)).toEqual([offEntry(), offEntry()])
    })

    it('链上帧下标对应的 off 帧不影响其余条目（off 帧可夹在链中间）', () => {
        const draft = draftFromFlowChain(
            [{ frame: 0, mode: 'fixed' }, { frame: 2, mode: 'paged' }],
            3,
        )
        expect(draft[0]).toEqual({ role: 'fixed', quota: null, omitIfEmpty: false })
        expect(draft[1]).toEqual(offEntry())
        expect(draft[2]).toEqual({ role: 'paged', quota: null, omitIfEmpty: false })
    })
})

describe('flowChainFromDraft（结构化条目 → wire 链，全 off 归 null）', () => {
    it('全 off → null（空链与 null 同义）', () => {
        expect(flowChainFromDraft([offEntry(), offEntry()])).toBeNull()
        expect(flowChainFromDraft([])).toBeNull()
    })

    it('seed 草稿 → seed 链（封闭 4 键、quota/omitIfEmpty 仅在值有意义时输出）', () => {
        expect(flowChainFromDraft(draftFromFlowChain(seedChain, 2))).toEqual(seedChain)
    })

    it('fixed 携带 quota 输出 quota 键；quota null 不输出；omitIfEmpty false 不输出', () => {
        expect(
            flowChainFromDraft([
                { role: 'fixed', quota: 3, omitIfEmpty: false },
                { role: 'paged', quota: null, omitIfEmpty: true },
            ]),
        ).toEqual([{ frame: 0, mode: 'fixed', quota: 3 }, { frame: 1, mode: 'paged', omitIfEmpty: true }])
    })

    it('off 帧夹链中：节点只含链上帧，frame = 条目下标（升序天然保持）', () => {
        expect(
            flowChainFromDraft([
                { role: 'fixed', quota: null, omitIfEmpty: false },
                offEntry(),
                { role: 'paged', quota: null, omitIfEmpty: false },
            ]),
        ).toEqual([{ frame: 0, mode: 'fixed' }, { frame: 2, mode: 'paged' }])
    })

    it('往返恒等：draftFromFlowChain(flowChainFromDraft(d), n) === d', () => {
        const d = draftFromFlowChain(
            [{ frame: 0, mode: 'fixed', quota: 2 }, { frame: 2, mode: 'paged', omitIfEmpty: true }],
            4,
        )
        expect(draftFromFlowChain(flowChainFromDraft(d), 4)).toEqual(d)
    })
})

describe('deleteFrameFromDraft（删帧草稿同步，27 票链重写语义的结构化承接）', () => {
    it('删中间帧 = 条目 splice：被删帧出链、其余条目下标自动前移、内容原样', () => {
        const draft = draftFromFlowChain(
            [{ frame: 0, mode: 'fixed' }, { frame: 1, mode: 'fixed', quota: 3 }, { frame: 2, mode: 'paged', omitIfEmpty: true }],
            3,
        )
        const next = deleteFrameFromDraft(draft, 1)
        expect(next).toEqual([
            { role: 'fixed', quota: null, omitIfEmpty: false },
            { role: 'paged', quota: null, omitIfEmpty: true },
        ])
        // 链输出与旧 rewriteFlowChainForDeletion 语义逐节点等价（下标前移后）
        expect(flowChainFromDraft(next)).toEqual([
            { frame: 0, mode: 'fixed' },
            { frame: 1, mode: 'paged', omitIfEmpty: true },
        ])
    })

    it('删链内帧至全 off：派生链归 null（空链与 null 同义）', () => {
        const draft = draftFromFlowChain([{ frame: 0, mode: 'fixed' }], 2)
        expect(flowChainFromDraft(deleteFrameFromDraft(draft, 0))).toBeNull()
    })

    it('原草稿不被原地修改（不可变更新，ref 替换触发 watch）', () => {
        const draft = draftFromFlowChain(seedChain, 2)
        const next = deleteFrameFromDraft(draft, 0)
        expect(draft).toHaveLength(2)
        expect(draft[0]).toEqual({ role: 'fixed', quota: null, omitIfEmpty: false })
        expect(next).toHaveLength(1)
    })
})

describe('appendFrameToDraft（增帧草稿同步，26 票尾部追加）', () => {
    it('尾部追加 off 条目，既有条目原样（尾部追加下标无一失效）', () => {
        const draft = draftFromFlowChain(seedChain, 2)
        const next = appendFrameToDraft(draft)
        expect(next).toEqual([...draft, offEntry()])
        expect(flowChainFromDraft(next)).toEqual(seedChain)
    })
})

describe('validateFlowChainDraft（本地校验，UI 拦截面）', () => {
    it('seed 草稿 + 两帧同 rowsPath 模板表 → 无错误', () => {
        const draft = draftFromFlowChain(seedChain, 2)
        expect(
            validateFlowChainDraft(draft, [frameInfo('主页', okTable), frameInfo('续页', okTable)]),
        ).toEqual([])
    })

    it('双 paged → 报「paged 至多一个」（帧名指认）', () => {
        const draft = draftFromFlowChain([{ frame: 0, mode: 'paged' }, { frame: 1, mode: 'paged' }], 2)
        const errors = validateFlowChainDraft(draft, [frameInfo('主页', okTable), frameInfo('续页', okTable)])
        expect(errors).toHaveLength(1)
        expect(errors[0]).toContain('paged 至多一个')
        expect(errors[0]).toContain('主页')
        expect(errors[0]).toContain('续页')
    })

    it('paged 之后还有链上帧 → 报「paged 必须在链尾」', () => {
        const draft = draftFromFlowChain([{ frame: 0, mode: 'paged' }, { frame: 1, mode: 'fixed' }], 2)
        const errors = validateFlowChainDraft(draft, [frameInfo('主页', okTable), frameInfo('续页', okTable)])
        expect(errors.some((e) => e.includes('paged 必须在链尾'))).toBe(true)
    })

    it('链上帧无模板态顶层表 → 报错指认帧名（tableCount 0 与 hasTemplate false 均拦）', () => {
        const noTable = frameInfo('空白', { tableCount: 0, rowsPath: null, hasTemplate: false })
        const notTemplate = frameInfo('实例表', { tableCount: 1, rowsPath: 'certificates', hasTemplate: false })
        const draft = draftFromFlowChain([{ frame: 0, mode: 'fixed' }], 2)
        expect(validateFlowChainDraft(draft, [noTable, frameInfo('续页', okTable)]).some((e) => e.includes('空白'))).toBe(true)
        expect(validateFlowChainDraft(draft, [notTemplate, frameInfo('续页', okTable)]).some((e) => e.includes('实例表'))).toBe(true)
    })

    it('链内 rowsPath 不一致 → 报错含两个帧名与两条路径', () => {
        const draft = draftFromFlowChain([{ frame: 0, mode: 'fixed' }, { frame: 1, mode: 'paged' }], 2)
        const errors = validateFlowChainDraft(draft, [
            frameInfo('主页', okTable),
            frameInfo('续页', { tableCount: 1, rowsPath: 'other', hasTemplate: true }),
        ])
        expect(errors).toHaveLength(1)
        expect(errors[0]).toContain('rowsPath 不一致')
        expect(errors[0]).toContain('certificates')
        expect(errors[0]).toContain('other')
    })

    it('表信息解析失败（null）跳过表相关校验——前端尽力提示，权威在服务端', () => {
        const draft = draftFromFlowChain(seedChain, 2)
        expect(validateFlowChainDraft(draft, [frameInfo('主页', null), frameInfo('续页', null)])).toEqual([])
    })

    it('空链（全 off）不触发任何表相关校验（链外帧不限定）', () => {
        const draft = draftFromFlowChain(null, 2)
        expect(
            validateFlowChainDraft(draft, [
                frameInfo('主页', { tableCount: 0, rowsPath: null, hasTemplate: false }),
                frameInfo('续页', null),
            ]),
        ).toEqual([])
    })
})

describe('frameTableInfo（帧 wire 顶层表信息解析，rowsPath 提示用）', () => {
    it('seed 主页 wire 形 → 恰 1 张模板态表、rowsPath certificates', () => {
        const wire = {
            canvas: { width: 794, height: 1123 },
            layers: [
                { type: 'ImageLayer', name: '底色', spec: {}, data: { valueType: 'StaticValue', value: null } },
                {
                    type: 'TableLayer',
                    name: '证书卡片区',
                    spec: {},
                    data: { rowsPath: 'certificates' },
                    template: { type: 'TableRowTemplate', spec: {}, cells: [] },
                },
            ],
        }
        expect(frameTableInfo(JSON.stringify(wire))).toEqual(okTable)
    })

    it('无表帧 → tableCount 0、rowsPath null、hasTemplate false；坏 JSON → null', () => {
        const wire = { canvas: { width: 100, height: 100 }, layers: [] }
        expect(frameTableInfo(JSON.stringify(wire))).toEqual({ tableCount: 0, rowsPath: null, hasTemplate: false })
        expect(frameTableInfo('not-json')).toBeNull()
    })

    it('多表取首个模板态表的 rowsPath；非模板态表不计 hasTemplate', () => {
        const wire = {
            canvas: { width: 100, height: 100 },
            layers: [
                { type: 'TableLayer', spec: {}, data: { rowsPath: 'loose' } },
                { type: 'TableLayer', spec: {}, data: { rowsPath: 'certificates' }, template: { type: 'TableRowTemplate' } },
            ],
        }
        expect(frameTableInfo(JSON.stringify(wire))).toEqual({
            tableCount: 2,
            rowsPath: 'certificates',
            hasTemplate: true,
        })
    })
})
