import { createRouter, createWebHistory } from 'vue-router'

import { api } from './api'
import DataSourceEditPage from './pages/DataSourceEditPage.vue'
import DataSourceListPage from './pages/DataSourceListPage.vue'
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
        // 25 票：数据源独立管理面——列表页 + 新建/编辑页。new 与 :id 走同一条
        // 路由记录（:id === 'new' 即新建态）：创建成功后的 replace 是同记录参数
        // 变化，不触发组件的 onBeforeRouteLeave 离开守卫（EditorPage 另存为同款；
        // 若拆成静态 + 参数两条记录，replace 跨记录会误弹脏内容 confirm）
        { path: '/datasources', name: 'datasource-list', component: DataSourceListPage },
        { path: '/datasources/:id', name: 'datasource-edit', component: DataSourceEditPage },
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
