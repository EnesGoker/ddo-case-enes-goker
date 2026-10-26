import { useEffect, useState } from 'react'
import { api, type Role } from '../api/client'
import type { ClaimTask, TaskWorkload } from '../api/types'
import { ClaimDetailLoader } from '../components/ClaimDetail'
import { Empty, ErrorBox, Loading } from '../components/States'
import { TaskTable } from '../components/TaskTable'
import { errorText, statusLabels } from '../format'
import { usePaged } from '../hooks/usePaged'

export function TasksPage({ role, fraudQueue = false }: { role: Role; fraudQueue?: boolean }) {
  const [status, setStatus] = useState('open')
  const [type, setType] = useState(fraudQueue ? 'FRAUD_REVIEW' : '')
  const [breached, setBreached] = useState(false)
  const [selected, setSelected] = useState<number | null>(null)
  const query = new URLSearchParams({ limit: '20', status, task_type: type, breached: String(breached) })
  const { state, reload, loadMore } = usePaged<ClaimTask>(`/api/v1/claim-tasks?${query}`)
  return <section>
    <div className="form-grid panel filter-panel">
      {!fraudQueue && <><label>Durum<select value={status} onChange={(e) => { setStatus(e.target.value); setSelected(null) }}><option value="open">Açık</option><option value="done">Kapanmış</option><option value="all">Tümü</option></select></label>
        <label>Görev tipi<select value={type} onChange={(e) => { setType(e.target.value); setSelected(null) }}><option value="">Tümü</option>{['APPRAISAL', 'ADJUSTMENT', 'FRAUD_REVIEW'].map((t) => <option key={t} value={t}>{statusLabels[t]}</option>)}</select></label></>}
      <label className="check"><input type="checkbox" checked={breached} onChange={(e) => { setBreached(e.target.checked); setSelected(null) }} />Yalnızca SLA ihlali</label>
    </div>
    <button className="secondary" onClick={reload}>Listeyi yenile</button>
    {role === 'APPRAISER' && <p>Yalnızca size atanan görevler gösterilir.</p>}
    {state.kind === 'loading' && <Loading />}
    {state.kind === 'error' && <ErrorBox message={state.message} onRetry={reload} />}
    {state.kind === 'ready' && (state.items.length === 0 ? <Empty text="Bu filtreye uygun görev yok." /> : <>
      <TaskTable tasks={state.items} onSelect={setSelected} />
      {state.next && <button disabled={state.loadingMore} onClick={loadMore}>{state.loadingMore ? 'Yükleniyor…' : 'Daha fazla'}</button>}
    </>)}
    {selected !== null && <ClaimDetailLoader id={selected} role={role} onChanged={reload} />}
    {role === 'CLAIMS_ADJUSTER' && <Workload />}
  </section>
}

function Workload() {
  const [items, setItems] = useState<TaskWorkload[] | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [attempt, setAttempt] = useState(0)
  useEffect(() => {
    let cancelled = false
    api<{ items: TaskWorkload[] }>('/api/v1/claim-tasks/workload').then((r) => { if (!cancelled) { setItems(r.items); setError(null) } }).catch((e: unknown) => !cancelled && setError(errorText(e)))
    return () => { cancelled = true }
  }, [attempt])
  return <section className="panel"><h3>Uzman iş yükü</h3><button className="secondary" onClick={() => setAttempt((a) => a + 1)}>İş yükünü yenile</button>
    {error ? <ErrorBox message={error} onRetry={() => setAttempt((a) => a + 1)} /> : !items ? <Loading /> : items.length === 0 ? <Empty text="Aktif uzman yok." /> : <ul className="workload-list">{items.map((u) => <li key={u.id}>{u.full_name} · {u.role === 'APPRAISER' ? 'Eksper' : 'Hasar uzmanı'} · {u.open_tasks} açık görev</li>)}</ul>}
  </section>
}
