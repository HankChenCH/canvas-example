// 功能导游纯逻辑（feature-tour 票 01 TDD 缝，spec §2.1/§2.2）：版本化「已看过」
// 读写 + 资格判定 + 首访轻导步骤清单 + 缺锚过滤。与 Vue/DOM 解耦——storage 与
// document 经参数注入（宿主传 localStorage / document，publish.ts / newtemplate.ts
// 同款注入惯例，测试免 mock 浏览器全局）。
//
// 降级语义（spec §2.1）：读写全部 try/catch，localStorage 不可用（隐私模式/被禁）
// 时降级为模块级会话内存——本会话不重复弹，不抛错不上报。缺 key/解析失败读数按 0
// （按首访处理）。内容改版 bump TOUR_VERSION，老用户 seen < 新版本重弹一次。
//
// 锚点纪律（spec §2.5）：全应用导游 selector 只允许集中此文件；selector 存裸
// data-* 属性名（查询时包 [ ] 成属性选择器）；禁锚 hover-only 元素（如
// data-edit-toolbar）。升级 canvas-editor-vue / 改顶栏工具栏先跑一遍导游走查。

/** 当前导游内容版本：步骤文案/清单改版时 bump，老用户 seen < 新版本重弹一次 */
export const TOUR_VERSION = 1

/** 「已看过导游版本号」的存储 key：单 key 存版本号（localStorage 版本化持久化） */
export const TOUR_STORAGE_KEY = 'feature-tour:seen'

/** storage 注入缝的最小结构面：DOM Storage 天然满足，测试传内存实现 */
export interface TourStorage {
    getItem(key: string): string | null
    setItem(key: string, value: string): void
}

/** 缺锚过滤的 document 注入缝：只要 querySelector（DOM Document 天然满足） */
export interface TourDoc {
    querySelector(selector: string): unknown
}

/** 单个导游步骤：selector 缺省 = 无锚居中步（欢迎/收尾） */
export interface TourStep {
    id: string
    title: string
    text: string
    selector?: string
}

/** localStorage 不可用时的会话级降级存储：仅注入 null/读抛/写抛三态落这里 */
let sessionSeen: number | null = null

/** 取存储通道：未注入（undefined/null）取全局 localStorage——访问本身在禁用态
 *  可抛，Node/测试环境无此全局，两种情况都归 null 走会话降级 */
function resolveStorage(storage?: TourStorage | null): TourStorage | null {
    if (storage !== undefined && storage !== null) return storage
    try {
        return globalThis.localStorage ?? null
    } catch {
        return null
    }
}

/** 读「已看过的导游版本号」：解析失败按 0（spec 文面，脏值优先于会话值）；缺 key
 *  与读抛错回落会话内存值——禁写可读形态（隐私模式 setItem 抛、getItem 返回 null）
 *  下先落过会话值，这里才能接住，保证本会话不重复弹（spec §2.1，优先于「缺 key
 *  按 0」的字面）。 */
export function readSeenVersion(storage?: TourStorage | null): number {
    const store = resolveStorage(storage)
    if (store === null) return sessionSeen ?? 0
    try {
        const raw = store.getItem(TOUR_STORAGE_KEY)
        if (raw === null) return sessionSeen ?? 0
        const seen = Number(raw)
        return Number.isFinite(seen) ? seen : 0
    } catch {
        return sessionSeen ?? 0
    }
}

/** 写「已看过的导游版本号」（弹过即看过，重看走常驻入口）：v 缺省 = 当前版。
 *  持久化失败（隐私模式/被禁）降级会话内存，不抛错不上报。 */
export function writeSeenVersion(storage?: TourStorage | null, v: number = TOUR_VERSION): void {
    const store = resolveStorage(storage)
    if (store === null) {
        sessionSeen = v
        return
    }
    try {
        store.setItem(TOUR_STORAGE_KEY, String(v))
    } catch {
        sessionSeen = v
    }
}

/** 资格判定 = seen < current：首访（0）与内容改版（bump TOUR_VERSION）后重弹 */
export function shouldShowTour(seen: number, current: number = TOUR_VERSION): boolean {
    return seen < current
}

/** 首访轻导步骤清单（spec §2.2 七步初稿，文案实施票微调）：锚 example 自有 DOM
 *  （顶栏/工具栏），顺序即走查顺序；selector 与 EditorPage.vue 既有 data-* 一一
 *  对应，改锚点的 PR 须对照 spec §4 走查清单。 */
export const TOUR_STEPS: TourStep[] = [
    {
        id: 'welcome',
        title: '欢迎',
        text: '这里是证书批量生成编辑器。跟着走一遍核心闭环，约 1 分钟；之后可随时从顶栏「功能导游」重看。',
    },
    {
        id: 'insert',
        title: '插入内容',
        text: '从这里插入文本、图片、二维码或表格；绑定数据源后，内容里可用 {{字段}} 表达式引用数据。',
        selector: 'data-insert-menu',
    },
    {
        id: 'datasource',
        title: '数据源',
        text: '打开数据源抽屉绑定数据源；绑定后文字、图片、二维码都可以引用数据内容，批量出证。',
        selector: 'data-open-datasource',
    },
    {
        id: 'save',
        title: '保存',
        text: '这里显示保存态（已保存 / 未保存）；手动保存也走顶栏的「保存」。',
        selector: 'data-save-state',
    },
    {
        id: 'publish',
        title: '发布与版本',
        text: '把当前草稿发布成一个版本；版本历史入口也在这里。',
        selector: 'data-publish-template',
    },
    {
        id: 'render',
        title: '渲染',
        text: '渲染一张终图看实际效果。',
        selector: 'data-render-final',
    },
    {
        id: 'finish',
        title: '开始编辑',
        text: '就这些！随时可从顶栏「功能导游」重看本导游；按 ⌘/ 查看快捷键。',
    },
]

/** 裸 data-* 属性名 → 属性选择器（[data-*]）：锚点查询形态的单点（spec §2.5
 *  锚点纪律的机械半边）——filterExistingSteps 与 driver 接线（tourDriver.ts）共用，
 *  查询形态改动只落此处。 */
export function tourAnchorSelector(selector: string): string {
    return `[${selector}]`
}

/** 缺锚过滤（spec §2.2「缺锚跳步不报错」）：selector 在 doc 上查不到的步跳过、
 *  保序；无锚步（居中）恒保留。查询形态经 tourAnchorSelector 单点包 []。 */
export function filterExistingSteps(steps: readonly TourStep[], doc: TourDoc): TourStep[] {
    return steps.filter((step) => {
        if (step.selector === undefined) return true
        try {
            return doc.querySelector(tourAnchorSelector(step.selector)) !== null
        } catch {
            return false
        }
    })
}
