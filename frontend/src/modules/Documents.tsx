import { FormEvent, useCallback, useEffect, useState } from 'react'
import { Edit3, FilePlus2, History, Save } from 'lucide-react'
import ReactMarkdown from 'react-markdown'
import { Account, api, Contact, Cost, Document, DocumentRevision, Opportunity, RecordLink, shortDate } from '../api'
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
  const [linkOptions, setLinkOptions] = useState<LinkOption[]>([])

  const load = useCallback(() => {
    setLoading(true)
    Promise.all([
      api<{ documents: Document[] }>('/api/v1/documents'),
      api<{ accounts: Account[] }>('/api/v1/accounts'),
      api<{ contacts: Contact[] }>('/api/v1/contacts'),
      api<{ opportunities: Opportunity[] }>('/api/v1/opportunities'),
      api<{ costs: Cost[] }>('/api/v1/costs'),
    ]).then(([documents, accounts, contacts, opportunities, costs]) => {
      setItems(documents.documents)
      setSelectedID((current) => current || documents.documents[0]?.id || '')
      setLinkOptions([
        ...accounts.accounts.map((item) => ({ type: 'account' as const, id: item.id, label: item.name })),
        ...contacts.contacts.map((item) => ({ type: 'contact' as const, id: item.id, label: item.name })),
        ...opportunities.opportunities.map((item) => ({ type: 'opportunity' as const, id: item.id, label: item.name })),
        ...costs.costs.map((item) => ({ type: 'cost' as const, id: item.id, label: item.description })),
        ...documents.documents.map((item) => ({ type: 'document' as const, id: item.id, label: item.title })),
      ])
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
      setLinkOptions((current) => [...current, { type: 'document', id: created.id, label: created.title }])
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
      {items.length ? <div className="document-layout"><aside className="document-list" aria-label="Documents">{items.map((item) => <button className={item.id === selectedID ? 'active' : ''} key={item.id} onClick={() => { setSelectedID(item.id); setEditing(false) }}><strong>{item.title}</strong><small>Updated {shortDate(item.updatedAt)}</small></button>)}</aside><section className="document-sheet">{selected && (editing ? <form className="document-editor" onSubmit={save}><input aria-label="Document title" name="title" defaultValue={selected.title} maxLength={160} required /><textarea aria-label="Markdown body" name="body" defaultValue={selected.body} maxLength={100000} autoFocus /><LinkFields links={selected.links} options={linkOptions} />{formError && <p className="form-error" role="alert">{formError}</p>}<div className="form-actions"><button type="button" className="secondary-button" onClick={() => setEditing(false)}>Cancel</button><button className="primary-button"><Save size={16} /> Save document</button></div></form> : <><header><div><p className="eyebrow">Document · Revision {selected.revision || 1}</p><h2>{selected.title}</h2><small>Updated {shortDate(selected.updatedAt)}</small></div><button className="secondary-button" onClick={() => setEditing(true)}><Edit3 size={16} /> Edit</button></header>{!!selected.links?.length && <div className="linked-records">{selected.links.map((link) => <a href={recordHref(link)} key={`${link.type}:${link.id}`}>{linkLabel(link, linkOptions)}</a>)}</div>}<article className="markdown"><ReactMarkdown>{selected.body || '_This document is empty. Choose Edit to start writing._'}</ReactMarkdown></article>{!!revisions.length && <p className="revision-note"><History size={15} /> {revisions.length} prior version{revisions.length === 1 ? '' : 's'} safely retained</p>}</>)}</section></div> : <EmptyState title="No documents yet" detail="Create a handbook, client brief, checklist, or anything else worth remembering." action={<button className="primary-button" onClick={() => setCreating(true)}><FilePlus2 size={17} /> Create your first document</button>} />}
    </Page>
    {creating && <Modal eyebrow="Knowledge" title="New document" onClose={() => setCreating(false)}><form onSubmit={create}><label>Title<input name="title" maxLength={160} required autoFocus /></label><label>Start writing in Markdown<textarea name="body" rows={10} maxLength={100000} placeholder="# What this is for" /></label><LinkFields options={linkOptions} />{formError && <p className="form-error" role="alert">{formError}</p>}<div className="form-actions"><button type="button" className="secondary-button" onClick={() => setCreating(false)}>Cancel</button><button className="primary-button">Create document</button></div></form></Modal>}
  </>
}

type LinkOption = RecordLink & { label: string }

function LinkFields({ links = [], options }: { links?: RecordLink[]; options: LinkOption[] }) {
  const [type, setType] = useState<RecordLink['type'] | ''>(links[0]?.type ?? '')
  const [id, setID] = useState(links[0]?.id ?? '')
  const available = options.filter((option) => option.type === type)
  return <div className="field-grid"><label>Link to<select name="linkType" value={type} onChange={(event) => { setType(event.target.value as RecordLink['type'] | ''); setID('') }}><option value="">Nothing yet</option><option value="account">Account</option><option value="contact">Contact</option><option value="opportunity">Opportunity</option><option value="cost">Cost</option><option value="document">Document</option></select></label><label>Linked record<select name="linkId" value={id} disabled={!type} onChange={(event) => setID(event.target.value)}><option value="">{type ? 'Choose a record' : 'Choose what to link first'}</option>{available.map((option) => <option value={option.id} key={`${option.type}:${option.id}`}>{option.label}</option>)}</select></label></div>
}

function documentLinks(form: FormData): RecordLink[] {
  const type = String(form.get('linkType') ?? '') as RecordLink['type']
  const id = String(form.get('linkId') ?? '').trim()
  return type && id ? [{ type, id }] : []
}

function recordHref(link: RecordLink) {
  if (link.type === 'account') return `/accounts/${link.id}`
  if (link.type === 'contact') return `/contacts/${link.id}`
  if (link.type === 'opportunity') return '/opportunities'
  if (link.type === 'cost') return '/operations'
  return '/documents'
}

function linkLabel(link: RecordLink, options: LinkOption[]) {
  const label = options.find((option) => option.type === link.type && option.id === link.id)?.label ?? 'Linked record'
  return `${link.type[0].toUpperCase()}${link.type.slice(1)} · ${label}`
}
