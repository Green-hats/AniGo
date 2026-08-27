import type { Result } from '../types'

// 后端统一返回 {code, message, data, t}
const BASE = ''

// 默认请求超时（毫秒），避免后端接口卡住时 UI 无限等待。
const DEFAULT_TIMEOUT = 30_000

async function request<T>(method: string, url: string, body?: unknown, timeoutMs = DEFAULT_TIMEOUT): Promise<T> {
  const opts: RequestInit = { method, headers: {} }
  if (body !== undefined) {
    opts.headers = { 'Content-Type': 'application/json' }
    opts.body = JSON.stringify(body)
  }
  const ctrl = new AbortController()
  const timer = setTimeout(() => ctrl.abort(), timeoutMs)
  opts.signal = ctrl.signal
  try {
    const resp = await fetch(BASE + url, opts)
    const json = (await resp.json()) as Result<T>
    if (json.code !== 200) {
      throw new Error(json.message)
    }
    return json.data
  } catch (e) {
    if ((e as Error).name === 'AbortError') {
      throw new Error('请求超时')
    }
    throw e
  } finally {
    clearTimeout(timer)
  }
}

export const api = {
  ping: () => request<null>('GET', '/api/ping'),

  // 配置
  getConfig: () => request<import('../types').Config>('POST', '/api/config'),
  setConfig: (cfg: Partial<import('../types').Config>) =>
    request<null>('POST', '/api/setConfig', cfg),
  exportConfig: () => fetch(BASE + '/api/exportConfig'),
  importConfig: (file: File) => {
    const form = new FormData()
    form.append('file', file)
    return fetch(BASE + '/api/importConfig', { method: 'POST', body: form })
  },

  // 订阅
  listAni: () => request<import('../types').ListAniData>('POST', '/api/listAni'),
  addAni: (ani: Partial<import('../types').Ani>) =>
    request<null>('POST', '/api/addAni', ani),
  setAni: (ani: Partial<import('../types').Ani>) =>
    request<null>('POST', '/api/setAni', ani),
  deleteAni: (ids: string[]) => request<null>('POST', '/api/deleteAni', ids),
  batchEnable: (ids: string[], value: boolean) =>
    request<null>('POST', `/api/batchEnable?value=${value}`, ids),
  previewAni: (ani: Partial<import('../types').Ani>) =>
    request<import('../types').PreviewAniData>('POST', '/api/previewAni', ani),
  downloadPath: (ani: Partial<import('../types').Ani>) =>
    request<{ downloadPath: string }>('POST', '/api/downloadPath', ani),
  refreshAll: () => request<null>('POST', '/api/refreshAll'),
  refreshAni: (id: string) => request<null>('POST', '/api/refreshAni', { id }),
  rssToAni: (dto: import('../types').RssToAniDTO) =>
    request<import('../types').Ani>('POST', '/api/rssToAni', dto),

  // 下载
  downloadStatus: () => request<import('../types').LoginStatus>('POST', '/api/downloadStatus'),
  downloadLoginTest: (cookie?: string) =>
    request<null>('POST', '/api/downloadLoginTest', cookie ? { pan115Cookie: cookie } : {}),
  playList: (id: string) =>
    request<import('../types').PlayItem[]>('POST', '/api/playList', { id }),

  // AI
  aiPing: () => request<{ reply: string }>('POST', '/api/aiPing'),

  // 元数据
  searchBgm: (text: string) =>
    request<import('../types').BgmInfo[]>('POST', '/api/searchBgm', { text }),
  gardenList: () =>
    request<import('../types').GardenWeek[]>('POST', '/api/gardenList'),
  gardenGroup: (subject: string) =>
    request<import('../types').GardenGroup[]>('POST', `/api/gardenGroup?subject=${subject}`),

  // 通知
  testNotification: (nc: import('../types').NotificationConfig) =>
    request<null>('POST', '/api/testNotification', nc),

  // 日志
  getLogs: () => request<import('../types').LogEntry[]>('POST', '/api/logs'),
  clearLogs: () => request<null>('POST', '/api/clearLogs'),

  // 状态
  getStatus: () => request<import('../types').ServiceStatus>('POST', '/api/status'),
}