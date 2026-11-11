# 운영 가이드

모임룸 서버를 운영할 때 필요한 헬스 체크, 로그, 부하 테스트, 장애 주입, 데이터 정합성 확인 방법을 정리합니다.

## 헬스 체크와 종료

| 경로 | 용도 | 동작 |
| --- | --- | --- |
| `GET /healthz` | 생존 확인 | 의존성과 상관없이 프로세스가 살아 있으면 200 |
| `GET /readyz` | 준비 확인 | DB 핑과 버킷 확인이 성공하면 200, 아니면 503 |

- 서버는 시작할 때 DB를 최대 1분 기다린 뒤 스키마를 적용하고 요청을 받습니다. 차트의 startupProbe가 이 시간을 허용합니다.
- SIGTERM을 받으면 `/readyz`를 503으로 바꾸고 5초 동안 요청을 계속 받은 뒤, 처리 중인 요청을 최대 20초 기다리고 종료합니다. `terminationGracePeriodSeconds`는 30초 이상으로 둡니다.
- DB 커넥션은 최대 30초(지터 5초)까지만 재사용합니다.

## 응답 헤더와 로그

- `X-Request-Id`: 요청 ID. 요청에 있으면 그대로 쓰고, 없으면 새로 만듭니다.
- `X-Served-By`: 응답한 인스턴스의 호스트 이름(파드 이름).
- `X-Region`: `SITE_REGION_LABEL`이 있을 때만 보냅니다.

로그는 표준 출력에 JSON 한 줄씩 남깁니다. 요청마다 `request_id`, `method`, `path`, `status`, `bytes`, `duration_ms`를 기록하고, 프로브 요청은 debug 수준이라 기본 설정에서는 남지 않습니다.

## 부하 테스트

`ops/k6/load.js`는 실제 이용자와 비슷한 요청을 고정 도착률로 보냅니다.

| 비율 | 동작 |
| --- | --- |
| 70% | 둘러보기: 목록(여러 필터), 상세와 후기, 날짜별 예약 현황 |
| 20% | 예약: 임의 공간, 14일 안의 날짜, 예약 현황에서 고른 1시간에서 3시간의 빈 구간 |
| 7% | 후기: 같은 VU가 만든 예약 코드로 작성. 아직 예약이 없으면 예약을 대신 합니다 |
| 3% | 호스트 사진 업로드(`room.jpg`). `HOST_CODE`가 없으면 둘러보기로 대신합니다 |

예약 사이에 다른 이용자가 같은 시간을 먼저 잡으면 409가 나오며, 이는 정상 응답으로 셉니다.

```bash
docker run --rm -v "$PWD/ops/k6:/scripts" -w /scripts \
  -e BASE_URL=https://moimroom.example.com \
  -e RATE=20 -e DURATION=10m \
  -e HOST_CODE=... \
  -e RECORDS_URL=https://collector.example.com/records -e RECORDS_TOKEN=... \
  grafana/k6:1.8.2 run load.js
```

로컬 compose를 대상으로 할 때는 `--network moimroom_default -e BASE_URL=http://app:8080`을 씁니다.

| 변수 | 기본값 | 설명 |
| --- | --- | --- |
| `BASE_URL` | `http://localhost:8080` | 대상 주소 |
| `RATE` | `20` | 초당 반복 수 |
| `DURATION` | `10m` | 실행 시간 |
| `HOST_CODE` | | 사진 업로드에 쓸 호스트 코드 |
| `RECORDS_URL` | | 쓰기 기록을 받을 주소(전체 URL). 비우면 보내지 않고 개수만 셉니다 |
| `RECORDS_TOKEN` | | `Authorization: Bearer` 토큰 |

### 쓰기 기록

성공한 쓰기마다 기본 키를 기록합니다.

- 예약 생성(2xx): `{"table":"app.reservations","pk":"<id>"}` 1건과, 예약한 시간마다 `{"table":"app.reservation_slots","pk":"(<space_id>,<date>,<hour>)"}` 1건. 예: `(3,2026-11-05,14)`
- 후기 작성(2xx): `{"table":"app.reviews","pk":"<id>"}`

VU마다 기록을 모아 두었다가 2초가 지나거나 200건이 쌓이면 `POST ${RECORDS_URL}`에 `{"records":[...]}`로 보냅니다. 전송이 실패하면 기록을 버리지 않고 2초 뒤 다시 보냅니다. 실행이 끝나기 직전(마지막 3%)에는 반복마다 바로 보내지만, 마지막 반복 뒤 VU 버퍼에 남은 기록은 보내지 못할 수 있습니다. 로컬 확인에서는 212건 중 4건이 남았습니다.

기록 전송 요청에는 `traffic:records` 태그를 붙이고, 사이트 요청에는 `traffic:site` 태그를 붙입니다. 임계값 `http_req_failed{traffic:site} < 1%`는 사이트 요청만 봅니다.

### 결과 요약

실행이 끝나면 사이트 요청 수, 실패율, p95 지연, 성공한 쓰기 수(예약, 예약 슬롯, 후기, 사진), 예약 충돌 수, 생성하거나 보낸 기록 수를 출력합니다. 로컬 compose에서 30초 동안 실행한 예시는 다음과 같습니다.

```
  site requests        787
  site failure rate    0.00% (threshold <1%: pass)
  p95 latency          38.5 ms
  iterations           601
  successful writes
    reservations       123
    reservation slots  218
    reviews            37
    photos             14
  records produced     378
```

## 장애 주입

운영 API는 `ADMIN_TOKEN`이 있을 때만 열리고, 없으면 `/admin/*`가 404입니다. 화면에는 아무것도 드러나지 않습니다.

```bash
# 120초 동안 /api 요청의 30%를 503으로, 모든 /api 요청에 300ms 지연 추가
curl -X POST https://moimroom.example.com/admin/fault \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -d '{"errorRate":0.3,"extraLatencyMs":300,"durationSeconds":120}'

# 즉시 해제
curl -X DELETE https://moimroom.example.com/admin/fault -H "Authorization: Bearer $ADMIN_TOKEN"
```

- `errorRate`는 0부터 1, `extraLatencyMs`는 0부터 10000, `durationSeconds`는 1부터 3600입니다. 시간이 지나면 저절로 풀립니다.
- `/api/*`에만 적용합니다. `/healthz`, `/readyz`, 정적 파일, `/admin/*`는 영향을 받지 않으므로 파드가 빠지지 않고, 주입한 비율이 그대로 응답에 나타납니다.
- 주입한 오류는 실제 장애와 같은 503 본문을 돌려줍니다.
- 상태는 인스턴스마다 따로 있습니다. 로드밸런서 뒤에서는 아래 스크립트처럼 여러 번 호출해 모든 레플리카에 닿게 합니다.

### 스크립트

| 스크립트 | 사용법 | 동작 |
| --- | --- | --- |
| `ops/faults/inject.sh` | `ADMIN_TOKEN=... inject.sh <base-url> <error-rate> <seconds> [latency-ms]` | 장애 주입. `REPEAT`(기본 10)번 호출하고 응답한 인스턴스를 보여 줍니다 |
| `ops/faults/clear.sh` | `ADMIN_TOKEN=... clear.sh <base-url>` | 장애 해제 |
| `ops/faults/kill-pods.sh` | `kill-pods.sh <context> <namespace> [selector]` | 앱 파드를 유예 시간 없이 강제 삭제 |
| `ops/faults/block-db.sh` | `block-db.sh <context> <namespace>` | DB 포트(기본 5432, `DB_PORT`)만 막는 NetworkPolicy 적용 |
| `ops/faults/unblock-db.sh` | `unblock-db.sh <context> <namespace>` | 위 NetworkPolicy 삭제 |

`block-db.sh`는 `endPort`를 지원하는 CNI(Calico, Cilium, GKE Dataplane V2, 네트워크 정책을 켠 EKS)가 필요합니다. 이미 열린 연결은 CNI에 따라 유지될 수 있지만, 서버가 커넥션을 30초 남짓마다 새로 맺으므로 약 35초 안에 차단이 완전히 적용됩니다. 그동안 `/readyz`가 503이 되어 파드가 Service에서 빠집니다.

## 데이터 정합성 확인

모든 테이블은 `app` 스키마에 있고 기본 키가 있습니다. 백업 복원이나 복제 뒤 두 데이터베이스를 비교할 때 아래 기본 키를 기준으로 합니다.

| 테이블 | 기본 키 | 비고 |
| --- | --- | --- |
| `app.spaces` | `id` | bigserial |
| `app.space_photos` | `id` | bigserial. 사진 파일은 오브젝트 스토리지의 `spaces/{space_id}/{uuid}.{ext}` |
| `app.reservations` | `id` | bigserial. `code`는 유일 |
| `app.reservation_slots` | `(space_id, date, hour)` | 예약한 시간 하나에 한 행. 중복 예약을 막는 키 |
| `app.reviews` | `id` | bigserial. `reservation_id`는 유일 |

- 예약을 취소하면 그 예약의 `app.reservation_slots` 행을 지우고 `app.reservations.status`를 `cancelled`로 바꿉니다. 부하 테스트는 예약을 취소하지 않으므로 기록한 슬롯은 그대로 남아야 합니다.
- 사진 객체는 키가 매번 새로 만들어지고 덮어쓰지 않습니다. DB에는 키만 저장합니다.
- 논리 복제는 시퀀스 값을 옮기지 않습니다. 복제한 데이터베이스로 쓰기를 옮길 때는 각 `id` 시퀀스를 `setval`로 최댓값 이상으로 맞춥니다.
- 다른 곳에서 데이터를 받을 빈 데이터베이스에는 `SEED_SPACES=false`(차트 `seedSpaces: false`)로 시작해 초기 공간이 기본 키와 겹치지 않게 합니다.


## OpenTelemetry HTTP 계측

`OTEL_EXPORTER_OTLP_ENDPOINT`가 없거나 `OTEL_TRACES_EXPORTER=none`이면 기존 동작을 유지합니다. 공통 플랫폼 profile이 HTTPS endpoint, OTLP http/protobuf, 공개 CA 파일(`OTEL_EXPORTER_OTLP_CERTIFICATE`), `OTEL_SERVICE_NAME`, `OTEL_RESOURCE_ATTRIBUTES`를 주입하면 HTTP server span과 W3C traceparent 전파를 활성화합니다. per-signal endpoint/insecure override는 exporter의 명시적 HTTPS 설정을 덮어쓰지 않습니다. OTLP 전송 경로는 `/v1/traces`입니다.

요청 로그의 trace_id/span_id는 해당 server span과 같습니다. `/healthz`, `/readyz`는 span을 만들지 않습니다. span에는 HTTP method와 status만 담으며 URL 경로/query, 인증 헤더, body, SQL은 추가하지 않습니다. DB/client span은 아직 없습니다. SDK 없는 다른 앱의 내부 trace까지 자동 생성하지 않습니다.

batch queue1024, batch1초, export timeout3초, 종료 flush 최대4초입니다. 수집기 장애가 요청 처리를 기다리게 하지 않지만 큐 초과나 종료 제한 시 trace 유실이 가능합니다. SDK TLS 인증은 공개 CA와 hostname 검증을 유지합니다. 실제 플랫폼 gateway 배포와 환경 profile 활성화는 infra PR 및 별도 실행 승인이 필요합니다.
