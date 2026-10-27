// @vitest-environment jsdom
// theme.ts 单例的行为锁（跟随 canvas-editor-vue useCnTheme.test.ts 的体例：每个
// 用例 vi.resetModules() + 动态导入，保证模块级单例在各用例间隔离）。jsdom 没有
// matchMedia，先装桩再导入——useDark 缺省 initialValue 'auto' 会经 useMediaQuery
// 查询系统偏好；桩恒 matches:false = 系统偏好日间。
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'

function stubMatchMedia(matches: boolean): void {
    vi.stubGlobal('matchMedia', (query: string) => ({
        matches,
        media: query,
        onchange: null,
        addEventListener: () => {},
        removeEventListener: () => {},
        addListener: () => {},
        removeListener: () => {},
    }))
}

async function loadTheme() {
    return await import('./theme')
}

beforeEach(() => {
    vi.resetModules()
    window.localStorage.clear()
    document.documentElement.removeAttribute('data-cn-theme')
})

afterEach(() => {
    vi.unstubAllGlobals()
})

describe('全应用主题单例（useDark 驱动）', () => {
    it('无存储值时跟随系统偏好（桩 = 日间），信号写到 html', async () => {
        stubMatchMedia(false)
        const { isDark } = await loadTheme()
        expect(isDark.value).toBe(false)
        expect(document.documentElement.getAttribute('data-cn-theme')).toBe('light')
    })

    it('toggle 往返：信号与 localStorage 同步翻转', async () => {
        stubMatchMedia(false)
        const { isDark, toggleDark, THEME_STORAGE_KEY } = await loadTheme()
        // useDark 的信号写入走 watcher（pre flush），ref 置值后需等冲刷再断言 DOM
        toggleDark()
        await nextTick()
        expect(isDark.value).toBe(true)
        expect(document.documentElement.getAttribute('data-cn-theme')).toBe('dark')
        expect(window.localStorage.getItem(THEME_STORAGE_KEY)).toBe('dark')
        toggleDark()
        await nextTick()
        expect(isDark.value).toBe(false)
        expect(document.documentElement.getAttribute('data-cn-theme')).toBe('light')
    })

    it('有存储值时忽略系统偏好，按存储值初始化', async () => {
        stubMatchMedia(false)
        window.localStorage.setItem('example-web.cn-theme', 'dark')
        const { isDark } = await loadTheme()
        expect(isDark.value).toBe(true)
        expect(document.documentElement.getAttribute('data-cn-theme')).toBe('dark')
    })

    it('多处导入共享同一响应式状态（单例同一性）', async () => {
        stubMatchMedia(false)
        const a = await loadTheme()
        const b = await loadTheme()
        expect(a.isDark).toBe(b.isDark)
        expect(a.toggleDark).toBe(b.toggleDark)
    })
})
