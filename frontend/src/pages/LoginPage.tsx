import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { Button, Card, ConfigProvider, Form, Input, Typography, message } from 'antd'
import { api, setToken } from '../api/client'

export default function LoginPage({ onSuccess }: { onSuccess?: () => void }) {
  const [loading, setLoading] = useState(false)
  const nav = useNavigate()

  const handleLogin = async (values: { username: string; password: string }) => {
    setLoading(true)
    try {
      const res = await api.login(values.username, values.password)
      if (res.token) {
        setToken(res.token)
      }
      message.success('登录成功')
      onSuccess?.()
      nav('/home', { replace: true })
    } catch (e) {
      message.error((e as Error).message)
    } finally {
      setLoading(false)
    }
  }

  return (
    <div
      style={{
        minHeight: '100vh',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        background: '#fff',
      }}
    >
      <Card style={{ width: 360, border: '1px solid #f0f0f0' }}>
        <div style={{ textAlign: 'center', marginBottom: 24 }}>
          <Typography.Title level={3} style={{ marginBottom: 0 }}>
            AniGo
          </Typography.Title>
        </div>
        <ConfigProvider theme={{ token: { colorPrimary: '#0284c7' } }}>
          <Form layout="vertical" onFinish={handleLogin} autoComplete="off">
<Form.Item name="username" rules={[{ required: true, message: '请输入用户名' }]}>
            <Input placeholder="用户名" size="large" />
          </Form.Item>
          <Form.Item name="password" rules={[{ required: true, message: '请输入密码' }]}>
            <Input.Password placeholder="密码" size="large" />
          </Form.Item>
            <Button type="primary" htmlType="submit" block size="large" loading={loading}>
              登录
            </Button>
          </Form>
        </ConfigProvider>
      </Card>
    </div>
  )
}