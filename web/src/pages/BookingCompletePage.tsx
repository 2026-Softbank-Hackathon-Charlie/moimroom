import { useState } from 'react'
import { Link, useLocation } from 'react-router'
import type { Reservation } from '../api'
import { dateLabel, rangeLabel, won } from '../format'

export default function BookingCompletePage() {
  const reservation = (useLocation().state as { reservation?: Reservation } | null)?.reservation
  const [copied, setCopied] = useState(false)

  if (!reservation) {
    return (
      <div className="container page narrow center">
        <h1 className="page__title">예약 내역은 예약 확인에서 볼 수 있어요</h1>
        <p className="muted">예약 코드와 휴대폰 번호 뒤 4자리로 언제든 조회할 수 있습니다.</p>
        <Link to="/reservations" className="btn btn--primary">
          예약 확인
        </Link>
      </div>
    )
  }
  const r = reservation

  async function copy() {
    try {
      await navigator.clipboard.writeText(r.code)
      setCopied(true)
    } catch {
      setCopied(false)
    }
  }

  return (
    <div className="container page narrow">
      <div className="complete">
        <p className="complete__check" aria-hidden="true">
          ✓
        </p>
        <h1>예약이 완료되었습니다</h1>
        <p className="muted">아래 예약 코드를 꼭 보관해 주세요. 예약 확인과 취소, 후기 작성에 필요합니다.</p>
        <div className="code-box">
          <span className="code-box__label">예약 코드</span>
          <strong className="code-box__code">{r.code}</strong>
          <button type="button" className="btn btn--ghost btn--small" onClick={copy}>
            {copied ? '복사됨' : '복사'}
          </button>
        </div>
      </div>
      <section className="summary-card">
        <h2>{r.spaceName}</h2>
        <dl>
          <div>
            <dt>날짜</dt>
            <dd>{dateLabel(r.date)}</dd>
          </div>
          <div>
            <dt>시간</dt>
            <dd>{rangeLabel(r.startHour, r.endHour)}</dd>
          </div>
          <div>
            <dt>인원</dt>
            <dd>{r.headcount}명</dd>
          </div>
          <div>
            <dt>예약자</dt>
            <dd>
              {r.guestName} (휴대폰 뒤 4자리 {r.phoneLast4})
            </dd>
          </div>
          <div>
            <dt>결제 예정 금액</dt>
            <dd className="strong">{won(r.totalPrice)}</dd>
          </div>
        </dl>
      </section>
      <div className="actions">
        <Link to={`/reservations?code=${encodeURIComponent(r.code)}`} className="btn btn--ghost">
          예약 확인
        </Link>
        <Link to={`/spaces/${r.spaceId}`} className="btn btn--primary">
          공간 정보 보기
        </Link>
      </div>
    </div>
  )
}
