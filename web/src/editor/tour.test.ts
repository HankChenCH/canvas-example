// 功能导游纯逻辑测试（feature-tour 票 01 TDD 缝，spec §2.1/§2.2）：版本资格判定
// 边界、已看过读写的 storage 注入与降级（throw 的 storage / null）、缺锚过滤、
// 步骤清单数据完整性。全部 Node 无 DOM 环境——storage/document 以注入桩替代。
import { describe, expect, it } from 'vitest'

import {
    filterExistingSteps,
    readSeenVersion,
    shouldShowTour,
    TOUR_STORAGE_KEY,
    TOUR_STEPS,
    TOUR_VERSION,
    tourAnchorSelector,
    tourEndpointsReady,
    writeSeenVersion,
    type TourDoc,
    type TourStep,
    type TourStorage,
} from './tour'

/** 内存 storage 造数器：localStorage 注入缝的测试替身 */
function memoryStorage(initial: Record<string, string> = {}): TourStorage & { data: Map<string, string> } {
    const data = new Map(Object.entries(initial))
    return {
        data,
        getItem: (key) => data.get(key) ?? null,
        setItem: (key, value) => { data.set(key, value) },
    }
}

/** 必抛 storage：读写一律抛（隐私模式 / 安全策略禁用形态），接口契约是不上抛 */
const throwingStorage: TourStorage = {
    getItem: () => { throw new Error('SecurityError: denied') },
    setItem: () => { throw new Error('QuotaExceededError: denied') },
}

/** document 注入桩：present 列出「存在」的完整选择器（[data-*] 形态） */
function fakeDoc(present: string[]): TourDoc {
    const set = new Set(present)
    return { querySelector: (selector) => (set.has(selector) ? {} : null) }
}

describe('常量（spec §2.1）', () => {
    it('TOUR_VERSION = 1、TOUR_STORAGE_KEY = feature-tour:seen', () => {
        expect(TOUR_VERSION).toBe(1)
        expect(TOUR_STORAGE_KEY).toBe('feature-tour:seen')
    })
})

describe('shouldShowTour（资格判定 = seen < current）', () => {
    it('seen=0（首访 / 缺 key / 解析失败归零）→ 弹', () => {
        expect(shouldShowTour(0)).toBe(true)
    })

    it('seen=TOUR_VERSION（已看过当前版）→ 不弹', () => {
        expect(shouldShowTour(TOUR_VERSION)).toBe(false)
    })

    it('seen 更大（新版本写入后内容回滚）→ 不弹', () => {
        expect(shouldShowTour(TOUR_VERSION + 1)).toBe(false)
    })

    it('current 显式传入：内容改版 bump 后老 seen 重弹一次', () => {
        expect(shouldShowTour(1, 2)).toBe(true)
        expect(shouldShowTour(2, 2)).toBe(false)
    })
})

describe('readSeenVersion / writeSeenVersion（storage 注入缝与会话降级）', () => {
    it('工作 storage：写入 String 形态、可读回；换一个同内容实例也读得到', () => {
        const storage = memoryStorage()
        writeSeenVersion(storage, 2)
        expect(storage.data.get(TOUR_STORAGE_KEY)).toBe('2')
        expect(readSeenVersion(memoryStorage({ [TOUR_STORAGE_KEY]: '2' }))).toBe(2)
    })

    it('writeSeenVersion 缺省 v = TOUR_VERSION（onFinish/onSkip 都写当前版）', () => {
        const storage = memoryStorage()
        writeSeenVersion(storage)
        expect(readSeenVersion(storage)).toBe(TOUR_VERSION)
    })

    it('缺 key 读数按 0（弹）', () => {
        expect(readSeenVersion(memoryStorage())).toBe(0)
    })

    it('解析失败读数按 0（脏数据不炸、按首访处理）', () => {
        expect(readSeenVersion(memoryStorage({ [TOUR_STORAGE_KEY]: 'not-a-number' }))).toBe(0)
    })

    it('throw 的 storage：读写都不抛，降级模块级会话内存（本会话不重复弹）', () => {
        expect(() => writeSeenVersion(throwingStorage, 3)).not.toThrow()
        expect(readSeenVersion(throwingStorage)).toBe(3)
    })

    it('无 storage（显式 null / Node 无 localStorage）：同走会话内存', () => {
        writeSeenVersion(null, 5)
        expect(readSeenVersion(null)).toBe(5)
    })

    it('降级写入后：可用 storage 有值以 storage 为准；storage 缺 key 回落会话值（禁写可读形态不重弹）', () => {
        writeSeenVersion(throwingStorage, 9)
        expect(readSeenVersion(memoryStorage({ [TOUR_STORAGE_KEY]: '4' }))).toBe(4)
        expect(readSeenVersion(memoryStorage())).toBe(9)
    })
})

describe('filterExistingSteps（缺锚跳步纯函数）', () => {
    const steps: TourStep[] = [
        { id: 'welcome', title: '欢迎', text: '无锚居中步' },
        { id: 'here', title: 'B', text: '…', selector: 'data-here' },
        { id: 'gone', title: 'C', text: '…', selector: 'data-gone' },
        { id: 'also-here', title: 'D', text: '…', selector: 'data-also-here' },
    ]

    it('部分缺失：缺锚跳步不报错、保序、无锚步保留', () => {
        expect(filterExistingSteps(steps, fakeDoc(['[data-here]', '[data-also-here]']))).toEqual([
            steps[0], steps[1], steps[3],
        ])
    })

    it('查询形态 = [selector] 属性选择器（selector 字段存裸 data-* 名）', () => {
        const seen: string[] = []
        filterExistingSteps([steps[1]!], { querySelector: (selector) => { seen.push(selector); return null } })
        expect(seen).toEqual(['[data-here]'])
    })

    it('带锚步全缺 → 空数组（无锚步仍保留）', () => {
        expect(filterExistingSteps(steps.slice(1), fakeDoc([]))).toEqual([])
        expect(filterExistingSteps([], fakeDoc([]))).toEqual([])
        expect(filterExistingSteps([steps[0]!], fakeDoc([]))).toEqual([steps[0]])
    })

    it('querySelector 抛错按缺锚跳步，不报错', () => {
        expect(filterExistingSteps(steps, { querySelector: () => { throw new Error('boom') } })).toEqual([steps[0]])
    })
})

describe('tourEndpointsReady（触发门锚点半边「轻导首尾锚点均在 DOM」，03 票/spec §2.4）', () => {
    const steps: TourStep[] = [
        { id: 'welcome', title: '欢迎', text: '无锚居中步' },
        { id: 'first', title: 'F', text: '…', selector: 'data-first' },
        { id: 'mid', title: 'M', text: '…', selector: 'data-mid' },
        { id: 'last', title: 'L', text: '…', selector: 'data-last' },
        { id: 'finish', title: '收尾', text: '无锚居中步' },
    ]

    it('首尾锚点均在 → true（中间锚缺席不挡起步——运行期跳步归 driver）', () => {
        expect(tourEndpointsReady(steps, fakeDoc(['[data-first]', '[data-last]']))).toBe(true)
        expect(tourEndpointsReady(steps, fakeDoc(['[data-first]', '[data-mid]', '[data-last]']))).toBe(true)
    })

    it('首锚或尾锚缺席 → false（台面未挂/载入慢 = 触发门静默不弹）', () => {
        expect(tourEndpointsReady(steps, fakeDoc(['[data-last]']))).toBe(false)
        expect(tourEndpointsReady(steps, fakeDoc(['[data-first]']))).toBe(false)
        expect(tourEndpointsReady(steps, fakeDoc([]))).toBe(false)
    })

    it('判定对象是带锚步的首尾，不是清单首尾步（无锚居中步不参与判定）', () => {
        // 尾步无锚：按「清单尾步在 DOM」解释会恒真（无锚步查询恒保留）；按带锚步
        // 解释，唯一带锚步（兼任首尾带锚步）缺席即 false
        const tailUnanchored: TourStep[] = [
            { id: 'welcome', title: '欢迎', text: '无锚居中步' },
            { id: 'only', title: 'F', text: '…', selector: 'data-only' },
            { id: 'finish', title: '收尾', text: '无锚居中步' },
        ]
        expect(tourEndpointsReady(tailUnanchored, fakeDoc(['[data-only]']))).toBe(true)
        expect(tourEndpointsReady(tailUnanchored, fakeDoc([]))).toBe(false)
    })

    it('全为无锚步（异常配置）恒 true；空清单恒 true（锚点判定不拦无锚导游）', () => {
        expect(tourEndpointsReady([steps[0]!, steps[4]!], fakeDoc([]))).toBe(true)
        expect(tourEndpointsReady([], fakeDoc([]))).toBe(true)
    })

    it('querySelector 抛错按锚缺席处理，不报错', () => {
        expect(tourEndpointsReady(steps, { querySelector: () => { throw new Error('boom') } })).toBe(false)
    })

    it('对真实 TOUR_STEPS：全锚在场 → true；缺任一首尾锚 → false', () => {
        const all = TOUR_STEPS.flatMap((s) => (s.selector === undefined ? [] : [tourAnchorSelector(s.selector)]))
        expect(tourEndpointsReady(TOUR_STEPS, fakeDoc(all))).toBe(true)
        expect(tourEndpointsReady(TOUR_STEPS, fakeDoc(all.slice(1)))).toBe(false)
        expect(tourEndpointsReady(TOUR_STEPS, fakeDoc(all.slice(0, -1)))).toBe(false)
    })
})

describe('TOUR_STEPS（首访轻导七步初稿，spec §2.2）', () => {
    it('七步：首尾无锚居中，中间五步带锚', () => {
        expect(TOUR_STEPS).toHaveLength(7)
        expect(TOUR_STEPS[0]?.selector).toBeUndefined()
        expect(TOUR_STEPS.at(-1)?.selector).toBeUndefined()
        expect(TOUR_STEPS.slice(1, -1).every((s) => typeof s.selector === 'string' && s.selector.length > 0)).toBe(true)
    })

    it('每步有唯一 id + 非空 title/text', () => {
        const ids = TOUR_STEPS.map((s) => s.id)
        expect(new Set(ids).size).toBe(ids.length)
        for (const step of TOUR_STEPS) {
            expect(step.id.length).toBeGreaterThan(0)
            expect(step.title.length).toBeGreaterThan(0)
            expect(step.text.length).toBeGreaterThan(0)
        }
    })

    it('带锚步 selector 一律 data-* 裸属性名（锚点纪律：selector 只住此文件）', () => {
        for (const step of TOUR_STEPS) {
            if (step.selector !== undefined) expect(step.selector.startsWith('data-')).toBe(true)
        }
    })

    it('五个锚点与既有 DOM data-* 一一对应（spec §2.2 清单顺序）', () => {
        expect(TOUR_STEPS.flatMap((s) => (s.selector === undefined ? [] : [s.selector]))).toEqual([
            'data-insert-menu',
            'data-open-datasource',
            'data-save-state',
            'data-publish-template',
            'data-render-final',
        ])
    })
})
