// Realistic traffic for moimroom.
//
//   k6 run -e BASE_URL=https://example.com ops/k6/load.js
//
// Env: BASE_URL, RATE (iterations/s, default 20), DURATION (default 10m),
// HOST_CODE (enables photo uploads), RECORDS_URL and RECORDS_TOKEN (optional
// collector for written primary keys). See docs/operations.md.

import http from 'k6/http'
import exec from 'k6/execution'
import { check } from 'k6'
import { Counter } from 'k6/metrics'

const BASE_URL = (__ENV.BASE_URL || 'http://localhost:8080').replace(/\/+$/, '')
const RATE = Number(__ENV.RATE || 20)
const DURATION = __ENV.DURATION || '10m'
const HOST_CODE = __ENV.HOST_CODE || ''
const RECORDS_URL = __ENV.RECORDS_URL || ''
const RECORDS_TOKEN = __ENV.RECORDS_TOKEN || ''

const FLUSH_INTERVAL_MS = 2000
const FLUSH_MAX_RECORDS = 200

const PHOTO = open('./room.jpg', 'b')

export const options = {
  scenarios: {
    site: {
      executor: 'constant-arrival-rate',
      rate: RATE,
      timeUnit: '1s',
      duration: DURATION,
      preAllocatedVUs: Math.max(10, RATE),
      maxVUs: Math.max(50, RATE * 5),
    },
  },
  // Only site traffic counts; collector calls are tagged traffic:records.
  thresholds: {
    'http_req_failed{traffic:site}': ['rate<0.01'],
  },
}

const bookingsCreated = new Counter('bookings_created')
const bookingConflicts = new Counter('booking_conflicts')
const bookingNoSlot = new Counter('booking_no_free_slot')
const slotsCreated = new Counter('slots_created')
const reviewsCreated = new Counter('reviews_created')
const photosUploaded = new Counter('photos_uploaded')
const recordsTotal = new Counter('records_total')
const recordsSent = new Counter('records_sent')
const recordsSendFailed = new Counter('records_send_failed')

// Per-VU state: every VU runs its own copy of this module.
let buffer = []
let lastFlush = Date.now()
let retryAfter = 0
const myCodes = [] // booking codes this VU may review

const SURNAMES = ['김', '이', '박', '최', '정', '강', '조', '윤', '장', '임', '한', '오', '서', '신', '권']
const GIVEN = ['민준', '서연', '도윤', '지우', '하준', '서윤', '시우', '하은', '지호', '수아', '예준', '지민', '유진', '현우', '채원']
const REVIEWS = [
  '깔끔하고 조용해서 스터디하기 좋았어요.',
  '사진보다 공간이 넓고 채광이 좋았습니다.',
  '화이트보드와 모니터가 있어 회의가 수월했어요.',
  '위치가 역에서 가까워서 찾기 쉬웠습니다.',
  '친구들과 생일 파티를 했는데 분위기가 좋았어요.',
  '냉난방이 잘 되고 의자가 편했습니다. 다음에도 이용할게요.',
]
const FILTERS = [
  {},
  { area: '강남' },
  { area: '홍대' },
  { area: '성수' },
  { area: '잠실' },
  { area: '여의도' },
  { area: '신촌' },
  { category: 'study' },
  { category: 'meeting', minCapacity: 8 },
  { category: 'party', sort: 'rating' },
  { maxPrice: 20000, sort: 'price_asc' },
]

const pick = (arr) => arr[Math.floor(Math.random() * arr.length)]
const randInt = (min, max) => min + Math.floor(Math.random() * (max - min + 1))

function site(name, extra = {}) {
  return Object.assign({ tags: { traffic: 'site', name } }, extra)
}

function json() {
  return { 'Content-Type': 'application/json' }
}

function qs(params) {
  return Object.entries(params)
    .map(([k, v]) => `${encodeURIComponent(k)}=${encodeURIComponent(v)}`)
    .join('&')
}

// Seoul date with an offset, as YYYY-MM-DD.
function seoulDate(offsetDays) {
  return new Date(Date.now() + 9 * 3600e3 + offsetDays * 86400e3).toISOString().slice(0, 10)
}

export function setup() {
  const res = http.get(`${BASE_URL}/api/spaces?limit=100`, site('GET /api/spaces'))
  if (res.status !== 200) throw new Error(`cannot list spaces: HTTP ${res.status}`)
  const spaces = res.json('items').map((s) => ({ id: s.id, capacity: s.capacity }))
  if (spaces.length === 0) throw new Error('no spaces to book')
  return { spaces }
}

function record(table, pk) {
  buffer.push({ table, pk: String(pk) })
  recordsTotal.add(1)
}

function browse(spaces) {
  const r = Math.random()
  if (r < 0.4) {
    const res = http.get(`${BASE_URL}/api/spaces?${qs(pick(FILTERS))}`, site('GET /api/spaces'))
    check(res, { 'list 200': (x) => x.status === 200 })
  } else if (r < 0.7) {
    const s = pick(spaces)
    const res = http.get(`${BASE_URL}/api/spaces/${s.id}`, site('GET /api/spaces/{id}'))
    check(res, { 'detail 200': (x) => x.status === 200 })
    if (Math.random() < 0.5) http.get(`${BASE_URL}/api/spaces/${s.id}/reviews`, site('GET /api/spaces/{id}/reviews'))
  } else {
    const s = pick(spaces)
    const res = http.get(
      `${BASE_URL}/api/spaces/${s.id}/availability?date=${seoulDate(randInt(0, 13))}`,
      site('GET /api/spaces/{id}/availability'),
    )
    check(res, { 'availability 200': (x) => x.status === 200 })
  }
}

// Free runs of 1 to 3 consecutive hours from an availability response.
function freeRange(hours) {
  const starts = hours.filter((h) => h.status === 'free').map((h) => h.hour)
  if (starts.length === 0) return null
  const start = pick(starts)
  const free = new Set(starts)
  let end = start + 1
  const want = randInt(1, 3)
  while (end - start < want && free.has(end)) end++
  return { start, end }
}

function book(spaces) {
  const s = pick(spaces)
  const date = seoulDate(randInt(0, 13))
  const av = http.get(`${BASE_URL}/api/spaces/${s.id}/availability?date=${date}`, site('GET /api/spaces/{id}/availability'))
  if (av.status !== 200) return
  const range = freeRange(av.json('hours'))
  if (!range) {
    bookingNoSlot.add(1)
    return
  }
  const body = {
    spaceId: s.id,
    date,
    startHour: range.start,
    endHour: range.end,
    headcount: randInt(1, Math.min(4, s.capacity)),
    guestName: pick(SURNAMES) + pick(GIVEN),
    phone: `010-${randInt(1000, 9999)}-${randInt(1000, 9999)}`,
  }
  // Another visitor may take the hours between the two calls; 409 is expected.
  const res = http.post(
    `${BASE_URL}/api/reservations`,
    JSON.stringify(body),
    site('POST /api/reservations', {
      headers: json(),
      responseCallback: http.expectedStatuses({ min: 200, max: 299 }, 409),
    }),
  )
  if (res.status === 409) {
    bookingConflicts.add(1)
    return
  }
  if (!check(res, { 'booking 201': (x) => x.status === 201 })) return
  const r = res.json()
  bookingsCreated.add(1)
  record('app.reservations', r.id)
  // Column order matches PRIMARY KEY (space_id, date, hour).
  for (let h = r.startHour; h < r.endHour; h++) {
    record('app.reservation_slots', `(${r.spaceId},${r.date},${h})`)
    slotsCreated.add(1)
  }
  myCodes.push(r.code)
}

function review(spaces) {
  const code = myCodes.shift()
  if (!code) return book(spaces) // nothing to review yet
  const body = { code, rating: randInt(3, 5), body: pick(REVIEWS) }
  const res = http.post(`${BASE_URL}/api/reviews`, JSON.stringify(body), site('POST /api/reviews', { headers: json() }))
  if (check(res, { 'review 201': (x) => x.status === 201 })) {
    reviewsCreated.add(1)
    record('app.reviews', res.json('id'))
  }
}

function upload(spaces) {
  if (!HOST_CODE) return browse(spaces)
  const s = pick(spaces)
  const res = http.post(
    `${BASE_URL}/api/host/spaces/${s.id}/photos`,
    { photo: http.file(PHOTO, 'room.jpg', 'image/jpeg') },
    site('POST /api/host/spaces/{id}/photos', { headers: { 'X-Host-Code': HOST_CODE } }),
  )
  if (check(res, { 'photo 201': (x) => x.status === 201 })) photosUploaded.add(1)
}

// Sends buffered records every 2 s or 200 records. Failed sends keep the
// buffer and retry after the interval. Near the end of the run every VU
// flushes on each iteration so little is left behind.
function flush() {
  if (buffer.length === 0) return
  const now = Date.now()
  const ending = exec.scenario.progress >= 0.97
  if (!ending && buffer.length < FLUSH_MAX_RECORDS && now - lastFlush < FLUSH_INTERVAL_MS) return
  if (now < retryAfter) return
  lastFlush = now
  if (!RECORDS_URL) {
    buffer = [] // counted in records_total only
    return
  }
  const res = http.post(RECORDS_URL, JSON.stringify({ records: buffer }), {
    headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${RECORDS_TOKEN}` },
    tags: { traffic: 'records', name: 'records flush' },
    timeout: '10s',
  })
  if (res.status >= 200 && res.status < 300) {
    recordsSent.add(buffer.length)
    buffer = []
  } else {
    recordsSendFailed.add(buffer.length)
    retryAfter = now + FLUSH_INTERVAL_MS
  }
}

export default function (data) {
  const r = Math.random()
  if (r < 0.7) browse(data.spaces)
  else if (r < 0.9) book(data.spaces)
  else if (r < 0.97) review(data.spaces)
  else upload(data.spaces)
  flush()
}

export function handleSummary(data) {
  const count = (name) => (data.metrics[name] ? data.metrics[name].values.count : 0)
  const failed = data.metrics['http_req_failed{traffic:site}']
  const siteReqs = failed ? failed.values.passes + failed.values.fails : 0
  const failRate = failed ? failed.values.rate : 0
  const ok = failed ? Object.values(failed.thresholds || {}).every((t) => t.ok) : true
  const p95 = data.metrics.http_req_duration ? data.metrics.http_req_duration.values['p(95)'] : 0
  const lines = [
    '',
    'moimroom load summary',
    `  site requests        ${siteReqs}`,
    `  site failure rate    ${(failRate * 100).toFixed(2)}% (threshold <1%: ${ok ? 'pass' : 'FAIL'})`,
    `  p95 latency          ${p95.toFixed(1)} ms`,
    `  iterations           ${count('iterations')}`,
    '  successful writes',
    `    reservations       ${count('bookings_created')}`,
    `    reservation slots  ${count('slots_created')}`,
    `    reviews            ${count('reviews_created')}`,
    `    photos             ${count('photos_uploaded')}`,
    `  booking conflicts    ${count('booking_conflicts')} (409, expected)`,
    `  no free slot         ${count('booking_no_free_slot')}`,
    `  records produced     ${count('records_total')}`,
    `  records sent         ${RECORDS_URL ? count('records_sent') : 'skipped (RECORDS_URL unset)'}`,
    `  record send failures ${count('records_send_failed')}`,
    '',
  ]
  return { stdout: lines.join('\n') }
}
