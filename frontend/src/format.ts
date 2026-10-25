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
