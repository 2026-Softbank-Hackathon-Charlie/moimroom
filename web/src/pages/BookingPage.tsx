import { useState, type FormEvent } from 'react'
import { Link, useNavigate, useParams, useSearchParams } from 'react-router'
import { ApiError, createReservation, getSpace } from '../api'
import { ErrorState, Loading } from '../components/Layout'
import { dateLabel, formatPhone, rangeLabel, won } from '../format'
import { errorMessage, useAsync } from '../useAsync'

export default function BookingPage() {
  const id = Number(useParams().id)
  const [params] = useSearchParams()
  const navigate = useNavigate()
  const date = params.get('date') ?? ''
  const start = Number(params.get('start'))
  const end = Number(params.get('end'))
  const headcount = Number(params.get('headcount')) || 1

  const space = useAsync(() => getSpace(id), [id])
  const [name, setName] = useState('')
  const [phone, setPhone] = useState('')
  const [agree, setAgree] = useState(false)
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState<{ message: string; conflict: boolean }>()

  const backToSpace = `/spaces/${id}?date=${date}&headcount=${headcount}`
  const validSelection = /^\d{4}-\d{2}-\d{2}$/.test(date) && end > start

  if (space.loading) return <Loading />
  if (space.error || !space.data) return <ErrorState message={space.error ?? '공간을 찾을 수 없습니다.'} />
  if (!validSelection) {
    return (
      <div className="container page narrow">
        <ErrorState message="예약할 시간을 다시 선택해 주세요." />
        <p className="center">
          <Link to={backToSpace}>공간 페이지로 돌아가기</Link>
        </p>
      </div>
    )
  }
  const s = space.data
  const total = (end - start) * s.pricePerHour

  async function onSubmit(e: FormEvent) {
    e.preventDefault()
    setSubmitting(true)
    setError(undefined)
    try {
      const reservation = await createReservation({
        spaceId: id,
        date,
        startHour: start,
        endHour: end,
        headcount,
        guestName: name.trim(),
        phone,
      })
      navigate('/booking/complete', { state: { reservation }, replace: true })
    } catch (err) {
      setError({ message: errorMessage(err), conflict: err instanceof ApiError && err.status === 409 })
      setSubmitting(false)
    }
  }

  return (
    <div className="container page narrow">
      <h1 className="page__title">예약 정보 입력</h1>
      <section className="summary-card" aria-label="예약 내용">
        <h2>{s.name}</h2>
        <dl>
          <div>
            <dt>날짜</dt>
            <dd>{dateLabel(date)}</dd>
          </div>
          <div>
            <dt>시간</dt>
            <dd>
              {rangeLabel(start, end)} ({end - start}시간)
            </dd>
          </div>
          <div>
            <dt>인원</dt>
            <dd>{headcount}명</dd>
          </div>
          <div>
            <dt>결제 예정 금액</dt>
            <dd className="strong">{won(total)}</dd>
          </div>
        </dl>
        <p className="hint">요금은 이용 당일 현장에서 결제합니다.</p>
      </section>

      <form className="form" onSubmit={onSubmit}>
        <label className="field">
          <span>예약자 이름</span>
          <input value={name} onChange={(e) => setName(e.target.value)} maxLength={30} required autoComplete="name" />
        </label>
        <label className="field">
          <span>휴대폰 번호</span>
          <input
            value={phone}
            onChange={(e) => setPhone(formatPhone(e.target.value))}
            inputMode="numeric"
            placeholder="010-0000-0000"
            required
            autoComplete="tel"
          />
          <small className="hint">예약 확인과 취소에 번호 뒤 4자리를 사용합니다. 전체 번호는 저장하지 않습니다.</small>
        </label>
        <label className="check">
          <input type="checkbox" checked={agree} onChange={(e) => setAgree(e.target.checked)} required />
          <span>공간 이용 규칙을 확인했으며 예약에 동의합니다.</span>
        </label>
        {error && (
          <div className="alert" role="alert">
            <p>{error.message}</p>
            {error.conflict && <Link to={backToSpace}>다른 시간 선택하기</Link>}
          </div>
        )}
        <button type="submit" className="btn btn--primary btn--block" disabled={submitting || !agree}>
          {submitting ? '예약하는 중' : `${won(total)} 예약하기`}
        </button>
      </form>
    </div>
  )
}
