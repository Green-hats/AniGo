import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
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
  const [selected, setSelected] = useState<string | null>(null)
  const { data: weeks } = useQuery({ queryKey: ['gardenList'], queryFn: api.gardenList })
  const { data: groups, isLoading: groupsLoading } = useQuery({
    queryKey: ['gardenGroup', selected],
    queryFn: () => api.gardenGroup(selected!),
    enabled: !!selected,
  })

  const handleSubscribe = async (group: GardenGroup) => {
    await api.rssToAni({ url: group.rss, type: 'garden' })
    message.success('已添加订阅')
    setSelected(null)
  }

  return (
    <div>
      <Typography.Title level={4} style={{ marginBottom: 16 }}>
        番剧源（animes.garden）
      </Typography.Title>
      <Collapse
        defaultActiveKey={weeks?.map((_, i) => String(i))}
        items={weeks?.map((week, i) => ({
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

      <Drawer
        title="选择字幕组"
        open={!!selected}
        onClose={() => setSelected(null)}
        width={520}
      >
        <List
          loading={groupsLoading}
          dataSource={groups}
          renderItem={(g) => (
            <List.Item
              actions={[
                <Popconfirm title={`订阅 ${g.name} 的字幕组？`} onConfirm={() => handleSubscribe(g)}>
                  <Button type="primary" size="small">订阅</Button>
                </Popconfirm>,
              ]}
            >
              <List.Item.Meta title={g.name} description={`${g.items?.length ?? 0} 条资源`} />
            </List.Item>
          )}
        />
      </Drawer>
    </div>
  )
}