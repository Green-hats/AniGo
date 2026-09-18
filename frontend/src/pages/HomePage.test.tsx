import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { act, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { message } from 'antd'
import HomePage from './HomePage'
import { api } from '../api/client'
import type { Ani, ListAniData } from '../types'

vi.mock('../api/client', () => ({ api: {
  listAni: vi.fn(), recoverTask: vi.fn(), refreshStatus: vi.fn(), refreshAll: vi.fn(), refreshAni: vi.fn(),
  batchEnable: vi.fn(), deleteAni: vi.fn(), playList: vi.fn(), playTicket: vi.fn(),
} }))

const ani = { id: 'one', title: '测试番剧', enable: true, season: 1, score: 0, downloadedEps: 0, currentEpisodeNumber: 5, totalEpisodeNumber: 12 } as Ani
const list = (completed = 0): ListAniData => ({ total: 1, releaseDateList: [], weekList: [{ weekLabel: '星期一', items: [{ ...ani, downloadedEps: completed }] }] })

function mount() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false, gcTime: 0 } } })
  render(<QueryClientProvider client={qc}><HomePage /></QueryClientProvider>)
  return qc
}

beforeEach(() => {
  vi.mocked(api.listAni).mockResolvedValue(list())
  vi.mocked(api.refreshStatus).mockResolvedValue([])
})
afterEach(() => { vi.restoreAllMocks(); vi.clearAllMocks() })

describe('HomePage feedback', () => {
  it('显示加载错误，支持重试', async () => {
    vi.mocked(api.listAni).mockRejectedValueOnce(new Error('网络断开'))
    mount()
    expect(await screen.findByText('网络断开')).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: /重\s*试/ }))
    expect(await screen.findByText('测试番剧')).toBeInTheDocument()
  })

  it('刷新完成后自动更新进度，不把 RSS 集数当作已完成数', async () => {
    const qc = mount()
    expect(await screen.findByText(/已完成 0/)).toBeInTheDocument()
    vi.mocked(api.listAni).mockResolvedValue(list(1))
    act(() => qc.setQueryData(['refreshStatus'], [{ id: 'one', state: 'completed', updatedAt: 123 }]))
    expect(await screen.findByText(/已完成 1/)).toBeInTheDocument()
  })

  it('后台任务运行期间保持刷新按钮忙碌，并显示失败原因', async () => {
    vi.mocked(api.refreshStatus).mockResolvedValue([{ id: 'one', state: 'running', updatedAt: 1 }])
    const qc = mount()
    const button = screen.getByRole('button', { name: /刷新全部/ })
    await waitFor(() => expect(button).toHaveClass('ant-btn-loading'))
    act(() => qc.setQueryData(['refreshStatus'], [{ id: 'one', state: 'failed', updatedAt: 2, error: 'RSS 无法访问' }]))
    expect(await screen.findByText('RSS 无法访问')).toBeInTheDocument()
    await waitFor(() => expect(button).not.toHaveClass('ant-btn-loading'))
  })

  it('启停操作失败时展示错误', async () => {
    const error = vi.spyOn(message, 'error').mockImplementation(() => (() => {}) as ReturnType<typeof message.error>)
    vi.mocked(api.batchEnable).mockRejectedValue(new Error('保存失败'))
    mount()
    await userEvent.click(await screen.findByRole('button', { name: /停\s*用/ }))
    await waitFor(() => expect(error).toHaveBeenCalledWith('保存失败'))
  })

  it('点击播放时申请单文件凭证，签发失败时提示用户', async () => {
    const error = vi.spyOn(message, 'error').mockImplementation(() => (() => {}) as ReturnType<typeof message.error>)
    vi.mocked(api.playList).mockResolvedValue([{ episode: 1, filename: '第一集.mkv', pickCode: 'pc1' }])
    vi.mocked(api.playTicket).mockRejectedValue(new Error('播放凭证申请失败'))
    mount()
    await screen.findByText('测试番剧')
    const icon = screen.getByRole('img', { name: 'play-circle' })
    await userEvent.click(icon.closest('button')!)
    await userEvent.click(await screen.findByRole('button', { name: /用 mpv 播放/ }))
    await waitFor(() => expect(api.playTicket).toHaveBeenCalledWith('one', 'pc1'))
    expect(error).toHaveBeenCalledWith('播放凭证申请失败')
  })
})


describe('下载任务恢复', () => {
  const failedList = (): ListAniData => ({ ...list(), weekList: [{ weekLabel: '星期一', items: [{ ...ani, downloadTasks: [{ hash: 'aaaa', episode: 1, state: 'exhausted', attempts: 3, retryAt: 0, error: '云端失败' }] }] }] })

  it('重试耗尽的任务可以手动重试，保存后刷新状态', async () => {
    vi.mocked(api.listAni).mockResolvedValue(failedList())
    vi.mocked(api.recoverTask).mockResolvedValue(null)
    mount()
    expect(await screen.findByText(/重试已耗尽/)).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: '重试第 1 集' }))
    await waitFor(() => expect(api.recoverTask).toHaveBeenCalledWith('one', 'aaaa', 1, 'retry'))
    await waitFor(() => expect(api.listAni).toHaveBeenCalledTimes(2))
  })

  it('换源需要确认，并将失败提示传给用户', async () => {
    vi.mocked(api.listAni).mockResolvedValue(failedList())
    vi.mocked(api.recoverTask).mockRejectedValue(new Error('当前账号下未找到该任务'))
    const error = vi.spyOn(message, 'error').mockImplementation(() => (() => {}) as ReturnType<typeof message.error>)
    mount()
    await userEvent.click(await screen.findByRole('button', { name: /换\s*源/ }))
    expect(api.recoverTask).not.toHaveBeenCalled()
    await userEvent.click(await screen.findByRole('button', { name: /OK|确\s*定/ }))
    await waitFor(() => expect(api.recoverTask).toHaveBeenCalledWith('one', 'aaaa', 1, 'replace'))
    expect(error).toHaveBeenCalledWith('当前账号下未找到该任务')
  })

  it('等待入队也保持忙碌；未知结果只允许查询', async () => {
    const data = failedList()
    data.weekList[0].items[0].downloadTasks![0].state = 'unknown'
    vi.mocked(api.listAni).mockResolvedValue(data)
    vi.mocked(api.refreshStatus).mockResolvedValue([{ id: 'one', state: 'waiting', updatedAt: 1 }])
    mount()
    expect(await screen.findByText('请刷新查询云端状态')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: '重试第 1 集' })).not.toBeInTheDocument()
    await waitFor(() => expect(screen.getByRole('button', { name: /刷新全部/ })).toHaveClass('ant-btn-loading'))
  })
})
