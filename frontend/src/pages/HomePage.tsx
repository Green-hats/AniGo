import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
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
} from 'antd'
import {
  DeleteOutlined,
  SyncOutlined,
  PlayCircleOutlined,
  PlaySquareOutlined,
} from '@ant-design/icons'
import { api } from '../api/client'
import type { Ani, PlayItem } from '../types'

const { Text } = Typography

// URL-safe base64（mpv-handler 协议要求）。
// btoa 只支持 Latin-1，中文文件名必须先用 TextEncoder 转成 UTF-8 字节再编码。
const b64u = (s: string) => {
  const bytes = new TextEncoder().encode(s)
  let bin = ''
  for (const b of bytes) bin += String.fromCharCode(b)
  return btoa(bin).replace(/\//g, '_').replace(/\+/g, '-').replace(/=/g, '')
}

export default function HomePage() {
  const { data, refetch, isFetching } = useQuery({ queryKey: ['listAni'], queryFn: api.listAni })
  const [refreshing, setRefreshing] = useState<string | null>(null)
  const [playAni, setPlayAni] = useState<Ani | null>(null)
  const [playItems, setPlayItems] = useState<PlayItem[] | null>(null)
  const [playLoading, setPlayLoading] = useState(false)

  const handleDelete = async (id: string) => {
    await api.deleteAni([id])
    message.success('已删除')
    refetch()
  }

  const handleRefresh = async (id: string) => {
    setRefreshing(id)
    try {
      await api.refreshAni(id)
      message.success('已开始刷新')
      refetch()
    } finally {
      setRefreshing(null)
    }
  }

  const handleToggle = async (ani: Ani) => {
    await api.batchEnable([ani.id], !ani.enable)
    refetch()
  }

  const handleRefreshAll = async () => {
    await api.refreshAll()
    message.success('已开始刷新全部')
    refetch()
  }

  const handlePlay = async (ani: Ani) => {
    setPlayAni(ani)
    setPlayItems(null)
    setPlayLoading(true)
    try {
      const items = await api.playList(ani.id)
      setPlayItems(items)
    } catch (e) {
      message.error((e as Error).message)
      setPlayAni(null)
    } finally {
      setPlayLoading(false)
    }
  }

const buildMpvUrl = (item: PlayItem) => {
    // 走本地代理转发（后端用 115 UA 拉流），mpv 只访问本地服务，规避 115 CDN 的 UA 绑定
    // mpv-handler 协议要求：play/<b64url>/?参数  —— b64url 后必须有 "/"，参数才生效
    const proxyUrl = `${window.location.origin}/api/file?pickcode=${encodeURIComponent(item.pickCode)}`
    return `mpv-handler://play/${b64u(proxyUrl)}/?v_title=${b64u(item.filename)}`
  }

  const sortedPlayItems = playItems ? [...playItems].sort((a, b) => a.episode - b.episode || a.filename.localeCompare(b.filename)) : []

  return (
    <div>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 16 }}>
        <Typography.Title level={4} style={{ margin: 0 }}>
          我的订阅 ({data?.total ?? 0})
        </Typography.Title>
        <Button icon={<SyncOutlined />} onClick={handleRefreshAll} loading={isFetching}>
          刷新全部
        </Button>
      </div>

      {!data ? null : (
      <Collapse
        defaultActiveKey={data.weekList.map((_, i) => String(i))}
        items={data.weekList.map((week, i) => ({
          key: String(i),
          label: `${week.weekLabel} (${week.items.length})`,
          children: (
            <Space direction="vertical" style={{ width: '100%' }} size="small">
              {week.items.length === 0 && <Text type="secondary">暂无订阅</Text>}
              {week.items.map((ani) => (
                <Card key={ani.id} size="small" styles={{ body: { padding: 12 } }}>
                  <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
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
                        已下载 {ani.downloadedEps || ani.currentEpisodeNumber} / 更新 {ani.bgmAiredEps || ani.totalEpisodeNumber || '?'} / 共 {ani.totalEpisodeNumber || '?'} 集
                        {ani.subgroup && ` · ${ani.subgroup}`}
                      </Text>
                    </div>
                    <Space>
                      <Tooltip title="播放">
                        <Button size="small" icon={<PlayCircleOutlined />} onClick={() => handlePlay(ani)} />
                      </Tooltip>
                      <Tooltip title={ani.enable ? '停用' : '启用'}>
                        <Button size="small" onClick={() => handleToggle(ani)}>
                          {ani.enable ? '停用' : '启用'}
                        </Button>
                      </Tooltip>
                      <Tooltip title="刷新">
                        <Button size="small" icon={<SyncOutlined />} loading={refreshing === ani.id} onClick={() => handleRefresh(ani.id)} />
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
      <Modal
        open={!!playAni}
        onCancel={() => setPlayAni(null)}
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
                <a
                  href={buildMpvUrl(item)}
                  style={{ flexShrink: 0 }}
                  onClick={(e) => e.stopPropagation()}
                >
                  <Button type="primary" size="small" icon={<PlaySquareOutlined />}>
                    用 mpv 播放
                  </Button>
                </a>
              </div>
            ))}
          </div>
        )}
      </Modal>
    </div>
  )
}