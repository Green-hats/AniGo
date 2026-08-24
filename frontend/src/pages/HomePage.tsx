import { useQuery } from '@tanstack/react-query'
import { Card, Collapse, Tag, Button, Space, Tooltip, Typography, message, Popconfirm } from 'antd'
import {
  DeleteOutlined,
  SyncOutlined,
} from '@ant-design/icons'
import { api } from '../api/client'
import type { Ani } from '../types'

const { Text } = Typography

export default function HomePage() {
  const { data, refetch, isLoading } = useQuery({ queryKey: ['listAni'], queryFn: api.listAni })

  const handleDelete = async (id: string) => {
    await api.deleteAni([id])
    message.success('已删除')
    refetch()
  }

  const handleRefresh = async (id: string) => {
    await api.refreshAni(id)
    message.success('已开始刷新')
  }

  const handleToggle = async (ani: Ani) => {
    await api.batchEnable([ani.id], !ani.enable)
    refetch()
  }

  const handleRefreshAll = async () => {
    await api.refreshAll()
    message.success('已开始刷新全部')
  }

  return (
    <div>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 16 }}>
        <Typography.Title level={4} style={{ margin: 0 }}>
          我的订阅 ({data?.total ?? 0})
        </Typography.Title>
        <Button icon={<SyncOutlined />} onClick={handleRefreshAll} loading={isLoading}>
          刷新全部
        </Button>
      </div>

      <Collapse
        defaultActiveKey={data?.weekList?.map((_, i) => String(i))}
        items={data?.weekList?.map((week, i) => ({
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
                        已下载 {ani.currentEpisodeNumber} / 更新 {ani.bgmAiredEps || ani.currentEpisodeNumber} / 共 {ani.totalEpisodeNumber || '?'} 集
                        {ani.subgroup && ` · ${ani.subgroup}`}
                      </Text>
                    </div>
                    <Space>
                      <Tooltip title={ani.enable ? '停用' : '启用'}>
                        <Button size="small" onClick={() => handleToggle(ani)}>
                          {ani.enable ? '停用' : '启用'}
                        </Button>
                      </Tooltip>
                      <Tooltip title="刷新">
                        <Button size="small" icon={<SyncOutlined />} onClick={() => handleRefresh(ani.id)} />
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
    </div>
  )
}