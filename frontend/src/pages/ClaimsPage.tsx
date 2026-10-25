import { useState } from 'react'
import type { Role } from '../api/client'
import type { ClaimItem } from '../api/types'
import { ClaimDetailLoader } from '../components/ClaimDetail'
import { Empty, ErrorBox, Loading } from '../components/States'
import { formatTL, statusLabels } from '../format'
import { usePaged } from '../hooks/usePaged'

const statuses = ['REPORTED', 'TRIAGED', 'APPRAISAL', 'ADJUSTMENT', 'APPROVED', 'REJECTED', 'PAID', 'CLOSED']
export function ClaimsPage({ role }: { role: Role }) {
  const [status, setStatus] = useState(role === 'APPRAISER' ? 'APPRAISAL' : role === 'ACCOUNTING' ? 'APPROVED' : '')
  const [selected, setSelected] = useState<number | null>(null)
  const { state, reload, loadMore } = usePaged<ClaimItem>(`/api/v1/claims?limit=20&status=${status}`)
  return <section>
    <label>Durum filtresi<select value={status} onChange={(e) => { setStatus(e.target.value); setSelected(null) }}><option value="">Tümü</option>{statuses.map((s) => <option key={s} value={s}>{statusLabels[s]}</option>)}</select></label>
    <button className="secondary" onClick={reload}>Listeyi yenile</button>
    {state.kind === 'loading' && <Loading />}
    {state.kind === 'error' && <ErrorBox message={state.message} onRetry={reload} />}
    {state.kind === 'ready' && (state.items.length === 0 ? <Empty text="Bu durumda hasar dosyası yok." /> : <>
      <div className="table-wrap"><table><thead><tr><th>Dosya</th><th>Poliçe / teminat</th><th>Tarih</th><th>Durum</th><th>Talep</th></tr></thead><tbody>{state.items.map((c) => <tr key={c.id} className={`clickable${selected === c.id ? ' selected' : ''}`} tabIndex={0} onClick={() => setSelected(c.id)} onKeyDown={(e) => e.key === 'Enter' && setSelected(c.id)}>
        <td data-label="Dosya">{c.claim_no}</td><td data-label="Poliçe / teminat">{c.policy_no} / {c.coverage_code}</td><td data-label="Tarih">{c.loss_date}</td><td data-label="Durum">{statusLabels[c.status]}</td><td data-label="Talep">{formatTL(c.claimed_amount_kurus)}</td>
      </tr>)}</tbody></table></div>
      {state.next && <button className="secondary" disabled={state.loadingMore} onClick={loadMore}>{state.loadingMore ? 'Yükleniyor…' : 'Daha fazla'}</button>}
    </>)}
    {selected !== null && <ClaimDetailLoader key={selected} id={selected} role={role} />}
  </section>
}
