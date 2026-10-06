// 数据源抽屉纯逻辑测试（23 票重构 TDD 缝，spec §4.3 三段式）：数据源实体
// 基线与展示文本、草稿 JSON 解析（仅解析不校验——校验权威在服务端）、段内
// 独立未保存标记、绑定变更判定、内容段载荷组装。全部 Node 无 DOM 环境。
import { describe, expect, it } from 'vitest'

import type { DataSourceRecord } from '../api'
import {
    dataSourceDraftPayload,
    isBindingChange,
    isSegmentDirty,
    parseJsonDraft,
    prettyJsonText,
    sourceDraftBaseline,
} from './datasource'

function source(id: number, overrides?: Partial<DataSourceRecord>): DataSourceRecord {
    return {
        id,
        name: `数据源 ${id}`,
        schema: { type: 'object' },
        data: { org: { name: '某机构' } },
        createdAt: '2026-10-06T00:00:00Z',
        updatedAt: '2026-10-06T00:00:00Z',
        ...overrides,
    }
}

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

describe('sourceDraftBaseline（内容段基线 = 绑定的数据源实体文本）', () => {
    it('已绑：name 原样、schema/data pretty 成文本', () => {
        const baseline = sourceDraftBaseline(source(7, { name: '证书数据' }))
        expect(baseline.nameText).toBe('证书数据')
        expect(baseline.schemaText).toBe('{\n  "type": "object"\n}')
        expect(baseline.dataText).toBe('{\n  "org": {\n    "name": "某机构"\n  }\n}')
    })

    it('未绑：三文本域空串（创建并绑定的起笔态）', () => {
        expect(sourceDraftBaseline(null)).toEqual({ nameText: '', schemaText: '', dataText: '' })
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

describe('isBindingChange（绑定段判定）', () => {
    it('选中项与当前绑定不同 → true（含绑定↔解绑互转）', () => {
        expect(isBindingChange(null, 3)).toBe(true)
        expect(isBindingChange(3, null)).toBe(true)
        expect(isBindingChange(3, 4)).toBe(true)
    })

    it('选中项与当前绑定一致 → false（动作钮禁用依据）', () => {
        expect(isBindingChange(3, 3)).toBe(false)
        expect(isBindingChange(null, null)).toBe(false)
    })
})

describe('dataSourceDraftPayload（内容段三文本域 → POST/PUT datasources 载荷）', () => {
    it('三文本域齐备 → ok + {name, schema, data} 载荷（name 裁剪）', () => {
        const result = dataSourceDraftPayload('  证书数据 ', '{"type":"object"}', '[1,2]')
        expect(result).toEqual({
            ok: true,
            payload: { name: '证书数据', schema: { type: 'object' }, data: [1, 2] },
        })
    })

    it('名字为空/纯空白 → 指名 name 字段，不打服务端', () => {
        const result = dataSourceDraftPayload('   ', '{"type":"object"}', '{}')
        expect(result).toEqual({ ok: false, field: 'name', message: expect.any(String) })
    })

    it('schema 解析失败 → 指名 schema 字段 + 带字段前缀的段内直显文案，不抛异常', () => {
        const result = dataSourceDraftPayload('名', '{bad', '{"a":1}')
        expect(result).toEqual({ ok: false, field: 'schema', message: expect.any(String) })
        if (!result.ok) expect(result.message).toMatch(/^schema JSON 解析失败：/)
    })

    it('data 解析失败 → 指名 data 字段；name 判定优先，schema 次之', () => {
        expect(dataSourceDraftPayload('名', '{"type":"object"}', '{bad')).toEqual({
            ok: false,
            field: 'data',
            message: expect.any(String),
        })
        expect(dataSourceDraftPayload('', '{bad', '{bad')).toMatchObject({ field: 'name' })
        expect(dataSourceDraftPayload('名', '{bad', '{bad')).toMatchObject({ field: 'schema' })
    })
})
