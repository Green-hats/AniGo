import { HashRouter, Routes, Route, Navigate } from 'react-router-dom'
import { Layout } from 'antd'
import SideMenu from './components/SideMenu'
import HomePage from './pages/HomePage'
import GardenPage from './pages/GardenPage'
import SettingsPage from './pages/SettingsPage'
import LogsPage from './pages/LogsPage'

const { Content } = Layout

export default function App() {
  return (
    <HashRouter>
      <Layout style={{ minHeight: '100vh' }}>
        <SideMenu />
        <Layout>
          <Content style={{ margin: 16, padding: 24, background: '#fff', borderRadius: 8 }}>
            <Routes>
              <Route path="/" element={<Navigate to="/home" replace />} />
              <Route path="/home" element={<HomePage />} />
              <Route path="/garden" element={<GardenPage />} />
              <Route path="/settings" element={<SettingsPage />} />
              <Route path="/logs" element={<LogsPage />} />
            </Routes>
          </Content>
        </Layout>
      </Layout>
    </HashRouter>
  )
}