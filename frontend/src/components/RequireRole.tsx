import type { ReactNode } from 'react'
import type { Role } from '../api/client'
import { useAuth } from '../auth/useAuth'

// Arayüz tarafı koruma yalnızca kullanıcı deneyimi içindir; asıl yetki denetimi sunucudadır.
export function RequireRole({ roles, children }: { roles: Role[]; children: ReactNode }) {
  const { state } = useAuth()
  if (state.status !== 'authenticated') return null
  if (!roles.includes(state.user.role)) {
    return (
      <div className="notice error" role="alert">
        Bu ekran için yetkiniz yok.
      </div>
    )
  }
  return <>{children}</>
}
