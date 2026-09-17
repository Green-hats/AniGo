import { afterEach, describe, expect, it, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { message } from 'antd'
import SettingsPage from './SettingsPage'
import { api } from '../api/client'
import type { Config } from '../types'

vi.mock('../api/client', () => ({ api: { getConfig: vi.fn(), setConfig: vi.fn(), downloadLoginTest: vi.fn() } }))
afterEach(() => { vi.restoreAllMocks(); vi.clearAllMocks() })
function mount(tool: string) {
  vi.mocked(api.getConfig).mockResolvedValue({ downloadToolType: tool, pan115Cookie: 'saved-cookie', pikpakEmail: 'saved@example.com', pikpakPassword: 'saved-pass', notificationConfigList: [] } as unknown as Config)
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false, gcTime: 0 } } })
  render(<QueryClientProvider client={qc}><SettingsPage /></QueryClientProvider>)
}
describe('网盘配置', () => {
  it('PikPak 显示账号密码并用未保存的表单测试登录', async () => {
    vi.mocked(api.downloadLoginTest).mockResolvedValue(null)
    const success = vi.spyOn(message, 'success').mockImplementation(() => (() => {}) as ReturnType<typeof message.success>)
    mount('pikpak')
    await userEvent.click(await screen.findByRole('tab', { name: '下载' }))
    const account = await screen.findByLabelText('PikPak 账号')
    await userEvent.clear(account)
    await userEvent.type(account, '+8613812345678')
    expect(screen.queryByLabelText('115 Cookie')).not.toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: '测试 PikPak 登录' }))
    await waitFor(() => expect(api.downloadLoginTest).toHaveBeenCalledWith(expect.objectContaining({ downloadToolType: 'pikpak', pikpakEmail: '+8613812345678', pikpakPassword: 'saved-pass' })))
    expect(api.setConfig).not.toHaveBeenCalled()
    expect(success).toHaveBeenCalledWith('PikPak 登录成功')
  })
  it('保留 115 Cookie 配置和对应登录测试入口', async () => {
    mount('115')
    await userEvent.click(await screen.findByRole('tab', { name: '下载' }))
    expect(await screen.findByLabelText('115 Cookie')).toHaveValue('saved-cookie')
    expect(screen.queryByLabelText('PikPak 密码')).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: '测试 115 登录' })).toBeInTheDocument()
  })
})
