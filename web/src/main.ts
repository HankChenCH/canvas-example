import { createApp } from 'vue'

// 外观设计令牌单源（canvas-web 2.2.0 editor-theme-light-dark，ADR 0001）：--cn-* 令牌
// 与日间信号覆盖全在 theme.css（panel-theme.css 已退化为它的兼容 shim）。主题信号
// data-cn-theme 由 src/theme.ts 的 useDark 写到 html——先于应用样式载入
// （spec §1.3 宿主三步之三）
import '@hankchen/canvas-editor-vue/theme.css'
// 发布产物里 SFC <style> 被抽到 dist/style.css（经 ./style.css 出口暴露），
// 组件样式（标尺/参考线/贴边浮层/画布表面网格等）须由宿主显式引入，漏引则画布 chrome 全裸奔
import '@hankchen/canvas-editor-vue/style.css'
import './style.css'
import App from './App.vue'
import { router } from './router'

createApp(App).use(router).mount('#app')
