import { AlertCircle, Inbox } from 'lucide-react'
import type { ReactNode } from 'react'

export function EmptyState({ title, detail, action }: { title: string; detail: string; action?: ReactNode }) {
  return <div className="empty-state"><span className="empty-icon"><Inbox size={22} /></span><strong>{title}</strong><p>{detail}</p>{action}</div>
}

export function ErrorState({ message, retry }: { message: string; retry?: () => void }) {
  return <div className="error-state" role="alert"><AlertCircle size={20} /><span><strong>Something went sideways.</strong><small>{message}</small></span>{retry && <button className="secondary-button" onClick={retry}>Try again</button>}</div>
}

export function LoadingState({ label = 'Loading your workspace' }: { label?: string }) {
  return <div className="loading-state" role="status"><span className="spinner" />{label}</div>
}
