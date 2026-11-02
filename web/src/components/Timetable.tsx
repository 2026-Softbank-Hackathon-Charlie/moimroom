import type { HourSlot } from '../api'
import { MAX_BOOKING_HOURS } from '../format'

export interface Range {
  start: number
  end: number // exclusive
}

interface Props {
  hours: HourSlot[]
  value: Range | null
  onChange: (r: Range | null) => void
}

/**
 * Next selection after a click on hour h: the first click picks one hour, a
 * later free hour extends the range when every hour in between is free, and
 * clicking a lone selected hour clears it.
 */
export function nextRange(hours: HourSlot[], current: Range | null, h: number): Range | null {
  const free = (x: number) => hours.some((s) => s.hour === x && s.status === 'free')
  if (!free(h)) return current
  if (current && current.end - current.start === 1) {
    if (h === current.start) return null
    if (h > current.start && h + 1 - current.start <= MAX_BOOKING_HOURS) {
      let ok = true
      for (let x = current.start; x <= h; x++) ok &&= free(x)
      if (ok) return { start: current.start, end: h + 1 }
    }
  }
  return { start: h, end: h + 1 }
}

const STATUS_LABEL = { free: '예약 가능', booked: '예약 마감', past: '지난 시간' } as const

export default function Timetable({ hours, value, onChange }: Props) {
  if (hours.length === 0) return <p className="muted">이 날짜에는 운영하지 않습니다.</p>
  const selected = (h: number) => value !== null && h >= value.start && h < value.end
  return (
    <div>
      <div className="timetable" role="group" aria-label="시간 선택">
        {hours.map((s) => (
          <button
            key={s.hour}
            type="button"
            className={`slot slot--${s.status}${selected(s.hour) ? ' slot--selected' : ''}`}
            disabled={s.status !== 'free'}
            aria-pressed={selected(s.hour)}
            aria-label={`${s.hour}시 ${selected(s.hour) ? '선택됨' : STATUS_LABEL[s.status]}`}
            onClick={() => onChange(nextRange(hours, value, s.hour))}
          >
            {s.hour}
          </button>
        ))}
      </div>
      <ul className="legend" aria-hidden="true">
        <li>
          <i className="slot--free" />
          예약 가능
        </li>
        <li>
          <i className="slot--selected" />
          선택
        </li>
        <li>
          <i className="slot--booked" />
          예약 마감
        </li>
      </ul>
      <p className="hint">시작 시간을 누른 뒤 마지막 이용 시간을 누르면 범위가 선택됩니다.</p>
    </div>
  )
}
