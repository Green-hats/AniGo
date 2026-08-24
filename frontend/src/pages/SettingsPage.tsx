import { useEffect, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Tabs, Form, Input, Switch, InputNumber, Button, Divider, message, Space, Select } from 'antd'
import { api } from '../api/client'
import type { Config } from '../types'

export default function SettingsPage() {
  const { data: cfg } = useQuery({ queryKey: ['config'], queryFn: api.getConfig })
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
      ]}
    />
  )
}