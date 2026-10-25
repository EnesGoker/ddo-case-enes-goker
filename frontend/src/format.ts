// Kuruş (tam sayı) → "1.234,56 TL". Hesap yapılmaz, yalnızca gösterim; tam sayı aritmetiği.
export function formatTL(kurus: number): string {
  const sign = kurus < 0 ? '−' : ''
  const abs = Math.abs(kurus)
  const lira = Math.floor(abs / 100).toLocaleString('tr-TR')
  const kr = String(abs % 100).padStart(2, '0')
  return `${sign}${lira},${kr} TL`
}

export function formatBP(bp: number): string {
  return '×' + (bp / 10000).toLocaleString('tr-TR', { minimumFractionDigits: 2, maximumFractionDigits: 4 })
}

export const statusLabels: Record<string, string> = {
  CALCULATED: 'Hesaplandı',
  UW_REVIEW: 'Underwriter onayında',
  APPROVED: 'Onaylandı',
  REJECTED: 'Reddedildi',
  CONVERTED: 'Poliçeleşti',
  EXPIRED: 'Süresi doldu',
  ACTIVE: 'Yürürlükte',
  CANCELLED: 'İptal',
}

export const reasonLabels: Record<string, string> = {
  YOUNG_DRIVER: 'Sürücü 21 yaşından küçük',
  HIGH_PREMIUM: 'Prim onay eşiğinin üstünde',
}

export function errorText(e: unknown): string {
  if (e && typeof e === 'object' && 'message' in e) {
    const err = e as { message: string; status?: number; details?: Record<string, string> }
    if (err.status === 403) return 'Bu işlem için yetkiniz yok.'
    if (err.details && Object.keys(err.details).length > 0) return Object.values(err.details).join(' · ')
    return err.message
  }
  return 'Beklenmeyen bir hata oluştu.'
}

// Takvim günü yardımcıları (YYYY-MM-DD, saat dilimi etkisi olmadan)
export function todayISO(): string {
  const d = new Date()
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`
}

export function addDays(iso: string, n: number): string {
  const [y, m, d] = iso.split('-').map(Number)
  const t = new Date(Date.UTC(y, m - 1, d + n))
  return t.toISOString().slice(0, 10)
}

export function maxISO(a: string, b: string): string {
  return a > b ? a : b
}

export const cancelReasonLabels: Record<string, string> = {
  CUSTOMER_REQUEST: 'Müşteri talebi (kısa dönem)',
  UNDERWRITER: 'Underwriter kararı (gün esaslı)',
  NON_PAYMENT: 'Ödenmeme (gün esaslı)',
  OTHER: 'Diğer şirket kaynaklı (gün esaslı)',
}

export const methodLabels: Record<string, string> = {
  PRO_RATA: 'Gün esaslı',
  SHORT_PERIOD: 'Kısa dönem',
}

export function newIdempotencyKey(): string {
  return typeof crypto !== 'undefined' && 'randomUUID' in crypto
    ? crypto.randomUUID()
    : `${Date.now()}-${Math.random().toString(36).slice(2)}`
}
