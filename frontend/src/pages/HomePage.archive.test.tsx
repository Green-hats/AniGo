import { beforeEach, afterEach, describe, expect, it, vi } from 'vitest'
import { render, screen, within, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { message } from 'antd'
import { api } from '../api/client'
import type { Ani } from '../types'
import HomePage from './HomePage'

vi.mock('../api/client', () => ({
  api: {
    listAni: vi.fn(),
    refreshStatus: vi.fn(),
    archiveAni: vi.fn(),
    playList: vi.fn(),
  },
}))

let subscriptions: Ani[]

beforeEach(() => {
  vi.resetAllMocks()
  vi.spyOn(message, 'success').mockImplementation(() => (() => {}) as ReturnType<typeof message.success>)
  vi.spyOn(message, 'error').mockImplementation(() => (() => {}) as ReturnType<typeof message.error>)
  subscriptions = [{
    id: 'finished', title: '待归档番剧', season: 1, enable: false, archived: false,
    image: '', downloadedEps: 12, currentEpisodeNumber: 12, totalEpisodeNumber: 12, score: 0,
  } as Ani]
  vi.mocked(api.listAni).mockImplementation(async () => ({
    total: subscriptions.length,
    releaseDateList: [],
    weekList: [{ weekLabel: '星期一', items: subscriptions.filter((ani) => !ani.archived) }],
    archivedList: subscriptions.filter((ani) => ani.archived),
  }))
  vi.mocked(api.archiveAni).mockImplementation(async (id, archived) => {
    subscriptions = subscriptions.map((ani) => ani.id === id ? { ...ani, archived } : ani)
    return null
  })
  vi.mocked(api.refreshStatus).mockResolvedValue([])
  vi.mocked(api.playList).mockResolvedValue([])
})

afterEach(() => {
  vi.restoreAllMocks()
})

function renderHome() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false, gcTime: 0 } } })
  return render(<QueryClientProvider client={client}><HomePage /></QueryClientProvider>)
}

describe('subscription archive', () => {
  it('手动归档后移到独立列表，保留播放并支持恢复', async () => {
    const user = userEvent.setup()
    renderHome()
    const card = await screen.findByRole('article', { name: '待归档番剧' })
    expect(within(card).getByText('停用')).toBeInTheDocument()
    await user.click(within(card).getByRole('button', { name: /归档/ }))
    await waitFor(() => expect(api.archiveAni).toHaveBeenCalledWith('finished', true))
    expect(await screen.findByText('暂无订阅')).toBeInTheDocument()
    expect(screen.queryByRole('article', { name: '待归档番剧' })).not.toBeInTheDocument()

    await user.click(screen.getByRole('tab', { name: '已归档 (1)' }))
    const archivedCard = await screen.findByRole('article', { name: '待归档番剧' })
    expect(within(archivedCard).getByRole('button', { name: '播放' })).toBeInTheDocument()
    expect(within(archivedCard).queryByRole('button', { name: '刷新' })).not.toBeInTheDocument()
    expect(within(archivedCard).queryByRole('button', { name: '启用' })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /刷新全部/ })).not.toBeInTheDocument()
    await user.click(within(archivedCard).getByRole('button', { name: /恢复订阅/ }))
    await waitFor(() => expect(api.archiveAni).toHaveBeenCalledWith('finished', false))
    expect(await screen.findByText('暂无归档订阅')).toBeInTheDocument()

    await user.click(screen.getByRole('tab', { name: '订阅中 (1)' }))
    const restoredCard = await screen.findByRole('article', { name: '待归档番剧' })
    expect(within(restoredCard).getByText('停用')).toBeInTheDocument()
    expect(subscriptions[0].downloadedEps).toBe(12)
  })

  it('归档后仍可打开选集播放', async () => {
    subscriptions[0].archived = true
    renderHome()
    const user = userEvent.setup()
    await user.click(await screen.findByRole('tab', { name: '已归档 (1)' }))
    const card = await screen.findByRole('article', { name: '待归档番剧' })
    await user.click(within(card).getByRole('button', { name: '播放' }))
    expect(await screen.findByRole('dialog')).toBeInTheDocument()
    expect(api.playList).toHaveBeenCalledWith('finished')
  })

  it('保存失败时保留原列表并显示错误', async () => {
    vi.mocked(api.archiveAni).mockRejectedValue(new Error('磁盘写入失败'))
    renderHome()
    const user = userEvent.setup()
    const card = await screen.findByRole('article', { name: '待归档番剧' })
    await user.click(within(card).getByRole('button', { name: /归档/ }))
    await waitFor(() => expect(message.error).toHaveBeenCalledWith('磁盘写入失败'))
    expect(screen.getByRole('article', { name: '待归档番剧' })).toBeInTheDocument()
    expect(screen.getByRole('tab', { name: '已归档 (0)' })).toBeInTheDocument()
  })
})
