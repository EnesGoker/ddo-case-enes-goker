import { useRef, useState } from 'react'
import { useActivity } from '../hooks/useActivity'
import { ActivityIndicator } from '../components/ActivityIndicator'
import { SystemPage } from './SystemPage'
import type { User } from '../api/client'
import { useAuth } from '../auth/useAuth'
import { RequireRole } from '../components/RequireRole'
import { menuFor, roleLabels } from '../roles'
import { AccountingPage } from './AccountingPage'
import { ClaimsPage } from './ClaimsPage'
import { CustomersPage } from './CustomersPage'
import { PoliciesPage } from './PoliciesPage'
import { QuoteFormPage } from './QuoteFormPage'
import { QuotesPage } from './QuotesPage'
import { TasksPage } from './TasksPage'
import { ReportsPage } from './ReportsPage'
import { AgentPerformancePage } from './AgentPerformancePage'
import { UIIcon, type UIIconName } from '../components/UIIcon'

const menuIcons: Record<string, UIIconName> = {
  profile: 'profile', 'new-quote': 'plus', quotes: 'quotes', policies: 'policies',
  customers: 'people', claims: 'claims', appraisals: 'appraisal', payments: 'wallet',
  tasks: 'tasks', fraud: 'shield', reports: 'chart', 'agent-performance': 'chart', system: 'system',
}

const descriptions: Record<string, string> = {
  profile: 'Hesap bilgilerinizi ve sistemdeki rolünüzü görüntüleyin.',
  'new-quote': 'Müşteri ve risk bilgileriyle açıklanabilir bir prim hesabı oluşturun.',
  quotes: 'Teklifleri, prim dökümlerini ve onay süreçlerini takip edin.',
  policies: 'Poliçelerinizi, durumlarını ve teminat bilgilerini tek yerden yönetin.',
  customers: 'Yetkiniz kapsamındaki müşteri kayıtlarını görüntüleyin.',
  claims: 'Hasar dosyalarının durumunu, geçmişini ve değerlendirmelerini takip edin.',
  appraisals: 'Ekspertiz bekleyen dosyaları inceleyin ve değerlendirmenizi kaydedin.',
  payments: 'Tahsilatları, ödeme planlarını ve muhasebe kayıtlarını takip edin.',
  tasks: 'Görevlerinizi, atamaları ve hizmet sürelerini takip edin.',
  fraud: 'İnceleme gerektiren dosyaları ve açıklanabilir sinyalleri değerlendirin.',
  reports: 'Yazılan prim ve tahakkuk eden tazminatı dönemler itibarıyla inceleyin.',
  'agent-performance': 'Acente performansını yazılan prim esasına göre karşılaştırın.',
  system: 'Sistem canlılığını ve yetkiniz kapsamındaki koşu bilgilerini görüntüleyin.',
}

export function Dashboard({ user }: { user: User }) {
  const { logout } = useAuth()
  const activityState = useActivity()
  const items = menuFor(user.role)
  const [active, setActive] = useState(items[0].key)
  const current = items.find((i) => i.key === active) ?? items[0]
  const [menuOpen, setMenuOpen] = useState(false)
  const menuButton = useRef<HTMLButtonElement>(null)
  const initials = user.full_name.split(/\s+/).filter(Boolean).slice(0, 2).map((part) => part[0]).join('').toLocaleUpperCase('tr-TR')

  return (
    <div className="layout" onKeyDown={(event) => {
        if (event.key === 'Escape' && menuOpen) { setMenuOpen(false); menuButton.current?.focus() }
      }}>
      <a className="skip-link" href="#workspace">İçeriğe geç</a>
      <header className="topbar">
        <div className="topbar-left">
          <button ref={menuButton} type="button" className="secondary mobile-toggle" aria-label={menuOpen ? 'Menüyü kapat' : 'Menüyü aç'}
            aria-expanded={menuOpen} aria-controls="main-navigation" onClick={() => setMenuOpen(!menuOpen)}>
            <UIIcon name={menuOpen ? 'close' : 'menu'} />
          </button>
          <div className="breadcrumb"><span className="breadcrumb-root">Çalışma alanı</span><UIIcon name="chevron" /><strong>{current.label}</strong></div>
        </div>
        <div className="topbar-tools">
          <ActivityIndicator activity={activityState.activity} error={activityState.error} checkedAt={activityState.checkedAt} />
          <div className="user"><span className="avatar" aria-hidden="true">{initials}</span><span className="user-copy"><strong>{user.full_name}</strong><small>{roleLabels[user.role]}</small></span></div>
          <button type="button" className="secondary logout" aria-label="Çıkış" onClick={() => void logout()}><UIIcon name="logout" /><span>Çıkış</span></button>
        </div>
      </header>
      <aside id="main-navigation" className={`sidebar${menuOpen ? ' is-open' : ''}`}>
        <div className="brand"><UIIcon name="shield" /><div><strong>GüvenPoliçe</strong><small>SİGORTA YÖNETİMİ</small></div></div>
        <div className="nav-caption">Çalışma alanı</div>
        <nav className="menu" aria-label="Ana menü">
          {items.map((i) => (
            <button key={i.key} type="button" className={i.key === active ? 'active' : ''}
              aria-current={i.key === active ? 'page' : undefined} onClick={() => {
                setActive(i.key)
                if (menuOpen) { setMenuOpen(false); menuButton.current?.focus() }
              }}>
              <UIIcon name={menuIcons[i.key] ?? 'policies'} /><span>{i.label}{!i.ready && <small> ({i.phase})</small>}</span>
            </button>
          ))}
        </nav>
        <div className="sidebar-user"><span className="avatar" aria-hidden="true">{initials}</span><span className="user-copy"><strong>{user.full_name}</strong><small>{roleLabels[user.role]}</small></span></div>
      </aside>
      <main className="content" id="workspace" tabIndex={-1}>
        <div className="page-heading"><div className="eyebrow">GüvenPoliçe / {roleLabels[user.role]}</div><h2>{current.label}</h2><p>{descriptions[current.key]}</p></div>
        {current.key === 'system' && <SystemPage {...activityState} />}
        {current.key === 'profile' && (
          <section className="panel profile-card"><div className="section-heading"><div><div className="eyebrow">Hesabım</div><h3>Kullanıcı bilgileri</h3></div><UIIcon name="profile" /></div>
            <dl className="profile">
              <dt>Ad Soyad</dt><dd>{user.full_name}</dd>
              <dt>E-posta</dt><dd>{user.email}</dd>
              <dt>Rol</dt><dd>{roleLabels[user.role]}</dd>
            </dl>
          </section>
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
        {current.key === 'payments' && (user.role === 'ACCOUNTING' ? <AccountingPage /> : <PoliciesPage role={user.role} />)}
        {current.key === 'policies' && <PoliciesPage role={user.role} />}
        {(current.key === 'claims' || current.key === 'appraisals') && <ClaimsPage role={user.role} />}
        {current.key === 'tasks' && <RequireRole roles={['CLAIMS_ADJUSTER', 'APPRAISER']}><TasksPage role={user.role} /></RequireRole>}
        {current.key === 'fraud' && <RequireRole roles={['CLAIMS_ADJUSTER']}><TasksPage key="fraud" role={user.role} fraudQueue /></RequireRole>}
        {current.key === 'reports' && <RequireRole roles={['ACCOUNTING', 'UNDERWRITER']}><ReportsPage /></RequireRole>}
        {current.key === 'agent-performance' && <RequireRole roles={['AGENT', 'ACCOUNTING', 'UNDERWRITER']}><AgentPerformancePage /></RequireRole>}
        {!current.ready && (
          <div className="notice">Bu ekran {current.phase} aşamasında eklenecek; henüz uygulanmadı.</div>
        )}
        <footer className="page-footer"><span>GüvenPoliçe · Sigorta yönetimi</span><span>Veriler yetkiniz kapsamında gösterilir.</span></footer>
      </main>
    </div>
  )
}
