import { useEffect, useState } from 'react'
import { Link, useNavigate, useParams, useSearchParams } from 'react-router'
import { getAvailability, getReviews, getSpace, photoUrl, type Space } from '../api'
import AIUsageGuide from '../components/AIUsageGuide'
import { ErrorState, Loading } from '../components/Layout'
import PhotoPlaceholder from '../components/PhotoPlaceholder'
import { Stars } from '../components/StarRating'
import Timetable, { type Range } from '../components/Timetable'
import {
  addDays,
  BOOKING_WINDOW_DAYS,
  CATEGORY_LABEL,
  dateLabel,
  hoursLabel,
  rangeLabel,
  todayInSeoul,
  won,
} from '../format'
import { useAsync } from '../useAsync'

function Gallery({ space }: { space: Space }) {
  const photos = space.photoIds ?? []
  const [current, setCurrent] = useState(0)
  if (photos.length === 0) {
    return (
      <div className="gallery">
        <div className="gallery__main">
          <PhotoPlaceholder category={space.category} label={space.name} />
        </div>
      </div>
    )
  }
  const shown = photos.slice(0, 8)
  return (
    <div className="gallery">
      <div className="gallery__main">
        <img src={photoUrl(space.id, shown[current] ?? shown[0])} alt={`${space.name} 사진 ${current + 1}`} />
      </div>
      {shown.length > 1 && (
        <div className="gallery__thumbs">
          {shown.map((pid, i) => (
            <button
              key={pid}
              type="button"
              className={i === current ? 'active' : ''}
              onClick={() => setCurrent(i)}
              aria-label={`사진 ${i + 1} 보기`}
            >
              <img src={photoUrl(space.id, pid)} alt="" loading="lazy" />
            </button>
          ))}
        </div>
      )}
    </div>
  )
}

function Reviews({ space }: { space: Space }) {
  const reviews = useAsync(() => getReviews(space.id), [space.id])
  return (
    <section className="detail-section" aria-labelledby="reviews-title">
      <h2 id="reviews-title">
        후기 {space.reviewCount > 0 && <small>★ {space.rating.toFixed(1)} ({space.reviewCount})</small>}
      </h2>
      {reviews.loading && <Loading />}
      {reviews.error && <ErrorState message={reviews.error} onRetry={reviews.reload} />}
      {reviews.data && reviews.data.length === 0 && (
        <p className="muted">아직 작성된 후기가 없습니다. 이용 후 첫 후기를 남겨 주세요.</p>
      )}
      <ul className="review-list">
        {reviews.data?.map((r) => (
          <li key={r.id}>
            <div className="review-list__head">
              <Stars value={r.rating} />
              <span>{r.guestName}</span>
              <time dateTime={r.createdAt}>{new Date(r.createdAt).toLocaleDateString('ko-KR')}</time>
            </div>
            <p>{r.body}</p>
          </li>
        ))}
      </ul>
    </section>
  )
}

export default function SpaceDetailPage() {
  const id = Number(useParams().id)
  const [params] = useSearchParams()
  const navigate = useNavigate()
  const today = todayInSeoul()
  const lastDay = addDays(today, BOOKING_WINDOW_DAYS)
  const initialDate = params.get('date') ?? ''
  const [date, setDate] = useState(initialDate >= today && initialDate <= lastDay ? initialDate : today)
  const [range, setRange] = useState<Range | null>(null)
  const [headcount, setHeadcount] = useState(Number(params.get('headcount')) || 1)

  const space = useAsync(() => getSpace(id), [id])
  const availability = useAsync(() => getAvailability(id, date), [id, date])
  useEffect(() => setRange(null), [date])

  useEffect(() => {
    if (space.data) document.title = `${space.data.name} | 모임룸`
    return () => {
      document.title = '모임룸 | 시간 단위 공간 예약'
    }
  }, [space.data])

  if (space.loading) return <Loading />
  if (space.error || !space.data) {
    return (
      <div className="container page">
        <ErrorState message={space.error ?? '공간을 찾을 수 없습니다.'} />
        <p className="center">
          <Link to="/spaces">다른 공간 둘러보기</Link>
        </p>
      </div>
    )
  }
  const s = space.data
  const people = Math.min(Math.max(headcount, 1), s.capacity)

  function book() {
    if (!range) return
    const q = new URLSearchParams({
      date,
      start: String(range.start),
      end: String(range.end),
      headcount: String(people),
    })
    navigate(`/spaces/${id}/book?${q}`)
  }

  return (
    <div className="container page detail">
      <div className="detail__main">
        <Gallery space={s} />
        <header className="detail__header">
          <p className="space-card__meta">
            {CATEGORY_LABEL[s.category]} <span aria-hidden="true">|</span> {s.address}
          </p>
          <h1>{s.name}</h1>
          <p className="detail__summary">{s.summary}</p>
          <dl className="facts">
            <div>
              <dt>최대 인원</dt>
              <dd>{s.capacity}인</dd>
            </div>
            <div>
              <dt>운영 시간</dt>
              <dd>{hoursLabel(s.openHour, s.closeHour)}</dd>
            </div>
            <div>
              <dt>시간당 요금</dt>
              <dd>{won(s.pricePerHour)}</dd>
            </div>
          </dl>
        </header>

        <section className="detail-section">
          <h2>공간 소개</h2>
          <p className="prewrap">{s.description}</p>
        </section>
        {s.amenities.length > 0 && (
          <section className="detail-section">
            <h2>편의시설</h2>
            <ul className="amenities">
              {s.amenities.map((a) => (
                <li key={a}>{a}</li>
              ))}
            </ul>
          </section>
        )}
        {(s.rules?.length ?? 0) > 0 && (
          <section className="detail-section">
            <h2>이용 규칙</h2>
            <ul className="rules">
              {s.rules!.map((r) => (
                <li key={r}>{r}</li>
              ))}
            </ul>
          </section>
        )}
        <AIUsageGuide key={s.id} spaceId={s.id} />
        <Reviews space={s} />
      </div>

      <aside className="booking-panel" aria-label="예약하기">
        <div className="booking-panel__price">
          <strong>{won(s.pricePerHour)}</strong> / 시간
        </div>
        <label className="field">
          <span>날짜</span>
          <input type="date" value={date} min={today} max={lastDay} onChange={(e) => e.target.value && setDate(e.target.value)} />
        </label>
        <div className="field">
          <span>{dateLabel(date)} 예약 현황</span>
          {availability.loading && !availability.data && <Loading />}
          {availability.error && <ErrorState message={availability.error} onRetry={availability.reload} />}
          {availability.data && !availability.error && (
            <Timetable hours={availability.data.hours} value={range} onChange={setRange} />
          )}
        </div>
        <label className="field">
          <span>인원</span>
          <input
            type="number"
            min={1}
            max={s.capacity}
            value={people}
            onChange={(e) => setHeadcount(Number(e.target.value) || 1)}
          />
        </label>
        <div className="booking-panel__total">
          {range ? (
            <>
              <span>
                {rangeLabel(range.start, range.end)} ({range.end - range.start}시간)
              </span>
              <strong>{won((range.end - range.start) * s.pricePerHour)}</strong>
            </>
          ) : (
            <span className="muted">이용할 시간을 선택해 주세요.</span>
          )}
        </div>
        <button type="button" className="btn btn--primary btn--block" disabled={!range} onClick={book}>
          예약하기
        </button>
      </aside>
    </div>
  )
}
