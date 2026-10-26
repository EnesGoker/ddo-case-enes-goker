import { statusLabels } from '../format'

// Presentation only: the existing server status and caller-provided label are preserved.
const tones: Record<string, string> = {
  DONE: 'success', OPEN: 'accent', IN_PROGRESS: 'accent', PARTIAL: 'warning', PENDING: 'warning',
  ACTIVE: 'success', PAID: 'success', APPROVED: 'success', CONVERTED: 'success',
  REJECTED: 'critical', FRAUD_REVIEW: 'warning', UNDER_REVIEW: 'warning', UW_REVIEW: 'warning',
  APPRAISAL: 'accent', ADJUSTMENT: 'accent', TRIAGED: 'accent', CALCULATED: 'accent',
}

export function StatusBadge({ status, label }: { status: string; label?: string }) {
  const text = label ?? statusLabels[status] ?? status
  const tone = text === statusLabels.EXPIRED ? 'neutral' : tones[status] ?? 'neutral'
  return <span className={`badge status ${tone}`}>{text}</span>
}
