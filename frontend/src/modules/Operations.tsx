import { FormEvent, useCallback, useEffect, useState } from 'react'
import { Download, FileUp, RefreshCw, Sheet, UploadCloud } from 'lucide-react'
import { AcceptedJob, api, Attachment, Contact, Cost, Document, GoogleStatus, money, Opportunity, Transaction } from '../api'
import { Page } from '../components/Page'
import { EmptyState, ErrorState, LoadingState } from '../components/States'
import { Costs } from './Costs'

export function Operations() {
  const [google, setGoogle] = useState<GoogleStatus | null>(null)
  const [transactions, setTransactions] = useState<Transaction[]>([])
  const [attachments, setAttachments] = useState<Attachment[]>([])
  const [costs, setCosts] = useState<Cost[]>([])
  const [loading, setLoading] = useState(true)
  const [notice, setNotice] = useState('')
  const [error, setError] = useState('')
	const [syncing, setSyncing] = useState(false)
	const [recordType, setRecordType] = useState('document')
	const [records, setRecords] = useState<Record<string, { id: string; label: string }[]>>({ document: [], cost: [], contact: [], opportunity: [] })

  const load = useCallback(() => {
    Promise.all([api<GoogleStatus>('/api/v1/integrations/google'), api<{ transactions: Transaction[] }>('/api/v1/transactions'), api<{ attachments: Attachment[] }>('/api/v1/attachments'), api<{ documents: Document[] }>('/api/v1/documents'), api<{ costs: Cost[] }>('/api/v1/costs'), api<{ contacts: Contact[] }>('/api/v1/contacts'), api<{ opportunities: Opportunity[] }>('/api/v1/opportunities')])
      .then(([connection, imported, files, documents, costsResponse, contacts, opportunities]) => { setGoogle(connection); setTransactions(imported.transactions); setAttachments(files.attachments); setCosts(costsResponse.costs); setRecords({ document: documents.documents.map((item) => ({ id: item.id, label: item.title })), cost: costsResponse.costs.map((item) => ({ id: item.id, label: `${item.description} (${item.vendor || 'No vendor'})` })), contact: contacts.contacts.map((item) => ({ id: item.id, label: item.name })), opportunity: opportunities.opportunities.map((item) => ({ id: item.id, label: item.name })) }); setError('') })
      .catch((reason: Error) => setError(reason.message)).finally(() => setLoading(false))
  }, [])
  useEffect(load, [load])

  async function configureTiller(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    const form = new FormData(event.currentTarget)
    try {
      await api('/api/v1/integrations/tiller', { method: 'PUT', body: JSON.stringify({ spreadsheetId: form.get('spreadsheetId'), range: form.get('range') }) })
      setNotice('Tiller sheet connected.')
      load()
    } catch (reason) { setNotice(reason instanceof Error ? reason.message : 'Could not connect Tiller') }
  }

  async function syncTiller() {
    setSyncing(true)
    setNotice('Queueing a Tiller import...')
    try {
      await api<AcceptedJob>('/api/v1/integrations/tiller/sync', { method: 'POST' })
      setNotice('Tiller import queued. Imported transactions and anything needing review will appear here when it finishes.')
    } catch (reason) {
      setNotice(reason instanceof Error ? reason.message : 'Could not sync Tiller')
    } finally {
      setSyncing(false)
    }
  }

  async function upload(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
	const formElement = event.currentTarget
    const form = new FormData(event.currentTarget)
    const response = await fetch('/api/v1/attachments', { method: 'POST', headers: { 'X-Kosmos-CSRF': '1' }, body: form })
    if (!response.ok) {
      const body = await response.json().catch(() => ({})) as { error?: { message?: string } }
      setNotice(body.error?.message ?? 'Could not upload file')
      return
    }
    formElement.reset()
    setNotice('File stored privately.')
    load()
  }

  async function review(item: Transaction, status: 'matched' | 'ignored') {
    await api(`/api/v1/transactions/${item.id}`, { method: 'PATCH', body: JSON.stringify({ contactId: item.contactId ?? '', costId: item.costId ?? '', matchStatus: status }) })
    load()
  }

  if (loading) return <LoadingState />
  if (error) return <ErrorState message={error} retry={load} />

  const reviewCount = transactions.filter((item) => item.matchStatus === 'review').length
  return <Page eyebrow="Back office" title="Business operations" detail="Manage costs, transactions, receipts, private files, and accounting exports in one place.">
    {notice && <p className="inline-notice" role="status">{notice}</p>}
    <Costs embedded initialItems={costs} />
    <section className="split-grid">
      <div className="panel"><div className="panel-heading"><div><p className="eyebrow">Tiller</p><h2>Transaction import</h2></div>{google?.connection?.tiller && <button className="text-button" onClick={syncTiller} disabled={syncing}><RefreshCw size={15} /> {syncing ? 'Queueing...' : 'Sync now'}</button>}</div>
        {!google?.connected ? <EmptyState title="Connect Google first" detail="Tiller lives in Google Sheets, so the Google Workspace connection provides read-only access." /> : <form onSubmit={configureTiller}><label>Spreadsheet ID<input name="spreadsheetId" defaultValue={google.connection?.tiller?.spreadsheetId} required placeholder="From the Google Sheets URL" /></label><label>Sheet range<input name="range" defaultValue={google.connection?.tiller?.range ?? 'Transactions!A:Z'} required /></label><button className="primary-button"><Sheet size={16} /> Save Tiller connection</button></form>}
      </div>
      <div className="panel"><p className="eyebrow">Portable by design</p><h2>Exports</h2><p className="muted-copy">Download ordinary CSV files. Your business data is not trapped in Kosmos.</p><div className="button-stack"><a className="secondary-button" href="/api/v1/exports/contacts"><Download size={16} /> Export contacts</a><a className="secondary-button" href="/api/v1/exports/costs"><Download size={16} /> Export costs</a></div></div>
    </section>
    <section className="panel lower-panel"><div className="panel-heading"><div><p className="eyebrow">Review queue</p><h2>{reviewCount ? `${reviewCount} transaction${reviewCount === 1 ? '' : 's'} need a decision` : 'Imported transactions'}</h2></div></div>{transactions.length ? <div className="table-list">{transactions.map((item) => <article className="transaction-row" key={item.id}><span><strong>{item.merchant || item.description}</strong><small>{item.date} · {item.description}</small></span><strong>{money(item.amountCents)}</strong><span className={`status-badge ${item.matchStatus}`}>{item.matchStatus}</span>{item.matchStatus === 'review' && <span className="row-actions"><button className="text-button" onClick={() => review(item, 'matched')}>Confirm</button><button className="text-button" onClick={() => review(item, 'ignored')}>Ignore</button></span>}</article>)}</div> : <EmptyState title="No transactions imported" detail="Connect a Tiller sheet and sync it. Exact customer matches are linked automatically; ambiguous rows wait for you." />}</section>
    <section className="split-grid lower-grid">
      <div className="panel"><p className="eyebrow">Private storage</p><h2>Upload a file or receipt</h2><form onSubmit={upload} encType="multipart/form-data"><label>File<input name="file" type="file" accept="image/jpeg,image/png,image/webp,application/pdf,text/plain" required /></label><div className="field-grid"><label>Type<select name="kind"><option value="attachment">Attachment</option><option value="receipt">Receipt</option></select></label><label>Linked record type<select name="recordType" value={recordType} onChange={(event) => setRecordType(event.target.value)}><option value="document">Document</option><option value="cost">Cost</option><option value="contact">Contact</option><option value="opportunity">Opportunity</option></select></label></div><label>Linked record<select name="recordId" required defaultValue=""><option value="" disabled>Choose a record</option>{records[recordType].map((item) => <option value={item.id} key={item.id}>{item.label}</option>)}</select></label><button className="primary-button"><UploadCloud size={16} /> Upload privately</button></form></div>
      <div className="panel"><p className="eyebrow">Files</p><h2>Recent uploads</h2>{attachments.length ? <div className="file-list">{attachments.map((item) => <a className="record-row compact" href={item.downloadUrl} key={item.id}><FileUp size={18} /><span><strong>{item.fileName}</strong><small>{item.kind} · {Math.ceil(item.size / 1024)} KB</small></span></a>)}</div> : <EmptyState title="No files yet" detail="Receipts and attachments are private, versioned, and served through expiring links." />}</div>
    </section>
  </Page>
}
