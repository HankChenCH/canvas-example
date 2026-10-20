// 工程数据快照宿主聚合纯逻辑测试（工单 04 + 模板页序列工票 03，spec §6 TDD 缝）：
// 快照组装（flush 侧）、快照宽容解析（载入侧）、悬空页键剔除。全部 Node 无 DOM 环境，
// 切片造数对齐内核 CanvasEditorState 形状（guides/lockedPaths 两数组键齐 = 形状合法，
// 宽容读权威在内核）。
import { describe, expect, it } from 'vitest'

import { defaultNextTextStyle } from '@hankchen/canvas-editor'
import type { CanvasEditorState, EditorPrefsState, LayerPath } from '@hankchen/canvas-editor'

import {
    EDITOR_STATE_SNAPSHOT_SCHEMA_VERSION,
    buildEditorStateSnapshot,
    parseTemplateEditorStateSnapshot,
    pickPageStates,
} from './editorState'

/** 切片造数器：单垂直参考线 + 单锁路径（内容不解释，仅形状；锁路径 = 根层路径） */
function slice(guidePosition = 100, lockedPath: LayerPath = [2]): CanvasEditorState {
    return {
        guides: [{ id: 1, orientation: 'vertical', position: guidePosition }],
        lockedPaths: [lockedPath],
    }
}

const prefs: EditorPrefsState = {
    rulersVisible: true,
    anchorExpanded: false,
    nextTextStyle: defaultNextTextStyle(),
}

describe('buildEditorStateSnapshot（flush 侧组装）', () => {
    it('schemaVersion 取宿主常量 v2（键面切页 id 升档），frames 按页 id 组装、null 切片不产生键，prefs 原样', () => {
        const snapshot = buildEditorStateSnapshot({
            pageIds: ['pg_a', 'pg_b', 'pg_c'],
            slices: [slice(), null, slice(200, [3])],
            prefs,
        })
        expect(snapshot.schemaVersion).toBe(EDITOR_STATE_SNAPSHOT_SCHEMA_VERSION)
        expect(snapshot.schemaVersion).toBe(2)
        expect(Object.keys(snapshot.frames)).toEqual(['pg_a', 'pg_c'])
        expect(snapshot.frames['pg_a']).toEqual(slice())
        expect(snapshot.frames['pg_c']).toEqual(slice(200, [3]))
        expect(snapshot.prefs).toEqual(prefs)
    })

    it('快照自持：组装出的 frames/prefs 是拷贝（元素级不别名），事后改输入不串快照', () => {
        const input = slice()
        const snapshot = buildEditorStateSnapshot({ pageIds: ['pg_a'], slices: [input], prefs })
        // Guide 是 readonly 型：以元素身份断言拷贝（引用不同）+ 数组可变面独立
        expect(snapshot.frames['pg_a']!.guides[0]).not.toBe(input.guides[0])
        snapshot.frames['pg_a']!.guides = []
        snapshot.prefs.rulersVisible = false
        expect(input.guides).toHaveLength(1)
        expect(prefs.rulersVisible).toBe(true)
    })
})

describe('parseTemplateEditorStateSnapshot（载入侧宽容解析）', () => {
    it('完整快照：frames/prefs 逐键提取，未知顶层键忽略', () => {
        const raw = {
            schemaVersion: 2,
            frames: { pg_a: slice(), pg_b: slice(300, [9]) },
            prefs,
            futureKey: { anything: true },
        }
        const parsed = parseTemplateEditorStateSnapshot(raw)
        expect(parsed.frames['pg_a']).toEqual(slice())
        expect(parsed.frames['pg_b']).toEqual(slice(300, [9]))
        expect(parsed.prefs).toEqual(prefs)
    })

    it('null / undefined / 标量 / 数组 → 全默认态（frames 空 + prefs null），不抛错', () => {
        for (const raw of [null, undefined, 42, 'snapshot', [], true]) {
            const parsed = parseTemplateEditorStateSnapshot(raw)
            expect(parsed.frames).toEqual({})
            expect(parsed.prefs).toBeNull()
        }
    })

    it('宽容读不分支 schemaVersion：缺键/未知版本照常提取（迁移器不存在的既定立场）', () => {
        for (const schemaVersion of [undefined, 0, 1, 99, 'two']) {
            const parsed = parseTemplateEditorStateSnapshot({ schemaVersion, frames: { pg_a: slice() }, prefs })
            expect(parsed.frames['pg_a']).toEqual(slice())
            expect(parsed.prefs).toEqual(prefs)
        }
    })

    it('frames 非对象（标量/数组）→ frames 空而非整体拒', () => {
        for (const frames of ['x', 42, [slice()], null]) {
            const parsed = parseTemplateEditorStateSnapshot({ schemaVersion: 2, frames, prefs })
            expect(parsed.frames).toEqual({})
            expect(parsed.prefs).toEqual(prefs)
        }
    })

    it('单帧形状不合法只丢该键（缺 lockedPaths / guides 非数组 / 标量），合法兄弟键保留', () => {
        const parsed = parseTemplateEditorStateSnapshot({
            schemaVersion: 2,
            frames: {
                坏一: { guides: [] },
                坏二: { guides: 'x', lockedPaths: [] },
                坏三: 42,
                pg_a: slice(),
            },
            prefs,
        })
        expect(Object.keys(parsed.frames)).toEqual(['pg_a'])
        expect(parsed.frames['pg_a']).toEqual(slice())
    })

    it('prefs 双布尔键齐才收：缺键/半键/非对象 → null（不应用，内核自持默认）', () => {
        for (const bad of [undefined, null, {}, { rulersVisible: true }, { anchorExpanded: false }, 'yes', 1]) {
            const parsed = parseTemplateEditorStateSnapshot({ schemaVersion: 2, frames: {}, prefs: bad })
            expect(parsed.prefs).toBeNull()
        }
    })

    it('prefs 带未知附加键：双布尔齐仍收，附加键剔除（载荷只含已知两键）', () => {
        const parsed = parseTemplateEditorStateSnapshot({
            schemaVersion: 2,
            frames: {},
            prefs: { rulersVisible: false, anchorExpanded: true, futurePref: 'x' },
        })
        expect(parsed.prefs).toEqual({ rulersVisible: false, anchorExpanded: true })
    })

    it('build → parse 往返恒等：组装出的快照解析回同构 frames/prefs', () => {
        const snapshot = buildEditorStateSnapshot({
            pageIds: ['pg_a', 'pg_b'],
            slices: [slice(), null],
            prefs,
        })
        const parsed = parseTemplateEditorStateSnapshot(JSON.parse(JSON.stringify(snapshot)))
        expect(parsed.frames).toEqual(snapshot.frames)
        expect(parsed.prefs).toEqual(prefs)
    })
})

describe('pickPageStates（载入侧悬空页键剔除）', () => {
    it('只取现役页 id 对应的切片：删页/回滚遗留的悬空键宽容跳过', () => {
        const picked = pickPageStates({ pg_a: slice(), pg_gone: slice(5) }, ['pg_a', 'pg_new'])
        expect(Object.keys(picked)).toEqual(['pg_a'])
        expect(picked['pg_a']).toEqual(slice())
    })

    it('现役页 id 无切片不产生键（该页全默认态）；空映射/空名单 → 空', () => {
        expect(pickPageStates({}, ['pg_a'])).toEqual({})
        expect(pickPageStates({ pg_a: slice() }, [])).toEqual({})
    })
})
