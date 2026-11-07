import { act, fireEvent, render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes, useLocation } from 'react-router'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import {
  ApiError, generateSpaceBrief, getAvailability, getReviews, getSpace,
  type AIBrief, type Space,
} from '../api'
import SpaceDetailPage from './SpaceDetailPage'

vi.mock('../api', async (original) => ({
  ...await original<typeof import('../api')>(),
  getSpace: vi.fn(), getAvailability: vi.fn(), getReviews: vi.fn(), generateSpaceBrief: vi.fn(),
}))

const space: Space = {
  id: 7, name: '테스트 스터디룸', category: 'study', area: '강남', address: '서울 강남구',
  summary: '테스트 공간', description: '조용한 공간', capacity: 4, pricePerHour: 8000,
  openHour: 10, closeHour: 12, amenities: ['와이파이'], rules: ['금연'],
  rating: 0, reviewCount: 0, coverPhotoId: null, createdAt: '2026-11-01T00:00:00Z',
}

function BookingDestination() {
  const location = useLocation()
  return <output aria-label="예약 이동 경로">{location.pathname}{location.search}</output>
}

beforeEach(() => {
  vi.mocked(getSpace).mockReset().mockResolvedValue(space)
  vi.mocked(getReviews).mockReset().mockResolvedValue([])
  vi.mocked(getAvailability).mockReset().mockResolvedValue({
    date: '2026-11-06', openHour: 10, closeHour: 12,
    hours: [{ hour: 10, status: 'free' }, { hour: 11, status: 'free' }],
  })
  vi.mocked(generateSpaceBrief).mockReset()
})

describe('SpaceDetailPage AI isolation', () => {
  it('preserves booking state through an AI failure and keeps the existing booking navigation', async () => {
    const user = userEvent.setup()
    let fail!: (error: unknown) => void
    vi.mocked(generateSpaceBrief).mockImplementationOnce(() => new Promise<AIBrief>((_resolve, reject) => { fail = reject }))
    render(
      <MemoryRouter initialEntries={['/spaces/7']}>
        <Routes>
          <Route path="/spaces/:id" element={<SpaceDetailPage />} />
          <Route path="/spaces/:id/book" element={<BookingDestination />} />
        </Routes>
      </MemoryRouter>,
    )
    await screen.findByRole('heading', { name: '테스트 스터디룸' })
    await user.click(await screen.findByRole('button', { name: '10시 예약 가능' }))
    const people = screen.getByRole('spinbutton', { name: '인원' })
    fireEvent.change(people, { target: { value: '2' } })
    const booking = screen.getByRole('complementary', { name: '예약하기' })
    const book = within(booking).getByRole('button', { name: '예약하기' })
    await user.click(screen.getByRole('button', { name: 'AI 이용 안내 만들기' }))
    expect(book).toBeEnabled()
    expect(people).toHaveValue(2)
    await act(async () => { fail(new ApiError(503, '일시적인 오류가 발생했습니다.')) })
    const panel = screen.getByRole('region', { name: 'AI 이용 안내' })
    expect(within(panel).getByRole('alert')).toHaveTextContent('일시적인 오류')
    expect(within(booking).queryByRole('alert')).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: '10시 선택됨' })).toHaveAttribute('aria-pressed', 'true')
    expect(people).toHaveValue(2)
    expect(book).toBeEnabled()
    await user.click(book)
    const destination = screen.getByLabelText('예약 이동 경로').textContent ?? ''
    expect(destination.startsWith('/spaces/7/book?')).toBe(true)
    const query = new URLSearchParams(destination.split('?')[1])
    expect(query.get('start')).toBe('10')
    expect(query.get('end')).toBe('11')
    expect(query.get('headcount')).toBe('2')
    expect(query.get('date')).toMatch(/^\d{4}-\d{2}-\d{2}$/)
  })
})
