// 功能导游 driver.js 接线件（feature-tour 票 02，spec §2.3）：对 tour.ts 数据负责，
// 把 TOUR_STEPS 接上 driver.js 的遮罩高亮引导——配置、销毁语义与回调路由单点在此，
// 不 import 任何 Vue 组件/宿主页（宿主接线在 03 票 EditorPage）。
//
// 缺锚跳步（票 02 调研项，已核实 driver.js 1.10.1 能力面）：官方覆盖——
//   - skipMissingElement（全局 Config + 每步覆盖）：锚点查不到的步自动跳过，方向
//     朝行进侧找下一个有效步；若前方再无有效步以 done 收尾（同走完）；
//   - waitForElement（同两级的 ms 数）：缺锚等锚出现（MutationObserver），超时
//     照常走该步。轻导锚的全是顶栏/工具栏静态元素，就绪门在宿主（03 票判定首尾
//     锚点在 DOM 才起步），故只开 skipMissingElement、不设 waitForElement；
//   - tour.ts 的 filterExistingSteps 不接进 driver——它留给宿主就绪判定复用
//     （tour.ts 单测覆盖），运行期跳步全归库内官方能力。
//
// 层级表（票 02「实施时以实际层级表为准」，核实于 2026-10-17）：cn-* 浮层最高层 =
// RenderResultDrawer 放大覆盖层 z-index 50，其下弹窗/下拉 40/41、抽屉 30、HelpDialog
// 50；tour 的遮罩 svg 内联 z-index 10000、popover driver.css 基样式 z-index 1e9——
// 两级都天然压过全部 cn-* 浮层，本接线不另设 z-index。
//
// 销毁语义（弹过即看过，spec §2.3）：driver 的 onDestroyed 是全退出路径单点——
//   done（走完）→ onFinish；close/escape/overlay（✕ 钮 / Esc / 点遮罩，allowClose
//   放行）→ onSkip；api/replaced（宿主调 handle.destroy() / 新实例顶替）→ 不回调
//   宿主（销毁是宿主自己发起或实例已让位）。无论哪种路径都先 writeSeenVersion 落
//   seen——完成与跳过等价，重看走顶栏常驻入口（03 票）。
import { driver } from 'driver.js'
import type { Config, DestroyReason, DriveStep } from 'driver.js'

import { TOUR_STEPS, tourAnchorSelector, writeSeenVersion, type TourStep } from './tour'

// driver.js 结构基样式（遮罩/弹层骨架/定位）。先于本目录 tourDriver.css 载入，
// 令牌覆盖样式以双类名选择器压过其单类基样式（panel-theme.css 同款手法）。
import 'driver.js/dist/driver.css'
import './tourDriver.css'

/** popover 挂的令牌类名：样式在 tourDriver.css（cn-* 令牌暗色主题） */
export const TOUR_POPOVER_CLASS = 'cn-tour-popover'

/** driver.js popover 内容根节点的选择器：库创建 popover 时写死 id =
 *  driver-popover-content（1.10.1 源码 S() 内 m.id 赋值）。宿主快捷键门（03 票）
 *  以此识别「导游自身 UI 内的按键」放行焦点按钮激活；值锁库内契约，升级 driver.js
 *  时按 spec §4 走查清单一并核对。 */
export const TOUR_POPOVER_CONTENT_SELECTOR = '#driver-popover-content'

/** driver.js 单步形态（TourStep 映射目标）：element 可省 = 居中弹层 */
export type TourDriverStep = DriveStep

/** 销毁原因 → 回调路由的三分类：finish 走完 / skip 提前退出 / discard 宿主侧销毁 */
export type TourDestroyOutcome = 'finish' | 'skip' | 'discard'

/** TourStep → driver.js DriveStep：selector 存裸 data-* 属性名（tour.ts 锚点纪律），
 *  经 tourAnchorSelector 单点包 [] 成属性选择器（与 filterExistingSteps 同形态）；
 *  无 selector 的步（欢迎/收尾）不带 element 键，driver 弹居中层。文案原样透传，
 *  锚位取库默认（bottom/start，放不下自动翻侧）。 */
export function toDriverSteps(steps: readonly TourStep[]): TourDriverStep[] {
    return steps.map((step) => ({
        ...(step.selector === undefined ? {} : { element: tourAnchorSelector(step.selector) }),
        popover: { title: step.title, description: step.text },
    }))
}

/** 销毁原因分类（纯函数）：done → finish；close/escape/overlay → skip；
 *  api/replaced → discard（宿主主动 destroy 或被新实例顶替，不回调宿主）。
 *  所有原因在 startFeatureTour 里统一先落 seen，与本分类无关。 */
export function classifyDestroy(reason: DestroyReason): TourDestroyOutcome {
    if (reason === 'done') return 'finish'
    if (reason === 'close' || reason === 'escape' || reason === 'overlay') return 'skip'
    return 'discard'
}

/** 启动回调面：finish = 走完收尾步；skip = ✕/Esc/点遮罩提前退出。两路 seen 落库
 *  在接线内自理，宿主回调只管自己的 UI 态（如反馈文案）。 */
export interface StartFeatureTourOptions {
    onFinish?: () => void
    onSkip?: () => void
}

/** 导游句柄：destroy 供宿主卸载/路由离开守卫收尾（收尾本身也落 seen——弹过即看过）；
 *  isActive 供宿主挂导游态门（03 票消费：导游中宿主快捷键 early-return，Esc 归导游）。
 *  内核 useShortcuts 零改动（票 02 约束）。 */
export interface TourDriverHandle {
    destroy(): void
    isActive(): boolean
}

/** 启动功能导游（命令式，宿主在触发门通过后调用；重播入口同口无视 seen）：
 *  轻导形态 = allowClose（✕/Esc/点遮罩随时退）+ 上一步/下一步/完成 + 进度读数；
 *  disableActiveInteraction 高亮元素导游中不可点（模态走查，Esc 留给导游）；
 *  缺锚跳步走官方 skipMissingElement（见文件头调研结论）。 */
export function startFeatureTour(options: StartFeatureTourOptions = {}): TourDriverHandle {
    const config: Config = {
        steps: toDriverSteps(TOUR_STEPS),
        popoverClass: TOUR_POPOVER_CLASS,
        // 轻导形态：可随时退出 + 中文按钮文案 + 「当前 / 总数」进度读数
        allowClose: true,
        showProgress: true,
        progressText: '{{current}} / {{total}}',
        nextBtnText: '下一步',
        prevBtnText: '上一步',
        doneBtnText: '完成',
        closeBtnLabel: '关闭功能导游',
        // 模态走查：高亮元素导游中不可交互，宿主快捷键由宿主态门拦（03 票）
        disableActiveInteraction: true,
        // 缺锚跳步：库内官方能力（票 02 调研核实），不接 filterExistingSteps 预过滤
        skipMissingElement: true,
        // 全退出路径单点：先落 seen（弹过即看过），再按原因路由宿主回调
        onDestroyed: (_element, _step, opts) => {
            writeSeenVersion()
            const outcome = classifyDestroy(opts.reason ?? 'api')
            if (outcome === 'finish') options.onFinish?.()
            if (outcome === 'skip') options.onSkip?.()
        },
    }
    const instance = driver(config)
    instance.drive()
    return {
        destroy: () => instance.destroy(),
        isActive: () => instance.isActive(),
    }
}
