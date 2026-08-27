import type { Result } from '../types'

// 后端统一返回 {code, message, data, t}
const BASE = ''

// 默认请求超时（毫秒），避免后端接口卡住时 UI 无限等待。
const DEFAULT_TIMEOUT = 30_000

// localStorage 中保存登录 token 的键名。
const TOKEN_KEY = 'anigo_token'

export function getToken(): string {
  return localStorage.getItem(TOKEN_KEY) ?? ''
}

export function setToken(token: string) {
  localStorage.setItem(TOKEN_KEY, token)
}

export function clearToken() {
  localStorage.removeItem(TOKEN_KEY)
}

// 401 时清空凭证并通知 App 进入未登录态（渲染登录页）。
// 不直接改 hash：App 未登录分支对任意路由都渲染登录页，
// 避免"登录页 → 重定向回首页 → 再次 401"的跳转死循环。
function handleUnauthorized() {
  clearToken()
  window.dispatchEvent(new Event('anigo:unauthorized'))
}

async function request<T>(method: string, url: string, body?: unknown, timeoutMs = DEFAULT_TIMEOUT): Promise<T> {
  const opts: RequestInit = { method, headers: {} }
  const token = getToken()
  if (token) {
    opts.headers = { Authorization: `Bearer ${token}` }
  }
  if (body !== undefined) {
    opts.headers = { ...opts.headers, 'Content-Type': 'application/json' }
    opts.body = JSON.stringify(body)
  }
  const ctrl = new AbortController()
  const timer = setTimeout(() => ctrl.abort(), timeoutMs)
  opts.signal = ctrl.signal
  try {
    const resp = await fetch(BASE + url, opts)
    const json = (await resp.json()) as Result<T>
    if (json.code === 401) {
      handleUnauthorized()
    }
    if (json.code !== 200) {
      throw new Error(json.message)
    }
    return json.data
  } catch (e) {
    if ((e as Error).name === 'AbortError') {
      throw new Error('请求超时', { cause: e })
    }
    throw e
  } finally {
    clearTimeout(timer)
  }
}

export const api = {
  ping: () => request<null>('GET', '/api/ping'),

  // 登录
  login: (username: string, password: string) =>
    request<import('../types').LoginResp>('POST', '/api/login', { username, password }),
  logout: () => request<null>('POST', '/api/logout'),
  checkLogin: () => request<import('../types').CheckLoginResp>('POST', '/api/checkLogin'),

  // 配置
  getConfig: () => request<import('../types').Config>('POST', '/api/config'),
  setConfig: (cfg: Partial<import('../types').Config>) =>
    request<null>('POST', '/api/setConfig', cfg),
  exportConfig: () =>
    fetch(BASE + '/api/exportConfig', { headers: { Authorization: `Bearer ${getToken()}` } }),
  importConfig: (file: File) => {
    const form = new FormData()
    form.append('file', file)
    return fetch(BASE + '/api/importConfig', {
      method: 'POST',
      body: form,
      headers: { Authorization: `Bearer ${getToken()}` },
    })
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