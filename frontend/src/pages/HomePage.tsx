import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Card, Collapse, Tag, Button, Space, Tooltip, Typography, message, Popconfirm, Modal, List } from 'antd'
import {
  DeleteOutlined,
  SyncOutlined,
  PlayCircleOutlined,
} from '@ant-design/icons'
import { api } from '../api/client'
import type { Ani, PlayItem } from '../types'

const { Text } = Typography

// URL-safe base64（mpv-handler 协议要求）
const b64u = (s: string) => btoa(s).replace(/\//g, '_').replace(/\+/g, '-').replace(/=/g, '')

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
        title={playAni ? `${playAni.title} 选集播放` : '播放'}
        open={!!playAni}
        onCancel={() => setPlayAni(null)}
        footer={null}
        width={560}
      >
        {!playItems ? (
          <div style={{ textAlign: 'center', padding: 24, color: '#999' }}>
            {playLoading ? '正在获取云端文件…' : '无播放文件'}
          </div>
        ) : (
          <List
            size="small"
            dataSource={sortedPlayItems}
            renderItem={(item) => (
              <List.Item
                actions={[
                  <a key="play" href={buildMpvUrl(item)} style={{ color: '#1677ff' }}>
                    mpv 播放
                  </a>,
                ]}
              >
                <Text style={{ fontSize: 13 }}>
                  {item.episode > 0 ? `第 ${item.episode} 集` : '未知集'} · {item.filename}
                </Text>
              </List.Item>
            )}
          />
        )}
      </Modal>
    </div>
  )
}