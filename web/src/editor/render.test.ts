// 渲染终图纯逻辑测试（19 票 TDD 缝，spec §4.6）：单张终图下载文件名（结果抽屉
// 缩略图的 <a download> 建议名）。Node 无 DOM 环境。
import { describe, expect, it } from 'vitest'

import type { RenderImage } from '../api'
import { renderImageFileName } from './render'

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
