// 从零新建空白模板纯逻辑测试（22 票 TDD 缝，spec §4.1 列表页新建入口）：空白
// 载荷工厂形状与编排——名字裁剪/空名就地拒绝、POST 失败稳定码文案。全部 Node
// 无 DOM 环境。（24 票追加图片快捷建模板：载荷工厂 + 编排同缝测试；ADR 0022
// 迁移后载荷为画布背景形态。）
import { describe, expect, it } from 'vitest'

import { decodeGraph, encodeGraph } from '@hankchen/canvas'

import { ApiError } from '../api'
import type { TemplateCreatePayload, TemplateRecord } from '../api'
import {
    blankTemplatePayload,
    createBlankTemplate,
    createImageTemplate,
    imageTemplatePayload,
    type BackgroundImageFile,
} from './newtemplate'

/** 模板记录造数器：只带编排消费的字段 */
function record(id: number): TemplateRecord {
    return {
        id,
        name: '未命名模板',
        canvases: [],
        flowChain: null,
        dataSourceId: null,
        editorState: null,
        createdAt: '2026-10-06T00:00:00Z',
        updatedAt: '2026-10-06T00:00:00Z',
    }
}

describe('blankTemplatePayload（空白模板载荷，spec §2.4 #3 形状）', () => {
    it('单帧空白 A4 竖版图（帧名「第 1 帧」）+ flowChain null + 未绑数据源（dataSourceId null = 键省略）', () => {
        expect(blankTemplatePayload('未命名模板', null)).toEqual({
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

    it('随建随绑：传入数据源 id 时载荷带 dataSourceId（POST 通道存在性先验由服务端把守）', () => {
        expect(blankTemplatePayload('未命名模板', 3)).toEqual({
            name: '未命名模板',
            canvases: [
                {
                    name: '第 1 帧',
                    graph: { canvas: { width: 794, height: 1123 }, layers: [] },
                },
            ],
            flowChain: null,
            dataSourceId: 3,
        })
    })
})

describe('createBlankTemplate（从零建模板编排）', () => {
    it('成功：名字裁剪后进载荷，dataSourceId 随建随绑透传，返回 POST 全量记录', async () => {
        const calls: TemplateCreatePayload[] = []
        const result = await createBlankTemplate({
            name: '  未命名模板  ',
            dataSourceId: 5,
            createTemplate: async (payload) => {
                calls.push(payload)
                return record(9)
            },
        })
        expect(calls.map((p) => p.name)).toEqual(['未命名模板'])
        expect(calls[0]?.dataSourceId).toBe(5)
        expect(result).toEqual({ ok: true, record: record(9) })
    })

    it('dataSourceId null（暂不绑定）：载荷省略该键，编排行为不变', async () => {
        const payloads: TemplateCreatePayload[] = []
        await createBlankTemplate({
            name: '未命名模板',
            dataSourceId: null,
            createTemplate: async (payload) => {
                payloads.push(payload)
                return record(1)
            },
        })
        expect(payloads).toHaveLength(1)
        expect('dataSourceId' in payloads[0]!).toBe(false)
    })

    it('空名/纯空白就地拒绝不打服务端（服务端无 DELETE，防弃置不可回收）', async () => {
        let called = 0
        for (const name of ['', '   ']) {
            const result = await createBlankTemplate({ name, dataSourceId: null, createTemplate: async () => { called += 1; return record(1) } })
            expect(result).toEqual({ ok: false, message: '模板名不能为空' })
        }
        expect(called).toBe(0)
    })

    it('POST 打回 → 失败文案稳定码在前（不抛），无记录落地', async () => {
        const result = await createBlankTemplate({
            name: '未命名模板',
            dataSourceId: null,
            createTemplate: async () => {
                throw new ApiError(400, 'unknown_layer_type', '未知图层类型')
            },
        })
        expect(result).toEqual({ ok: false, message: 'unknown_layer_type：未知图层类型' })
    })

    it('网络层异常（非 ApiError）同样归入失败文案，不向上抛', async () => {
        const result = await createBlankTemplate({
            name: '未命名模板',
            dataSourceId: null,
            createTemplate: async () => {
                throw new TypeError('failed to fetch')
            },
        })
        expect(result).toEqual({ ok: false, message: 'TypeError: failed to fetch' })
    })
})

/** 待建背景图造数器：字节/尺寸由浏览器通道（选图解码）产出，这里给定值 */
function backgroundImage(overrides: Partial<BackgroundImageFile> = {}): BackgroundImageFile {
    return {
        name: 'poster.png',
        mime: 'image/png',
        bytes: new Uint8Array([1, 2, 3]),
        width: 1240,
        height: 1754,
        ...overrides,
    }
}

describe('imageTemplatePayload（图片模板载荷：画布背景形态，24 票 + ADR 0022 迁移）', () => {
    it('单帧图：画布宽高 = 图片像素宽高，图片声明为画布背景（background 只落 image 分量，color 未设不落键）+ layers 空', () => {
        expect(imageTemplatePayload('海报模板', { src: '/assets/u/abc.png', width: 1240, height: 1754 }, null)).toEqual({
            name: '海报模板',
            canvases: [
                {
                    name: '第 1 帧',
                    graph: {
                        // wire 逐键同 @hankchen/canvas encodeGraph canonical 输出：
                        // canvas 键序 width,height,background；background 仅 image
                        // 分量落键（条件写键，spec §3）
                        canvas: { width: 1240, height: 1754, background: { image: '/assets/u/abc.png' } },
                        layers: [],
                    },
                },
            ],
            flowChain: null,
        })
    })

    it('随建随绑：传入数据源 id 时载荷带 dataSourceId', () => {
        const payload = imageTemplatePayload('海报模板', { src: '/assets/u/abc.png', width: 800, height: 600 }, 11)
        expect(payload.dataSourceId).toBe(11)
    })

    it('载荷 graph 过 decodeGraph → encodeGraph 往返恒等（与编辑器 canonical 产出字节对齐）', () => {
        const payload = imageTemplatePayload('海报模板', { src: '/assets/u/abc.png', width: 800, height: 600 }, null)
        const graph = payload.canvases[0]!.graph
        expect(encodeGraph(decodeGraph(graph))).toEqual(graph)
    })

    it('解码语义：画布尺寸与背景图引用落域正确，背景不产生图层', () => {
        const payload = imageTemplatePayload('海报模板', { src: '/assets/u/abc.png', width: 800, height: 600 }, null)
        const doc = decodeGraph(payload.canvases[0]!.graph)
        expect(doc.width).toBe(800)
        expect(doc.height).toBe(600)
        expect(doc.background).toEqual({ color: null, image: '/assets/u/abc.png' })
        expect(doc.layers).toHaveLength(0)
    })
})

describe('createImageTemplate（图片建模板编排，24 票）', () => {
    it('成功：先上传后建模板，url 进画布背景 background.image，dataSourceId 随建随绑透传', async () => {
        const calls: string[] = []
        const uploaded: unknown[] = []
        let payloadSeen: TemplateCreatePayload | null = null
        const result = await createImageTemplate({
            name: '  海报模板  ',
            dataSourceId: 7,
            image: backgroundImage(),
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
        expect(uploaded).toEqual([{ name: 'poster.png', mime: 'image/png', bytes: backgroundImage().bytes }])
        expect((payloadSeen as TemplateCreatePayload | null)?.canvases[0]?.graph).toMatchObject({
            canvas: { width: 1240, height: 1754, background: { image: '/assets/u/abc.png' } },
        })
        expect((payloadSeen as TemplateCreatePayload | null)?.dataSourceId).toBe(7)
        expect(result).toEqual({ ok: true, record: record(7) })
    })

    it('空名就地拒绝：不上传不建模板', async () => {
        let called = 0
        const result = await createImageTemplate({
            name: '   ',
            dataSourceId: null,
            image: backgroundImage(),
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
            dataSourceId: null,
            image: null,
            uploadAsset: async () => { called += 1; return { url: '/assets/u/x.png' } },
            createTemplate: async () => { called += 1; return record(1) },
        })
        expect(result).toEqual({ ok: false, message: '请先选择背景图片' })
        expect(called).toBe(0)
    })

    it('上传打回 → 失败文案稳定码在前（不抛），不建模板', async () => {
        let created = 0
        const result = await createImageTemplate({
            name: '海报模板',
            dataSourceId: null,
            image: backgroundImage(),
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
            dataSourceId: null,
            image: backgroundImage(),
            uploadAsset: async () => ({ url: '/assets/u/abc.png' }),
            createTemplate: async () => {
                throw new ApiError(400, 'frames_empty', '模板不含任何帧')
            },
        })
        expect(apiFailure).toEqual({ ok: false, message: 'frames_empty：模板不含任何帧' })

        const networkFailure = await createImageTemplate({
            name: '海报模板',
            dataSourceId: null,
            image: backgroundImage(),
            uploadAsset: async () => ({ url: '/assets/u/abc.png' }),
            createTemplate: async () => {
                throw new TypeError('failed to fetch')
            },
        })
        expect(networkFailure).toEqual({ ok: false, message: 'TypeError: failed to fetch' })
    })
})
