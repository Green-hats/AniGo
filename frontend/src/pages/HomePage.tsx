import { useEffect, useRef, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import {
  Card,
  Collapse,
  Tag,
  Button,
  Space,
  Tooltip,
  Typography,
  message,
  Popconfirm,
  Modal,
  Empty,
  Skeleton,
  Alert,
  Input,
  Select,
  Checkbox,
  Table,
} from 'antd'
import {
  DeleteOutlined,
  SyncOutlined,
  PlayCircleOutlined,
  PlaySquareOutlined,
} from '@ant-design/icons'
import { api } from '../api/client'
import type { Ani, PlayItem, DownloadTask } from '../types'

const { Text } = Typography
const isActive = (state: string) => ['waiting', 'queued', 'running'].includes(state)
const taskLabels: Record<string, string> = { pending: '准备提交', submitted: '云端处理中', completed: '已完成', failed: '下载失败', exhausted: '重试已耗尽', unknown: '待确认', abandoned: '已换源' }


// URL-safe base64（mpv-handler 协议要求）。
// btoa 只支持 Latin-1，中文文件名必须先用 TextEncoder 转成 UTF-8 字节再编码。
const b64u = (s: string) => {
  const bytes = new TextEncoder().encode(s)
  let bin = ''
  for (const b of bytes) bin += String.fromCharCode(b)
  return btoa(bin).replace(/\//g, '_').replace(/\+/g, '-').replace(/=/g, '')
}

export default function HomePage() {
  const qc = useQueryClient()
  const { data: jobs = [] } = useQuery({ queryKey: ['refreshStatus'], queryFn: api.refreshStatus, refetchInterval: q => q.state.data?.some(j => isActive(j.state)) ? 2000 : 15_000 })
  const { data, refetch, isPending, error } = useQuery({ queryKey: ['listAni'], queryFn: api.listAni, refetchInterval: jobs.some(j => isActive(j.state)) ? 5000 : 60_000 })
  const [search, setSearch] = useState('')
  const [filter, setFilter] = useState('all')
  const [selected, setSelected] = useState<string[]>([])
  const [bulkBusy, setBulkBusy] = useState(false)
  const [historyAni, setHistoryAni] = useState<Ani | null>(null)
  const [historyPage, setHistoryPage] = useState(1)
  const history = useQuery({ queryKey: ['taskHistory', historyAni?.id, historyPage], queryFn: () => api.taskHistory(historyAni!.id, historyPage), enabled: !!historyAni })
  const all = data?.weekList.flatMap(w => w.items) ?? []
  const failed = (a: Ani) => a.downloadTasks?.some(t => ['failed', 'exhausted', 'unknown'].includes(t.state)) ?? false
  const visible = (a: Ani) => `${a.title} ${a.jpTitle ?? ''} ${a.subgroup ?? ''}`.toLowerCase().includes(search.toLowerCase().trim()) && (filter === 'all' || (filter === 'failed' ? failed(a) : filter === 'enabled' ? a.enable : !a.enable))
  const weeks = data?.weekList.map(w => ({ ...w, items: w.items.filter(visible) })).filter(w => w.items.length > 0) ?? []
  const visibleIDs = weeks.flatMap(w => w.items.map(a => a.id))
  const picked = all.filter(a => selected.includes(a.id))
  const handleBulk = async (action: 'enable' | 'disable' | 'refresh' | 'delete') => {
    setBulkBusy(true)
    try {
      const ids = picked.map(a => a.id)
      if (action === 'refresh') await api.refreshBatch(picked.filter(a => a.enable).map(a => a.id))
      else if (action === 'delete') await api.deleteAni(ids)
      else await api.batchEnable(ids, action === 'enable')
      setSelected([])
      message.success(action === 'refresh' ? '所选启用订阅已加入刷新队列' : '批量操作完成')
      await Promise.all([refetch(), qc.invalidateQueries({ queryKey: ['refreshStatus'] }), qc.invalidateQueries({ queryKey: ['gardenList'] })])
    } catch (e) { message.error((e as Error).message) }
    finally { setBulkBusy(false) }
  }
  const completed = jobs.filter(j => j.state !== 'waiting' && j.state !== 'queued' && j.state !== 'running').map(j => `${j.id}:${j.updatedAt}`).join('|')
  useEffect(() => { if (completed) void qc.invalidateQueries({ queryKey: ['listAni'] }) }, [completed, qc])
  const active = (id?: string) => jobs.some(j => (!id || j.id === id) && (j.state === 'waiting' || j.state === 'queued' || j.state === 'running'))
  const [refreshAllPending, setRefreshAllPending] = useState(false)
  const [launching, setLaunching] = useState<string | null>(null)
  const playRequest = useRef(0)
  const [refreshing, setRefreshing] = useState<string | null>(null)
  const [playAni, setPlayAni] = useState<Ani | null>(null)
  const [playItems, setPlayItems] = useState<PlayItem[] | null>(null)
  const [playLoading, setPlayLoading] = useState(false)
  const [recovering, setRecovering] = useState<string | null>(null)
  const handleRecover = async (ani: Ani, task: DownloadTask, action: 'retry' | 'replace') => {
    const key = `${ani.id}:${task.episode}:${task.hash}`
    setRecovering(key)
    try {
      await api.recoverTask(ani.id, task.hash, task.episode, action)
      message.success(action === 'retry' ? '已重置重试次数；刷新后继续下载' : '已跳过此资源；刷新时寻找其他版本')
      await Promise.all([refetch(), qc.invalidateQueries({ queryKey: ['refreshStatus'] })])
    } catch (e) { message.error((e as Error).message) }
    finally { setRecovering(null) }
  }

  const handleDelete = async (id: string) => {
    try {
      await api.deleteAni([id])
      message.success('已删除')
      await qc.invalidateQueries({ queryKey: ['gardenList'] })
      await refetch()
    } catch (e) { message.error((e as Error).message) }
  }

  const handleRefresh = async (id: string) => {
    setRefreshing(id)
    try {
      await api.refreshAni(id)
      message.success('刷新任务已加入队列')
      await qc.invalidateQueries({ queryKey: ['refreshStatus'] })
    } catch (e) { message.error((e as Error).message) } finally {
      setRefreshing(null)
    }
  }

  const handleToggle = async (ani: Ani) => {
    try { await api.batchEnable([ani.id], !ani.enable); await refetch() }
    catch (e) { message.error((e as Error).message) }
  }

  const handleRefreshAll = async () => {
    setRefreshAllPending(true)
    try {
      await api.refreshAll()
      message.success('刷新任务已加入队列')
      await qc.invalidateQueries({ queryKey: ['refreshStatus'] })
    } catch (e) { message.error((e as Error).message) }
    finally { setRefreshAllPending(false) }
  }

  const handlePlay = async (ani: Ani) => {
    const request = ++playRequest.current
    setPlayAni(ani)
    setPlayItems(null)
    setPlayLoading(true)
    try {
      const items = await api.playList(ani.id)
      if (request === playRequest.current) setPlayItems(items)
    } catch (e) {
      if (request === playRequest.current) { message.error((e as Error).message); setPlayAni(null) }
    } finally {
      if (request === playRequest.current) setPlayLoading(false)
    }
  }

  const handleLaunch = async (item: PlayItem) => {
    if (!playAni) return
    setLaunching(item.pickCode)
    try {
      const ticket = await api.playTicket(playAni.id, item.pickCode)
      const proxyUrl = new URL(ticket.url, window.location.origin).href
      window.location.assign(`mpv-handler://play/${b64u(proxyUrl)}/?v_title=${b64u(item.filename)}`)
    } catch (e) { message.error((e as Error).message) }
    finally { setLaunching(null) }
  }

  const sortedPlayItems = playItems ? [...playItems].sort((a, b) => a.episode - b.episode || a.filename.localeCompare(b.filename)) : []

  return (
    <div>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 16 }}>
        <Typography.Title level={4} style={{ margin: 0 }}>
          我的订阅 ({data?.total ?? 0})
        </Typography.Title>
        <Button icon={<SyncOutlined />} onClick={handleRefreshAll} loading={refreshAllPending || active()}>
          刷新全部
        </Button>
      </div>

      <Space wrap style={{ marginBottom: 16 }}>
        <Input.Search aria-label="搜索订阅" placeholder="搜索番剧、字幕组" allowClear value={search} onChange={e => { setSearch(e.target.value); setSelected([]) }} style={{ width: 260 }} />
        <Select aria-label="订阅筛选" value={filter} onChange={v => { setFilter(v); setSelected([]) }} style={{ width: 140 }} options={[{ value: 'all', label: '全部订阅' }, { value: 'failed', label: '失败 / 待确认' }, { value: 'enabled', label: '已启用' }, { value: 'disabled', label: '已停用' }]} />
        <Checkbox checked={visibleIDs.length > 0 && visibleIDs.every(id => selected.includes(id))} indeterminate={picked.length > 0 && !visibleIDs.every(id => selected.includes(id))} onChange={e => setSelected(e.target.checked ? visibleIDs : [])}>选择当前结果</Checkbox>
        <Text>已选 {picked.length} 项</Text>
        <Button disabled={!picked.some(a => a.enable) || bulkBusy} onClick={() => handleBulk('refresh')}>批量刷新</Button>
        <Button disabled={!picked.length || bulkBusy} onClick={() => handleBulk('enable')}>批量启用</Button>
        <Button disabled={!picked.length || bulkBusy} onClick={() => handleBulk('disable')}>批量停用</Button>
        <Popconfirm title={`删除所选 ${picked.length} 个订阅？`} onConfirm={() => handleBulk('delete')}><Button danger disabled={!picked.length || bulkBusy}>批量删除</Button></Popconfirm>
      </Space>
      {data && data.total > 0 && visibleIDs.length === 0 && <Empty description="没有符合条件的订阅" />}
      {error && <Alert type="error" title="订阅加载失败" description={error.message} action={<Button onClick={() => refetch()}>重试</Button>} />}
      {isPending && <Skeleton active />}
      {data?.total === 0 && <Empty description="还没有订阅，去番剧源添加吧" />}
      {!data ? null : (
      <Collapse
        defaultActiveKey={data.weekList.map(w => w.weekLabel)}
        items={weeks.map((week) => ({
          key: week.weekLabel,
          label: `${week.weekLabel} (${week.items.length})`,
          children: (
            <Space orientation="vertical" style={{ width: '100%' }} size="small">
              {week.items.length === 0 && <Text type="secondary">暂无订阅</Text>}
              {week.items.map((ani) => (
                <Card key={ani.id} size="small" styles={{ body: { padding: 12 } }}>
                  <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
                    <Checkbox aria-label={`选择 ${ani.title}`} checked={selected.includes(ani.id)} onChange={e => setSelected(prev => e.target.checked ? [...prev, ani.id] : prev.filter(id => id !== ani.id))} />
                    {ani.image ? (
                      <img src={ani.image} alt="" style={{ width: 40, height: 56, objectFit: 'cover', borderRadius: 4 }} />
                    ) : (
                      <div style={{ width: 40, height: 56, background: '#f0f0f0', borderRadius: 4 }} />
                    )}
                    <div style={{ flex: 1, minWidth: 0 }}>
                      <div>
                        <Text strong>{ani.title}</Text>
                        {ani.season > 1 && <Tag style={{ marginLeft: 8 }}>第{ani.season}季</Tag>}
                        <Tag color={ani.enable ? 'green' : 'default'} style={{ marginLeft: 4 }}>
                          {ani.enable ? '启用' : '停用'}
                        </Tag>
                        {ani.score > 0 && <Tag color="gold">{ani.score.toFixed(1)}</Tag>}
                      </div>
                      <Text type="secondary" style={{ fontSize: 12 }}>
                        已完成 {ani.downloadedEps ?? 0} / 更新 {ani.bgmAiredEps || ani.totalEpisodeNumber || '?'} / 共 {ani.totalEpisodeNumber || '?'} 集
                        {ani.subgroup && ` · ${ani.subgroup}`}
                      </Text>
                      {ani.downloadTasks?.some(t => t.state === 'submitted' || t.state === 'pending') && <Tag color="processing">云端处理中</Tag>}
                      {ani.downloadTasks?.filter(t => ['failed', 'exhausted', 'unknown', 'abandoned'].includes(t.state)).map(task => (
                        <div key={`${task.accountId}:${task.episode}:${task.hash}`} style={{ marginTop: 4 }}>
                          <Tooltip title={task.error}>
                            <Tag color={task.state === 'unknown' || task.state === 'abandoned' ? 'warning' : 'error'}>
                              第 {task.episode} 集 · {{ failed: '下载失败', exhausted: '重试已耗尽', unknown: '待确认', abandoned: '等待其他资源', pending: '', submitted: '', completed: '' }[task.state]}
                            </Tag>
                          </Tooltip>
                          {task.state === 'unknown' && <Text type="secondary">请刷新查询云端状态</Text>}
                          {(task.state === 'failed' || task.state === 'exhausted') && <Space size="small">
                            <Button size="small" disabled={!ani.enable || active(ani.id) || recovering !== null} onClick={() => handleRecover(ani, task, 'retry')}>重试第 {task.episode} 集</Button>
                            <Popconfirm title="跳过此资源并寻找其他版本？" description="没有其他版本时会等待 RSS 更新。" onConfirm={() => handleRecover(ani, task, 'replace')}>
                              <Button size="small" disabled={!ani.enable || active(ani.id) || recovering !== null}>换源</Button>
                            </Popconfirm>
                          </Space>}
                        </div>
                      ))}
                      {jobs.find(j => j.id === ani.id)?.state === 'failed' && <Text type="danger" style={{ display: 'block' }}>{jobs.find(j => j.id === ani.id)?.error}</Text>}
                    </div>
                    <Space wrap>
                      <Button size="small" onClick={() => { setHistoryAni(ani); setHistoryPage(1) }}>任务记录</Button>
                      <Tooltip title="播放">
                        <Button size="small" icon={<PlayCircleOutlined />} onClick={() => handlePlay(ani)} />
                      </Tooltip>
                      <Tooltip title={ani.enable ? '停用' : '启用'}>
                        <Button size="small" onClick={() => handleToggle(ani)}>
                          {ani.enable ? '停用' : '启用'}
                        </Button>
                      </Tooltip>
                      <Tooltip title="刷新">
                        <Button size="small" icon={<SyncOutlined />} disabled={!ani.enable} loading={refreshing === ani.id || active(ani.id)} onClick={() => handleRefresh(ani.id)} />
                      </Tooltip>
                      <Popconfirm title="删除该订阅？" onConfirm={() => handleDelete(ani.id)}>
                        <Button size="small" danger icon={<DeleteOutlined />} />
                      </Popconfirm>
                    </Space>
                  </div>
                </Card>
              ))}
            </Space>
          ),
        }))}
      />
      )}
      <Modal title={`${historyAni?.title ?? ''} · 任务记录`} open={!!historyAni} footer={null} onCancel={() => setHistoryAni(null)}>
        {history.error && <Alert type="error" title={history.error.message} action={<Button onClick={() => history.refetch()}>重试</Button>} />}
        <Table<DownloadTask> size="small" rowKey={t => `${t.accountId}:${t.episode}:${t.hash}`} loading={history.isFetching} dataSource={history.data?.items ?? []} pagination={{ current: historyPage, total: history.data?.total ?? 0, pageSize: 20, showSizeChanger: false, onChange: setHistoryPage }} columns={[
          { title: '集数', dataIndex: 'episode' },
          { title: '状态', dataIndex: 'state', render: (v: string) => taskLabels[v] ?? v },
          { title: '尝试次数', dataIndex: 'attempts' },
          { title: '详情', dataIndex: 'error' },
        ]} />
      </Modal>
      <Modal
        open={!!playAni}
        onCancel={() => { ++playRequest.current; setPlayAni(null) }}
        footer={null}
        width={620}
        title={
          <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
            {playAni?.image ? (
              <img
                src={playAni.image}
                alt=""
                style={{ width: 40, height: 56, objectFit: 'cover', borderRadius: 6 }}
              />
            ) : (
              <div style={{ width: 40, height: 56, background: '#f0f0f0', borderRadius: 6 }} />
            )}
            <div style={{ minWidth: 0 }}>
              <Text strong style={{ fontSize: 16, display: 'block' }}>
                {playAni?.title ?? ''}
              </Text>
              <Text type="secondary" style={{ fontSize: 12 }}>
                选集播放 · 共 {sortedPlayItems.length} 集
              </Text>
            </div>
          </div>
        }
      >
        {!playItems ? (
          playLoading ? (
            <div style={{ padding: 8 }}>
              <Skeleton active paragraph={{ rows: 5 }} />
            </div>
          ) : (
            <Empty description="无播放文件" />
          )
        ) : sortedPlayItems.length === 0 ? (
          <Empty description="无播放文件" />
        ) : (
          <div
            style={{
              maxHeight: 420,
              overflowY: 'auto',
              display: 'flex',
              flexDirection: 'column',
              gap: 8,
              paddingRight: 4,
            }}
          >
            {sortedPlayItems.map((item) => (
              <div
                key={item.pickCode}
                style={{
                  display: 'flex',
                  alignItems: 'center',
                  gap: 12,
                  padding: '10px 12px',
                  borderRadius: 8,
                  border: '1px solid #f0f0f0',
                  background: '#fff',
                  transition: 'all 0.2s',
                  cursor: 'pointer',
                }}
                onMouseEnter={(e) => {
                  e.currentTarget.style.borderColor = '#1677ff'
                  e.currentTarget.style.background = '#f5f9ff'
                }}
                onMouseLeave={(e) => {
                  e.currentTarget.style.borderColor = '#f0f0f0'
                  e.currentTarget.style.background = '#fff'
                }}
              >
                <div
                  style={{
                    width: 40,
                    height: 40,
                    flexShrink: 0,
                    borderRadius: 8,
                    display: 'flex',
                    alignItems: 'center',
                    justifyContent: 'center',
                    fontSize: 15,
                    fontWeight: 600,
                    color: item.episode > 0 ? '#1677ff' : '#999',
                    background: item.episode > 0 ? '#e6f4ff' : '#f5f5f5',
                  }}
                >
                  {item.episode > 0 ? item.episode : '?'}
                </div>
                <div style={{ flex: 1, minWidth: 0 }}>
                  <Text style={{ fontSize: 13, display: 'block' }} ellipsis>
                    {item.filename}
                  </Text>
                  {item.episode > 0 && (
                    <Text type="secondary" style={{ fontSize: 12 }}>
                      第 {item.episode} 集
                    </Text>
                  )}
                </div>
                <Button type="primary" size="small" icon={<PlaySquareOutlined />} loading={launching === item.pickCode} onClick={() => handleLaunch(item)}>
                  用 mpv 播放
                </Button>
              </div>
            ))}
          </div>
        )}
      </Modal>
    </div>
  )
}