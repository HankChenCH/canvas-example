// 页 flow 编辑纯逻辑测试（模板页序列工票 03 TDD 缝，spec 页形状契约）：页 flow 角色
// 读数（off = 无 flow 键 = 链外页）、本地校验（UI 拦截面——校验权威仍在服务端保存
// 预检）、页 wire 顶层表信息解析（rowsPath 提示用）。全部 Node 无 DOM 环境。
// 平行链草稿数组（flowchain.ts 时代）的 draftFromFlowChain/flowChainFromDraft/增删
// 同步测试随模型退役——flow 转换钉在 pages.test.ts（flowFromWire/wireFlowOf）。
import { describe, expect, it } from 'vitest'

import {
    flowRoleOf,
    pageTableInfo,
    validatePageFlows,
    type PageFlowRowInfo,
} from './pageflow'
import type { PageFlow } from './pages'

/** seed 默认模板两页 flow（example/server/seed 形状语义：fixed + paged·omitIfEmpty） */
const seedFlows: PageFlow[] = [
    { mode: 'fixed', quota: null, omitIfEmpty: false },
    { mode: 'paged', quota: null, omitIfEmpty: true },
]

/** 链上页信息造数器 */
function pageInfo(id: string, label: string, table: { tableCount: number; rowsPath: string | null; hasTemplate: boolean } | null): PageFlowRowInfo {
    return { id, label, table }
}

const okTable = { tableCount: 1, rowsPath: 'certificates', hasTemplate: true }

describe('flowRoleOf（页 flow 角色读数）', () => {
    it('flow null = off（键缺省与 off 同义，不设第三态）；fixed/paged 透传', () => {
        expect(flowRoleOf(null)).toBe('off')
        expect(flowRoleOf({ mode: 'fixed', quota: null, omitIfEmpty: false })).toBe('fixed')
        expect(flowRoleOf({ mode: 'paged', quota: null, omitIfEmpty: false })).toBe('paged')
    })
})

describe('validatePageFlows（本地校验，UI 拦截面；flows 与 pages 页序对齐）', () => {
    it('seed 双页 flow + 两页同 rowsPath 模板表 → 无错误', () => {
        expect(
            validatePageFlows(seedFlows, [pageInfo('pg_a', '主页', okTable), pageInfo('pg_b', '续页', okTable)]),
        ).toEqual([])
    })

    it('双 paged → 报「paged 至多一个」（页名指认）', () => {
        const flows: PageFlow[] = [
            { mode: 'paged', quota: null, omitIfEmpty: false },
            { mode: 'paged', quota: null, omitIfEmpty: false },
        ]
        const errors = validatePageFlows(flows, [pageInfo('pg_a', '主页', okTable), pageInfo('pg_b', '续页', okTable)])
        expect(errors).toHaveLength(1)
        expect(errors[0]).toContain('paged 至多一个')
        expect(errors[0]).toContain('主页')
        expect(errors[0]).toContain('续页')
    })

    it('paged 之后还有链上页 → 报「paged 必须在链尾」', () => {
        const flows: PageFlow[] = [
            { mode: 'paged', quota: null, omitIfEmpty: false },
            { mode: 'fixed', quota: null, omitIfEmpty: false },
        ]
        const errors = validatePageFlows(flows, [pageInfo('pg_a', '主页', okTable), pageInfo('pg_b', '续页', okTable)])
        expect(errors.some((e) => e.includes('paged 必须在链尾'))).toBe(true)
    })

    it('链上页无模板态顶层表 → 报错指认页名（tableCount 0 与 hasTemplate false 均拦）', () => {
        const noTable = pageInfo('pg_a', '空白', { tableCount: 0, rowsPath: null, hasTemplate: false })
        const notTemplate = pageInfo('pg_a', '实例表', { tableCount: 1, rowsPath: 'certificates', hasTemplate: false })
        const flows: PageFlow[] = [{ mode: 'fixed', quota: null, omitIfEmpty: false }, { mode: 'paged', quota: null, omitIfEmpty: false }]
        expect(validatePageFlows(flows, [noTable, pageInfo('pg_b', '续页', okTable)]).some((e) => e.includes('空白'))).toBe(true)
        expect(validatePageFlows(flows, [notTemplate, pageInfo('pg_b', '续页', okTable)]).some((e) => e.includes('实例表'))).toBe(true)
    })

    it('链内 rowsPath 不一致 → 报错含两个页名与两条路径', () => {
        const flows: PageFlow[] = [
            { mode: 'fixed', quota: null, omitIfEmpty: false },
            { mode: 'paged', quota: null, omitIfEmpty: false },
        ]
        const errors = validatePageFlows(flows, [
            pageInfo('pg_a', '主页', okTable),
            pageInfo('pg_b', '续页', { tableCount: 1, rowsPath: 'other', hasTemplate: true }),
        ])
        expect(errors).toHaveLength(1)
        expect(errors[0]).toContain('rowsPath 不一致')
        expect(errors[0]).toContain('certificates')
        expect(errors[0]).toContain('other')
    })

    it('表信息解析失败（null）跳过表相关校验——前端尽力提示，权威在服务端', () => {
        expect(
            validatePageFlows(seedFlows, [pageInfo('pg_a', '主页', null), pageInfo('pg_b', '续页', null)]),
        ).toEqual([])
    })

    it('空链（全 off，flow 全 null）不触发任何表相关校验（链外页不限定）', () => {
        expect(
            validatePageFlows([null, null], [
                pageInfo('pg_a', '主页', { tableCount: 0, rowsPath: null, hasTemplate: false }),
                pageInfo('pg_b', '续页', null),
            ]),
        ).toEqual([])
    })

    it('链外页夹链中（off 页可夹在链中间）：链上页序仍按页序升序判链尾', () => {
        const flows: Array<PageFlow | null> = [
            { mode: 'fixed', quota: null, omitIfEmpty: false },
            null,
            { mode: 'paged', quota: null, omitIfEmpty: false },
        ]
        expect(
            validatePageFlows(flows, [
                pageInfo('pg_a', '主页', okTable),
                pageInfo('pg_b', '夹链页', okTable),
                pageInfo('pg_c', '尾页', okTable),
            ]),
        ).toEqual([])
        // paged 不在链尾（页序意义下）仍拦
        const tailFixed: Array<PageFlow | null> = [
            { mode: 'paged', quota: null, omitIfEmpty: false },
            null,
            { mode: 'fixed', quota: null, omitIfEmpty: false },
        ]
        expect(
            validatePageFlows(tailFixed, [
                pageInfo('pg_a', '主页', okTable),
                pageInfo('pg_b', '夹链页', okTable),
                pageInfo('pg_c', '尾页', okTable),
            ]).some((e) => e.includes('paged 必须在链尾')),
        ).toBe(true)
    })
})

describe('pageTableInfo（页 wire 顶层表信息解析，rowsPath 提示用）', () => {
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
        expect(pageTableInfo(JSON.stringify(wire))).toEqual(okTable)
    })

    it('无表页 → tableCount 0、rowsPath null、hasTemplate false；坏 JSON → null', () => {
        const wire = { canvas: { width: 100, height: 100 }, layers: [] }
        expect(pageTableInfo(JSON.stringify(wire))).toEqual({ tableCount: 0, rowsPath: null, hasTemplate: false })
        expect(pageTableInfo('not-json')).toBeNull()
    })

    it('多表取首个模板态表的 rowsPath；非模板态表不计 hasTemplate', () => {
        const wire = {
            canvas: { width: 100, height: 100 },
            layers: [
                { type: 'TableLayer', spec: {}, data: { rowsPath: 'loose' } },
                { type: 'TableLayer', spec: {}, data: { rowsPath: 'certificates' }, template: { type: 'TableRowTemplate' } },
            ],
        }
        expect(pageTableInfo(JSON.stringify(wire))).toEqual({
            tableCount: 2,
            rowsPath: 'certificates',
            hasTemplate: true,
        })
    })
})
