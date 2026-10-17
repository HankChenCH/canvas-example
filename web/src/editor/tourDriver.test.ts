// 功能导游 driver 接线测试（feature-tour 票 02 TDD 缝）：TourStep → driver.js
// DriveStep 的映射（selector 裸属性名包 []、无锚步不带 element 居中）与销毁原因
// → 回调路由的分类。全部 Node 无 DOM 环境——driver.js 只在 startFeatureTour 里
// 实例化，本文件只测两个纯函数。
import { describe, expect, it } from 'vitest'

import { TOUR_STEPS } from './tour'
import { classifyDestroy, toDriverSteps, type TourDriverStep } from './tourDriver'

describe('toDriverSteps（TourStep → DriveStep 映射）', () => {
    it('有锚步：裸 data-* 属性名包 [] 成属性选择器，文案进 popover', () => {
        const mapped = toDriverSteps([
            { id: 'insert', title: '插入内容', text: '从这里插入文本。', selector: 'data-insert-menu' },
        ])
        expect(mapped).toHaveLength(1)
        expect(mapped[0]!.element).toBe('[data-insert-menu]')
        expect(mapped[0]!.popover?.title).toBe('插入内容')
        expect(mapped[0]!.popover?.description).toBe('从这里插入文本。')
    })

    it('无锚步：不带 element 键（driver 走居中弹层），文案照常', () => {
        const mapped = toDriverSteps([{ id: 'welcome', title: '欢迎', text: '这里是编辑器。' }])
        expect(mapped).toHaveLength(1)
        expect('element' in mapped[0]!).toBe(false)
        expect(mapped[0]!.popover?.title).toBe('欢迎')
        expect(mapped[0]!.popover?.description).toBe('这里是编辑器。')
    })

    it('TOUR_STEPS 全量映射：保序等长，仅首尾两步无锚（spec §2.2 七步形态）', () => {
        const mapped = toDriverSteps(TOUR_STEPS)
        expect(mapped).toHaveLength(TOUR_STEPS.length)
        expect('element' in (mapped[0] as TourDriverStep)).toBe(false)
        expect('element' in (mapped[mapped.length - 1] as TourDriverStep)).toBe(false)
        for (let i = 1; i < mapped.length - 1; i++) {
            expect(typeof (mapped[i] as TourDriverStep).element).toBe('string')
        }
        // 每步 popover 文案与源步骤一一对应（title 原样透传）
        mapped.forEach((step, i) => {
            expect(step.popover?.title).toBe(TOUR_STEPS[i]?.title)
        })
    })
})

describe('classifyDestroy（销毁原因 → 回调路由）', () => {
    it('done（走完最后一步）→ finish', () => {
        expect(classifyDestroy('done')).toBe('finish')
    })

    it('close / escape / overlay（用户提前退出）→ skip', () => {
        expect(classifyDestroy('close')).toBe('skip')
        expect(classifyDestroy('escape')).toBe('skip')
        expect(classifyDestroy('overlay')).toBe('skip')
    })

    it('api / replaced（宿主主动销毁 / 新实例顶替）→ discard，不回调宿主', () => {
        expect(classifyDestroy('api')).toBe('discard')
        expect(classifyDestroy('replaced')).toBe('discard')
    })
})
