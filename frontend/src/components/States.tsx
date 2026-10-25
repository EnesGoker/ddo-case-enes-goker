export function Loading({ text = 'Yükleniyor…' }: { text?: string }) {
  return (
    <div className="notice" role="status" aria-live="polite">
      <span className="spinner" aria-hidden="true" /> {text}
    </div>
  )
}

export function Empty({ text }: { text: string }) {
  return <div className="notice">{text}</div>
}

export function ErrorBox({ message, onRetry }: { message: string; onRetry?: () => void }) {
  return (
    <div className="notice error" role="alert">
      <span>{message}</span>
      {onRetry && (
        <button type="button" className="secondary" onClick={onRetry}>
          Tekrar dene
        </button>
      )}
    </div>
  )
}
