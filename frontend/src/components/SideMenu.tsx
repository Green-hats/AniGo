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
          color: '#fff',
          fontSize: 18,
          fontWeight: 600,
          textAlign: 'center',
          padding: '16px 0',
        }}
      >
        🌸 anigo
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