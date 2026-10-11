// 工程数据页签关闭通道传输契约测试（工单 05，project-data spec §6 写入时点三）：
// beforeunload 期经 fetch keepalive 发出 PUT——sendBeacon 只能 POST 保不住写分道
// 语义，keepalive 是 unload 期携带 PUT 的唯一原语。断言钉住三件载荷事实：方法
// PUT（写分道）、keepalive:true（unload 存活）、body = 快照 JSON 自足串（同步组装
// 同步发出，不依赖页面存活）；失败路径静默 console.warn（spec §6 失败面）。
import { afterEach, describe, expect, it, vi } from 'vitest'

import { EDITOR_STATE_SCHEMA_VERSION } from '@hankchen/canvas-editor'
import type { LayerPath } from '@hankchen/canvas-editor'

import { api, type TemplateEditorStateSnapshot } from './api'

const lockedPath: LayerPath = [2]
const snapshot: TemplateEditorStateSnapshot = {
    schemaVersion: EDITOR_STATE_SCHEMA_VERSION,
    frames: { 主页: { guides: [{ id: 1, orientation: 'vertical', position: 100 }], lockedPaths: [lockedPath] } },
    prefs: { rulersVisible: true, anchorExpanded: false },
}

describe('api.updateTemplateEditorStateKeepalive（页签关闭通道）', () => {
    afterEach(() => {
        vi.unstubAllGlobals()
        vi.restoreAllMocks()
    })

    it('PUT + keepalive:true，载荷 = 快照原样 JSON（自足）', () => {
        const fetchMock = vi.fn(() => Promise.resolve(new Response(null, { status: 200 })))
        vi.stubGlobal('fetch', fetchMock)
        api.updateTemplateEditorStateKeepalive(14, snapshot)
        expect(fetchMock).toHaveBeenCalledTimes(1)
        const [url, init] = fetchMock.mock.calls[0] as unknown as [string, RequestInit]
        expect(url).toBe('/api/templates/14/editor-state')
        expect(init.method).toBe('PUT')
        expect(init.keepalive).toBe(true)
        expect(JSON.parse(init.body as string)).toEqual(snapshot)
    })

    it('失败静默：fetch reject 不外抛、console.warn 记因（无 toast 无状态栏面）', async () => {
        const fetchMock = vi.fn(() => Promise.reject(new Error('network dead')))
        vi.stubGlobal('fetch', fetchMock)
        const warn = vi.spyOn(console, 'warn').mockImplementation(() => {})
        api.updateTemplateEditorStateKeepalive(14, snapshot)
        await Promise.resolve()
        expect(warn).toHaveBeenCalledTimes(1)
        expect(warn.mock.calls[0]?.[0]).toContain('[editor-state]')
    })
})
