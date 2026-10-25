import { useState } from 'react'
import type { User } from '../api/client'
import { useAuth } from '../auth/useAuth'
import { RequireRole } from '../components/RequireRole'
import { menuFor, roleLabels } from '../roles'
import { CustomersPage } from './CustomersPage'
import { PoliciesPage } from './PoliciesPage'
import { QuoteFormPage } from './QuoteFormPage'
import { QuotesPage } from './QuotesPage'

export function Dashboard({ user }: { user: User }) {
  const { logout } = useAuth()
  const items = menuFor(user.role)
  const [active, setActive] = useState(items[0].key)
  const current = items.find((i) => i.key === active) ?? items[0]

  return (
    <div className="layout">
      <header className="topbar">
        <strong>GüvenPoliçe</strong>
        <span className="user">
          {user.full_name} · <span className="badge">{roleLabels[user.role]}</span>
        </span>
        <button type="button" className="secondary" onClick={() => void logout()}>
          Çıkış
        </button>
      </header>
      <nav className="menu" aria-label="Ana menü">
        {items.map((i) => (
          <button key={i.key} type="button" className={i.key === active ? 'active' : ''}
            aria-current={i.key === active ? 'page' : undefined} onClick={() => setActive(i.key)}>
            {i.label}
            {!i.ready && <small> ({i.phase})</small>}
          </button>
        ))}
      </nav>
      <main className="content">
        <h2>{current.label}</h2>
        {current.key === 'profile' && (
          <dl className="profile">
            <dt>Ad Soyad</dt><dd>{user.full_name}</dd>
            <dt>E-posta</dt><dd>{user.email}</dd>
            <dt>Rol</dt><dd>{roleLabels[user.role]}</dd>
          </dl>
        )}
        {current.key === 'customers' && (
          <RequireRole roles={['AGENT', 'UNDERWRITER']}>
            <CustomersPage />
          </RequireRole>
        )}
        {current.key === 'new-quote' && (
          <RequireRole roles={['AGENT']}>
            <QuoteFormPage />
          </RequireRole>
        )}
        {current.key === 'quotes' && <QuotesPage role={user.role} />}
        {current.key === 'policies' && <PoliciesPage />}
        {!current.ready && (
          <div className="notice">Bu ekran {current.phase} aşamasında eklenecek; henüz uygulanmadı.</div>
        )}
      </main>
    </div>
  )
}
