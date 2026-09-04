import { ReactNode } from 'react'

export function Page({ eyebrow, title, detail, action, children }: { eyebrow: string; title: string; detail: string; action?: ReactNode; children: ReactNode }) {
  return <div className="page"><header className="page-heading"><div><p className="eyebrow">{eyebrow}</p><h1>{title}</h1><p className="subhead">{detail}</p></div>{action}</header>{children}</div>
}
