import { useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import {
  Collapse,
  Card,
  Tag,
  Button,
  Drawer,
  List,
  Typography,
  message,
  Popconfirm,
} from 'antd'
import { api } from '../api/client'
import type { GardenGroup } from '../types'

const { Text } = Typography

export default function GardenPage() {
  const qc = useQueryClient()
  const [selected, setSelected] = useState<string | null>(null)
  const [selectedGroup, setSelectedGroup] = useState<string | null>(null)
  const [subscribing, setSubscribing] = useState(false)
  const { data: weeks } = useQuery({ queryKey: ['gardenList'], queryFn: api.gardenList })
  const { data: groups, isLoading: groupsLoading } = useQuery({
    queryKey: ['gardenGroup', selected],
    queryFn: () => api.gardenGroup(selected!),
    enabled: !!selected,
  })

  const handleSubscribe = async (group: GardenGroup) => {
    setSubscribing(true)
    try {
      // 1. rssToAni 构建订阅（填充 BGM 元数据）
      const ani = await api.rssToAni({ url: group.rss, type: 'garden' })
      // 2. addAni 真正保存到订阅列表
      await api.addAni(ani)
      // 3. 使首页订阅列表缓存失效（下次访问即刷新）
      await qc.invalidateQueries({ queryKey: ['listAni'] })
      // 4. 刷新番剧源的"已订阅"标记
      await qc.invalidateQueries({ queryKey: ['gardenList'] })
      message.success(`已添加订阅：${ani.title}`)
      setSelected(null)
    } catch (e) {
      message.error((e as Error).message)
    } finally {
      setSubscribing(false)
    }
  }

  return (
    <div>
      <Typography.Title level={4} style={{ marginBottom: 16 }}>
        番剧源（animes.garden）
      </Typography.Title>
      {!weeks ? null : (
      <Collapse
        defaultActiveKey={weeks.map((_, i) => String(i))}
        items={weeks.map((week, i) => ({
          key: String(i),
          label: `${week.weekLabel} (${week.subjects.length})`,
          children: (
            <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(180px, 1fr))', gap: 12 }}>
              {week.subjects.map((s) => (
                <Card key={s.id} size="small" hoverable onClick={() => setSelected(s.id)}>
                  <Text>{s.name}</Text>
                  {s.exists && <Tag color="green" style={{ marginLeft: 6 }}>已订阅</Tag>}
                </Card>
              ))}
            </div>
          ),
        }))}
      />
      )}

      <Drawer
        title="选择字幕组"
        open={!!selected}
        onClose={() => setSelected(null)}
        width={620}
      >
        <List
          loading={groupsLoading}
          dataSource={groups}
          renderItem={(g) => (
            <List.Item
              actions={[
                <Button size="small" onClick={() => setSelectedGroup(g.id)}>
                  查看资源
                </Button>,
                <Popconfirm title={`订阅 ${g.name} 的字幕组？`} onConfirm={() => handleSubscribe(g)}>
                  <Button type="primary" size="small" loading={subscribing}>
                    订阅
                  </Button>
                </Popconfirm>,
              ]}
            >
              <List.Item.Meta
                title={<a onClick={() => setSelectedGroup(g.id)}>{g.name}</a>}
                description={`${g.items?.length ?? 0} 条资源`}
              />
            </List.Item>
          )}
        />
      </Drawer>
      {/* 字幕组详情：点击展开查看资源 */}
      <Drawer
        title={groups?.find((g) => g.id === selectedGroup)?.name ?? '资源列表'}
        open={!!selectedGroup}
        onClose={() => setSelectedGroup(null)}
        width={680}
      >
        <List
          loading={groupsLoading}
          dataSource={groups?.find((g) => g.id === selectedGroup)?.items ?? []}
          renderItem={(it) => (
            <List.Item>
              <List.Item.Meta
                title={<Text style={{ fontSize: 13 }}>{it.title}</Text>}
                description={`大小: ${it.formatSize || '未知'} · ${it.type || ''}`}
              />
            </List.Item>
          )}
        />
      </Drawer>
    </div>
  )
}