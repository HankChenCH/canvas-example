import { createApp } from 'vue'

// 面板设计令牌（shadcn 语义变量，纯 CSS）先于应用样式载入（spec §1.3 宿主三步之三）
import '@hankchen/canvas-next-editor-vue/panel-theme.css'
import './style.css'
import App from './App.vue'
import { router } from './router'

createApp(App).use(router).mount('#app')
