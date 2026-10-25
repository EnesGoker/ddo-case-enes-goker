import { createContext, useCallback, useEffect, useMemo, useState, type ReactNode } from 'react'
import * as client from '../api/client'
import type { User } from '../api/client'

type AuthState =
  | { status: 'loading' }
  | { status: 'anonymous' }
  | { status: 'authenticated'; user: User }

export interface AuthValue {
  state: AuthState
  login: (email: string, password: string) => Promise<void>
  logout: () => Promise<void>
}

export const AuthContext = createContext<AuthValue | null>(null)

export function AuthProvider({ children }: { children: ReactNode }) {
  const [state, setState] = useState<AuthState>({ status: 'loading' })

  // Sayfa yenilenince httpOnly refresh cookie ile oturumu geri yükle
  useEffect(() => {
    let cancelled = false
    client
      .refreshSession()
      .then((s) => !cancelled && setState({ status: 'authenticated', user: s.user }))
      .catch(() => !cancelled && setState({ status: 'anonymous' }))
    return () => {
      cancelled = true
    }
  }, [])

  const login = useCallback(async (email: string, password: string) => {
    const s = await client.login(email, password)
    setState({ status: 'authenticated', user: s.user })
  }, [])

  const logout = useCallback(async () => {
    try {
      await client.logout()
    } finally {
      setState({ status: 'anonymous' })
    }
  }, [])

  const value = useMemo(() => ({ state, login, logout }), [state, login, logout])
  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>
}
