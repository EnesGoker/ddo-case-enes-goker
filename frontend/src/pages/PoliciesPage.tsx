import type { PolicyItem } from '../api/types'
import { Empty, ErrorBox, Loading } from '../components/States'
import { formatTL, statusLabels } from '../format'
import { usePaged } from '../hooks/usePaged'

export function PoliciesPage() {
  const { state, reload, loadMore } = usePaged<PolicyItem>('/api/v1/policies?limit=20')

  if (state.kind === 'loading') return <Loading text="Poliçeler yükleniyor…" />
  if (state.kind === 'error') return <ErrorBox message={state.message} onRetry={reload} />
  if (state.items.length === 0) return <Empty text="Henüz poliçe yok." />

  return (
    <section>
      <div className="table-wrap">
        <table>
          <thead>
            <tr>
              <th>Poliçe no</th>
              <th>Ürün</th>
              <th>Sigortalı</th>
              <th>Dönem</th>
              <th>Durum</th>
              <th className="num">Yıllık prim</th>
            </tr>
          </thead>
          <tbody>
            {state.items.map((p) => (
              <tr key={p.id}>
                <td data-label="Poliçe no">{p.policy_no}</td>
                <td data-label="Ürün">{p.product}{p.vehicle_plate ? ` · ${p.vehicle_plate}` : ''}</td>
                <td data-label="Sigortalı">{p.customer_name}</td>
                <td data-label="Dönem">{p.start_date} → {p.end_date}</td>
                <td data-label="Durum">{statusLabels[p.status] ?? p.status}</td>
                <td data-label="Yıllık prim" className="num">{formatTL(p.annual_premium_kurus)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      {state.next && (
        <button type="button" className="secondary" onClick={loadMore} disabled={state.loadingMore}>
          {state.loadingMore ? 'Yükleniyor…' : 'Daha fazla'}
        </button>
      )}
    </section>
  )
}
