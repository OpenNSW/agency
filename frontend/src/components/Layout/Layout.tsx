import { Outlet } from 'react-router-dom'
import { Sidebar } from './Sidebar'
import { TopBar } from './TopBar'
import { Footer } from './Footer'
import { useState } from 'react'

export function Layout() {
  const [isSidebarExpanded, setIsSidebarExpanded] = useState(() => {
    const savedState = localStorage.getItem('sidebarExpanded')
    return savedState !== null ? savedState === 'true' : true
  })
  const sidebarWidth = isSidebarExpanded ? 256 : 80

  const handleToggleSidebar = () => {
    setIsSidebarExpanded((prev) => {
      const newState = !prev
      localStorage.setItem('sidebarExpanded', String(newState))
      return newState
    })
  }

  return (
    <div className="min-h-screen bg-gray-50">
      <TopBar />
      <div className="flex">
        <Sidebar isExpanded={isSidebarExpanded} onToggle={handleToggleSidebar} />
        <main
          style={{ marginLeft: `${sidebarWidth}px`, width: `calc(100% - ${sidebarWidth}px)` }}
          className="flex min-h-[calc(100vh-64px)] flex-col transition-all duration-300 mt-16 p-8 pb-16 sm:pb-8"
        >
          {/* Plain block wrapper: flex items with mx-auto stop stretching, which
              would shrink pages using max-w-* mx-auto to fit-content width. */}
          <div className="w-full">
            <Outlet />
          </div>
        </main>
      </div>
      <Footer />
    </div>
  )
}
