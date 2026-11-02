import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import { describe, expect, it } from 'vitest'
import type { HourSlot } from '../api'
import Timetable, { nextRange, type Range } from './Timetable'

const hours: HourSlot[] = [
  { hour: 9, status: 'past' },
  { hour: 10, status: 'free' },
  { hour: 11, status: 'free' },
  { hour: 12, status: 'booked' },
  { hour: 13, status: 'free' },
  { hour: 14, status: 'free' },
]

function Harness() {
  const [range, setRange] = useState<Range | null>(null)
  return (
    <>
      <Timetable hours={hours} value={range} onChange={setRange} />
      <output>{range ? `${range.start}-${range.end}` : 'none'}</output>
    </>
  )
}

const slot = (h: number) => screen.getByRole('button', { name: new RegExp(`^${h}시`) })

describe('Timetable', () => {
  it('disables booked and past hours', () => {
    render(<Harness />)
    expect(slot(9)).toBeDisabled()
    expect(slot(12)).toBeDisabled()
    expect(slot(10)).toBeEnabled()
  })

  it('selects a start hour and extends to a later free hour', async () => {
    const user = userEvent.setup()
    render(<Harness />)
    await user.click(slot(13))
    expect(screen.getByText('13-14')).toBeInTheDocument()
    await user.click(slot(14))
    expect(screen.getByText('13-15')).toBeInTheDocument()
    expect(slot(13)).toHaveAttribute('aria-pressed', 'true')
  })

  it('does not extend across a booked hour', async () => {
    const user = userEvent.setup()
    render(<Harness />)
    await user.click(slot(10))
    await user.click(slot(13))
    expect(screen.getByText('13-14')).toBeInTheDocument()
  })

  it('clears a lone selected hour and ignores unavailable ones', () => {
    expect(nextRange(hours, { start: 10, end: 11 }, 10)).toBeNull()
    expect(nextRange(hours, { start: 10, end: 12 }, 14)).toEqual({ start: 14, end: 15 })
    expect(nextRange(hours, null, 12)).toBeNull()
  })

  it('explains when there are no hours that day', () => {
    render(<Timetable hours={[]} value={null} onChange={() => {}} />)
    expect(screen.getByText('이 날짜에는 운영하지 않습니다.')).toBeInTheDocument()
  })
})
