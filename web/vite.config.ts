import vue from '@vitejs/plugin-vue'
import tailwindcss from '@tailwindcss/vite'
import { defineConfig } from 'vite'

// dev 双轨(spec §1.3/§1.4)：/api /assets /renders 反代 Go 服务端(:8080)，
// 其余走宿主静态；compose 轨由 nginx 反代同三前缀(nginx.conf)。
//
// build 产物必须挪出 vite 默认的 assetsDir="assets"：server 的上传/字体占用了
// /assets/* URL 空间(§2.2)，compose 轨 nginx 把 /assets 全量反代到 Go——默认命名
// 的 bundle 被 Go FileServer 404，模块不执行整页白屏；dev 轨模块走 /src/ 与
// /node_modules/ 不经 assetsDir，故不受影响(20 票实施勘误，spec §1.3 已注记)。
export default defineConfig({
    plugins: [vue(), tailwindcss()],
    build: {
        assetsDir: 'static',
    },
    server: {
        proxy: {
            '/api': 'http://localhost:8080',
            '/assets': 'http://localhost:8080',
            '/renders': 'http://localhost:8080',
        },
    },
})
