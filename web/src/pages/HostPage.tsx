import { useState, type FormEvent } from 'react'
import {
  hostCreateSpace,
  hostListSpaces,
  hostReservations,
  hostUploadPhoto,
  photoUrl,
  type Category,
  type NewSpace,
  type Space,
} from '../api'
import { ErrorState, Loading } from '../components/Layout'
import PhotoPlaceholder from '../components/PhotoPlaceholder'
import { AREAS, CATEGORY_LABEL, dateLabel, rangeLabel, todayInSeoul, won } from '../format'
import { errorMessage, useAsync } from '../useAsync'

// The host code lives only in this tab; the server checks it on every call.
const STORAGE_KEY = 'moimroom.hostCode'

function readCode(): string {
  try {
    return sessionStorage.getItem(STORAGE_KEY) ?? ''
  } catch {
    return ''
  }
}

function HostLogin({ onLogin }: { onLogin: (code: string) => void }) {
  const [code, setCode] = useState('')
  const [error, setError] = useState<string>()
  const [busy, setBusy] = useState(false)

  async function onSubmit(e: FormEvent) {
    e.preventDefault()
    setBusy(true)
    setError(undefined)
    try {
      await hostListSpaces(code)
      onLogin(code)
    } catch {
      setError('호스트 코드가 올바르지 않습니다.')
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="container page narrow">
      <h1 className="page__title">호스트 센터</h1>
      <p className="muted">공간을 등록하고 사진과 예약 현황을 관리합니다. 발급받은 호스트 코드를 입력해 주세요.</p>
      <form className="form" onSubmit={onSubmit}>
        <label className="field">
          <span>호스트 코드</span>
          <input type="password" value={code} onChange={(e) => setCode(e.target.value)} required autoComplete="off" />
        </label>
        {error && (
          <div className="alert" role="alert">
            {error}
          </div>
        )}
        <button type="submit" className="btn btn--primary btn--block" disabled={busy}>
          들어가기
        </button>
      </form>
    </div>
  )
}

function PhotoUpload({ hostCode, space, onDone }: { hostCode: string; space: Space; onDone: (msg: string) => void }) {
  const [busy, setBusy] = useState(false)
  async function onChange(file: File | undefined) {
    if (!file) return
    if (file.size > 5 * 1024 * 1024) {
      onDone('사진은 5MB 이하만 올릴 수 있습니다.')
      return
    }
    setBusy(true)
    try {
      await hostUploadPhoto(hostCode, space.id, file)
      onDone(`${space.name}에 사진을 등록했습니다.`)
    } catch (err) {
      onDone(errorMessage(err))
    } finally {
      setBusy(false)
    }
  }
  return (
    <label className={`btn btn--ghost btn--small${busy ? ' is-busy' : ''}`}>
      {busy ? '올리는 중' : '사진 추가'}
      <input
        type="file"
        accept="image/jpeg,image/png,image/webp,image/gif"
        className="visually-hidden"
        disabled={busy}
        onChange={(e) => {
          void onChange(e.target.files?.[0])
          e.target.value = ''
        }}
      />
    </label>
  )
}

function MySpaces({ hostCode }: { hostCode: string }) {
  const spaces = useAsync(() => hostListSpaces(hostCode), [hostCode])
  const [notice, setNotice] = useState<string>()
  if (spaces.loading && !spaces.data) return <Loading />
  if (spaces.error) return <ErrorState message={spaces.error} onRetry={spaces.reload} />
  return (
    <>
      {notice && (
        <div className="alert alert--ok" role="status">
          {notice}
        </div>
      )}
      <ul className="host-list">
        {spaces.data?.map((s) => (
          <li key={s.id}>
            <div className="host-list__thumb">
              {s.coverPhotoId ? (
                <img src={photoUrl(s.id, s.coverPhotoId)} alt="" />
              ) : (
                <PhotoPlaceholder category={s.category} label={s.name} />
              )}
            </div>
            <div className="host-list__body">
              <strong>{s.name}</strong>
              <span className="muted">
                {s.area} <span aria-hidden="true">|</span> {CATEGORY_LABEL[s.category]}{' '}
                <span aria-hidden="true">|</span> 최대 {s.capacity}인 <span aria-hidden="true">|</span>{' '}
                {won(s.pricePerHour)}
              </span>
            </div>
            <PhotoUpload
              hostCode={hostCode}
              space={s}
              onDone={(msg) => {
                setNotice(msg)
                spaces.reload()
              }}
            />
          </li>
        ))}
      </ul>
    </>
  )
}

const EMPTY_SPACE = {
  name: '',
  category: 'study' as Category,
  area: AREAS[0] as string,
  address: '',
  summary: '',
  description: '',
  capacity: 4,
  pricePerHour: 10000,
  openHour: 9,
  closeHour: 22,
  amenities: '',
  rules: '',
}

function NewSpaceForm({ hostCode, onCreated }: { hostCode: string; onCreated: (s: Space) => void }) {
  const [f, setF] = useState(EMPTY_SPACE)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string>()
  const set = <K extends keyof typeof EMPTY_SPACE>(k: K, v: (typeof EMPTY_SPACE)[K]) => setF({ ...f, [k]: v })

  async function onSubmit(e: FormEvent) {
    e.preventDefault()
    setBusy(true)
    setError(undefined)
    const input: NewSpace = {
      ...f,
      amenities: f.amenities.split(',').map((a) => a.trim()).filter(Boolean),
      rules: f.rules.split('\n').map((r) => r.trim()).filter(Boolean),
    }
    try {
      onCreated(await hostCreateSpace(hostCode, input))
      setF(EMPTY_SPACE)
    } catch (err) {
      setError(errorMessage(err))
    } finally {
      setBusy(false)
    }
  }

  const hours = Array.from({ length: 25 }, (_, h) => h)
  return (
    <form className="form form--grid" onSubmit={onSubmit}>
      <label className="field field--wide">
        <span>공간 이름</span>
        <input value={f.name} onChange={(e) => set('name', e.target.value)} required maxLength={60} />
      </label>
      <label className="field">
        <span>유형</span>
        <select value={f.category} onChange={(e) => set('category', e.target.value as Category)}>
          {Object.entries(CATEGORY_LABEL).map(([k, v]) => (
            <option key={k} value={k}>
              {v}
            </option>
          ))}
        </select>
      </label>
      <label className="field">
        <span>지역</span>
        <select value={f.area} onChange={(e) => set('area', e.target.value)}>
          {AREAS.map((a) => (
            <option key={a}>{a}</option>
          ))}
        </select>
      </label>
      <label className="field field--wide">
        <span>주소</span>
        <input value={f.address} onChange={(e) => set('address', e.target.value)} required maxLength={100} placeholder="서울 마포구 망원동" />
      </label>
      <label className="field field--wide">
        <span>한 줄 소개</span>
        <input value={f.summary} onChange={(e) => set('summary', e.target.value)} maxLength={80} />
      </label>
      <label className="field field--wide">
        <span>상세 설명</span>
        <textarea value={f.description} onChange={(e) => set('description', e.target.value)} rows={5} maxLength={2000} />
      </label>
      <label className="field">
        <span>최대 인원</span>
        <input type="number" min={1} max={100} value={f.capacity} onChange={(e) => set('capacity', Number(e.target.value))} required />
      </label>
      <label className="field">
        <span>시간당 요금(원)</span>
        <input
          type="number"
          min={0}
          step={500}
          value={f.pricePerHour}
          onChange={(e) => set('pricePerHour', Number(e.target.value))}
          required
        />
      </label>
      <label className="field">
        <span>오픈 시간</span>
        <select value={f.openHour} onChange={(e) => set('openHour', Number(e.target.value))}>
          {hours.slice(0, 24).map((h) => (
            <option key={h} value={h}>
              {h}시
            </option>
          ))}
        </select>
      </label>
      <label className="field">
        <span>마감 시간</span>
        <select value={f.closeHour} onChange={(e) => set('closeHour', Number(e.target.value))}>
          {hours.slice(1).map((h) => (
            <option key={h} value={h}>
              {h}시
            </option>
          ))}
        </select>
      </label>
      <label className="field field--wide">
        <span>편의시설 (쉼표로 구분)</span>
        <input value={f.amenities} onChange={(e) => set('amenities', e.target.value)} placeholder="와이파이, 화이트보드, 빔프로젝터" />
      </label>
      <label className="field field--wide">
        <span>이용 규칙 (한 줄에 하나씩)</span>
        <textarea value={f.rules} onChange={(e) => set('rules', e.target.value)} rows={3} />
      </label>
      {error && (
        <div className="alert field--wide" role="alert">
          {error}
        </div>
      )}
      <button type="submit" className="btn btn--primary field--wide" disabled={busy}>
        공간 등록
      </button>
    </form>
  )
}

function HostBookings({ hostCode }: { hostCode: string }) {
  const [spaceId, setSpaceId] = useState(0)
  const [from, setFrom] = useState(todayInSeoul())
  const spaces = useAsync(() => hostListSpaces(hostCode), [hostCode])
  const list = useAsync(() => hostReservations(hostCode, { spaceId, from }), [hostCode, spaceId, from])
  return (
    <>
      <div className="filters">
        <label>
          <span>공간</span>
          <select value={spaceId} onChange={(e) => setSpaceId(Number(e.target.value))}>
            <option value={0}>전체 공간</option>
            {spaces.data?.map((s) => (
              <option key={s.id} value={s.id}>
                {s.name}
              </option>
            ))}
          </select>
        </label>
        <label>
          <span>시작일</span>
          <input type="date" value={from} onChange={(e) => e.target.value && setFrom(e.target.value)} />
        </label>
      </div>
      {list.loading && !list.data && <Loading />}
      {list.error && <ErrorState message={list.error} onRetry={list.reload} />}
      {list.data && list.data.length === 0 && <p className="state">해당 기간에 예약이 없습니다.</p>}
      {list.data && list.data.length > 0 && (
        <div className="table-wrap">
          <table className="table">
            <thead>
              <tr>
                <th>날짜</th>
                <th>시간</th>
                <th>공간</th>
                <th>예약자</th>
                <th>연락처</th>
                <th>인원</th>
                <th>금액</th>
                <th>상태</th>
              </tr>
            </thead>
            <tbody>
              {list.data.map((r) => (
                <tr key={r.id} className={r.status === 'cancelled' ? 'is-cancelled' : ''}>
                  <td>{dateLabel(r.date)}</td>
                  <td>{rangeLabel(r.startHour, r.endHour)}</td>
                  <td>{r.spaceName}</td>
                  <td>{r.guestName}</td>
                  <td>뒤 4자리 {r.phoneLast4}</td>
                  <td>{r.headcount}명</td>
                  <td>{won(r.totalPrice)}</td>
                  <td>
                    <span className={`status status--${r.status}`}>{r.status === 'confirmed' ? '확정' : '취소'}</span>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </>
  )
}

type Tab = 'spaces' | 'new' | 'bookings'
const TABS: [Tab, string][] = [
  ['spaces', '내 공간'],
  ['new', '공간 등록'],
  ['bookings', '예약 현황'],
]

export default function HostPage() {
  const [hostCode, setHostCode] = useState(readCode)
  const [tab, setTab] = useState<Tab>('spaces')
  const [notice, setNotice] = useState<string>()

  function login(code: string) {
    try {
      sessionStorage.setItem(STORAGE_KEY, code)
    } catch {
      // Private mode: stay signed in for this page view only.
    }
    setHostCode(code)
  }
  function logout() {
    try {
      sessionStorage.removeItem(STORAGE_KEY)
    } catch {
      // ignore
    }
    setHostCode('')
  }

  if (!hostCode) return <HostLogin onLogin={login} />

  return (
    <div className="container page">
      <div className="section__head">
        <h1 className="page__title">호스트 센터</h1>
        <button type="button" className="btn btn--ghost btn--small" onClick={logout}>
          나가기
        </button>
      </div>
      <div className="tabs" role="tablist">
        {TABS.map(([k, label]) => (
          <button
            key={k}
            type="button"
            role="tab"
            aria-selected={tab === k}
            className={tab === k ? 'active' : ''}
            onClick={() => {
              setTab(k)
              setNotice(undefined)
            }}
          >
            {label}
          </button>
        ))}
      </div>
      {notice && (
        <div className="alert alert--ok" role="status">
          {notice}
        </div>
      )}
      {tab === 'spaces' && <MySpaces hostCode={hostCode} />}
      {tab === 'new' && (
        <NewSpaceForm
          hostCode={hostCode}
          onCreated={(s) => {
            setTab('spaces')
            setNotice(`${s.name}을(를) 등록했습니다. 사진을 추가해 공간을 소개해 보세요.`)
          }}
        />
      )}
      {tab === 'bookings' && <HostBookings hostCode={hostCode} />}
    </div>
  )
}
