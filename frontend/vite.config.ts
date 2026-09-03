import path from 'node:path'
import tailwindcss from '@tailwindcss/vite'
import react from '@vitejs/plugin-react'
import { defineConfig } from 'vite'

// 前端源码只请求相对路径 /api/v1 与 /media：
// 开发时由 Vite 代理转发到联调后端（可用 DEV_API_ORIGIN 覆盖），
// 生产环境由 Nginx 将同一路径转发到 127.0.0.1:18083。
const backendOrigin = process.env.DEV_API_ORIGIN ?? 'http://123.56.161.234:18083'

export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: {
    alias: {
      '@': path.resolve(__dirname, 'src'),
    },
  },
  server: {
    // 默认监听全部网卡：本机 localhost 与 Tailscale IP（如 100.77.91.52）
    // 均可访问，方便同一 Tailscale 网络下的其他设备联调；可用 DEV_HOST 覆盖。
    host: process.env.DEV_HOST ?? true,
    port: 5173,
    proxy: {
      '/api/v1': {
        target: backendOrigin,
        changeOrigin: true,
      },
      '/media': {
        target: backendOrigin,
        changeOrigin: true,
      },
    },
  },
})
