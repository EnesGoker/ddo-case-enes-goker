import type { ClaimTask } from '../api/types'
import { statusLabels } from '../format'
import { StatusBadge } from './StatusBadge'

const taskLabels: Record<string, string> = { OPEN: 'Açık', IN_PROGRESS: 'İşleniyor', DONE: 'Tamamlandı', CANCELLED: 'İptal' }

export function SLABadge({ task }: { task: ClaimTask }) {
  const duration = (Date.parse(task.sla_due_at) - Date.parse(task.created_at)) / 1000
  const tone = task.breached ? 'late' : !task.completed_at && task.remaining_seconds <= duration / 4 ? 'soon' : 'ontime'
  const hours = Math.ceil(Math.abs(task.remaining_seconds) / 3600)
  const text = task.breached ? `${hours} sa gecikmiş` : task.completed_at ? 'Zamanında kapandı' : `${hours} sa kaldı`
  return <span className={`badge sla-${tone}`}>{text}</span>
}

export function TaskTable({ tasks, onSelect }: { tasks: ClaimTask[]; onSelect?: (id: number) => void }) {
  return <div className="table-wrap"><table><thead><tr><th>Dosya / görev</th><th>Atanan</th><th>Durum / SLA</th><th>Skor / sinyaller</th></tr></thead>
    <tbody>{tasks.map((t) => <tr key={t.id}>
      <td data-label="Dosya / görev"><div>{onSelect ? <button className="secondary" onClick={() => onSelect(t.claim_id)}>{t.claim_no}</button> : t.claim_no}<br />{statusLabels[t.task_type]}</div></td>
      <td data-label="Atanan">{t.assigned_to?.full_name ?? 'Atamasız'}</td>
      <td data-label="Durum / SLA"><div><StatusBadge status={t.status} label={taskLabels[t.status] ?? t.status} /> · <SLABadge task={t} /><br /><small>Son: {new Date(t.sla_due_at).toLocaleString('tr-TR')}</small></div></td>
      <td data-label="Skor / sinyaller">{t.fraud_score === undefined ? '—' : `${t.fraud_score} · ${t.signal_codes?.join(', ') ?? ''}`}</td>
    </tr>)}</tbody></table></div>
}
