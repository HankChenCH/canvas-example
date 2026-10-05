import { createRouter, createWebHistory } from 'vue-router'

import { api } from './api'
import EditorPage from './pages/EditorPage.vue'
import TemplateListPage from './pages/TemplateListPage.vue'

// 「首访」= 本会话第一次路由导航（spec §4.1）：落在 / 时重定向到 updatedAt
// 最新模板的编辑器（列表首项，服务端 updatedAt 降序）；此后返回列表不再
// 重定向，换模板正常。列表空或读取失败不拦——落列表页展示空态/错误态。
let entryHandled = false

export const router = createRouter({
    history: createWebHistory(),
    routes: [
        { path: '/', name: 'template-list', component: TemplateListPage },
        // 14 票占位页：能取模板名显示即可；完整装配在 15 票
        { path: '/editor/:id', name: 'editor', component: EditorPage },
    ],
})

router.beforeEach(async (to) => {
    if (entryHandled || to.path !== '/') return true
    entryHandled = true
    try {
        const templates = await api.listTemplates()
        const latest = templates[0]
        if (latest) return { path: `/editor/${latest.id}` }
    } catch {
        // 列表读失败（服务端未起等）放行到列表页，由页面展示错误态
    }
    return true
})
