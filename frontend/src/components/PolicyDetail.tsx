import { useEffect, useState, type FormEvent } from 'react'
import { api, ApiError } from '../api/client'
import type { Role } from '../api/client'
import type { CoverageOnDate, PolicyDetail } from '../api/types'
import { cancelReasonLabels, errorText, formatTL, statusLabels, todayISO } from '../format'
import { ReportClaimForm } from './ClaimForms'
import { CalcSteps } from './CalcSteps'
import { CancelForm, VehicleChangeForm } from './PolicyChangeForms'
import { ErrorBox, Loading } from './States'

const typeLabels: Record<string, string> = { VEHICLE_CHANGE: 'Araç değişikliği', CANCELLATION: 'İptal' }

export function PolicyDetailLoader({ id, role, onChanged }: { id: number; role: Role; onChanged?: () => void }) {
  const [policy, setPolicy] = useState<PolicyDetail | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [attempt, setAttempt] = useState(0)

  useEffect(() => {
    let cancelled = false
    api<PolicyDetail>(`/api/v1/policies/${id}`)
      .then((p) => {
        if (!cancelled) {
          setPolicy(p)
          setError(null)
        }
      })
      .catch((e: unknown) => !cancelled && setError(errorText(e)))
    return () => {
      cancelled = true
    }
  }, [id, attempt])

  if (error) return <ErrorBox message={error} onRetry={() => setAttempt((a) => a + 1)} />
  if (!policy || policy.id !== id) return <Loading text="Poliçe yükleniyor…" />
  return (
    <PolicyDetailView
      policy={policy}
      role={role}
      reload={() => {
        setAttempt((a) => a + 1)
        onChanged?.()
      }}
    />
  )
}

function PolicyDetailView({ policy, role, reload }: { policy: PolicyDetail; role: Role; reload: () => void }) {
  const [action, setAction] = useState<'none' | 'vehicle' | 'cancel' | 'claim'>('none')
  const canChange = (role === 'AGENT' || role === 'UNDERWRITER') && policy.status === 'ACTIVE'
  const isVehicle = policy.product !== 'KONUT'
  const current = policy.versions[policy.versions.length - 1]

  return (
    <section className="panel">
      <h3>
        {policy.policy_no} · {policy.product} · <span className="badge">{statusLabels[policy.status] ?? policy.status}</span>
      </h3>
      <dl className="profile">
        <dt>Sigortalı</dt><dd>{policy.customer_name}</dd>
        <dt>Dönem</dt><dd>{policy.start_date} → {policy.end_date} ({policy.period_days} gün)</dd>
        {policy.cancel_effective_date && (
          <>
            <dt>İptal</dt>
            <dd>{policy.cancel_effective_date} · {cancelReasonLabels[policy.cancel_reason ?? ''] ?? policy.cancel_reason}</dd>
          </>
        )}
        <dt>İlk prim</dt><dd>{formatTL(policy.premium.initial_premium_kurus)}</dd>
        <dt>Zeyil farkları</dt><dd>{formatTL(policy.premium.endorsement_deltas_kurus)}</dd>
        <dt>Yazılmış prim</dt><dd>{formatTL(policy.premium.written_premium_kurus)}</dd>
        {policy.premium.cancellation_refund_kurus > 0 && (
          <><dt>İptal iadesi</dt><dd>{formatTL(policy.premium.cancellation_refund_kurus)}</dd></>
        )}
        <dt>Net prim</dt><dd><strong>{formatTL(policy.premium.net_premium_kurus)}</strong></dd>
      </dl>

      <h4>Sürüm geçmişi</h4>
      <div className="table-wrap">
        <table>
          <thead>
            <tr>
              <th>Sürüm</th>
              <th>Geçerlilik</th>
              <th>Plaka / grup</th>
              <th>Zeyil</th>
              <th className="num">Yıllık prim</th>
            </tr>
          </thead>
          <tbody>
            {policy.versions.map((v) => (
              <tr key={v.id}>
                <td data-label="Sürüm">v{v.version_no}</td>
                <td data-label="Geçerlilik">{v.effective_from} → {v.effective_to ?? (policy.cancel_effective_date ? `iptal ${policy.cancel_effective_date}` : 'açık')}</td>
                <td data-label="Plaka / grup">{v.vehicle_plate ?? '—'} {v.risk.vehicle_group ? `/ ${v.risk.vehicle_group}` : ''}</td>
                <td data-label="Zeyil">{v.endorsement_no ? `#${v.endorsement_no}` : 'ilk düzenleme'}</td>
                <td data-label="Yıllık prim" className="num">{formatTL(v.annual_premium_kurus)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      <h4>Güncel teminatlar (v{current.version_no})</h4>
      <CoverageTable version={current} />

      {policy.endorsements.length > 0 && (
        <>
          <h4>Zeyiller</h4>
          {policy.endorsements.map((e) => (
            <details key={e.id} className="endorsement">
              <summary>
                #{e.endorsement_no} {typeLabels[e.type] ?? e.type} · {e.effective_date} ·{' '}
                {e.premium_delta_kurus >= 0 ? 'ek prim ' : 'iade '}
                {formatTL(Math.abs(e.premium_delta_kurus))}
              </summary>
              <CalcSteps calc={e.calc_detail} />
            </details>
          ))}
        </>
      )}

      <CoverageQuery policy={policy} />

      {(role === 'CUSTOMER' || role === 'AGENT' || role === 'CLAIMS_ADJUSTER') && <div className="actions"><button onClick={() => setAction(action === 'claim' ? 'none' : 'claim')}>Hasar bildir</button></div>}
      {action === 'claim' && <ReportClaimForm policyID={policy.id} onDone={() => setAction('none')} />}
      {canChange && (
        <div className="actions">
          {isVehicle && (
            <button type="button" className={action === 'vehicle' ? '' : 'secondary'} onClick={() => setAction(action === 'vehicle' ? 'none' : 'vehicle')}>
              Araç değişikliği
            </button>
          )}
          <button type="button" className={action === 'cancel' ? 'danger' : 'secondary'} onClick={() => setAction(action === 'cancel' ? 'none' : 'cancel')}>
            İptal et
          </button>
        </div>
      )}
      {canChange && action === 'vehicle' && <VehicleChangeForm key={current.id} policy={policy} onDone={() => { setAction('none'); reload() }} />}
      {canChange && action === 'cancel' && <CancelForm key={current.id} policy={policy} role={role} onDone={() => { setAction('none'); reload() }} />}
    </section>
  )
}

function CoverageTable({ version }: { version: PolicyDetail['versions'][number] }) {
  return (
    <div className="table-wrap">
      <table>
        <thead>
          <tr>
            <th>Teminat</th>
            <th className="num">Limit</th>
            <th>Muafiyet</th>
            <th className="num">Prim</th>
          </tr>
        </thead>
        <tbody>
          {version.coverages.map((c) => (
            <tr key={c.code}>
              <td data-label="Teminat">{c.name ?? c.code}</td>
              <td data-label="Limit" className="num">{formatTL(c.limit_kurus)}</td>
              <td data-label="Muafiyet">
                {c.deductible_type === 'NONE' ? 'yok' : c.deductible_type === 'FIXED' ? formatTL(c.deductible_value) : `%${c.deductible_value / 100}`}
              </td>
              <td data-label="Prim" className="num">{c.premium_kurus !== undefined ? formatTL(c.premium_kurus) : '—'}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}

// CoverageQuery seçilen tarihte geçerli sürümü ve teminatları sunucudan sorar.
function CoverageQuery({ policy }: { policy: PolicyDetail }) {
  const today = todayISO()
  const [date, setDate] = useState(today >= policy.start_date && today < policy.end_date ? today : policy.start_date)
  const [state, setState] = useState<
    { kind: 'idle' } | { kind: 'loading' } | { kind: 'none' } | { kind: 'error'; message: string } | { kind: 'ok'; data: CoverageOnDate }
  >({ kind: 'idle' })

  async function onSubmit(e: FormEvent) {
    e.preventDefault()
    setState({ kind: 'loading' })
    try {
      const data = await api<CoverageOnDate>(`/api/v1/policies/${policy.id}/coverage?date=${date}`)
      setState({ kind: 'ok', data })
    } catch (err) {
      if (err instanceof ApiError && err.code === 'NO_COVERAGE_ON_DATE') setState({ kind: 'none' })
      else setState({ kind: 'error', message: errorText(err) })
    }
  }

  return (
    <div className="coverage-query">
      <h4>Tarihte geçerli teminat</h4>
      <form className="inline-form" onSubmit={onSubmit}>
        <input type="date" value={date} required onChange={(e) => { setDate(e.target.value); setState({ kind: 'idle' }) }} />
        <button type="submit" className="secondary" disabled={state.kind === 'loading'}>Sorgula</button>
      </form>
      {state.kind === 'loading' && <Loading text="Sorgulanıyor…" />}
      {state.kind === 'none' && <div className="notice">{date} tarihinde geçerli teminat yok (dönem dışı veya iptal sonrası).</div>}
      {state.kind === 'error' && <ErrorBox message={state.message} />}
      {state.kind === 'ok' && (
        <>
          <p className="muted">
            {state.data.date}: sürüm v{state.data.version.version_no} ({state.data.version.effective_from} başlangıçlı)
            {state.data.version.vehicle_plate ? ` · ${state.data.version.vehicle_plate}` : ''}
          </p>
          <CoverageTable version={state.data.version} />
        </>
      )}
    </div>
  )
}
