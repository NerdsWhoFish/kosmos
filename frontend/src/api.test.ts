import { afterEach, describe, expect, it, vi } from 'vitest'
import { api } from './api'

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  })
}

describe('api', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('aggregates every page while preserving the list envelope', async () => {
    const fetch = vi.fn()
      .mockResolvedValueOnce(jsonResponse({ contacts: [{ id: 'one' }], page: { limit: 1, nextCursor: 'next-page' } }))
      .mockResolvedValueOnce(jsonResponse({ contacts: [{ id: 'two' }], page: { limit: 1, nextCursor: '' } }))
    vi.stubGlobal('fetch', fetch)

    const result = await api<{ contacts: { id: string }[]; page: { limit: number; nextCursor: string } }>('/api/v1/contacts?limit=1')

    expect(result.contacts).toEqual([{ id: 'one' }, { id: 'two' }])
    expect(result.page).toEqual({ limit: 1, nextCursor: '' })
    expect(fetch).toHaveBeenCalledTimes(2)
    expect(fetch.mock.calls[1][0]).toBe('/api/v1/contacts?limit=1&cursor=next-page')
  })

  it('leaves non-paginated responses unchanged', async () => {
    const response = { contacts: 7, openOpportunities: 2 }
    const fetch = vi.fn().mockResolvedValue(jsonResponse(response))
    vi.stubGlobal('fetch', fetch)

    await expect(api('/api/v1/summary')).resolves.toEqual(response)
    expect(fetch).toHaveBeenCalledTimes(1)
  })

  it('rejects a repeated cursor instead of looping forever', async () => {
    const fetch = vi.fn()
      .mockResolvedValueOnce(jsonResponse({ contacts: [{ id: 'one' }], page: { nextCursor: 'repeat' } }))
      .mockResolvedValueOnce(jsonResponse({ contacts: [{ id: 'two' }], page: { nextCursor: 'repeat' } }))
    vi.stubGlobal('fetch', fetch)

    await expect(api('/api/v1/contacts')).rejects.toThrow('Paginated response repeated a cursor')
    expect(fetch).toHaveBeenCalledTimes(2)
  })
})
