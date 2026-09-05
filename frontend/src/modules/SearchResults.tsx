import { useCallback, useEffect, useState } from 'react'
import { ArrowUpRight, Search } from 'lucide-react'
import { api, SearchResult } from '../api'
import { Page } from '../components/Page'
import { EmptyState, ErrorState, LoadingState } from '../components/States'
import { useAsyncLoad } from '../useAsyncLoad'

export function SearchResults({ query, navigate }: { query: string; navigate: (path: string) => void }) {
  const [items, setItems] = useState<SearchResult[]>([])
  const { loading, error, run } = useAsyncLoad()
  const load = useCallback(() => {
    void run(() => api<{ results: SearchResult[] }>(`/api/v1/search?q=${encodeURIComponent(query)}`), (response) => setItems(response.results))
  }, [query, run])
  useEffect(load, [load])
  if (loading) return <LoadingState label="Searching Kosmos" />
  if (error) return <ErrorState message={error} retry={load} />
  return <Page eyebrow="Search" title={`Results for “${query}”`} detail="People, opportunities, documents, and costs from your private workspace.">
    {items.length ? <div className="search-results">{items.map((item) => <button key={`${item.kind}-${item.id}`} onClick={() => navigate(item.href)}><span className="search-result-icon"><Search size={17} /></span><span><small>{item.kind}</small><strong>{item.title}</strong><p>{item.subtitle}</p></span><ArrowUpRight size={17} /></button>)}</div> : <EmptyState title="Nothing matched" detail="Try a person's name, company, document title, vendor, or opportunity." />}
  </Page>
}
