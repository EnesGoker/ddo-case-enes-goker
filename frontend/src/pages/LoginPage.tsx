import { useState, type FormEvent } from 'react'
import { ApiError } from '../api/client'
import { useAuth } from '../auth/useAuth'
import { UIIcon } from '../components/UIIcon'

export function LoginPage() {
  const { login } = useAuth()
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState<string | null>(null)

  async function onSubmit(e: FormEvent) {
    e.preventDefault()
    setSubmitting(true)
    setError(null)
    try {
      await login(email, password)
    } catch (err) {
      if (err instanceof ApiError && err.code === 'INVALID_CREDENTIALS') {
        setError('E-posta veya parola hatalı.')
      } else if (err instanceof ApiError && err.code === 'VALIDATION_ERROR') {
        setError(Object.values(err.details ?? {}).join(', ') || err.message)
      } else {
        setError(err instanceof Error ? err.message : 'Giriş yapılamadı.')
      }
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <main className="login">
      <section className="login-story" aria-label="GüvenPoliçe">
        <div className="brand"><UIIcon name="shield" /><div><strong>GüvenPoliçe</strong><small>SİGORTA YÖNETİMİ</small></div></div>
        <div><p className="login-tagline">Her adımda açıklık.<br />Her işlemde güven.</p><p>Tekliften poliçeye, hasardan ödemeye; sigorta süreçlerinizi tek bir çalışma alanında yönetin.</p>
          <div className="login-qualities"><span><UIIcon name="policies" />Poliçe ve teminat yönetimi</span><span><UIIcon name="claims" />Açıklanabilir hasar süreçleri</span><span><UIIcon name="chart" />Mutabakat ve finansal görünürlük</span></div>
        </div>
        <small>Kurgusal demo verileri · Gerçek uygulama akışları</small>
      </section>
      <div className="login-form-area"><form className="card" onSubmit={onSubmit} noValidate>
        <div><div className="eyebrow">GüvenPoliçe'ye hoş geldiniz</div><h1>Oturum açın</h1><p className="muted">Çalışma alanınıza erişmek için hesap bilgilerinizi girin.</p></div>
        <label>
          E-posta
          <input type="email" autoComplete="username" value={email}
            onChange={(e) => setEmail(e.target.value)} required />
        </label>
        <label>
          Parola
          <input type="password" autoComplete="current-password" value={password}
            onChange={(e) => setPassword(e.target.value)} required />
        </label>
        {error && <div className="notice error" role="alert">{error}</div>}
        <button type="submit" disabled={submitting}>
          {submitting ? 'Giriş yapılıyor…' : 'Giriş yap'}
        </button>
        <p className="login-footnote">Erişebileceğiniz kayıtlar ve işlemler<br />hesabınızın rolüne göre belirlenir.</p>
      </form></div>
    </main>
  )
}
