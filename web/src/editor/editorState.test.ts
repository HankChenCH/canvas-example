// 工程数据快照宿主聚合纯逻辑测试（工单 04，spec §6 TDD 缝）：快照组装（flush 侧）、
// 快照宽容解析（载入侧）、悬空帧键剔除。全部 Node 无 DOM 环境，切片造数对齐内核
// CanvasEditorState 形状（guides/lockedPaths 两数组键齐 = 形状合法，宽容读权威在内核）。
import { describe, expect, it } from 'vitest'

import { EDITOR_STATE_SCHEMA_VERSION } from '@hankchen/canvas-editor'
import type { CanvasEditorState, EditorPrefsState, LayerPath } from '@hankchen/canvas-editor'

import {
    buildEditorStateSnapshot,
    dropFrameStateKey,
    migrateFrameStateKey,
    parseTemplateEditorStateSnapshot,
    pickFrameStates,
} from './editorState'

/** 切片造数器：单垂直参考线 + 单锁路径（内容不解释，仅形状；锁路径 = 根层路径） */
function slice(guidePosition = 100, lockedPath: LayerPath = [2]): CanvasEditorState {
    return {
        guides: [{ id: 1, orientation: 'vertical', position: guidePosition }],
        lockedPaths: [lockedPath],
    }
}

const prefs: EditorPrefsState = { rulersVisible: true, anchorExpanded: false }

describe('buildEditorStateSnapshot（flush 侧组装）', () => {
    it('schemaVersion 取内核常量，frames 按帧名组装、null 切片不产生键，prefs 原样', () => {
        const snapshot = buildEditorStateSnapshot({
            frameNames: ['主页', '续页', '第三帧'],
            slices: [slice(), null, slice(200, [3])],
            prefs,
        })
        expect(snapshot.schemaVersion).toBe(EDITOR_STATE_SCHEMA_VERSION)
        expect(snapshot.schemaVersion).toBe(1)
        expect(Object.keys(snapshot.frames)).toEqual(['主页', '第三帧'])
        expect(snapshot.frames['主页']).toEqual(slice())
        expect(snapshot.frames['第三帧']).toEqual(slice(200, [3]))
        expect(snapshot.prefs).toEqual(prefs)
    })

    it('快照自持：组装出的 frames/prefs 是拷贝（元素级不别名），事后改输入不串快照', () => {
        const input = slice()
        const snapshot = buildEditorStateSnapshot({ frameNames: ['主页'], slices: [input], prefs })
        // Guide 是 readonly 型：以元素身份断言拷贝（引用不同）+ 数组可变面独立
        expect(snapshot.frames['主页']!.guides[0]).not.toBe(input.guides[0])
        snapshot.frames['主页']!.guides = []
        snapshot.prefs.rulersVisible = false
        expect(input.guides).toHaveLength(1)
        expect(prefs.rulersVisible).toBe(true)
    })

    it('重名帧后者覆盖前者（帧名非唯一键，spec §3 形状的既定代价，记录不报错）', () => {
        const snapshot = buildEditorStateSnapshot({
            frameNames: ['同名', '同名'],
            slices: [slice(1), slice(2)],
            prefs,
        })
        expect(snapshot.frames['同名']).toEqual(slice(2))
    })
})

describe('parseTemplateEditorStateSnapshot（载入侧宽容解析）', () => {
    it('完整快照：frames/prefs 逐键提取，未知顶层键忽略', () => {
        const raw = {
            schemaVersion: 1,
            frames: { 主页: slice(), 续页: slice(300, [9]) },
            prefs,
            futureKey: { anything: true },
        }
        const parsed = parseTemplateEditorStateSnapshot(raw)
        expect(parsed.frames['主页']).toEqual(slice())
        expect(parsed.frames['续页']).toEqual(slice(300, [9]))
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
        for (const schemaVersion of [undefined, 0, 99, 'one']) {
            const parsed = parseTemplateEditorStateSnapshot({ schemaVersion, frames: { 主页: slice() }, prefs })
            expect(parsed.frames['主页']).toEqual(slice())
            expect(parsed.prefs).toEqual(prefs)
        }
    })

    it('frames 非对象（标量/数组）→ frames 空而非整体拒', () => {
        for (const frames of ['x', 42, [slice()], null]) {
            const parsed = parseTemplateEditorStateSnapshot({ schemaVersion: 1, frames, prefs })
            expect(parsed.frames).toEqual({})
            expect(parsed.prefs).toEqual(prefs)
        }
    })

    it('单帧形状不合法只丢该键（缺 lockedPaths / guides 非数组 / 标量），合法兄弟帧保留', () => {
        const parsed = parseTemplateEditorStateSnapshot({
            schemaVersion: 1,
            frames: {
                坏一: { guides: [] },
                坏二: { guides: 'x', lockedPaths: [] },
                坏三: 42,
                主页: slice(),
            },
            prefs,
        })
        expect(Object.keys(parsed.frames)).toEqual(['主页'])
        expect(parsed.frames['主页']).toEqual(slice())
    })

    it('prefs 双布尔键齐才收：缺键/半键/非对象 → null（不应用，内核自持默认）', () => {
        for (const bad of [undefined, null, {}, { rulersVisible: true }, { anchorExpanded: false }, 'yes', 1]) {
            const parsed = parseTemplateEditorStateSnapshot({ schemaVersion: 1, frames: {}, prefs: bad })
            expect(parsed.prefs).toBeNull()
        }
    })

    it('prefs 带未知附加键：双布尔齐仍收，附加键剔除（载荷只含已知两键）', () => {
        const parsed = parseTemplateEditorStateSnapshot({
            schemaVersion: 1,
            frames: {},
            prefs: { rulersVisible: false, anchorExpanded: true, futurePref: 'x' },
        })
        expect(parsed.prefs).toEqual({ rulersVisible: false, anchorExpanded: true })
    })

    it('build → parse 往返恒等：组装出的快照解析回同构 frames/prefs', () => {
        const snapshot = buildEditorStateSnapshot({
            frameNames: ['主页', '续页'],
            slices: [slice(), null],
            prefs,
        })
        const parsed = parseTemplateEditorStateSnapshot(JSON.parse(JSON.stringify(snapshot)))
        expect(parsed.frames).toEqual(snapshot.frames)
        expect(parsed.prefs).toEqual(prefs)
    })
})

describe('pickFrameStates（载入侧悬空帧键剔除）', () => {
    it('只取现役帧名对应的切片：改名遗留的悬空键宽容跳过', () => {
        const picked = pickFrameStates({ 主页: slice(), 改名前: slice(5) }, ['主页', '续页'])
        expect(Object.keys(picked)).toEqual(['主页'])
        expect(picked['主页']).toEqual(slice())
    })

    it('现帧名无切片不产生键（该帧全默认态）；空映射/空名单 → 空', () => {
        expect(pickFrameStates({}, ['主页'])).toEqual({})
        expect(pickFrameStates({ 主页: slice() }, [])).toEqual({})
    })
})

describe('migrateFrameStateKey / dropFrameStateKey（会话内映射记账）', () => {
    it('重命名迁移：切片随新键保留、旧键消失；旧键无切片 = 恒等映射', () => {
        const frames = { 主页: slice(), 续页: slice(9) }
        const migrated = migrateFrameStateKey(frames, '主页', '封面')
        expect(migrated['封面']).toEqual(slice())
        expect('主页' in migrated).toBe(false)
        expect(migrated['续页']).toEqual(slice(9))
        // 未访问帧（旧键无切片）：映射原样
        expect(migrateFrameStateKey(frames, '不存在', '新名')).toEqual(frames)
    })

    it('重命名迁移不改输入（映射不可变更新）', () => {
        const frames = { 主页: slice() }
        migrateFrameStateKey(frames, '主页', '封面')
        expect('主页' in frames).toBe(true)
    })

    it('删帧出映射：键消失；有同名幸存帧则保守保留（帧名非唯一键代价）', () => {
        const frames = { 主页: slice(), 第3帧: slice(7) }
        const dropped = dropFrameStateKey(frames, '第3帧', ['主页'])
        expect('第3帧' in dropped).toBe(false)
        expect(dropped['主页']).toEqual(slice())
        // 同名幸存：不删（别名共享键）
        expect(dropFrameStateKey(frames, '第3帧', ['主页', '第3帧'])).toEqual(frames)
    })
})
