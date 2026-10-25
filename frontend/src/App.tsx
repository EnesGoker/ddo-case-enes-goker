import { useAuth } from './auth/useAuth'
import { Loading } from './components/States'
import { Dashboard } from './pages/Dashboard'
import { LoginPage } from './pages/LoginPage'

export function App() {
  const { state } = useAuth()
  if (state.status === 'loading') return <Loading text="Oturum kontrol ediliyor…" />
  if (state.status === 'anonymous') return <LoginPage />
  return <Dashboard key={state.user.id} user={state.user} />
}
