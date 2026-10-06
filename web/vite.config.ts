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
    // 注意：@hankchen 四包是 file: 链接的同仓源码包，Vite 的依赖预构建产物
    // （node_modules/.vite/deps）不会因链接包源码变化而失效——改了
    // canvas-web/packages/* 源码后，dev 轨需 `pnpm dev --force`（或删除
    // node_modules/.vite）重启，否则浏览器继续跑旧包代码；build 轨每次全量
    // 打包不受影响。勿把这些包 exclude 出预构建：编辑器包的 CJS 依赖
    // （qrcode）在源码直引形态下具名导入互操作会炸（mounted 前整页白屏）。
    server: {
        proxy: {
            '/api': 'http://localhost:8080',
            '/assets': 'http://localhost:8080',
            '/renders': 'http://localhost:8080',
        },
    },
})
