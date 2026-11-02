import { useState, type FormEvent } from 'react'
import { Link, useNavigate } from 'react-router'
import { listSpaces } from '../api'
import SpaceCard from '../components/SpaceCard'
import { ErrorState, Loading } from '../components/Layout'
import { addDays, AREAS, BOOKING_WINDOW_DAYS, CATEGORY_LABEL, todayInSeoul } from '../format'
import { useAsync } from '../useAsync'

const CATEGORY_COPY = {
  study: '조용히 집중하는 2인부터 8인까지의 스터디 공간',
  meeting: '화면 공유와 화이트보드를 갖춘 업무용 회의실',
  party: '생일, 모임, 작은 행사를 위한 파티 공간',
} as const

export default function HomePage() {
  const navigate = useNavigate()
  const today = todayInSeoul()
  const [area, setArea] = useState('')
  const [date, setDate] = useState(today)
  const [headcount, setHeadcount] = useState(2)
  const featured = useAsync(() => listSpaces({ sort: 'recommended', limit: 6 }), [])

  function onSearch(e: FormEvent) {
    e.preventDefault()
    const q = new URLSearchParams()
    if (area) q.set('area', area)
    if (date) q.set('date', date)
    if (headcount > 1) q.set('headcount', String(headcount))
    navigate(`/spaces?${q}`)
  }

  return (
    <>
      <section className="hero">
        <div className="container">
          <h1>
            필요한 시간만큼,
            <br />
            딱 맞는 공간을 빌리세요
          </h1>
          <p className="hero__lead">스터디룸부터 회의실, 파티룸까지 서울 곳곳의 공간을 시간 단위로 바로 예약할 수 있습니다.</p>
          <form className="search-box" onSubmit={onSearch} aria-label="공간 검색">
            <label>
              <span>지역</span>
              <select value={area} onChange={(e) => setArea(e.target.value)}>
                <option value="">서울 전체</option>
                {AREAS.map((a) => (
                  <option key={a} value={a}>
                    {a}
                  </option>
                ))}
              </select>
            </label>
            <label>
              <span>날짜</span>
              <input
                type="date"
                value={date}
                min={today}
                max={addDays(today, BOOKING_WINDOW_DAYS)}
                onChange={(e) => setDate(e.target.value)}
              />
            </label>
            <label>
              <span>인원</span>
              <input
                type="number"
                min={1}
                max={50}
                value={headcount}
                onChange={(e) => setHeadcount(Math.max(1, Number(e.target.value) || 1))}
              />
            </label>
            <button type="submit" className="btn btn--primary">
              공간 검색
            </button>
          </form>
          <div className="area-chips" aria-label="인기 지역">
            {AREAS.map((a) => (
              <Link key={a} to={`/spaces?area=${encodeURIComponent(a)}`} className="chip">
                {a}
              </Link>
            ))}
          </div>
        </div>
      </section>

      <section className="container section">
        <h2>어떤 공간이 필요하세요?</h2>
        <div className="category-grid">
          {(Object.keys(CATEGORY_LABEL) as (keyof typeof CATEGORY_LABEL)[]).map((c) => (
            <Link key={c} to={`/spaces?category=${c}`} className={`category-card category-card--${c}`}>
              <strong>{CATEGORY_LABEL[c]}</strong>
              <span>{CATEGORY_COPY[c]}</span>
            </Link>
          ))}
        </div>
      </section>

      <section className="container section">
        <div className="section__head">
          <h2>지금 많이 찾는 공간</h2>
          <Link to="/spaces">전체 보기</Link>
        </div>
        {featured.loading && <Loading />}
        {featured.error && <ErrorState message={featured.error} onRetry={featured.reload} />}
        {featured.data && (
          <div className="card-grid">
            {featured.data.map((s) => (
              <SpaceCard key={s.id} space={s} />
            ))}
          </div>
        )}
      </section>

      <section className="container section steps">
        <h2>예약은 이렇게 진행돼요</h2>
        <ol>
          <li>
            <strong>공간 고르기</strong>
            <span>지역, 인원, 가격으로 원하는 공간을 찾습니다.</span>
          </li>
          <li>
            <strong>시간 선택</strong>
            <span>예약 현황표에서 비어 있는 시간을 골라 바로 예약합니다.</span>
          </li>
          <li>
            <strong>예약 코드로 관리</strong>
            <span>예약 코드와 휴대폰 번호 뒤 4자리로 확인하고 취소할 수 있습니다.</span>
          </li>
        </ol>
      </section>
    </>
  )
}
