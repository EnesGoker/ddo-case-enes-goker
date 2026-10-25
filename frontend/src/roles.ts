import type { Role } from './api/client'

export const roleLabels: Record<Role, string> = {
  CUSTOMER: 'Müşteri',
  AGENT: 'Acente',
  UNDERWRITER: 'Underwriter',
  APPRAISER: 'Ekspertiz Uzmanı',
  CLAIMS_ADJUSTER: 'Hasar Uzmanı',
  ACCOUNTING: 'Muhasebe',
}

export interface MenuItem {
  key: string
  label: string
  roles: Role[]
  // ready=false: ekran henüz yok, hangi aşamada geleceği gösterilir (sahte veri gösterilmez)
  ready: boolean
  phase?: string
}

// Rol bazlı menü. Sunucu her uçta ayrıca yetki denetler; bu liste yalnızca gezinme içindir.
export const menu: MenuItem[] = [
  { key: 'profile', label: 'Profilim', roles: ['CUSTOMER', 'AGENT', 'UNDERWRITER', 'APPRAISER', 'CLAIMS_ADJUSTER', 'ACCOUNTING'], ready: true },
  { key: 'new-quote', label: 'Yeni teklif', roles: ['AGENT'], ready: true },
  { key: 'quotes', label: 'Teklifler', roles: ['AGENT', 'UNDERWRITER', 'CUSTOMER'], ready: true },
  { key: 'policies', label: 'Poliçeler', roles: ['CUSTOMER', 'AGENT', 'UNDERWRITER', 'CLAIMS_ADJUSTER', 'ACCOUNTING'], ready: true },
  { key: 'customers', label: 'Müşteriler', roles: ['AGENT', 'UNDERWRITER'], ready: true },
  { key: 'claims', label: 'Hasar Dosyaları', roles: ['CUSTOMER', 'CLAIMS_ADJUSTER', 'APPRAISER'], ready: false, phase: 'A4' },
  { key: 'appraisals', label: 'Ekspertiz', roles: ['APPRAISER'], ready: false, phase: 'A4' },
  { key: 'payments', label: 'Tahsilat ve Defter', roles: ['ACCOUNTING', 'CUSTOMER'], ready: false, phase: 'A5' },
  { key: 'reports', label: 'Raporlar', roles: ['ACCOUNTING', 'UNDERWRITER'], ready: false, phase: 'A6' },
]

export function menuFor(role: Role): MenuItem[] {
  return menu.filter((m) => m.roles.includes(role))
}
