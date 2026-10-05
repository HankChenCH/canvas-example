<script setup lang="ts">
// 宿主壳：只做路由出口（spec §4.1 路由表）。列表页与编辑器页各自成页。
// RouterView 不加 :key（18 票调整原「按全路径重挂」）：editor→editor 参数级导航
// （另存为 replace 到副本）复用编辑器实例延续会话，不重走载入闪烁；会话与路由
// id 的绑定改由 EditorPage 自守——另存为 replace 前先重绑 template.id（watcher
// 判同不重载），其余 id 脱钩的参数变化走防御性重载，补齐原 :key 的保证。列表 ↔
// 编辑器是组件切换，仍整页新挂。
</script>

<template>
    <RouterView />
</template>
