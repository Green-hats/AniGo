import { useState } from 'react'
import { Typography, Card, Row, Col, Button, Space, Tag, Input } from 'antd'
import {
  ApiOutlined,
  CloudServerOutlined,
  RobotOutlined,
  DatabaseOutlined,
  ThunderboltOutlined,
  CheckCircleTwoTone,
  CloseCircleTwoTone,
} from '@ant-design/icons'
import { api } from '../api/client'

interface TestResult {
  status: 'ok' | 'fail'
  detail: string
}

export default function TestPage() {
  const [results, setResults] = useState<Record<string, TestResult>>({})
  const [bgmKeyword, setBgmKeyword] = useState('间谍过家家')
  const [tmdbTitle, setTmdbTitle] = useState('间谍过家家')

  const setResult = (key: string, ok: boolean, detail: string) => {
    setResults((r) => ({ ...r, [key]: { status: ok ? 'ok' : 'fail', detail } }))
  }

  const testAI = async () => {
    try {
      const r = await api.aiPing()
      setResult('ai', true, `AI 响应: ${r.reply}`)
    } catch (e) {
      setResult('ai', false, (e as Error).message)
    }
  }

  const test115 = async () => {
    try {
      await api.downloadLoginTest()
      setResult('cloud', true, '115 登录成功')
    } catch (e) {
      setResult('cloud', false, (e as Error).message)
    }
  }

  const testBGM = async () => {
    try {
      const list = await api.searchBgm(bgmKeyword)
      setResult('bgm', true, `搜索到 ${list.length} 条结果`)
    } catch (e) {
      setResult('bgm', false, (e as Error).message)
    }
  }

  const testGarden = async () => {
    try {
      const weeks = await api.gardenList()
      setResult('garden', true, `获取到 ${weeks.length} 个星期分组`)
    } catch (e) {
      setResult('garden', false, (e as Error).message)
    }
  }

  const testTMDB = async () => {
    try {
      const r = await api.getThemoviedbName(tmdbTitle, false)
      setResult('tmdb', true, `TMDB 命名: ${r.themoviedbName}`)
    } catch (e) {
      setResult('tmdb', false, (e as Error).message)
    }
  }

  const testAll = async () => {
    await Promise.allSettled([testAI(), test115(), testBGM(), testGarden(), testTMDB()])
  }

  const renderResult = (key: string) => {
    const r = results[key]
    if (!r) return null
    return r.status === 'ok' ? (
      <span style={{ marginLeft: 8 }}>
        <CheckCircleTwoTone twoToneColor="#52c41a" /> <Tag color="green">{r.detail}</Tag>
      </span>
    ) : (
      <span style={{ marginLeft: 8 }}>
        <CloseCircleTwoTone twoToneColor="#ff4d4f" /> <Tag color="red">{r.detail}</Tag>
      </span>
    )
  }

  return (
    <div>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 16 }}>
        <Typography.Title level={4} style={{ margin: 0 }}>
          服务测试
        </Typography.Title>
        <Button type="primary" icon={<ThunderboltOutlined />} onClick={testAll}>
          全部测试
        </Button>
      </div>

      <Row gutter={[16, 16]}>
        <Col xs={24} md={12}>
          <Card size="small" title={<Space><RobotOutlined /> AI 解析服务</Space>}>
            <Space direction="vertical" style={{ width: '100%' }}>
              <Button onClick={testAI}>测试 AI 连通</Button>
              {renderResult('ai')}
            </Space>
          </Card>
        </Col>
        <Col xs={24} md={12}>
          <Card size="small" title={<Space><CloudServerOutlined /> 115 网盘</Space>}>
            <Space direction="vertical" style={{ width: '100%' }}>
              <Button onClick={test115}>测试 115 登录</Button>
              {renderResult('cloud')}
            </Space>
          </Card>
        </Col>
        <Col xs={24} md={12}>
          <Card size="small" title={<Space><DatabaseOutlined /> Bangumi 元数据</Space>}>
            <Space direction="vertical" style={{ width: '100%' }}>
              <Space>
                <Input
                  value={bgmKeyword}
                  onChange={(e) => setBgmKeyword(e.target.value)}
                  placeholder="搜索关键词"
                  style={{ width: 160 }}
                />
                <Button onClick={testBGM}>搜索测试</Button>
              </Space>
              {renderResult('bgm')}
            </Space>
          </Card>
        </Col>
        <Col xs={24} md={12}>
          <Card size="small" title={<Space><ApiOutlined /> animes.garden 番剧源</Space>}>
            <Space direction="vertical" style={{ width: '100%' }}>
              <Button onClick={testGarden}>获取番剧列表</Button>
              {renderResult('garden')}
            </Space>
          </Card>
        </Col>
        <Col xs={24} md={12}>
          <Card size="small" title="TMDB 标题命名">
            <Space direction="vertical" style={{ width: '100%' }}>
              <Space>
                <Input
                  value={tmdbTitle}
                  onChange={(e) => setTmdbTitle(e.target.value)}
                  placeholder="标题"
                  style={{ width: 160 }}
                />
                <Button onClick={testTMDB}>命名测试</Button>
              </Space>
              {renderResult('tmdb')}
            </Space>
          </Card>
        </Col>
      </Row>
    </div>
  )
}