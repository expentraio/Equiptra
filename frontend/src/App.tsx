import type { ReactNode } from 'react'
import { Navigate, Route, Routes } from 'react-router-dom'
import { AuthProvider, useAuth } from './context/AuthContext'
import { Layout } from './components/Layout'
import { Login } from './pages/Login'
import { Assets } from './pages/Assets'
import { Projects } from './pages/Projects'
import { BookingBoard } from './pages/BookingBoard'
import { Carnets } from './pages/Carnets'
import { DeliveryNotes } from './pages/DeliveryNotes'

function RequireAuth({ children }: { children: ReactNode }) {
  const { user, loading } = useAuth()
  if (loading) return <div className="p-8 text-[13px] text-ink-soft">Loading…</div>
  if (!user) return <Navigate to="/login" replace />
  return <>{children}</>
}

function AppRoutes() {
  return (
    <Routes>
      <Route path="/login" element={<Login />} />
      <Route
        element={
          <RequireAuth>
            <Layout />
          </RequireAuth>
        }
      >
        <Route path="/" element={<Navigate to="/assets" replace />} />
        <Route path="/assets" element={<Assets />} />
        <Route path="/projects" element={<Projects />} />
        <Route path="/projects/:id" element={<BookingBoard />} />
        <Route path="/carnets" element={<Carnets />} />
        <Route path="/delivery-notes" element={<DeliveryNotes />} />
      </Route>
      <Route path="*" element={<Navigate to="/" replace />} />
    </Routes>
  )
}

export function App() {
  return (
    <AuthProvider>
      <AppRoutes />
    </AuthProvider>
  )
}
