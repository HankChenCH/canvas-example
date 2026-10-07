// 从零新建空白模板纯逻辑测试（22 票 TDD 缝，spec §4.1 列表页新建入口）：空白
// 载荷工厂形状与编排——名字裁剪/空名就地拒绝、POST 失败稳定码文案。全部 Node
// 无 DOM 环境。（24 票追加底图快捷建模板：载荷工厂 + 编排同缝测试。）
import { describe, expect, it } from 'vitest'

import { decodeGraph, encodeGraph } from '@hankchen/canvas'

import { ApiError } from '../api'
import type { TemplateRecord, TemplateWritePayload } from '../api'
import {
    blankTemplatePayload,
    createBlankTemplate,
    createImageTemplate,
    imageTemplatePayload,
    type BaseImageFile,
} from './newtemplate'

/** 模板记录造数器：只带编排消费的字段 */
function record(id: number): TemplateRecord {
    return {
        id,
        name: '未命名模板',
        canvases: [],
        flowChain: null,
        dataSourceId: null,
        createdAt: '2026-10-06T00:00:00Z',
        updatedAt: '2026-10-06T00:00:00Z',
    }
}

describe('blankTemplatePayload（空白模板载荷，spec §2.4 #3 形状）', () => {
    it('单帧空白 A4 竖版图（帧名「第 1 帧」）+ flowChain null + 未绑数据源', () => {
        expect(blankTemplatePayload('未命名模板')).toEqual({
            name: '未命名模板',
            canvases: [
                {
                    name: '第 1 帧',
                    // 形状与 §6.2 冒烟断言 4 同源：服务端预检合法最小模板
                    graph: { canvas: { width: 794, height: 1123 }, layers: [] },
                },
            ],
            flowChain: null,
        })
    })
})

describe('createBlankTemplate（从零建模板编排）', () => {
    it('成功：名字裁剪后进载荷，返回 POST 全量记录', async () => {
        const calls: TemplateWritePayload[] = []
        const result = await createBlankTemplate({
            name: '  未命名模板  ',
            createTemplate: async (payload) => {
                calls.push(payload)
                return record(9)
            },
        })
        expect(calls.map((p) => p.name)).toEqual(['未命名模板'])
        expect(result).toEqual({ ok: true, record: record(9) })
    })

    it('空名/纯空白就地拒绝不打服务端（服务端无 DELETE，防弃置不可回收）', async () => {
        let called = 0
        for (const name of ['', '   ']) {
            const result = await createBlankTemplate({ name, createTemplate: async () => { called += 1; return record(1) } })
            expect(result).toEqual({ ok: false, message: '模板名不能为空' })
        }
        expect(called).toBe(0)
    })

    it('POST 打回 → 失败文案稳定码在前（不抛），无记录落地', async () => {
        const result = await createBlankTemplate({
            name: '未命名模板',
            createTemplate: async () => {
                throw new ApiError(400, 'unknown_layer_type', '未知图层类型')
            },
        })
        expect(result).toEqual({ ok: false, message: 'unknown_layer_type：未知图层类型' })
    })

    it('网络层异常（非 ApiError）同样归入失败文案，不向上抛', async () => {
        const result = await createBlankTemplate({
            name: '未命名模板',
            createTemplate: async () => {
                throw new TypeError('failed to fetch')
            },
        })
        expect(result).toEqual({ ok: false, message: 'TypeError: failed to fetch' })
    })
})

/** 底图造数器：字节/尺寸由浏览器通道（选图解码）产出，这里给定值 */
function baseImage(overrides: Partial<BaseImageFile> = {}): BaseImageFile {
    return {
        name: 'poster.png',
        mime: 'image/png',
        bytes: new Uint8Array([1, 2, 3]),
        width: 1240,
        height: 1754,
        ...overrides,
    }
}

describe('imageTemplatePayload（底图模板载荷，24 票）', () => {
    it('单帧图：画布宽高 = 图片像素宽高，单层「底图」铺满画布 + StaticValue 引用', () => {
        expect(imageTemplatePayload('海报模板', { src: '/assets/u/abc.png', width: 1240, height: 1754 })).toEqual({
            name: '海报模板',
            canvases: [
                {
                    name: '第 1 帧',
                    graph: {
                        canvas: { width: 1240, height: 1754 },
                        layers: [
                            {
                                type: 'ImageLayer',
                                name: '底图',
                                // 最高优先级 = 视觉最底（seed「页面底色」同约定）
                                priority: 100,
                                spec: {
                                    shape: {
                                        width: 1240,
                                        height: 1754,
                                        autoWidth: false,
                                        autoHeight: false,
                                        lineHeight: 1,
                                        padding: { top: 0, bottom: 0, left: 0, right: 0 },
                                        border: { top: null, bottom: null, left: null, right: null },
                                        backgroundColor: null,
                                    },
                                    align: { horizontal: 'left', vertical: 'top' },
                                    position: { x: 0, y: 0, position: 'top-left' },
                                },
                                // 前导斜杠引用串原样进 data.value（StaticValue 两键形态，
                                // 三端 wire 同源：PHP ImageLayer::fromGraph / Go n.Data.Value / JS decode）
                                data: { valueType: 'StaticValue', value: '/assets/u/abc.png' },
                            },
                        ],
                    },
                },
            ],
            flowChain: null,
        })
    })

    it('载荷 graph 过 decodeGraph → encodeGraph 往返恒等（与编辑器 canonical 产出字节对齐）', () => {
        const payload = imageTemplatePayload('海报模板', { src: '/assets/u/abc.png', width: 800, height: 600 })
        const graph = payload.canvases[0]!.graph
        expect(encodeGraph(decodeGraph(graph))).toEqual(graph)
    })

    it('解码语义：画布尺寸与底图 src 落域正确', () => {
        const payload = imageTemplatePayload('海报模板', { src: '/assets/u/abc.png', width: 800, height: 600 })
        const doc = decodeGraph(payload.canvases[0]!.graph)
        expect(doc.width).toBe(800)
        expect(doc.height).toBe(600)
        expect(doc.layers).toHaveLength(1)
        expect(doc.layers[0]).toMatchObject({ type: 'ImageLayer', name: '底图', src: '/assets/u/abc.png' })
    })
})

describe('createImageTemplate（底图建模板编排，24 票）', () => {
    it('成功：先上传后建模板，url 进载荷 data.value，返回 POST 全量记录', async () => {
        const calls: string[] = []
        const uploaded: unknown[] = []
        let payloadSeen: TemplateWritePayload | null = null
        const result = await createImageTemplate({
            name: '  海报模板  ',
            image: baseImage(),
            uploadAsset: async (file) => {
                uploaded.push(file)
                calls.push('upload')
                return { url: '/assets/u/abc.png' }
            },
            createTemplate: async (payload) => {
                payloadSeen = payload
                calls.push('create')
                return record(7)
            },
        })
        expect(calls).toEqual(['upload', 'create'])
        expect(uploaded).toEqual([{ name: 'poster.png', mime: 'image/png', bytes: baseImage().bytes }])
        expect((payloadSeen as TemplateWritePayload | null)?.canvases[0]?.graph).toMatchObject({
            canvas: { width: 1240, height: 1754 },
        })
        expect(result).toEqual({ ok: true, record: record(7) })
    })

    it('空名就地拒绝：不上传不建模板', async () => {
        let called = 0
        const result = await createImageTemplate({
            name: '   ',
            image: baseImage(),
            uploadAsset: async () => { called += 1; return { url: '/assets/u/x.png' } },
            createTemplate: async () => { called += 1; return record(1) },
        })
        expect(result).toEqual({ ok: false, message: '模板名不能为空' })
        expect(called).toBe(0)
    })

    it('未选图就地拒绝：不上传不建模板', async () => {
        let called = 0
        const result = await createImageTemplate({
            name: '海报模板',
            image: null,
            uploadAsset: async () => { called += 1; return { url: '/assets/u/x.png' } },
            createTemplate: async () => { called += 1; return record(1) },
        })
        expect(result).toEqual({ ok: false, message: '请先选择底图图片' })
        expect(called).toBe(0)
    })

    it('上传打回 → 失败文案稳定码在前（不抛），不建模板', async () => {
        let created = 0
        const result = await createImageTemplate({
            name: '海报模板',
            image: baseImage(),
            uploadAsset: async () => {
                throw new ApiError(413, 'request_too_large', '请求体超限')
            },
            createTemplate: async () => { created += 1; return record(1) },
        })
        expect(result).toEqual({ ok: false, message: 'request_too_large：请求体超限' })
        expect(created).toBe(0)
    })

    it('建模板打回（含网络层异常）同样归入失败文案，不向上抛', async () => {
        const apiFailure = await createImageTemplate({
            name: '海报模板',
            image: baseImage(),
            uploadAsset: async () => ({ url: '/assets/u/abc.png' }),
            createTemplate: async () => {
                throw new ApiError(400, 'frames_empty', '模板不含任何帧')
            },
        })
        expect(apiFailure).toEqual({ ok: false, message: 'frames_empty：模板不含任何帧' })

        const networkFailure = await createImageTemplate({
            name: '海报模板',
            image: baseImage(),
            uploadAsset: async () => ({ url: '/assets/u/abc.png' }),
            createTemplate: async () => {
                throw new TypeError('failed to fetch')
            },
        })
        expect(networkFailure).toEqual({ ok: false, message: 'TypeError: failed to fetch' })
    })
})
