import { createApp } from 'vue'

// 面板设计令牌（shadcn 语义变量，纯 CSS）先于应用样式载入（spec §1.3 宿主三步之三）
import '@hankchen/canvas-editor-vue/panel-theme.css'
// 发布产物里 SFC <style> 被抽到 dist/style.css（经 ./style.css 出口暴露），
// 组件样式（标尺/参考线/贴边浮层/画布表面网格等）须由宿主显式引入，漏引则画布 chrome 全裸奔
import '@hankchen/canvas-editor-vue/style.css'
import './style.css'
import App from './App.vue'
import { router } from './router'

createApp(App).use(router).mount('#app')
