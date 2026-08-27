import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import App from './App'

function renderApp() {
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={qc}>
      <App />
    </QueryClientProvider>
  )
}

const emptyListData = {
  code: 200,
  message: '',
  data: { releaseDateList: [], weekList: [], total: 0 },
  t: 0,
}

describe('App', () => {
  beforeEach(() => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({ json: async () => emptyListData }),
    )
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('渲染侧边导航与标题', async () => {
    renderApp()
    expect(screen.getByText('🌸 AniGo')).toBeInTheDocument()
    expect(screen.getByText('我的订阅')).toBeInTheDocument()
    expect(screen.getByText('番剧源')).toBeInTheDocument()
    expect(screen.getByText('日志')).toBeInTheDocument()
    expect(screen.getByText('设置')).toBeInTheDocument()
  })

  it('默认路由重定向到首页，并加载订阅列表', async () => {
    renderApp()
    // lazy 页面加载后 HomePage 渲染，展示空列表计数
    expect(await screen.findByText('我的订阅 (0)')).toBeInTheDocument()
  })
})