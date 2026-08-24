import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Routes, Route, useLocation } from 'react-router-dom'
import SideMenu from './SideMenu'

function LocationProbe() {
  const loc = useLocation()
  return <div data-testid="loc">{loc.pathname}</div>
}

describe('SideMenu', () => {
  it('渲染四个导航项', () => {
    render(
      <MemoryRouter initialEntries={['/home']}>
        <SideMenu />
      </MemoryRouter>,
    )
    expect(screen.getByText('我的订阅')).toBeInTheDocument()
    expect(screen.getByText('番剧源')).toBeInTheDocument()
    expect(screen.getByText('日志')).toBeInTheDocument()
    expect(screen.getByText('设置')).toBeInTheDocument()
  })

  it('点击"设置"跳转到 /settings', async () => {
    render(
      <MemoryRouter initialEntries={['/home']}>
        <SideMenu />
        <Routes>
          <Route path="*" element={<LocationProbe />} />
        </Routes>
      </MemoryRouter>,
    )
    await userEvent.click(screen.getByText('设置'))
    expect(screen.getByTestId('loc')).toHaveTextContent('/settings')
  })

  it('点击"番剧源"跳转到 /garden', async () => {
    render(
      <MemoryRouter initialEntries={['/home']}>
        <SideMenu />
        <Routes>
          <Route path="*" element={<LocationProbe />} />
        </Routes>
      </MemoryRouter>,
    )
    await userEvent.click(screen.getByText('番剧源'))
    expect(screen.getByTestId('loc')).toHaveTextContent('/garden')
  })

  it('当前路由高亮对应菜单项', () => {
    render(
      <MemoryRouter initialEntries={['/logs']}>
        <SideMenu />
      </MemoryRouter>,
    )
    const selected = document.querySelector('.ant-menu-item-selected')
    expect(selected?.textContent).toContain('日志')
  })
})