import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { api } from './client'

describe('api client', () => {
  const mockFetch = vi.fn()

  beforeEach(() => {
    vi.stubGlobal('fetch', mockFetch)
  })

  afterEach(() => {
    vi.unstubAllGlobals()
    mockFetch.mockReset()
  })

  const jsonResp = (body: unknown) => ({ json: async () => body })

  it('POST 请求序列化 body 并返回 data', async () => {
    mockFetch.mockResolvedValue(
      jsonResp({ code: 200, message: '', data: null, t: 0 }),
    )
    const data = await api.addAni({ title: '测试番剧' })
    expect(data).toBeNull()

    const [url, opts] = mockFetch.mock.calls[0]
    expect(url).toBe('/api/addAni')
    expect(opts.method).toBe('POST')
    expect(opts.headers['Content-Type']).toBe('application/json')
    expect(JSON.parse(opts.body)).toEqual({ title: '测试番剧' })
  })

  it('GET 请求不带 body', async () => {
    mockFetch.mockResolvedValue(jsonResp({ code: 200, message: '', data: null, t: 0 }))
    await api.ping()

    const [url, opts] = mockFetch.mock.calls[0]
    expect(url).toBe('/api/ping')
    expect(opts.method).toBe('GET')
    expect(opts.body).toBeUndefined()
  })

  it('返回 data 字段', async () => {
    const listData = { releaseDateList: ['周一'], weekList: [], total: 1 }
    mockFetch.mockResolvedValue(jsonResp({ code: 200, message: '', data: listData, t: 0 }))
    const data = await api.listAni()
    expect(data).toEqual(listData)
  })

  it('code !== 200 时抛错并带出后端消息', async () => {
    mockFetch.mockResolvedValue(jsonResp({ code: 500, message: '内部错误', data: null, t: 0 }))
    await expect(api.ping()).rejects.toThrow('内部错误')
  })

  it('带 token 时注入 Authorization 头', async () => {
    localStorage.setItem('anigo_token', 'tok123')
    mockFetch.mockResolvedValue(jsonResp({ code: 200, message: '', data: null, t: 0 }))
    await api.ping()

    const [, opts] = mockFetch.mock.calls[0]
    expect(opts.headers['Authorization']).toBe('Bearer tok123')
  })

  it('401 时清空 token 并抛错', async () => {
    localStorage.setItem('anigo_token', 'tok123')
    mockFetch.mockResolvedValue(jsonResp({ code: 401, message: '未登录或登录已过期', data: null, t: 0 }))
    await expect(api.getConfig()).rejects.toThrow('未登录或登录已过期')
    expect(localStorage.getItem('anigo_token')).toBeNull()
  })

  it('login 返回 token', async () => {
    mockFetch.mockResolvedValue(
      jsonResp({ code: 200, message: '', data: { login: true, token: 'abc' }, t: 0 }),
    )
    const res = await api.login('admin', 'admin')
    expect(res.token).toBe('abc')
  })

  it('导入配置使用 FormData 而非 JSON', async () => {
    mockFetch.mockResolvedValue(new Response(null, { status: 200 }))
    const file = new File(['{}'], 'config.v2.json', { type: 'application/json' })
    await api.importConfig(file)

    const [url, opts] = mockFetch.mock.calls[0]
    expect(url).toBe('/api/importConfig')
    expect(opts.method).toBe('POST')
    expect(opts.body).toBeInstanceOf(FormData)
  })

  it('query 参数正确拼接', async () => {
    mockFetch.mockResolvedValue(jsonResp({ code: 200, message: '', data: null, t: 0 }))
    await api.batchEnable(['id1'], true)
    expect(mockFetch.mock.calls[0][0]).toBe('/api/batchEnable?value=true')
  })

  it('请求超时抛出"请求超时"', async () => {
    vi.useFakeTimers()
    try {
      mockFetch.mockImplementation(
        (_url: string, opts: RequestInit) =>
          new Promise((_resolve, reject) => {
            opts.signal?.addEventListener('abort', () =>
              reject(Object.assign(new Error('aborted'), { name: 'AbortError' })),
            )
          }),
      )
      const p = api.ping()
      vi.advanceTimersByTime(30_001)
      await expect(p).rejects.toThrow('请求超时')
    } finally {
      vi.useRealTimers()
    }
  })
})