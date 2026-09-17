import { defineConfig } from 'vitest/config'
import react from '@vitejs/plugin-react'

export default defineConfig({
  plugins: [react()],
  test: {
    environment: 'jsdom',
    environmentOptions: {
      jsdom: { url: 'http://localhost/' },
    },
    setupFiles: ['./src/test/setup.ts'],
    css: false,
    coverage: {
      provider: 'v8',
      reporter: ['text', 'json-summary'],
      // 保守阈值：拦截覆盖率大幅回退，页面组件可逐步补测提升
      thresholds: {
        statements: 40,
        branches: 20,
        functions: 15,
        lines: 40,
      },
    },
  },
})