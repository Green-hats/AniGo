import '@testing-library/jest-dom/vitest'
import { vi, afterEach } from 'vitest'
import { cleanup } from '@testing-library/react'

// 未启用 vitest globals 时，testing-library 的自动 cleanup 不会注册
afterEach(() => {
  cleanup()
})

// jsdom 环境未暴露 localStorage，提供内存实现（client.ts 依赖它存登录 token）
class MemoryStorage implements Storage {
  private store = new Map<string, string>()

  get length() {
    return this.store.size
  }

  clear() {
    this.store.clear()
  }

  getItem(key: string) {
    return this.store.has(key) ? this.store.get(key)! : null
  }

  key(index: number) {
    return Array.from(this.store.keys())[index] ?? null
  }

  removeItem(key: string) {
    this.store.delete(key)
  }

  setItem(key: string, value: string) {
    this.store.set(key, String(value))
  }
}

const memStorage = new MemoryStorage()
Object.defineProperty(window, 'localStorage', { value: memStorage, configurable: true })
if (typeof globalThis.localStorage === 'undefined') {
  Object.defineProperty(globalThis, 'localStorage', { value: memStorage, configurable: true })
}

// antd 依赖 matchMedia，jsdom 未实现
Object.defineProperty(window, 'matchMedia', {
  writable: true,
  value: vi.fn().mockImplementation((query: string) => ({
    matches: false,
    media: query,
    onchange: null,
    addListener: vi.fn(),
    removeListener: vi.fn(),
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
    dispatchEvent: vi.fn(),
  })),
})

// jsdom 不执行布局；保留组件生命周期，只替代浏览器布局观察接口。
class TestResizeObserver implements ResizeObserver {
  observe() {}
  unobserve() {}
  disconnect() {}
}
Object.defineProperty(globalThis, 'ResizeObserver', { value: TestResizeObserver, configurable: true })
Object.defineProperty(window, 'ResizeObserver', { value: TestResizeObserver, configurable: true })
const nativeGetComputedStyle = window.getComputedStyle.bind(window)
// jsdom 不支持伪元素计算样式，普通元素样式仍由 jsdom 计算。
window.getComputedStyle = (element) => nativeGetComputedStyle(element)
