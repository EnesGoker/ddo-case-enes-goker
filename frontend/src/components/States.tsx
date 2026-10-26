import { UIIcon } from './UIIcon'

export function Loading({ text = 'Yükleniyor…' }: { text?: string }) {
  return <div className="notice" role="status"><span className="notice-copy"><span className="spinner" aria-hidden="true" />{text}</span></div>
}

export function Empty({ text }: { text: string }) {
  return <div className="empty-state"><UIIcon name="empty" /><p>{text}</p></div>
}

export function ErrorBox({ message, onRetry }: { message: string; onRetry?: () => void }) {
  return (
    <div className="notice error" role="alert">
      <span className="notice-copy"><UIIcon name="alert" /><span>{message}</span></span>
      {onRetry && (
        <button type="button" className="secondary" onClick={onRetry}>
          Tekrar dene
        </button>
      )}
    </div>
  )
}
