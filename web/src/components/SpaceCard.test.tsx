import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router'
import { describe, expect, it } from 'vitest'
import type { Space } from '../api'
import SpaceCard from './SpaceCard'

const base: Space = {
  id: 7,
  name: '포커스랩 강남 4인 스터디룸',
  category: 'study',
  area: '강남',
  address: '서울 강남구 역삼동',
  summary: '강남역 5분, 집중하기 좋은 4인 소형 스터디룸',
  capacity: 4,
  pricePerHour: 8000,
  openHour: 7,
  closeHour: 24,
  amenities: ['와이파이'],
  rating: 0,
  reviewCount: 0,
  coverPhotoId: null,
  createdAt: '2026-11-01T00:00:00Z',
}

const renderCard = (space: Space, search?: string) =>
  render(
    <MemoryRouter>
      <SpaceCard space={space} search={search} />
    </MemoryRouter>,
  )

describe('SpaceCard', () => {
  it('shows a placeholder and a new badge before photos and reviews exist', () => {
    renderCard(base)
    expect(screen.getByRole('img', { name: `${base.name} 사진 준비 중` })).toBeInTheDocument()
    expect(screen.getByText('새로 등록')).toBeInTheDocument()
    expect(screen.getByText(/8,000원/)).toBeInTheDocument()
    expect(screen.getByText(/최대 4인/)).toBeInTheDocument()
  })

  it('uses the cover photo and rating when present', () => {
    renderCard({ ...base, coverPhotoId: 31, rating: 4.25, reviewCount: 8 }, '?date=2026-11-03')
    expect(screen.getByRole('img', { name: base.name })).toHaveAttribute('src', '/api/spaces/7/photos/31')
    expect(screen.getByLabelText('평점 4.3점, 후기 8개')).toBeInTheDocument()
    expect(screen.getByRole('link')).toHaveAttribute('href', '/spaces/7?date=2026-11-03')
  })
})
