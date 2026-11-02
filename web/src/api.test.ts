import { afterEach, describe, expect, it, vi } from 'vitest'
import { ApiError, createReservation, hostUploadPhoto, listSpaces, lookupReservation } from './api'

function mockFetch(status: number, body: unknown) {
  const fn = vi.fn().mockResolvedValue(new Response(JSON.stringify(body), { status }))
  vi.stubGlobal('fetch', fn)
  return fn
}

afterEach(() => vi.unstubAllGlobals())

describe('api client', () => {
  it('builds the space query and skips empty filters', async () => {
    const fetch = mockFetch(200, { items: [] })
    await listSpaces({ area: '성수', category: '', minCapacity: 0, maxPrice: 30000, sort: 'price_asc' })
    const url = fetch.mock.calls[0][0] as string
    expect(url.startsWith('/api/spaces?')).toBe(true)
    const q = new URLSearchParams(url.split('?')[1])
    expect(Object.fromEntries(q)).toEqual({ area: '성수', maxPrice: '30000', sort: 'price_asc' })
  })

  it('sends JSON bodies with POST', async () => {
    const fetch = mockFetch(201, { id: 1, code: 'K7QM-29XA' })
    await createReservation({
      spaceId: 3,
      date: '2026-11-03',
      startHour: 10,
      endHour: 12,
      headcount: 2,
      guestName: '김민수',
      phone: '010-1234-5678',
    })
    const [url, init] = fetch.mock.calls[0] as [string, RequestInit]
    expect(url).toBe('/api/reservations')
    expect(init.method).toBe('POST')
    expect(new Headers(init.headers).get('Content-Type')).toBe('application/json')
    expect(JSON.parse(init.body as string)).toMatchObject({ spaceId: 3, startHour: 10, endHour: 12 })
  })

  it('surfaces the server message for client errors', async () => {
    mockFetch(409, { error: '선택하신 시간에 이미 예약이 있습니다.' })
    const err = await lookupReservation('X', '1234').catch((e: unknown) => e)
    expect(err).toBeInstanceOf(ApiError)
    expect((err as ApiError).status).toBe(409)
    expect((err as ApiError).message).toBe('선택하신 시간에 이미 예약이 있습니다.')
  })

  it('uses one friendly message for server errors', async () => {
    mockFetch(503, { error: 'database: connection refused' })
    const err = (await listSpaces().catch((e: unknown) => e)) as ApiError
    expect(err.status).toBe(503)
    expect(err.message).not.toContain('database')
  })

  it('reports network failures', async () => {
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new TypeError('failed')))
    const err = (await listSpaces().catch((e: unknown) => e)) as ApiError
    expect(err.status).toBe(0)
  })

  it('uploads photos as multipart with the host code header', async () => {
    const fetch = mockFetch(201, { id: 9, spaceId: 3 })
    const file = new File([new Uint8Array([137, 80, 78, 71])], 'room.png', { type: 'image/png' })
    await hostUploadPhoto('host-123', 3, file)
    const [url, init] = fetch.mock.calls[0] as [string, RequestInit]
    expect(url).toBe('/api/host/spaces/3/photos')
    expect(new Headers(init.headers).get('X-Host-Code')).toBe('host-123')
    // The browser must set the multipart boundary itself.
    expect(new Headers(init.headers).has('Content-Type')).toBe(false)
    expect((init.body as FormData).get('photo')).toBeInstanceOf(File)
  })
})
