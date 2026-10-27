// 全应用日/夜主题单例（canvas-web 2.2.0 editor-theme-light-dark 集成）。
//
// VueUse useDark 是唯一信号驱动源：attribute 模式把 `data-cn-theme="dark"/"light"`
// 写到 html（selector 缺省即 document.documentElement，信号只设一处，ADR 0001 契约），
// 编辑器外观（theme.css 的 --cn-* 日间覆盖）、壳层（EditorPage 的 --shell-*）、
// 印坊管理页（style.css 的 --pc-*）与七个 Teleport 弹层的日间块全部跟随该信号。
// valueLight 必须显式给 'light'：VueUse 缺省是空串（删属性），而 canvas-web 语义里
// 属性缺席 = 夜间，日间会失效（ADR 0001 已知限制）。
// storageKey 自管（'example-web.cn-theme'）：切换后 localStorage 持久化，刷新保持；
// 无存储值时 initialValue 走 useDark 缺省 'auto'，跟随系统 prefers-color-scheme
// （playground 先例是钉死 'dark'，本应用按集成共识跟随系统）。
// 已知 VueUse 行为（playground 注记同款）：把开关拨回与系统偏好一致的一侧时
// storage 会写 'auto'——state 恒解析回具体值，信号观感不变。
// 不调用包内 useCnTheme()：两套写信号方会互相打架，全应用只留 useDark 一个驱动源。
import { useDark, useToggle } from '@vueuse/core'

export const THEME_STORAGE_KEY = 'example-web.cn-theme'

/** 夜间为 true（与 data-cn-theme="dark" 对应）——全应用共享同一响应式状态 */
export const isDark = useDark({
    attribute: 'data-cn-theme',
    valueDark: 'dark',
    valueLight: 'light',
    storageKey: THEME_STORAGE_KEY,
})

/** 日/夜往返（各页面切换钮共用） */
export const toggleDark = useToggle(isDark)
