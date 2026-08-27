import { describe, it, expect, vi, afterEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
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

const loginOkData = { code: 200, message: '', data: { login: true, token: 't' }, t: 0 }
const checkLoginOkData = { code: 200, message: '', data: { login: true }, t: 0 }
const checkLoginUnauthorized = { code: 401, message: '未登录或登录已过期', data: null, t: 0 }

function mockFetchImpl({ authed }: { authed: boolean }) {
  return vi.fn().mockImplementation((url: string) => {
    if (url === '/api/checkLogin') {
      return Promise.resolve({
        json: async () => (authed ? checkLoginOkData : checkLoginUnauthorized),
      })
    }
    if (url === '/api/login') {
      return Promise.resolve({ json: async () => loginOkData })
    }
    return Promise.resolve({ json: async () => emptyListData })
  })
}

describe('App', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
    localStorage.clear()
  })

  it('已登录时渲染侧边导航与标题', async () => {
    vi.stubGlobal('fetch', mockFetchImpl({ authed: true }))
    renderApp()
    expect(await screen.findByText('AniGo')).toBeInTheDocument()
    expect(screen.getByText('我的订阅')).toBeInTheDocument()
    expect(screen.getByText('番剧源')).toBeInTheDocument()
    expect(screen.getByText('日志')).toBeInTheDocument()
    expect(screen.getByText('设置')).toBeInTheDocument()
  })

  it('已登录时默认路由重定向到首页，并加载订阅列表', async () => {
    vi.stubGlobal('fetch', mockFetchImpl({ authed: true }))
    renderApp()
    // lazy 页面加载后 HomePage 渲染，展示空列表计数
    expect(await screen.findByText('我的订阅 (0)')).toBeInTheDocument()
  })

  it('未登录时渲染登录页', async () => {
    vi.stubGlobal('fetch', mockFetchImpl({ authed: false }))
    renderApp()
    expect(await screen.findByPlaceholderText('用户名')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '登 录' })).toBeInTheDocument()
    // 不应渲染侧边导航
    expect(screen.queryByText('我的订阅')).not.toBeInTheDocument()
  })

  it('登录成功后进入主界面', async () => {
    vi.stubGlobal('fetch', mockFetchImpl({ authed: false }))
    renderApp()
    await userEvent.type(await screen.findByPlaceholderText('用户名'), 'admin')
    await userEvent.type(screen.getByPlaceholderText('密码'), 'admin')
    await userEvent.click(screen.getByRole('button', { name: '登 录' }))
    expect(await screen.findByText('我的订阅')).toBeInTheDocument()
  })
})