import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { App } from 'antd'
import LogsPage from './LogsPage'
import { api } from '../api/client'

vi.mock('../api/client', () => ({ api: { getLogs: vi.fn(), getStatus: vi.fn(), notifications: vi.fn(), aiPing: vi.fn(), retryNotification: vi.fn(), clearLogs: vi.fn() } }))
function mount() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false, gcTime: 0 } } })
  render(<QueryClientProvider client={client}><App><LogsPage /></App></QueryClientProvider>)
}
beforeEach(() => {
  vi.mocked(api.getLogs).mockResolvedValue([])
  vi.mocked(api.notifications).mockResolvedValue([])
  vi.mocked(api.getStatus).mockResolvedValue({ ai: { configured: true, enabled: true, ok: false, message: '', reply: '', checkedAt: 0 }, cloudName: 'pikpak', cloud: { configured: true, loginOK: false, message: '', checkedAt: 0 }, memory: { allocMB: 1, totalAllocMB: 1, sysMB: 1, numGC: 0 }, cache: { count: 0, bytes: 0, sizeKB: 0 }, uptimeSeconds: 1 })
})
afterEach(() => { vi.clearAllMocks() })
it('显示当前网盘，查看页面不会自动测试 AI', async () => {
  mount()
  expect(await screen.findByText('PikPak 网盘')).toBeInTheDocument()
  expect(screen.queryByText('115 网盘')).not.toBeInTheDocument()
  expect(screen.getAllByText('尚未检查').length).toBeGreaterThan(0)
  expect(api.aiPing).not.toHaveBeenCalled()
  vi.mocked(api.aiPing).mockResolvedValue({ reply: 'ok' })
  await userEvent.click(screen.getByRole('button', { name: '测试 AI 连接' }))
  await waitFor(() => expect(api.aiPing).toHaveBeenCalledTimes(1))
  await waitFor(() => expect(api.getStatus).toHaveBeenCalledTimes(2))
})
it('失败通知支持确认补发', async () => {
  vi.mocked(api.notifications).mockResolvedValue([{ id: 'delivery', channel: 'BARK', title: '测试番剧', state: 'failed', attempts: 2, error: '网络不可用', updatedAt: 1 }])
  vi.mocked(api.retryNotification).mockResolvedValue(null)
  mount()
  expect(await screen.findByText('网络不可用')).toBeInTheDocument()
  await userEvent.click(screen.getByRole('button', { name: /补\s*发/ }))
  expect(api.retryNotification).not.toHaveBeenCalled()
  await userEvent.click(await screen.findByRole('button', { name: /OK|确\s*定/ }))
  await waitFor(() => expect(api.retryNotification).toHaveBeenCalledWith('delivery'))
})
