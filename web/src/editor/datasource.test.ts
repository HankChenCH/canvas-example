// 数据源抽屉纯逻辑测试（17 票 TDD 缝，spec §4.3 双通道 / §4.4 独立标记）：
// 文本域基线与展示文本、草稿 JSON 解析（仅解析不校验——校验权威在服务端）、
// 段内独立未保存标记、数据源段载荷组装。全部 Node 无 DOM 环境。
import { describe, expect, it } from 'vitest'

import {
    datasetDraftPayload,
    drawerBaselineFromRecord,
    isSegmentDirty,
    parseJsonDraft,
    prettyJsonText,
} from './datasource'

describe('prettyJsonText（值 → 文本域展示文本）', () => {
    it('对象/数组按 2 空格 pretty 输出', () => {
        expect(prettyJsonText({ a: 1 })).toBe('{\n  "a": 1\n}')
        expect(prettyJsonText([{ frame: 0 }])).toBe('[\n  {\n    "frame": 0\n  }\n]')
    })

    it('null / undefined 归 null 字面（空链与 null 同义，spec §3.1）', () => {
        expect(prettyJsonText(null)).toBe('null')
        expect(prettyJsonText(undefined)).toBe('null')
    })
})

describe('drawerBaselineFromRecord（载入基线 = 文本域初值）', () => {
    it('三字段各自 pretty 成文本；缺省字段落 null 字面', () => {
        const baseline = drawerBaselineFromRecord({
            datasetSchema: { type: 'object' },
            dataset: { org: { name: '某机构' } },
            flowChain: [{ frame: 0, mode: 'fixed' }],
        })
        expect(baseline.schemaText).toBe('{\n  "type": "object"\n}')
        expect(baseline.dataText).toBe('{\n  "org": {\n    "name": "某机构"\n  }\n}')
        expect(baseline.flowChainText).toBe('[\n  {\n    "frame": 0,\n    "mode": "fixed"\n  }\n]')

        const empty = drawerBaselineFromRecord({ datasetSchema: null, dataset: null, flowChain: null })
        expect(empty.schemaText).toBe('null')
        expect(empty.dataText).toBe('null')
        expect(empty.flowChainText).toBe('null')
    })
})

describe('parseJsonDraft（仅解析不校验）', () => {
    it('合法 JSON 值 → ok + value（对象/数组/null 字面皆可）', () => {
        expect(parseJsonDraft('{"a":1}')).toEqual({ ok: true, value: { a: 1 } })
        expect(parseJsonDraft('[]')).toEqual({ ok: true, value: [] })
        expect(parseJsonDraft('null')).toEqual({ ok: true, value: null })
    })

    it('非法 JSON → ok: false + 非空消息（不抛异常）', () => {
        const bad = parseJsonDraft('[{"frame":0,')
        expect(bad.ok).toBe(false)
        if (!bad.ok) expect(bad.message.length).toBeGreaterThan(0)
        expect(parseJsonDraft('').ok).toBe(false)
        expect(parseJsonDraft('not json').ok).toBe(false)
    })
})

describe('isSegmentDirty（段内独立未保存标记：文本域 vs 载入基线）', () => {
    it('文本一致 → false；任何文本差异（含仅缩进）→ true', () => {
        const baseline = '{\n  "a": 1\n}'
        expect(isSegmentDirty(baseline, baseline)).toBe(false)
        expect(isSegmentDirty('{"a":1}', baseline)).toBe(true)
        expect(isSegmentDirty('{\n    "a": 1\n}', baseline)).toBe(true)
    })
})

describe('datasetDraftPayload（数据源段 → PUT /dataset 载荷）', () => {
    it('双文本域均可解析 → ok + {schema, data} 载荷', () => {
        const result = datasetDraftPayload('{"type":"object"}', '[1,2]')
        expect(result).toEqual({ ok: true, payload: { schema: { type: 'object' }, data: [1, 2] } })
    })

    it('schema 解析失败 → 指名 schema 字段 + 带字段前缀的段内直显文案，不抛异常', () => {
        const result = datasetDraftPayload('{bad', '{"a":1}')
        expect(result).toEqual({ ok: false, field: 'schema', message: expect.any(String) })
        if (!result.ok) expect(result.message).toMatch(/^schema JSON 解析失败：/)
    })

    it('data 解析失败 → 指名 data 字段；两者皆坏时 schema 优先', () => {
        expect(datasetDraftPayload('{"type":"object"}', '{bad')).toEqual({
            ok: false,
            field: 'data',
            message: expect.any(String),
        })
        const dataFail = datasetDraftPayload('{"type":"object"}', '{bad')
        if (!dataFail.ok) expect(dataFail.message).toMatch(/^data JSON 解析失败：/)
        expect(datasetDraftPayload('{bad', '{bad')).toMatchObject({ field: 'schema' })
    })
})
