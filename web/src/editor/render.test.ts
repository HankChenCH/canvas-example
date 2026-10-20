// 渲染终图纯逻辑测试（19 票 TDD 缝，spec §4.6）：单张终图下载文件名（结果抽屉
// 缩略图的 <a download> 建议名）+ 渲染历史版本标注 + 渲染促存链成算。Node 无 DOM
// 环境。
import { describe, expect, it } from 'vitest'

import { ApiError } from '../api'
import type { RenderImage } from '../api'
import { isPdfRenderImage, renderPdfFileName, renderImageFileName, renderVersionLabel, runRenderFlow } from './render'

describe('renderImageFileName（单张终图下载文件名，spec §4.6 <a download>）', () => {
    it('模板名-终图-页序-帧名.png：页序取帧序×页序扁平下标（1 起），同帧多页不重名', () => {
        const home: RenderImage = { frame: 0, name: '主页', url: '/renders/7/1.png' }
        const cont1: RenderImage = { frame: 1, name: '续页', url: '/renders/7/2.png' }
        const cont2: RenderImage = { frame: 1, name: '续页', url: '/renders/7/3.png' }
        expect(renderImageFileName('结业证书', home, 0)).toBe('结业证书-终图-1-主页.png')
        expect(renderImageFileName('结业证书', cont1, 1)).toBe('结业证书-终图-2-续页.png')
        expect(renderImageFileName('结业证书', cont2, 2)).toBe('结业证书-终图-3-续页.png')
    })

    it('模板名与帧名中的文件名非法字符折叠为下划线（与预览导出 previewFileName 同口径）', () => {
        expect(
            renderImageFileName('a/b\\c:d*e?f"g<h>i|j', { frame: 0, name: '主/页', url: '/renders/1/1.png' }, 0),
        ).toBe('a_b_c_d_e_f_g_h_i_j-终图-1-主_页.png')
    })

    it('空模板名落 template 兜底；空帧名落 帧N 兜底', () => {
        expect(renderImageFileName('', { frame: 2, name: '', url: '/renders/1/1.png' }, 0)).toBe(
            'template-终图-1-帧2.png',
        )
        expect(renderImageFileName('   ', { frame: 0, name: '主页', url: '/renders/1/1.png' }, 4)).toBe(
            'template-终图-5-主页.png',
        )
    })
})

describe('renderVersionLabel（渲染记录版本标注，渲染历史对比词汇）', () => {
    it('null = 草稿渲染（诚实语义），正整数 = vN', () => {
        expect(renderVersionLabel(null)).toBe('草稿')
        expect(renderVersionLabel(1)).toBe('v1')
        expect(renderVersionLabel(12)).toBe('v12')
    })
})

describe('runRenderFlow（渲染促存链编排，对齐 runPublishFlow 促存先例）', () => {
    const record = { id: 7, templateId: 1, version: null, createdAt: '2026-10-16T00:00:00Z', images: [] }

    it('clean 直接渲染，不触发无谓保存', async () => {
        let saved = 0
        let rendered = 0
        const outcome = await runRenderFlow({
            dirty: false,
            save: async () => {
                saved += 1
                return true
            },
            render: async () => {
                rendered += 1
                return record
            },
        })
        expect(saved).toBe(0)
        expect(rendered).toBe(1)
        expect(outcome).toEqual({ ok: true, record, saved: false })
    })

    it('dirty 先促存成功再渲染（渲的即所见），saved 标记随行', async () => {
        const order: string[] = []
        const outcome = await runRenderFlow({
            dirty: true,
            save: async () => {
                order.push('save')
                return true
            },
            render: async () => {
                order.push('render')
                return record
            },
        })
        expect(order).toEqual(['save', 'render'])
        expect(outcome).toEqual({ ok: true, record, saved: true })
    })

    it('dirty 且保存失败：中止渲染不覆盖保存步错误面（note null），渲染零调用', async () => {
        let rendered = 0
        const outcome = await runRenderFlow({
            dirty: true,
            save: async () => false,
            render: async () => {
                rendered += 1
                return record
            },
        })
        expect(rendered).toBe(0)
        expect(outcome).toEqual({ ok: false, note: null })
    })

    it('渲染打回（如 invalid_graph）归稳定码在前文案，不向上抛', async () => {
        const outcome = await runRenderFlow({
            dirty: false,
            save: async () => true,
            render: async () => {
                throw new ApiError(422, 'invalid_graph', '图病态')
            },
        })
        expect(outcome).toEqual({ ok: false, note: '渲染失败：invalid_graph：图病态' })
    })
})

// --- PDF 产物轨纯逻辑（PDF 集成共识）：文档下载文件名 + 条目扩展名派生 ---

describe('PDF 产物轨纯逻辑（文档下载文件名 + 扩展名派生呈现）', () => {
    it('renderPdfFileName：模板名.pdf 单文档语义（无页序/帧名维度），非法字符折叠同口径', () => {
        expect(renderPdfFileName('结业证书')).toBe('结业证书.pdf')
        expect(renderPdfFileName('a/b:c')).toBe('a_b_c.pdf')
        expect(renderPdfFileName('   ')).toBe('template.pdf')
    })

    it('isPdfRenderImage：按服务端定名扩展名派生（渲染记录不落 format 字段）', () => {
        expect(isPdfRenderImage({ frame: 0, name: 'document.pdf', url: '/renders/1/document.pdf' })).toBe(true)
        expect(isPdfRenderImage({ frame: 0, name: '主页', url: '/renders/1/1.png' })).toBe(false)
    })
})
