import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { faro } from '@grafana/faro-web-sdk'
import { App } from './App'

vi.mock('@grafana/faro-web-sdk', () => ({ faro: { api: { pushError: vi.fn() } } }))
vi.mock('../modules/Documents', () => { throw new TypeError('Synthetic Documents import failure') })
vi.mock('../modules/PublicSigning', () => { throw new TypeError('Synthetic signing import failure') })
vi.mock('../modules/Contacts', () => ({ Contacts: () => <h1>Contacts</h1> }))

beforeEach(() => {
  vi.spyOn(console, 'error').mockImplementation(() => {})
  vi.spyOn(window, 'scrollTo').mockImplementation(() => {})
  vi.mocked(faro.api.pushError).mockClear()
  vi.stubGlobal('fetch', vi.fn(async (url: string) => new Response(JSON.stringify(
    url === '/api/v1/me' ? { name: 'Test Operator', email: 'test@example.com', role: 'owner' } : { modules: [] },
  ), { headers: { 'Content-Type': 'application/json' } })))
})

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
})

describe.each([390, 1440])('app import failure at width %i', (width) => {
  it('preserves the shell and allows navigation away from a failed Documents import', async () => {
    Object.defineProperty(window, 'innerWidth', { configurable: true, value: width })
    window.history.replaceState({}, '', '/documents')
    render(<App />)

    expect(await screen.findByRole('button', { name: 'Reload page' })).toBeVisible()
    expect(screen.getByRole('navigation', { name: 'Workspace' })).toBeVisible()
    expect(faro.api.pushError).toHaveBeenCalledWith(expect.any(Error))

    fireEvent.click(screen.getByRole('link', { name: 'Contacts' }))

    expect(await screen.findByRole('heading', { name: 'Contacts' })).toBeVisible()
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
  })

  it('offers recovery when the public signing page cannot load', async () => {
    Object.defineProperty(window, 'innerWidth', { configurable: true, value: width })
    window.history.replaceState({}, '', '/sign?token=synthetic')
    render(<App />)

    expect(await screen.findByRole('button', { name: 'Reload page' })).toBeVisible()
    expect(faro.api.pushError).toHaveBeenCalledWith(expect.any(Error))
    expect(window.location.search).toBe('?token=synthetic')
  })
})
