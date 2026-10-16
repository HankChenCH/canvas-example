// 工程数据页签关闭通道传输契约测试（工单 05，project-data spec §6 写入时点三）：
// beforeunload 期经 fetch keepalive 发出 PUT——sendBeacon 只能 POST 保不住写分道
// 语义，keepalive 是 unload 期携带 PUT 的唯一原语。断言钉住三件载荷事实：方法
// PUT（写分道）、keepalive:true（unload 存活）、body = 快照 JSON 自足串（同步组装
// 同步发出，不依赖页面存活）；失败路径静默 console.warn（spec §6 失败面）。
import { afterEach, describe, expect, it, vi } from 'vitest'

import { EDITOR_STATE_SCHEMA_VERSION, defaultNextTextStyle } from '@hankchen/canvas-editor'
import type { LayerPath } from '@hankchen/canvas-editor'

import { api, ApiError, type TemplateEditorStateSnapshot, type TemplateRecord } from './api'

/** JSON 响应造数器：正常体与错误信封（{error:{code,message}}）两用 */
function jsonResponse(payload: unknown, status = 200): Response {
    return new Response(JSON.stringify(payload), { status, headers: { 'Content-Type': 'application/json' } })
}

const lockedPath: LayerPath = [2]
const snapshot: TemplateEditorStateSnapshot = {
    schemaVersion: EDITOR_STATE_SCHEMA_VERSION,
    frames: { 主页: { guides: [{ id: 1, orientation: 'vertical', position: 100 }], lockedPaths: [lockedPath] } },
    prefs: { rulersVisible: true, anchorExpanded: false, nextTextStyle: defaultNextTextStyle() },
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

// ---- 模板版本化客户端面（template-draft-publish 批 B ①，spec §3/§4）：发布/版本
//      读家族/回滚四方法 + 渲染选版三态。断言钉住 URL/method/body 字节面与错误信封
//      稳定码（按 code 判定语义，spec §2.5 惯例）。 ----

describe('api 版本化四方法（spec §3 发布与版本 API）', () => {
    afterEach(() => {
        vi.unstubAllGlobals()
    })

    it('publishTemplate：POST …/publish 无 body，201/200 响应透传（duplicate 字段随行）', async () => {
        const fetchMock = vi.fn(() =>
            Promise.resolve(jsonResponse({ version: 2, createdAt: '2026-10-11T08:00:00Z', duplicate: true })),
        )
        vi.stubGlobal('fetch', fetchMock)
        const result = await api.publishTemplate(7)
        expect(result).toEqual({ version: 2, createdAt: '2026-10-11T08:00:00Z', duplicate: true })
        const [url, init] = fetchMock.mock.calls[0] as unknown as [string, RequestInit]
        expect(url).toBe('/api/templates/7/publish')
        expect(init.method).toBe('POST')
        expect(init.body).toBeUndefined()
    })

    it('listTemplateVersions：GET …/versions 轻量列表透传（version 降序由服务端保证）', async () => {
        const versions = [
            { version: 3, createdAt: '2026-10-11T08:00:00Z' },
            { version: 1, createdAt: '2026-10-10T08:00:00Z' },
        ]
        const fetchMock = vi.fn(() => Promise.resolve(jsonResponse(versions)))
        vi.stubGlobal('fetch', fetchMock)
        expect(await api.listTemplateVersions(7)).toEqual(versions)
        // request(url) 无 init：fetch 第二参 undefined（GET 缺省语义）
        const [url, init] = fetchMock.mock.calls[0] as unknown as [string, RequestInit | undefined]
        expect(url).toBe('/api/templates/7/versions')
        expect(init).toBeUndefined()
    })

    it('getTemplateVersion：GET …/versions/{n} 单版本全量（canvases + flowChain）', async () => {
        const record = { version: 3, canvases: [{ name: '主页', graph: {} }], flowChain: null, createdAt: '2026-10-11T08:00:00Z' }
        const fetchMock = vi.fn(() => Promise.resolve(jsonResponse(record)))
        vi.stubGlobal('fetch', fetchMock)
        expect(await api.getTemplateVersion(7, 3)).toEqual(record)
        const [url] = fetchMock.mock.calls[0] as unknown as [string, RequestInit]
        expect(url).toBe('/api/templates/7/versions/3')
    })

    it('revertTemplate：POST …/revert body {version}，回全量模板记录（无需二次 GET）', async () => {
        const record = { id: 7, name: '证书', canvases: [], flowChain: null, dataSourceId: null, editorState: null, createdAt: '', updatedAt: '' } satisfies TemplateRecord
        const fetchMock = vi.fn(() => Promise.resolve(jsonResponse(record)))
        vi.stubGlobal('fetch', fetchMock)
        expect(await api.revertTemplate(7, 3)).toEqual(record)
        const [url, init] = fetchMock.mock.calls[0] as unknown as [string, RequestInit]
        expect(url).toBe('/api/templates/7/revert')
        expect(init.method).toBe('POST')
        expect(JSON.parse(init.body as string)).toEqual({ version: 3 })
    })

    it('错误信封稳定码映射：422 invalid_graph（发布守门）与 404 version_not_found', async () => {
        const fetchMock = vi.fn()
        fetchMock.mockReturnValueOnce(Promise.resolve(jsonResponse({ error: { code: 'invalid_graph', message: '图预检失败' } }, 422)))
        fetchMock.mockReturnValueOnce(Promise.resolve(jsonResponse({ error: { code: 'version_not_found', message: 'no such version' } }, 404)))
        vi.stubGlobal('fetch', fetchMock)
        const publishErr = await api.publishTemplate(7).catch((e) => e)
        expect(publishErr).toBeInstanceOf(ApiError)
        expect((publishErr as ApiError).status).toBe(422)
        expect((publishErr as ApiError).code).toBe('invalid_graph')
        const versionErr = await api.getTemplateVersion(7, 99).catch((e) => e)
        expect((versionErr as ApiError).status).toBe(404)
        expect((versionErr as ApiError).code).toBe('version_not_found')
    })
})

describe('api.renderTemplate 选版三态（spec §4，票 03 draft 哨兵契约修订）', () => {
    afterEach(() => {
        vi.unstubAllGlobals()
    })

    it('无 opts 保持无 body POST（列表页批量渲染路径零改）', async () => {
        const fetchMock = vi.fn(() => Promise.resolve(jsonResponse({ id: 1, templateId: 9, version: 2, createdAt: '', images: [] })))
        vi.stubGlobal('fetch', fetchMock)
        await api.renderTemplate(9)
        const [url, init] = fetchMock.mock.calls[0] as unknown as [string, RequestInit]
        expect(url).toBe('/api/templates/9/render')
        expect(init.method).toBe('POST')
        expect(init.body).toBeUndefined()
    })

    it('正整数版本 → JSON body {"version":N}（指定历史版本）', async () => {
        const fetchMock = vi.fn(() => Promise.resolve(jsonResponse({ id: 1, templateId: 9, version: 2, createdAt: '', images: [] })))
        vi.stubGlobal('fetch', fetchMock)
        await api.renderTemplate(9, { version: 2 })
        const [, init] = fetchMock.mock.calls[0] as unknown as [string, RequestInit]
        expect(init.headers).toEqual({ 'Content-Type': 'application/json' })
        expect(JSON.parse(init.body as string)).toEqual({ version: 2 })
    })

    it('"draft" 哨兵 → JSON body {"version":"draft"}（编辑器预览渲草稿列现值）', async () => {
        const fetchMock = vi.fn(() => Promise.resolve(jsonResponse({ id: 1, templateId: 9, version: null, createdAt: '', images: [] })))
        vi.stubGlobal('fetch', fetchMock)
        await api.renderTemplate(9, { version: 'draft' })
        const [, init] = fetchMock.mock.calls[0] as unknown as [string, RequestInit]
        expect(JSON.parse(init.body as string)).toEqual({ version: 'draft' })
    })

    it('RenderRecord.version 透传；未发布缺省渲染 409 template_unpublished 可达', async () => {
        const fetchMock = vi.fn()
        fetchMock.mockReturnValueOnce(Promise.resolve(jsonResponse({ id: 1, templateId: 9, version: null, createdAt: '', images: [] })))
        fetchMock.mockReturnValueOnce(
            Promise.resolve(jsonResponse({ error: { code: 'template_unpublished', message: '尚未发布任何版本' } }, 409)),
        )
        vi.stubGlobal('fetch', fetchMock)
        const draftRecord = await api.renderTemplate(9, { version: 'draft' })
        expect(draftRecord.version).toBeNull()
        const err = await api.renderTemplate(9).catch((e) => e)
        expect((err as ApiError).status).toBe(409)
        expect((err as ApiError).code).toBe('template_unpublished')
    })
})
