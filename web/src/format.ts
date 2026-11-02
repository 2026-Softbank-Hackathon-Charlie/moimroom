import type { Category } from './api'

// Keep in sync with the server's list (internal/api).
export const AREAS = ['강남', '홍대', '성수', '잠실', '여의도', '신촌'] as const

export const CATEGORY_LABEL: Record<Category, string> = {
  study: '스터디룸',
  meeting: '회의실',
  party: '파티룸',
}

export const BOOKING_WINDOW_DAYS = 60
export const MAX_BOOKING_HOURS = 12

export const won = (n: number) => `${n.toLocaleString('ko-KR')}원`

export const hourLabel = (h: number) => `${String(h).padStart(2, '0')}:00`

export const rangeLabel = (start: number, end: number) => `${hourLabel(start)} ~ ${hourLabel(end)}`

export function hoursLabel(open: number, close: number): string {
  return open === 0 && close === 24 ? '24시간 운영' : rangeLabel(open, close)
}

const seoulDate = new Intl.DateTimeFormat('en-CA', {
  timeZone: 'Asia/Seoul',
  year: 'numeric',
  month: '2-digit',
  day: '2-digit',
})

const seoulHour = new Intl.DateTimeFormat('en-GB', { timeZone: 'Asia/Seoul', hour: '2-digit', hour12: false })

/** Today's date in Seoul as YYYY-MM-DD, whatever the browser's time zone. */
export const todayInSeoul = (now = new Date()) => seoulDate.format(now)

export const hourInSeoul = (now = new Date()) => Number(seoulHour.format(now)) % 24

export function addDays(date: string, days: number): string {
  const d = new Date(`${date}T00:00:00Z`)
  d.setUTCDate(d.getUTCDate() + days)
  return d.toISOString().slice(0, 10)
}

const WEEKDAYS = ['일', '월', '화', '수', '목', '금', '토']

/** "2026-11-03" becomes "11월 3일 (화)". */
export function dateLabel(date: string): string {
  const d = new Date(`${date}T00:00:00Z`)
  return `${d.getUTCMonth() + 1}월 ${d.getUTCDate()}일 (${WEEKDAYS[d.getUTCDay()]})`
}

/** True once the reservation's first hour has started (Seoul time). */
export function hasStarted(date: string, startHour: number, now = new Date()): boolean {
  const today = todayInSeoul(now)
  return date < today || (date === today && startHour <= hourInSeoul(now))
}

export function formatPhone(raw: string): string {
  const d = raw.replace(/\D/g, '').slice(0, 11)
  if (d.length < 4) return d
  if (d.length < 8) return `${d.slice(0, 3)}-${d.slice(3)}`
  const mid = d.length === 10 ? 3 : 4
  return `${d.slice(0, 3)}-${d.slice(3, 3 + mid)}-${d.slice(3 + mid)}`
}
