import { useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import {
  Typography,
  Table,
  Tag,
  Button,
  Space,
  Segmented,
  App,
  Card,
  Row,
  Col,
  Statistic,
} from 'antd'
import { ReloadOutlined, ClearOutlined } from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import { api } from '../api/client'
import type { LogEntry } from '../types'

const levelColor: Record<string, string> = {
  DEBUG: 'default',
  INFO: 'blue',
  WARN: 'orange',
  ERROR: 'red',
}

export default function LogsPage() {
  const { message: antMessage } = App.useApp()
  const qc = useQueryClient()
  const [level, setLevel] = useState<string>('全部')
  const { data: logs, refetch, isFetching } = useQuery({
    queryKey: ['logs'],
    queryFn: api.getLogs,
    refetchInterval: 3000, // 每 3 秒自动刷新
  })
  const { data: status } = useQuery({
    queryKey: ['status'],
    queryFn: api.getStatus,
    refetchInterval: 3000,
  })

  // 级别筛选
  const filtered = logs?.filter((l) => level === '全部' || l.level === level) ?? []

  const handleClear = async () => {
    await api.clearLogs()
    await qc.invalidateQueries({ queryKey: ['logs'] })
    antMessage.success('已清空日志')
  }

  const columns: ColumnsType<LogEntry> = [
    {
      title: '时间',
      dataIndex: 'threadName',
      width: 90,
    },
    {
      title: '级别',
      dataIndex: 'level',
      width: 90,
      render: (v: string) => <Tag color={levelColor[v] ?? 'default'}>{v}</Tag>,
    },
    {
      title: '来源',
      dataIndex: 'loggerName',
      width: 120,
    },
    {
      title: '消息',
      dataIndex: 'message',
      ellipsis: true,
    },
  ]

  const uptime = status?.uptimeSeconds ?? 0
  const uptimeStr = `${Math.floor(uptime / 3600)}h ${Math.floor((uptime % 3600) / 60)}m`

  return (
    <div>
      {/* 状态卡片 */}
      <Row gutter={16} style={{ marginBottom: 16 }}>
        <Col xs={12} sm={8} md={6}>
          <Card size="small">
            <Statistic
              title="AI 服务"
              value={status?.ai.ok ? '已连接' : status?.ai.configured ? '连接失败' : '未配置'}
              valueStyle={{ color: status?.ai.ok ? '#52c41a' : status?.ai.configured ? '#ff4d4f' : '#999' }}
            />
          </Card>
        </Col>
        <Col xs={12} sm={8} md={6}>
          <Card size="small">
            <Statistic
              title="115 网盘"
              value={status?.cloud.loginOK ? '已连接' : status?.cloud.configured ? '登录失败' : '未配置'}
              valueStyle={{ color: status?.cloud.loginOK ? '#52c41a' : status?.cloud.configured ? '#ff4d4f' : '#999' }}
            />
          </Card>
        </Col>
        <Col xs={12} sm={8} md={6}>
          <Card size="small">
            <Statistic
              title="内存占用"
              value={status?.memory.allocMB.toFixed(1) ?? '-'}
              suffix="MB"
            />
          </Card>
        </Col>
        <Col xs={12} sm={8} md={6}>
          <Card size="small">
            <Statistic
              title="缓存占用"
              value={status?.cache.count ?? 0}
              suffix={`条 / ${status?.cache.sizeKB.toFixed(0) ?? 0}KB`}
            />
          </Card>
        </Col>
      </Row>

      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 16 }}>
        <Typography.Title level={4} style={{ margin: 0 }}>
          日志 (运行 {uptimeStr})
        </Typography.Title>
        <Space>
          <Segmented
            options={['全部', 'DEBUG', 'INFO', 'WARN', 'ERROR']}
            value={level}
            onChange={(v) => setLevel(v as string)}
          />
          <Button icon={<ReloadOutlined />} onClick={() => refetch()} loading={isFetching}>
            刷新
          </Button>
          <Button danger icon={<ClearOutlined />} onClick={handleClear}>
            清空
          </Button>
        </Space>
      </div>
      <Table<LogEntry>
        rowKey={(r) => r.threadName + r.message}
        size="small"
        columns={columns}
        dataSource={filtered}
        loading={isFetching}
        pagination={{ pageSize: 50, showSizeChanger: false }}
      />
    </div>
  )
}