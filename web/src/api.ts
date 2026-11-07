// Thin client for the JSON API under /api.

export type Category = 'study' | 'meeting' | 'party'

export interface Space {
  id: number
  name: string
  category: Category
  area: string
  address: string
  summary: string
  description?: string
  capacity: number
  pricePerHour: number
  openHour: number
  closeHour: number
  amenities: string[]
  rules?: string[]
  rating: number
  reviewCount: number
  coverPhotoId: number | null
  photoIds?: number[]
  createdAt: string
}

export type HourStatus = 'free' | 'booked' | 'past'

export interface HourSlot {
  hour: number
  status: HourStatus
}

export interface Availability {
  date: string
  openHour: number
  closeHour: number
  hours: HourSlot[]
}

export interface Reservation {
  id: number
  code: string
  spaceId: number
  spaceName: string
  date: string
  startHour: number
  endHour: number
  headcount: number
  totalPrice: number
  guestName: string
  phoneLast4: string
  status: 'confirmed' | 'cancelled'
  reviewed: boolean
  createdAt: string
}

export interface Review {
  id: number
  spaceId: number
  rating: number
  body: string
  guestName: string
  createdAt: string
}

export interface Photo {
  id: number
  spaceId: number
  createdAt: string
}

export class ApiError extends Error {
  readonly status: number
  readonly requestId?: string
  constructor(status: number, message: string, requestId?: string) {
    super(message)
    this.name = 'ApiError'
    this.status = status
    this.requestId = requestId
  }
}

const GENERIC_ERROR = '일시적인 오류가 발생했습니다. 잠시 후 다시 시도해 주세요.'

interface RequestOptions {
  method?: string
  body?: unknown
  hostCode?: string
  signal?: AbortSignal
}

async function request<T>(path: string, opts: RequestOptions = {}): Promise<T> {
  const headers = new Headers()
  let body: BodyInit | undefined
  if (opts.body instanceof FormData) {
    body = opts.body
  } else if (opts.body !== undefined) {
    headers.set('Content-Type', 'application/json')
    body = JSON.stringify(opts.body)
  }
  if (opts.hostCode) headers.set('X-Host-Code', opts.hostCode)

  let res: Response
  try {
    res = await fetch(`/api${path}`, { method: opts.method ?? 'GET', headers, body, signal: opts.signal })
  } catch {
    throw new ApiError(0, '네트워크 연결을 확인해 주세요.')
  }
  const data = (await res.json().catch(() => null)) as { error?: string } | null
  if (!res.ok) {
    // Server errors get one friendly message; client errors carry the
    // server's explanation, which is written for users.
    const message = res.status < 500 && data?.error ? data.error : GENERIC_ERROR
    throw new ApiError(res.status, message, res.headers.get('X-Request-Id') ?? undefined)
  }
  return data as T
}

function query(params: Record<string, string | number | undefined>): string {
  const q = new URLSearchParams()
  for (const [k, v] of Object.entries(params)) {
    if (v !== undefined && v !== '' && v !== 0) q.set(k, String(v))
  }
  const s = q.toString()
  return s ? `?${s}` : ''
}

export interface SpaceQuery {
  area?: string
  category?: string
  minCapacity?: number
  maxPrice?: number
  sort?: string
  limit?: number
}

export async function listSpaces(q: SpaceQuery = {}): Promise<Space[]> {
  const r = await request<{ items: Space[] }>(`/spaces${query({ ...q })}`)
  return r.items
}

export const getSpace = (id: number) => request<Space>(`/spaces/${id}`)

export interface AIBrief {
  text: string
  requestId: string
  backend: 'gcp' | 'laptop'
  durationMs: number
}

export const generateSpaceBrief = (id: number, signal?: AbortSignal) =>
  request<AIBrief>(`/spaces/${id}/ai-brief`, { method: 'POST', signal })

export const getAvailability = (id: number, date: string) =>
  request<Availability>(`/spaces/${id}/availability${query({ date })}`)

export async function getReviews(id: number): Promise<Review[]> {
  const r = await request<{ items: Review[] }>(`/spaces/${id}/reviews`)
  return r.items
}

export const photoUrl = (spaceId: number, photoId: number) => `/api/spaces/${spaceId}/photos/${photoId}`

export interface NewReservation {
  spaceId: number
  date: string
  startHour: number
  endHour: number
  headcount: number
  guestName: string
  phone: string
}

export const createReservation = (input: NewReservation) =>
  request<Reservation>('/reservations', { method: 'POST', body: input })

export const lookupReservation = (code: string, phoneLast4: string) =>
  request<Reservation>('/reservations/lookup', { method: 'POST', body: { code, phoneLast4 } })

export const cancelReservation = (code: string, phoneLast4: string) =>
  request<Reservation>('/reservations/cancel', { method: 'POST', body: { code, phoneLast4 } })

export const createReview = (input: { code: string; rating: number; body: string }) =>
  request<Review>('/reviews', { method: 'POST', body: input })

export interface NewSpace {
  name: string
  category: Category
  area: string
  address: string
  summary: string
  description: string
  capacity: number
  pricePerHour: number
  openHour: number
  closeHour: number
  amenities: string[]
  rules: string[]
}

export async function hostListSpaces(hostCode: string): Promise<Space[]> {
  const r = await request<{ items: Space[] }>('/host/spaces', { hostCode })
  return r.items
}

export const hostCreateSpace = (hostCode: string, input: NewSpace) =>
  request<Space>('/host/spaces', { method: 'POST', body: input, hostCode })

export function hostUploadPhoto(hostCode: string, spaceId: number, file: File) {
  const form = new FormData()
  form.append('photo', file)
  return request<Photo>(`/host/spaces/${spaceId}/photos`, { method: 'POST', body: form, hostCode })
}

export async function hostReservations(
  hostCode: string,
  params: { spaceId?: number; from?: string } = {},
): Promise<Reservation[]> {
  const r = await request<{ items: Reservation[] }>(`/host/reservations${query(params)}`, { hostCode })
  return r.items
}
