import type { CoverageSpec, PremiumLine } from '../api/types'
import { formatBP, formatTL } from '../format'

// Prim dökümü: her satır bir faktörün TL etkisi; toplam satırların toplamına eşittir.
export function Breakdown({ lines, coverages, total }: { lines: PremiumLine[]; coverages: CoverageSpec[]; total: number }) {
  const names = new Map(coverages.map((c) => [c.code, c.name]))
  return (
    <div className="table-wrap">
      <table>
        <thead>
          <tr>
            <th>#</th>
            <th>Teminat</th>
            <th>Kalem</th>
            <th>Çarpan</th>
            <th className="num">Tutar</th>
          </tr>
        </thead>
        <tbody>
          {lines.map((l) => (
            <tr key={l.seq} className={l.line_type === 'BASE' ? 'base-row' : ''}>
              <td data-label="#">{l.seq}</td>
              <td data-label="Teminat">{names.get(l.coverage_code) ?? l.coverage_code}</td>
              <td data-label="Kalem">{l.factor_label}</td>
              <td data-label="Çarpan">{l.multiplier_bp ? formatBP(l.multiplier_bp) : '—'}</td>
              <td data-label="Tutar" className="num">{formatTL(l.amount_kurus)}</td>
            </tr>
          ))}
        </tbody>
        <tfoot>
          {coverages.map((c) => (
            <tr key={c.code}>
              <td colSpan={4}>{c.name} primi</td>
              <td className="num">{c.premium_kurus !== undefined ? formatTL(c.premium_kurus) : '—'}</td>
            </tr>
          ))}
          <tr className="total-row">
            <td colSpan={4}>Toplam yıllık prim</td>
            <td className="num">{formatTL(total)}</td>
          </tr>
        </tfoot>
      </table>
    </div>
  )
}
