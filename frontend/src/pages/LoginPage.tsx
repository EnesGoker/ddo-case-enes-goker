import { useState, type FormEvent } from 'react'
import { ApiError } from '../api/client'
import { useAuth } from '../auth/useAuth'

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
      <form className="card" onSubmit={onSubmit} noValidate>
        <h1>GüvenPoliçe</h1>
        <p className="muted">Sigorta yönetim sistemi — giriş</p>
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
      </form>
    </main>
  )
}
