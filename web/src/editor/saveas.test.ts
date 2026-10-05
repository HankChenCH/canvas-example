// 另存为纯逻辑测试（18 票 TDD 缝，spec §4.5 两连调用）：数据源复制载荷（未绑
// 数据源跳过第二步）与两连调用编排——POST /templates → PUT /templates/{newId}/dataset，
// 中途失败阶段归因（create / dataset）。全部 Node 无 DOM 环境。
import { describe, expect, it } from 'vitest'

import { ApiError } from '../api'
import type { TemplateRecord, TemplateWritePayload } from '../api'
import { datasetCopyPayload, saveAsCopy } from './saveas'

/** 模板记录造数器：只带编排消费的字段 */
function record(id: number): TemplateRecord {
    return {
        id,
        name: `模板 ${id}`,
        canvases: [],
        flowChain: null,
        datasetSchema: null,
        dataset: null,
        createdAt: '2026-10-05T00:00:00Z',
        updatedAt: '2026-10-05T00:00:00Z',
    }
}

const payload: TemplateWritePayload = { name: '副本', canvases: [], flowChain: null }
const dataset = { schema: { type: 'object' }, data: { org: { name: '某机构' } } }

/** 编排依赖造数器：记录调用序列，各步可注入响应或失败 */
function makeDeps(options?: {
    created?: TemplateRecord
    createError?: unknown
    updated?: TemplateRecord
    datasetError?: unknown
}) {
    const calls: string[] = []
    return {
        calls,
        createTemplate: async (p: TemplateWritePayload) => {
            calls.push(`create:${p.name}`)
            if (options?.createError) throw options.createError
            return options?.created ?? record(7)
        },
        putDataset: async (id: number) => {
            calls.push(`dataset:${id}`)
            if (options?.datasetError) throw options.datasetError
            return options?.updated ?? record(id)
        },
    }
}

describe('datasetCopyPayload（数据源复制载荷，spec §4.5 连带复制数据源）', () => {
    it('已绑数据源 → 原存储态 schema/data 原样随行', () => {
        expect(datasetCopyPayload({ datasetSchema: { type: 'object' }, dataset: { a: 1 } })).toEqual({
            schema: { type: 'object' },
            data: { a: 1 },
        })
    })

    it('未绑数据源（双 null）→ null = 跳过第二步：服务端 draft-07 校验 null schema 即 schema_invalid，副本 dataset 保持 POST 的 null 即与原件一致', () => {
        expect(datasetCopyPayload({ datasetSchema: null, dataset: null })).toBeNull()
    })
})

describe('saveAsCopy（两连调用编排）', () => {
    it('成功：先 POST 后 PUT dataset（newId 衔接），返回 PUT 全量记录', async () => {
        const deps = makeDeps({ created: record(7), updated: record(7) })
        const result = await saveAsCopy({
            payload,
            dataset,
            createTemplate: deps.createTemplate,
            putDataset: deps.putDataset,
        })
        expect(deps.calls).toEqual(['create:副本', 'dataset:7'])
        expect(result).toEqual({ ok: true, record: record(7) })
    })

    it('未绑数据源：只 POST 一步即完整副本（dataset 与原件同为 null），不调 PUT', async () => {
        const deps = makeDeps({ created: record(3) })
        const result = await saveAsCopy({
            payload,
            dataset: null,
            createTemplate: deps.createTemplate,
            putDataset: deps.putDataset,
        })
        expect(deps.calls).toEqual(['create:副本'])
        expect(result).toEqual({ ok: true, record: record(3) })
    })

    it('POST 失败 → create 阶段失败：稳定码文案，不调 PUT（无副本落地）', async () => {
        const deps = makeDeps({ createError: new ApiError(400, 'flow_chain_invalid', '流链不合法') })
        const result = await saveAsCopy({
            payload,
            dataset,
            createTemplate: deps.createTemplate,
            putDataset: deps.putDataset,
        })
        expect(result).toEqual({
            ok: false,
            failure: { stage: 'create', message: 'flow_chain_invalid：流链不合法' },
        })
        expect(deps.calls).toEqual(['create:副本'])
    })

    it('第二步打回 → dataset 阶段失败：稳定码文案（服务端半副本已知，客户端不进入）', async () => {
        const deps = makeDeps({ created: record(9), datasetError: new ApiError(400, 'schema_invalid', 'schema 不合法') })
        const result = await saveAsCopy({
            payload,
            dataset,
            createTemplate: deps.createTemplate,
            putDataset: deps.putDataset,
        })
        expect(result).toEqual({
            ok: false,
            failure: { stage: 'dataset', message: 'schema_invalid：schema 不合法' },
        })
        expect(deps.calls).toEqual(['create:副本', 'dataset:9'])
    })

    it('网络层异常（非 ApiError）也归入对应阶段，不向上抛', async () => {
        const deps = makeDeps({ datasetError: new TypeError('failed to fetch') })
        const result = await saveAsCopy({
            payload,
            dataset,
            createTemplate: deps.createTemplate,
            putDataset: deps.putDataset,
        })
        expect(result).toEqual({ ok: false, failure: { stage: 'dataset', message: 'TypeError: failed to fetch' } })
    })
})
