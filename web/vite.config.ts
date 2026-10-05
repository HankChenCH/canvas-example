import vue from '@vitejs/plugin-vue'
import tailwindcss from '@tailwindcss/vite'
import { defineConfig } from 'vite'

// dev 双轨(spec §1.3/§1.4)：/api /assets /renders 反代 Go 服务端(:8080)，
// 其余走宿主静态；compose 轨由 nginx 反代同三前缀(nginx.conf)
export default defineConfig({
    plugins: [vue(), tailwindcss()],
    server: {
        proxy: {
            '/api': 'http://localhost:8080',
            '/assets': 'http://localhost:8080',
            '/renders': 'http://localhost:8080',
        },
    },
})
