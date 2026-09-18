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
  Alert,
  Tooltip,
  Popconfirm,
} from 'antd'
import { ReloadOutlined, ClearOutlined } from '@ant-design/icons'
import type { ColumnsType } from 'antd/es/table'
import { api } from '../api/client'
import type { LogEntry, NotificationDelivery } from '../types'

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
    refetchInterval: 15_000,
  })
  const { data: status, error: statusError } = useQuery({
    queryKey: ['status'],
    queryFn: api.getStatus,
    refetchInterval: 15_000,
  })

  const { data: deliveries = [], error: deliveryError } = useQuery({ queryKey: ['notifications'], queryFn: api.notifications, refetchInterval: q => q.state.data?.some(n => ['queued', 'sending'].includes(n.state)) ? 3000 : 15_000 })
  const [testingAI, setTestingAI] = useState(false)
  const [retrying, setRetrying] = useState<string | null>(null)
  const testAI = async () => {
    setTestingAI(true)
    try { await api.aiPing(); antMessage.success('AI 连接测试成功') }
    catch (e) { antMessage.error((e as Error).message) }
    finally { setTestingAI(false); await qc.invalidateQueries({ queryKey: ['status'] }) }
  }
  const retryDelivery = async (id: string) => {
    setRetrying(id)
    try { await api.retryNotification(id); await qc.invalidateQueries({ queryKey: ['notifications'] }); antMessage.success('已加入补发队列') }
    catch (e) { antMessage.error((e as Error).message) }
    finally { setRetrying(null) }
  }
  const checked = (at?: number) => at ? `最近检查：${new Date(at).toLocaleString()}` : '尚未检查'
  const aiLabel = !status ? '加载中' : status.ai.enabled === false ? '已停用' : !status.ai.configured ? '未配置' : !status.ai.checkedAt ? '尚未检查' : status.ai.ok ? '最近请求成功' : '最近请求失败'
  const cloudLabel = !status ? '加载中' : !status.cloud.configured ? '未配置' : !status.cloud.checkedAt ? '尚未检查' : status.cloud.loginOK ? '最近登录成功' : '最近登录失败'

  // 级别筛选
  const filtered = logs?.filter((l) => level === '全部' || l.level === level) ?? []

  const handleClear = async () => {
    try {
      await api.clearLogs()
      await qc.invalidateQueries({ queryKey: ['logs'] })
      antMessage.success('已清空日志')
    } catch (e) { antMessage.error((e as Error).message) }
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
      {statusError && <Alert type="error" title={statusError.message} />}
      {/* 状态卡片 */}
      <Row gutter={16} style={{ marginBottom: 16 }}>
        <Col xs={12} sm={8} md={6}>
          <Card size="small">
            <Statistic
              title="AI 服务"
              value={aiLabel}
              valueStyle={{ color: status?.ai.ok ? '#52c41a' : status?.ai.checkedAt ? '#ff4d4f' : '#999' }}
            />
            <Tooltip title={status?.ai.message}><Typography.Text type="secondary">{checked(status?.ai.checkedAt)}</Typography.Text></Tooltip>
            <Button size="small" onClick={testAI} loading={testingAI}>测试 AI 连接</Button>
          </Card>
        </Col>
        <Col xs={12} sm={8} md={6}>
          <Card size="small">
            <Statistic
              title={status?.cloudName === 'pikpak' ? 'PikPak 网盘' : '115 网盘'}
              value={cloudLabel}
              valueStyle={{ color: status?.cloud.loginOK ? '#52c41a' : status?.cloud.checkedAt ? '#ff4d4f' : '#999' }}
            />
            <Tooltip title={status?.cloud.message}><Typography.Text type="secondary">{checked(status?.cloud.checkedAt)}</Typography.Text></Tooltip>
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
      <Typography.Title level={5}>通知发送记录</Typography.Title>
      {deliveryError && <Alert type="error" title={deliveryError.message} />}
      <Table<NotificationDelivery> size="small" rowKey="id" dataSource={deliveries} pagination={{ pageSize: 10, showSizeChanger: false }} columns={[
        { title: '渠道', dataIndex: 'channel' }, { title: '番剧', dataIndex: 'title' },
        { title: '状态', dataIndex: 'state', render: (state: string) => ({ queued: '排队中', sending: '发送中', sent: '已发送', failed: '发送失败', cancelled: '已取消' })[state] },
        { title: '尝试次数', dataIndex: 'attempts' }, { title: '失败原因', dataIndex: 'error' },
        { title: '操作', render: (_, item) => ['failed', 'cancelled'].includes(item.state) && <Popconfirm title="补发这条通知？" description="若上次请求结果不明确，补发可能产生重复消息。" onConfirm={() => retryDelivery(item.id)}><Button size="small" loading={retrying === item.id}>补发</Button></Popconfirm> },
      ]} />
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
