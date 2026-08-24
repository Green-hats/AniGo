import { lazy, Suspense } from 'react'
import { HashRouter, Routes, Route, Navigate } from 'react-router-dom'
import { Layout, Spin } from 'antd'
import SideMenu from './components/SideMenu'

const HomePage = lazy(() => import('./pages/HomePage'))
const GardenPage = lazy(() => import('./pages/GardenPage'))
const SettingsPage = lazy(() => import('./pages/SettingsPage'))
const LogsPage = lazy(() => import('./pages/LogsPage'))

const { Content } = Layout

export default function App() {
  return (
    <HashRouter>
      <Layout style={{ minHeight: '100vh' }}>
        <SideMenu />
        <Layout>
          <Content style={{ margin: 16, padding: 24, background: '#fff', borderRadius: 8 }}>
            <Suspense
              fallback={
                <div style={{ textAlign: 'center', padding: 48 }}>
                  <Spin />
                </div>
              }
            >
              <Routes>
                <Route path="/" element={<Navigate to="/home" replace />} />
                <Route path="/home" element={<HomePage />} />
                <Route path="/garden" element={<GardenPage />} />
                <Route path="/settings" element={<SettingsPage />} />
                <Route path="/logs" element={<LogsPage />} />
              </Routes>
            </Suspense>
          </Content>
        </Layout>
      </Layout>
    </HashRouter>
  )
}