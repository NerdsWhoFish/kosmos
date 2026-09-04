import { useCallback, useEffect, useState } from 'react'
import { ArrowUpRight, Search } from 'lucide-react'
import { api, SearchResult } from '../api'
import { Page } from '../components/Page'
import { EmptyState, ErrorState, LoadingState } from '../components/States'

export function SearchResults({ query, navigate }: { query: string; navigate: (path: string) => void }) {
  const [items, setItems] = useState<SearchResult[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const load = useCallback(() => {
    setLoading(true)
    api<{ results: SearchResult[] }>(`/api/v1/search?q=${encodeURIComponent(query)}`).then((response) => setItems(response.results)).catch((reason: Error) => setError(reason.message)).finally(() => setLoading(false))
  }, [query])
  useEffect(load, [load])
  if (loading) return <LoadingState label="Searching Kosmos" />
  if (error) return <ErrorState message={error} retry={load} />
  return <Page eyebrow="Search" title={`Results for “${query}”`} detail="People, opportunities, documents, and costs from your private workspace.">
    {items.length ? <div className="search-results">{items.map((item) => <button key={`${item.kind}-${item.id}`} onClick={() => navigate(item.href)}><span className="search-result-icon"><Search size={17} /></span><span><small>{item.kind}</small><strong>{item.title}</strong><p>{item.subtitle}</p></span><ArrowUpRight size={17} /></button>)}</div> : <EmptyState title="Nothing matched" detail="Try a person's name, company, document title, vendor, or opportunity." />}
  </Page>
}
