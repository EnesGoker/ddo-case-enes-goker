import type { Activity } from '../api/types'

export function ActivityIndicator({ activity, error, checkedAt }: { activity: Activity | null; error: string | null; checkedAt: number }) {
  const healthy = !error && activity?.db_ok
  const seconds = activity?.last_event_at ? Math.max(0, Math.floor((checkedAt - Date.parse(activity.last_event_at)) / 1000)) : null
  const ago = seconds === null ? 'henüz yok' : seconds < 60 ? `${seconds} sn önce` : `${Math.floor(seconds / 60)} dk önce`
  return <span className={`activity-indicator ${healthy ? 'connected' : 'disconnected'}`} role="status">
    <span aria-hidden="true">●</span> {error ? 'Bağlantı hatası' : !activity ? 'Bağlantı kontrol ediliyor…' : !activity.db_ok ? 'Veritabanı bağlantısı yok' : `Son olay: ${ago}`}
  </span>
}
