import { FormEvent, useCallback, useEffect, useState } from 'react'
import { Edit3, FilePlus2, History, Save } from 'lucide-react'
import ReactMarkdown from 'react-markdown'
import { api, Document, DocumentRevision, RecordLink, shortDate } from '../api'
import { Modal } from '../components/Modal'
import { Page } from '../components/Page'
import { EmptyState, ErrorState, LoadingState } from '../components/States'

export function Documents() {
  const [items, setItems] = useState<Document[]>([])
  const [selectedID, setSelectedID] = useState('')
  const [creating, setCreating] = useState(false)
  const [editing, setEditing] = useState(false)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [formError, setFormError] = useState('')
  const [revisions, setRevisions] = useState<DocumentRevision[]>([])

  const load = useCallback(() => {
    setLoading(true)
    api<{ documents: Document[] }>('/api/v1/documents').then((response) => {
      setItems(response.documents)
      setSelectedID((current) => current || response.documents[0]?.id || '')
    }).catch((reason: Error) => setError(reason.message)).finally(() => setLoading(false))
  }, [])
  useEffect(load, [load])

  const selected = items.find((item) => item.id === selectedID)

  useEffect(() => {
    if (!selectedID) return
    api<{ revisions: DocumentRevision[] }>(`/api/v1/documents/${selectedID}/revisions`).then((response) => setRevisions(response.revisions ?? [])).catch(() => setRevisions([]))
  }, [selectedID])

  async function create(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    setFormError('')
    const form = new FormData(event.currentTarget)
    try {
      const created = await api<Document>('/api/v1/documents', { method: 'POST', body: JSON.stringify({ title: form.get('title'), body: form.get('body'), links: documentLinks(form) }) })
      setItems((current) => [created, ...current])
      setSelectedID(created.id)
      setCreating(false)
    } catch (reason) {
      setFormError(reason instanceof Error ? reason.message : 'Could not save document')
    }
  }

  async function save(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (!selected) return
    const form = new FormData(event.currentTarget)
    setFormError('')
    try {
      const updated = await api<Document>(`/api/v1/documents/${selected.id}`, { method: 'PATCH', body: JSON.stringify({ title: form.get('title'), body: form.get('body'), links: documentLinks(form) }) })
      setItems((current) => current.map((item) => item.id === updated.id ? updated : item))
      setEditing(false)
      const history = await api<{ revisions: DocumentRevision[] }>(`/api/v1/documents/${selected.id}/revisions`)
      setRevisions(history.revisions)
    } catch (reason) {
      setFormError(reason instanceof Error ? reason.message : 'Could not save document')
    }
  }

  if (loading) return <LoadingState label="Opening your documents" />
  if (error) return <ErrorState message={error} retry={load} />

  return <>
    <Page eyebrow="Knowledge" title="Documents" detail="Write in Markdown, read it like a polished document, and keep the why beside the work." action={<button className="primary-button" onClick={() => setCreating(true)}><FilePlus2 size={17} /> New document</button>}>
      {items.length ? <div className="document-layout"><aside className="document-list" aria-label="Documents">{items.map((item) => <button className={item.id === selectedID ? 'active' : ''} key={item.id} onClick={() => { setSelectedID(item.id); setEditing(false) }}><strong>{item.title}</strong><small>Updated {shortDate(item.updatedAt)}</small></button>)}</aside><section className="document-sheet">{selected && (editing ? <form className="document-editor" onSubmit={save}><input aria-label="Document title" name="title" defaultValue={selected.title} maxLength={160} required /><textarea aria-label="Markdown body" name="body" defaultValue={selected.body} maxLength={100000} autoFocus /><LinkFields links={selected.links} />{formError && <p className="form-error" role="alert">{formError}</p>}<div className="form-actions"><button type="button" className="secondary-button" onClick={() => setEditing(false)}>Cancel</button><button className="primary-button"><Save size={16} /> Save document</button></div></form> : <><header><div><p className="eyebrow">Document · Revision {selected.revision || 1}</p><h2>{selected.title}</h2><small>Updated {shortDate(selected.updatedAt)}</small></div><button className="secondary-button" onClick={() => setEditing(true)}><Edit3 size={16} /> Edit</button></header>{!!selected.links?.length && <div className="linked-records">{selected.links.map((link) => <a href={recordHref(link)} key={`${link.type}:${link.id}`}>{link.type} · {link.id}</a>)}</div>}<article className="markdown"><ReactMarkdown>{selected.body || '_This document is empty. Choose Edit to start writing._'}</ReactMarkdown></article>{!!revisions.length && <p className="revision-note"><History size={15} /> {revisions.length} prior version{revisions.length === 1 ? '' : 's'} safely retained</p>}</>)}</section></div> : <EmptyState title="No documents yet" detail="Create a handbook, client brief, checklist, or anything else worth remembering." action={<button className="primary-button" onClick={() => setCreating(true)}><FilePlus2 size={17} /> Create your first document</button>} />}
    </Page>
    {creating && <Modal eyebrow="Knowledge" title="New document" onClose={() => setCreating(false)}><form onSubmit={create}><label>Title<input name="title" maxLength={160} required autoFocus /></label><label>Start writing in Markdown<textarea name="body" rows={10} maxLength={100000} placeholder="# What this is for" /></label><LinkFields />{formError && <p className="form-error" role="alert">{formError}</p>}<div className="form-actions"><button type="button" className="secondary-button" onClick={() => setCreating(false)}>Cancel</button><button className="primary-button">Create document</button></div></form></Modal>}
  </>
}

function LinkFields({ links = [] }: { links?: RecordLink[] }) {
  return <div className="field-grid"><label>Link to<select name="linkType" defaultValue={links[0]?.type ?? ''}><option value="">Nothing yet</option><option value="account">Account</option><option value="contact">Contact</option><option value="opportunity">Opportunity</option><option value="cost">Cost</option><option value="document">Document</option></select></label><label>Record ID<input name="linkId" defaultValue={links[0]?.id ?? ''} placeholder="Paste its ID" /></label></div>
}

function documentLinks(form: FormData): RecordLink[] {
  const type = String(form.get('linkType') ?? '') as RecordLink['type']
  const id = String(form.get('linkId') ?? '').trim()
  return type && id ? [{ type, id }] : []
}

function recordHref(link: RecordLink) {
  if (link.type === 'contact') return `/contacts/${link.id}`
  if (link.type === 'opportunity') return '/opportunities'
  if (link.type === 'cost') return '/costs'
  return '/documents'
}
