<script setup lang="ts">
// 宿主壳：只做路由出口（spec §4.1 路由表）。列表页与编辑器页各自成页。
// :key 按全路径重挂：/editor/1 → /editor/2 参数级导航强制重建页面组件，编辑器
// 会话与载入的模板严格一一对应（编辑器页按 id 加载并装配会话，复用实例会让
// 会话文档与路由 id 脱钩）。
</script>

<template>
    <RouterView :key="$route.fullPath" />
</template>
