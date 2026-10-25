import type { CalcDetail } from '../api/types'
import { methodLabels } from '../format'

// Hesap adımları: sunucunun calc_detail.steps alanını olduğu gibi gösterir (hesap tekrarlanmaz).
export function CalcSteps({ calc }: { calc: CalcDetail }) {
  return (
    <div className="calc">
      <p className="muted">
        {methodLabels[calc.method] ?? calc.method} · dönem {calc.period_days} gün
        {calc.leap_day_in_period ? ' (29 Şubat dahil)' : ''} · geçen {calc.elapsed_days} · kalan {calc.remaining_days}
      </p>
      <ol>
        {calc.steps.map((s, i) => (
          <li key={i}>{s}</li>
        ))}
      </ol>
    </div>
  )
}
