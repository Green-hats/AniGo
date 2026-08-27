import { useNavigate, useLocation } from 'react-router-dom'
import { Layout, Menu } from 'antd'
import {
  HomeOutlined,
  RocketOutlined,
  SettingOutlined,
  FileTextOutlined,
} from '@ant-design/icons'

const { Sider } = Layout

export default function SideMenu() {
  const nav = useNavigate()
  const loc = useLocation()
  const selected = '/' + (loc.pathname.split('/')[1] || 'home')

  return (
    <Sider theme="dark" width={200}>
      <div
        style={{
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          gap: 8,
          color: '#fff',
          fontSize: 18,
          fontWeight: 600,
          padding: '16px 0',
        }}
      >
        <img src="/favicon.svg" alt="AniGo" style={{ width: 28, height: 28 }} />
        AniGo
      </div>
      <Menu
        theme="dark"
        mode="inline"
        selectedKeys={[selected]}
        items={[
          { key: '/home', icon: <HomeOutlined />, label: '我的订阅' },
          { key: '/garden', icon: <RocketOutlined />, label: '番剧源' },
          { key: '/logs', icon: <FileTextOutlined />, label: '日志' },
          { key: '/settings', icon: <SettingOutlined />, label: '设置' },
        ]}
        onClick={({ key }) => nav(key)}
      />
    </Sider>
  )
}