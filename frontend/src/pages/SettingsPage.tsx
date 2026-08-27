import { useEffect, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Tabs, Form, Input, Switch, InputNumber, Button, Divider, message, Space, Select, Card, Upload } from 'antd'
import { DeleteOutlined, PlusOutlined, SendOutlined, DownloadOutlined, UploadOutlined } from '@ant-design/icons'
import { api } from '../api/client'
import type { Config, NotificationConfig } from '../types'

const CHANNEL_TYPES = [
  { value: 'TELEGRAM', label: 'Telegram' },
  { value: 'BARK', label: 'Bark' },
  { value: 'SERVER_CHAN', label: 'ServerChan' },
  { value: 'WEB_HOOK', label: 'WebHook' },
  { value: 'SHELL', label: 'Shell' },
  { value: 'SYSTEM', label: '系统日志' },
]

const STATUS_OPTIONS = [
  { value: 'DOWNLOAD_START', label: '下载开始' },
  { value: 'OMIT', label: '缺集' },
  { value: 'ERROR', label: '错误' },
  { value: 'COMPLETED', label: '订阅完成' },
  { value: 'PROCRASTINATING', label: '摸鱼' },
]

const DEFAULT_CHANNEL: Partial<NotificationConfig> = {
  enable: true,
  retry: 1,
  comment: '',
  notificationType: 'TELEGRAM',
  telegramImage: true,
  statusList: ['DOWNLOAD_START'],
}

function renderChannelFields(name: number, type?: string) {
  switch (type) {
    case 'TELEGRAM':
      return (
        <>
          <Form.Item label="Bot Token" name={[name, 'telegramBotToken']} style={{ marginBottom: 8 }}>
            <Input.Password placeholder="123456:ABC-..." />
          </Form.Item>
          <Form.Item label="Chat ID" name={[name, 'telegramChatId']} style={{ marginBottom: 8 }}>
            <Input placeholder="用户 ID 或 @频道名" />
          </Form.Item>
          <Form.Item label="Topic ID" name={[name, 'telegramTopicId']} style={{ marginBottom: 8 }}>
            <InputNumber min={0} style={{ width: '100%' }} />
          </Form.Item>
          <Form.Item label="API Host" name={[name, 'telegramApiHost']} style={{ marginBottom: 8 }}>
            <Input placeholder="https://api.telegram.org" />
          </Form.Item>
          <Form.Item label="带封面图" name={[name, 'telegramImage']} valuePropName="checked" style={{ marginBottom: 8 }}>
            <Switch />
          </Form.Item>
          <Form.Item label="格式" name={[name, 'telegramFormat']} style={{ marginBottom: 8 }}>
            <Select
              allowClear
              options={[
                { value: 'markdown', label: 'Markdown' },
                { value: 'html', label: 'HTML' },
              ]}
            />
          </Form.Item>
        </>
      )
    case 'BARK':
      return (
        <>
          <Form.Item label="Server URL" name={[name, 'barkServerUrl']} style={{ marginBottom: 8 }}>
            <Input placeholder="https://api.day.app" />
          </Form.Item>
          <Form.Item label="设备 Key" name={[name, 'barkDeviceKeys']} style={{ marginBottom: 8 }}>
            <Input placeholder="多个用逗号分隔" />
          </Form.Item>
          <Form.Item label="分组" name={[name, 'barkGroup']} style={{ marginBottom: 8 }}>
            <Input />
          </Form.Item>
          <Form.Item label="级别" name={[name, 'barkLevel']} style={{ marginBottom: 8 }}>
            <Input />
          </Form.Item>
          <Form.Item label="音量" name={[name, 'barkVolume']} style={{ marginBottom: 8 }}>
            <InputNumber min={0} max={10} style={{ width: '100%' }} />
          </Form.Item>
        </>
      )
    case 'SERVER_CHAN':
      return (
        <>
          <Form.Item label="类型" name={[name, 'serverChanType']} style={{ marginBottom: 8 }}>
            <Select
              options={[
                { value: 'SERVER_CHAN', label: 'ServerChan 2' },
                { value: 'SERVER_CHAN_3', label: 'ServerChan 3' },
              ]}
            />
          </Form.Item>
          <Form.Item label="SendKey" name={[name, 'serverChanSendKey']} style={{ marginBottom: 8 }}>
            <Input.Password />
          </Form.Item>
          <Form.Item label="ServerChan3 API URL" name={[name, 'serverChan3ApiUrl']} style={{ marginBottom: 8 }}>
            <Input placeholder="https://sctapi.ftqq.com" />
          </Form.Item>
        </>
      )
    case 'WEB_HOOK':
      return (
        <>
          <Form.Item label="Method" name={[name, 'webHookMethod']} style={{ marginBottom: 8 }}>
            <Select
              options={[
                { value: 'POST', label: 'POST' },
                { value: 'GET', label: 'GET' },
                { value: 'PUT', label: 'PUT' },
              ]}
            />
          </Form.Item>
          <Form.Item label="URL" name={[name, 'webHookUrl']} style={{ marginBottom: 8 }}>
            <Input placeholder="https://..." />
          </Form.Item>
          <Form.Item label="Header" name={[name, 'webHookHeader']} style={{ marginBottom: 8 }}>
            <Input.TextArea rows={2} placeholder="每行一个：Key: Value" />
          </Form.Item>
          <Form.Item label="Body" name={[name, 'webHookBody']} style={{ marginBottom: 8 }}>
            <Input.TextArea rows={2} placeholder="支持 ${text} 等占位符，以 { 开头会解析为 JSON" />
          </Form.Item>
        </>
      )
    case 'SHELL':
      return (
        <>
          <Form.Item label="Shell 命令" name={[name, 'shell']} style={{ marginBottom: 8 }}>
            <Input.TextArea rows={2} placeholder="环境变量 ANI_RSS_TEXT / ANI_RSS_TITLE" />
          </Form.Item>
        </>
      )
    default:
      return null
  }
}

export default function SettingsPage() {
  const { data: cfg } = useQuery({ queryKey: ['config'], queryFn: api.getConfig })
  const queryClient = useQueryClient()
  const [form] = Form.useForm<Config>()
  const [saving, setSaving] = useState(false)

  useEffect(() => {
    if (cfg) form.setFieldsValue(cfg)
  }, [cfg, form])

  const handleSave = async () => {
    const values = form.getFieldsValue()
    setSaving(true)
    try {
      await api.setConfig(values)
      message.success('已保存')
    } catch (e) {
      message.error((e as Error).message)
    } finally {
      setSaving(false)
    }
  }

  const handleAIPing = async () => {
    try {
      const r = await api.aiPing()
      message.success(`AI 连通成功: ${r.reply}`)
    } catch (e) {
      message.error(`AI 连通失败: ${(e as Error).message}`)
    }
  }

  const handle115Test = async () => {
    try {
      await api.downloadLoginTest(form.getFieldValue('pan115Cookie'))
      message.success('115 登录成功')
    } catch (e) {
      message.error(`115 登录失败: ${(e as Error).message}`)
    }
  }

  const handleExport = async () => {
    try {
      const resp = await api.exportConfig()
      const blob = await resp.blob()
      const url = URL.createObjectURL(blob)
      const a = document.createElement('a')
      a.href = url
      a.download = 'anigo.backup.zip'
      a.click()
      URL.revokeObjectURL(url)
    } catch (e) {
      message.error(`导出失败: ${(e as Error).message}`)
    }
  }

  const handleImport = async (file: File) => {
    setSaving(true)
    try {
      const resp = await api.importConfig(file)
      const json = (await resp.json()) as { code: number; message: string }
      if (json.code !== 200) throw new Error(json.message)
      message.success('导入成功，配置已重新加载')
      await queryClient.invalidateQueries()
    } catch (e) {
      message.error(`导入失败: ${(e as Error).message}`)
    } finally {
      setSaving(false)
    }
  }

  const handleTestChannel = async (index: number) => {
    const ch = form.getFieldsValue(true).notificationConfigList?.[index]
    if (!ch) return
    try {
      await api.testNotification(ch)
      message.success('通知发送成功')
    } catch (e) {
      message.error(`发送失败: ${(e as Error).message}`)
    }
  }

  const handleSecuritySave = async (values: { username: string; password?: string }) => {
    setSaving(true)
    try {
      await api.setConfig({ login: { username: values.username, password: values.password ?? '' } })
      message.success('已保存')
    } catch (e) {
      message.error(`保存失败: ${(e as Error).message}`)
    } finally {
      setSaving(false)
    }
  }

  const notificationList = Form.useWatch('notificationConfigList', form) ?? []

  return (
    <Tabs
      items={[
        {
          key: 'basic',
          label: '基本',
          children: (
            <Form form={form} layout="vertical" style={{ maxWidth: 600 }}>
              <Form.Item label="RSS 刷新间隔（分钟）" name="rssSleepMinutes">
                <InputNumber min={1} max={1440} style={{ width: '100%' }} />
              </Form.Item>
              <Form.Item label="下载路径模板" name="downloadPathTemplate">
                <Input placeholder="番剧/${title}/Season ${season}" />
              </Form.Item>
              <Form.Item label="剧场版下载路径模板" name="ovaDownloadPathTemplate">
                <Input placeholder="剧场版/${title}" />
              </Form.Item>
              <Form.Item label="重命名模板" name="renameTemplate">
                <Input placeholder="${title} S${seasonFormat}E${episodeFormat}" />
              </Form.Item>
              <Form.Item label="排除规则" name="exclude">
                <Select mode="tags" placeholder="输入排除正则后回车" />
              </Form.Item>
              <Divider />
              <Form.Item
                label="BGM 刷新间隔（小时）"
                name="bgmRefreshHours"
                extra="后台定期刷新订阅的评分/总集数/封面，0 使用默认 6 小时。"
              >
                <InputNumber min={0} max={168} style={{ width: '100%' }} />
              </Form.Item>
              <Divider />
              <Button type="primary" onClick={handleSave} loading={saving}>
                保存
              </Button>
            </Form>
          ),
        },
        {
          key: 'ai',
          label: 'AI 解析',
          children: (
            <Form form={form} layout="vertical" style={{ maxWidth: 600 }}>
              <Form.Item label="启用 AI" name="aiEnabled" valuePropName="checked">
                <Switch />
              </Form.Item>
              <Form.Item
                label="仅简体中文字幕"
                name="aiSubtitleSC"
                valuePropName="checked"
                extra="AI 筛选时仅保留含简体中文字幕的资源（简中或简中双语视为满足）。"
              >
                <Switch />
              </Form.Item>
              <Form.Item label="AI 提供方" name="aiProvider">
                <Select
                  options={[
                    { value: 'deepseek', label: 'DeepSeek' },
                    { value: 'openai', label: 'OpenAI' },
                    { value: 'qwen', label: '通义千问' },
                    { value: 'zhipu', label: '智谱' },
                  ]}
                />
              </Form.Item>
              <Form.Item label="API Key" name="aiApiKey">
                <Input.Password />
              </Form.Item>
              <Form.Item label="Base URL" name="aiBaseURL">
                <Input placeholder="https://api.deepseek.com" />
              </Form.Item>
              <Form.Item label="模型" name="aiModel">
                <Input placeholder="deepseek-v4-flash" />
              </Form.Item>
              <Space>
                <Button onClick={handleAIPing}>测试 AI 连通</Button>
                <Button type="primary" onClick={handleSave} loading={saving}>
                  保存
                </Button>
              </Space>
            </Form>
          ),
        },
        {
          key: 'download',
          label: '下载',
          children: (
            <Form form={form} layout="vertical" style={{ maxWidth: 600 }}>
              <Form.Item label="下载工具类型" name="downloadToolType">
                <Select
                  options={[
                    { value: '115', label: '115 网盘' },
                    { value: 'pikpak', label: 'PikPak' },
                  ]}
                />
              </Form.Item>
              <Form.Item label="115 Cookie" name="pan115Cookie">
                <Input.TextArea rows={3} placeholder="UID=...; CID=...; SEID=..." />
              </Form.Item>
              <Form.Item label="下载重试次数" name="downloadRetry">
                <InputNumber min={1} max={10} />
              </Form.Item>
              <Form.Item label="下载超时（分钟）" name="downloadTimeout">
                <InputNumber min={1} />
              </Form.Item>
              <Form.Item label="延迟下载（分钟）" name="delayedDownload">
                <InputNumber min={0} />
              </Form.Item>
              <Form.Item label="自动停订" name="autoDisabled" valuePropName="checked">
                <Switch />
              </Form.Item>
              <Space>
                <Button onClick={handle115Test}>测试 115 登录</Button>
                <Button type="primary" onClick={handleSave} loading={saving}>
                  保存
                </Button>
              </Space>
            </Form>
          ),
        },
        {
          key: 'notify',
          label: '通知',
          children: (
            <Form form={form} layout="vertical" style={{ maxWidth: 600 }}>
              <Form.Item
                label="全局通知模板"
                name="notificationTemplate"
                tooltip="支持 ${text} ${title} ${season} ${episode} ${score} ${subgroup} ${currentEpisodeNumber} ${totalEpisodeNumber} ${year} ${month} ${date} ${bgmUrl} ${emoji} ${action} 等；渠道未填模板时使用这里。"
                style={{ marginBottom: 16 }}
              >
                <Input.TextArea rows={4} placeholder="默认：${emoji}${emoji}${emoji}&#10;事件类型: ${action}&#10;标题: ${title}&#10;...&#10;事件: ${text}&#10;${emoji}${emoji}${emoji}" />
              </Form.Item>
              <Form.List name="notificationConfigList">
                {(fields, { add, remove }) => (
                  <>
                    {fields.map((field) => {
                      const item = notificationList[field.name] ?? {}
                      const type = item.notificationType
                      return (
                        <Card
                          key={field.key}
                          size="small"
                          title={type || '通知渠道'}
                          style={{ marginBottom: 12 }}
                          extra={
                            <Space size="small">
                              <Form.Item name={[field.name, 'enable']} valuePropName="checked" noStyle>
                                <Switch checkedChildren="启用" unCheckedChildren="停用" />
                              </Form.Item>
                              <Button
                                size="small"
                                danger
                                icon={<DeleteOutlined />}
                                onClick={() => remove(field.name)}
                              />
                            </Space>
                          }
                        >
                          <Space direction="vertical" style={{ width: '100%' }}>
                            <Form.Item label="类型" name={[field.name, 'notificationType']} style={{ marginBottom: 8 }}>
                              <Select options={CHANNEL_TYPES} />
                            </Form.Item>
                            <Form.Item label="备注" name={[field.name, 'comment']} style={{ marginBottom: 8 }}>
                              <Input placeholder="方便识别的备注" />
                            </Form.Item>
                            <Form.Item
                              label="触发状态"
                              name={[field.name, 'statusList']}
                              style={{ marginBottom: 8 }}
                              tooltip="勾选哪些状态下触发本渠道"
                            >
                              <Select mode="multiple" options={STATUS_OPTIONS} placeholder="选择触发通知的状态" />
                            </Form.Item>
                            <Space size="large">
                              <Form.Item label="重试次数" name={[field.name, 'retry']} style={{ marginBottom: 8 }}>
                                <InputNumber min={0} max={10} />
                              </Form.Item>
                              <Form.Item label="排序" name={[field.name, 'sort']} style={{ marginBottom: 8 }}>
                                <InputNumber min={0} />
                              </Form.Item>
                            </Space>
                            {renderChannelFields(field.name, type)}
                            <Button size="small" icon={<SendOutlined />} onClick={() => handleTestChannel(field.name)}>
                              测试
                            </Button>
                          </Space>
                        </Card>
                      )
                    })}
                    <Button
                      block
                      icon={<PlusOutlined />}
                      onClick={() =>
                        add({
                          ...DEFAULT_CHANNEL,
                          sort: (notificationList.length ? Math.max(...notificationList.map((c) => c.sort || 0)) : 0) + 1,
                        })
                      }
                    >
                      添加渠道
                    </Button>
                  </>
                )}
              </Form.List>
              <Divider />
              <Button type="primary" onClick={handleSave} loading={saving}>
                保存
              </Button>
            </Form>
          ),
        },
      {
          key: 'security',
          label: '安全',
          children: (
            <Form
              layout="vertical"
              style={{ maxWidth: 600 }}
              onFinish={handleSecuritySave}
              initialValues={{ username: cfg?.login?.username }}
            >
              <Form.Item
                label="登录用户名"
                name="username"
                rules={[{ required: true, message: '请输入用户名' }]}
              >
                <Input placeholder="admin" />
              </Form.Item>
              <Form.Item label="新密码" name="password" extra="留空则不修改密码">
                <Input.Password autoComplete="new-password" />
              </Form.Item>
              <Form.Item
                label="确认新密码"
                name="confirm"
                dependencies={['password']}
                rules={[
                  ({ getFieldValue }) => ({
                    validator: (_, value) =>
                      !value || value === getFieldValue('password')
                        ? Promise.resolve()
                        : Promise.reject(new Error('两次输入的密码不一致')),
                  }),
                ]}
              >
                <Input.Password autoComplete="new-password" />
              </Form.Item>
              <Form.Item label="登录有效期（小时）" name="loginEffectiveHours" extra="会话有效期，默认 3 小时。">
                <InputNumber min={1} max={720} style={{ width: '100%' }} />
              </Form.Item>
              <Button type="primary" htmlType="submit" loading={saving}>
                保存
              </Button>
            </Form>
          ),
        },
        {
          key: 'logs',
          label: '日志与备份',
          children: (
            <Form form={form} layout="vertical" style={{ maxWidth: 600 }}>
              <Form.Item
                label="日志级别"
                name="logsLevel"
                tooltip="低于该级别的日志不记录（内存与落盘均过滤）。"
              >
                <Select
                  options={[
                    { value: 'DEBUG', label: 'DEBUG' },
                    { value: 'INFO', label: 'INFO' },
                    { value: 'WARN', label: 'WARN' },
                    { value: 'ERROR', label: 'ERROR' },
                  ]}
                />
              </Form.Item>
              <Form.Item
                label="日志落盘文件"
                name="logsFile"
                tooltip="相对配置目录的路径（如 logs/anigo.log），留空则不落盘。日志重启后仍可回溯。"
              >
                <Input placeholder="留空不落盘" />
              </Form.Item>
              <Form.Item label="内存日志条数" name="logsMax" tooltip="日志页可查看的内存环形缓冲容量。">
                <InputNumber min={1} max={100000} style={{ width: '100%' }} />
              </Form.Item>
              <Divider />
              <Button type="primary" onClick={handleSave} loading={saving}>
                保存
              </Button>
              <Divider />
              <Card size="small" title="备份与恢复">
                <Space>
                  <Button icon={<DownloadOutlined />} onClick={handleExport}>
                    导出备份
                  </Button>
                  <Upload
                    showUploadList={false}
                    beforeUpload={(file) => {
                      handleImport(file as File)
                      return false
                    }}
                  >
                    <Button icon={<UploadOutlined />}>导入备份</Button>
                  </Upload>
                </Space>
              </Card>
            </Form>
          ),
        },
      ]}
    />
  )
}