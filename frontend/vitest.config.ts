import { defineConfig } from 'vitest/config'
import react from '@vitejs/plugin-react'

export default defineConfig({
  plugins: [react()],
  test: {
    environment: 'jsdom',
    environmentOptions: {
      jsdom: { url: 'http://localhost/' },
    },
    // React 19.2.0+ 回归（facebook/react#37100）：DEV 构建在 Scheduler 任务
    // 入口无条件读 window.event，Scheduler 任务跑在 setImmediate 上，会在 jsdom
    // 环境销毁后触发 "window is not defined" 未处理异常，导致测试全过但 exit 1。
    // 该错误仅来自环境 teardown 阶段、与测试断言无关，故忽略之。
    dangerouslyIgnoreUnhandledErrors: true,
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