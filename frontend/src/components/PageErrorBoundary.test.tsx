import { Suspense, lazy, useState } from 'react'
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { faro } from '@grafana/faro-web-sdk'
import { PageErrorBoundary } from './PageErrorBoundary'

vi.mock('@grafana/faro-web-sdk', () => ({ faro: { api: { pushError: vi.fn() } } }))

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
})

describe.each([390, 1440])('page loading at width %i', (width) => {
  it('keeps navigation and offers a reload when a page import rejects, while reporting the failure', async () => {
    Object.defineProperty(window, 'innerWidth', { configurable: true, value: width })
    vi.spyOn(console, 'error').mockImplementation(() => {})
    const failure = new TypeError('Failed to fetch dynamically imported module')
    const Page = lazy(() => Promise.reject(failure))

    render(<><nav aria-label="Workspace"><a href="/contacts">Contacts</a></nav><PageErrorBoundary resetKey="documents"><Suspense fallback="Loading"><Page /></Suspense></PageErrorBoundary></>)

    expect(await screen.findByRole('alert')).toHaveTextContent('Reload the page to try again.')
    expect(screen.getByRole('button', { name: 'Reload page' })).toBeVisible()
    expect(screen.getByRole('navigation', { name: 'Workspace' })).toBeVisible()
    expect(faro.api.pushError).toHaveBeenCalledWith(failure)
  })

  it('renders a successfully loaded page without reporting an error', async () => {
    const report = vi.mocked(faro.api.pushError).mockClear()
    const Page = lazy(async () => ({ default: () => <h1>Documents</h1> }))

    render(<PageErrorBoundary resetKey="documents"><Suspense fallback="Loading"><Page /></Suspense></PageErrorBoundary>)

    expect(await screen.findByRole('heading', { name: 'Documents' })).toBeVisible()
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
    expect(report).not.toHaveBeenCalled()
  })

  it('reports rendering failures too and permits another page after navigation', async () => {
    vi.spyOn(console, 'error').mockImplementation(() => {})
    const failure = new Error('Synthetic rendering failure')
    function BrokenPage(): never { throw failure }
    const { rerender } = render(<PageErrorBoundary resetKey="documents"><BrokenPage /></PageErrorBoundary>)

    expect(await screen.findByRole('alert')).toBeVisible()
    expect(faro.api.pushError).toHaveBeenCalledWith(failure)

    rerender(<PageErrorBoundary resetKey="contacts"><h1>Contacts</h1></PageErrorBoundary>)
    expect(screen.getByRole('heading', { name: 'Contacts' })).toBeVisible()
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
  })

  it('preserves working page state across route changes', () => {
    function Editor() {
      const [draft, setDraft] = useState('')
      return <input aria-label="Draft" value={draft} onChange={(event) => setDraft(event.target.value)} />
    }
    const { rerender } = render(<PageErrorBoundary resetKey="document"><Editor /></PageErrorBoundary>)
    fireEvent.change(screen.getByRole('textbox'), { target: { value: 'Keep my work' } })

    rerender(<PageErrorBoundary resetKey="document/edit"><Editor /></PageErrorBoundary>)

    expect(screen.getByRole('textbox')).toHaveValue('Keep my work')
  })
})
