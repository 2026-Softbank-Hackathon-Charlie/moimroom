import { useState, type FormEvent } from 'react'
import { Link, useSearchParams } from 'react-router'
import { cancelReservation, lookupReservation, type Reservation } from '../api'
import { dateLabel, hasStarted, rangeLabel, won } from '../format'
import { errorMessage } from '../useAsync'

export function ReservationCard({ r }: { r: Reservation }) {
  return (
    <section className="summary-card">
      <div className="summary-card__head">
        <h2>{r.spaceName}</h2>
        <span className={`status status--${r.status}`}>{r.status === 'confirmed' ? '예약 확정' : '취소됨'}</span>
      </div>
      <dl>
        <div>
          <dt>예약 코드</dt>
          <dd className="mono">{r.code}</dd>
        </div>
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
          <dd>{r.guestName}</dd>
        </div>
        <div>
          <dt>결제 예정 금액</dt>
          <dd className="strong">{won(r.totalPrice)}</dd>
        </div>
      </dl>
    </section>
  )
}

export default function LookupPage() {
  const [params] = useSearchParams()
  const [code, setCode] = useState(params.get('code') ?? '')
  const [last4, setLast4] = useState('')
  const [reservation, setReservation] = useState<Reservation>()
  const [error, setError] = useState<string>()
  const [busy, setBusy] = useState(false)
  const [notice, setNotice] = useState<string>()

  async function onLookup(e: FormEvent) {
    e.preventDefault()
    setBusy(true)
    setError(undefined)
    setNotice(undefined)
    try {
      setReservation(await lookupReservation(code, last4))
    } catch (err) {
      setReservation(undefined)
      setError(errorMessage(err))
    } finally {
      setBusy(false)
    }
  }

  async function onCancel() {
    if (!reservation || !window.confirm('예약을 취소할까요? 취소한 예약은 되돌릴 수 없습니다.')) return
    setBusy(true)
    setError(undefined)
    try {
      setReservation(await cancelReservation(reservation.code, last4))
      setNotice('예약이 취소되었습니다.')
    } catch (err) {
      setError(errorMessage(err))
    } finally {
      setBusy(false)
    }
  }

  const r = reservation
  const canCancel = r && r.status === 'confirmed' && !r.reviewed && !hasStarted(r.date, r.startHour)
  const canReview = r && r.status === 'confirmed' && !r.reviewed

  return (
    <div className="container page narrow">
      <h1 className="page__title">예약 확인</h1>
      <p className="muted">예약 완료 화면에서 받은 예약 코드와 휴대폰 번호 뒤 4자리를 입력해 주세요.</p>
      <form className="form form--inline" onSubmit={onLookup}>
        <label className="field">
          <span>예약 코드</span>
          <input
            value={code}
            onChange={(e) => setCode(e.target.value.toUpperCase())}
            placeholder="예: K7QM-29XA"
            required
            maxLength={12}
            autoComplete="off"
          />
        </label>
        <label className="field">
          <span>휴대폰 뒤 4자리</span>
          <input
            value={last4}
            onChange={(e) => setLast4(e.target.value.replace(/\D/g, '').slice(0, 4))}
            inputMode="numeric"
            pattern="\d{4}"
            required
          />
        </label>
        <button type="submit" className="btn btn--primary" disabled={busy}>
          조회
        </button>
      </form>

      {error && (
        <div className="alert" role="alert">
          {error}
        </div>
      )}
      {notice && (
        <div className="alert alert--ok" role="status">
          {notice}
        </div>
      )}
      {r && (
        <>
          <ReservationCard r={r} />
          <div className="actions">
            {canReview && (
              <Link to={`/reviews/new?code=${encodeURIComponent(r.code)}`} className="btn btn--primary">
                후기 작성
              </Link>
            )}
            {canCancel && (
              <button type="button" className="btn btn--danger" onClick={onCancel} disabled={busy}>
                예약 취소
              </button>
            )}
          </div>
          {r.reviewed && <p className="hint center">후기를 남겨 주셔서 감사합니다.</p>}
        </>
      )}
    </div>
  )
}
