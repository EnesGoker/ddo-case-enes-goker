import { useContext } from 'react'
import { AuthContext, type AuthValue } from './AuthContext'

export function useAuth(): AuthValue {
  const v = useContext(AuthContext)
  if (!v) throw new Error('useAuth, AuthProvider içinde kullanılmalı')
  return v
}
