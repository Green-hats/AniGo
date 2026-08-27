import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// 开发时 /api 代理到后端 :7789
export default defineConfig({
  plugins: [react()],
  server: {
    host: '0.0.0.0',
    port: 37789,
    proxy: {
      '/api': {
        target: 'http://127.0.0.1:7789',
        changeOrigin: true,
      },
    },
  },
  build: {
    outDir: 'dist',
    emptyOutDir: true,
    // antd 体积较大，适度放宽大 chunk 告警阈值
    chunkSizeWarningLimit: 700,
  },
})