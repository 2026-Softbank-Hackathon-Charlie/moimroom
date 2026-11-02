import { describe, expect, it } from 'vitest'
import { addDays, dateLabel, formatPhone, hasStarted, hoursLabel, todayInSeoul, won } from './format'

describe('format', () => {
  it('formats money, hours and dates', () => {
    expect(won(22000)).toBe('22,000원')
    expect(hoursLabel(0, 24)).toBe('24시간 운영')
    expect(hoursLabel(9, 22)).toBe('09:00 ~ 22:00')
    expect(addDays('2026-12-30', 3)).toBe('2027-01-02')
    expect(dateLabel('2026-11-03')).toBe('11월 3일 (화)')
  })

  it('uses Seoul time for today and start checks', () => {
    // 15:30 UTC is 00:30 the next day in Seoul.
    const now = new Date('2026-11-02T15:30:00Z')
    expect(todayInSeoul(now)).toBe('2026-11-03')
    expect(hasStarted('2026-11-03', 0, now)).toBe(true)
    expect(hasStarted('2026-11-03', 1, now)).toBe(false)
    expect(hasStarted('2026-11-02', 23, now)).toBe(true)
  })

  it('formats phone numbers while typing', () => {
    expect(formatPhone('01012345678')).toBe('010-1234-5678')
    expect(formatPhone('0111234567')).toBe('011-123-4567')
    expect(formatPhone('0101')).toBe('010-1')
  })
})
