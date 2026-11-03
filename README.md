# 모임룸

스터디룸, 회의실, 파티룸 같은 작은 공간을 시간 단위로 예약하는 서비스입니다.

## 주요 기능

- 지역(강남, 홍대, 성수, 잠실, 여의도, 신촌), 날짜, 인원으로 공간을 검색합니다.
- 공간 목록에서 지역, 유형, 인원, 시간당 요금으로 거르고 추천순, 평점순, 가격순으로 정렬합니다.
- 상세 페이지의 하루 시간표에서 비어 있는 시간을 골라 바로 예약합니다.
- 예약 코드와 휴대폰 번호 뒤 4자리로 예약을 조회하고 취소합니다.
- 예약 코드로 이용 후기(별점 1점부터 5점, 본문)를 남깁니다.
- 호스트 센터(`/host`)에서 공간을 등록하고 사진을 올리고 예약 현황을 봅니다.

휴대폰 번호는 뒤 4자리와 솔트를 넣은 해시만 저장합니다. 같은 시간대의 중복 예약은 데이터베이스 트랜잭션 안에서 막습니다.

## 기술 스택

| 영역 | 사용 기술 |
| --- | --- |
| 백엔드 | Go 1.26, net/http, pgx, minio-go |
| 프론트엔드 | React 19, TypeScript, Vite 7, React Router 7 |
| 데이터 | PostgreSQL 17, S3 호환 오브젝트 스토리지(AWS S3, Google Cloud Storage, MinIO) |
| 테스트 | Go test, Vitest, Testing Library, k6 |
| 배포 | Docker, Helm, CloudNativePG |

서버는 Go 바이너리 하나입니다. 빌드한 SPA를 바이너리에 내장해 함께 제공하고, `/api` 아래에 JSON API를 둡니다. 세션을 메모리에 두지 않으므로 여러 레플리카로 띄울 수 있습니다.

## 디렉터리 구조

```
cmd/moimroom/        서버 진입점
internal/api/        JSON API 핸들러
internal/store/      PostgreSQL 쿼리, 스키마(schema.sql), 초기 데이터(seed.sql)
internal/spa/        내장 SPA 제공
internal/blob/       S3 호환 저장소 클라이언트
internal/httpx/      요청 ID, 로그, 헬스 체크, 종료 처리
internal/fault/      운영용 장애 주입
web/                 React 프론트엔드
deploy/helm/         Helm 차트
deploy/cnpg/         CloudNativePG 클러스터 예시
ops/                 부하 테스트와 장애 주입 스크립트
docs/operations.md   운영 가이드
platform.yaml        배포 명세 (서비스, 포트, 헬스 체크, 데이터베이스와 버킷)
```

## 로컬에서 실행

Docker와 Docker Compose가 필요합니다.

```bash
docker compose up --build
```

http://localhost:8080 에서 사이트가 열립니다. PostgreSQL, MinIO, 버킷 생성 단계, 앱이 함께 뜨고, 처음 시작할 때 공간 16곳이 등록됩니다. 호스트 센터의 기본 코드는 `local-host-code`이며 `HOST_CODE` 환경 변수로 바꿀 수 있습니다. compose 파일의 계정과 비밀번호는 로컬 개발용입니다.

### 프론트엔드 개발

Node.js 24가 필요합니다. 개발 서버는 `/api` 요청을 http://localhost:8080 으로 넘기므로 compose를 띄워 둔 상태에서 실행합니다.

```bash
cd web
npm ci
npm run dev
```

`npm run build`는 결과물을 `internal/spa/dist`에 만들고, 이후 `go build`가 이를 바이너리에 내장합니다.

### 테스트

```bash
# Go 단위 테스트와 핸들러 테스트
go test ./...

# PostgreSQL이 필요한 테스트까지 실행 (변수가 없으면 건너뜀)
MOIMROOM_TEST_DATABASE_URL=postgres://user:pass@localhost:5432/moimroom_test?sslmode=disable go test ./...

# 프론트엔드 테스트
cd web && npm test
```

DB 테스트는 실행할 때마다 `app` 스키마를 지우고 다시 만들므로 전용 데이터베이스를 쓰세요.

## 설정

모든 설정은 환경 변수로 받습니다.

| 변수 | 필수 | 설명 |
| --- | --- | --- |
| `PORT` | | 수신 포트. 기본값 `8080` |
| `DATABASE_URL` | 예 | PostgreSQL 접속 URI |
| `S3_ENDPOINT` | 예 | 예: `https://s3.ap-northeast-2.amazonaws.com`, `https://storage.googleapis.com`, `http://minio:9000`. 스킴이 없으면 HTTPS |
| `S3_REGION` | | 버킷 리전 |
| `S3_BUCKET` | 예 | 사진을 저장할 버킷 |
| `S3_ACCESS_KEY`, `S3_SECRET_KEY` | | 접근 키. GCS는 HMAC 키를 씁니다 |
| `S3_PATH_STYLE` | | `true`(MinIO), `false`, 비우면 자동 |
| `HOST_CODE` | | 호스트 센터 접근 코드. 비우면 호스트 API가 꺼집니다 |
| `ADMIN_TOKEN` | | 운영 API(`/admin/*`) 토큰. 비우면 운영 API가 꺼집니다 |
| `SITE_REGION_LABEL` | | 값이 있으면 `X-Region` 응답 헤더로 보냅니다 |
| `SEED_SPACES` | | 빈 데이터베이스에 초기 공간을 넣을지 여부. 기본값 `true` |

## API 개요

| 메서드 | 경로 | 설명 |
| --- | --- | --- |
| GET | `/api/spaces` | 공간 목록. `area`, `category`, `minCapacity`, `maxPrice`, `sort`, `limit` |
| GET | `/api/spaces/{id}` | 공간 상세 |
| GET | `/api/spaces/{id}/availability?date=YYYY-MM-DD` | 시간대별 예약 현황 |
| GET | `/api/spaces/{id}/reviews` | 후기 목록 |
| GET | `/api/spaces/{id}/photos/{photoId}` | 사진 |
| POST | `/api/reservations` | 예약 생성. 겹치는 시간이 있으면 409 |
| POST | `/api/reservations/lookup` | 예약 코드와 휴대폰 뒤 4자리로 조회 |
| POST | `/api/reservations/cancel` | 예약 취소 |
| POST | `/api/reviews` | 후기 작성. 예약 하나에 후기 하나 |
| GET | `/api/host/spaces` | 호스트: 공간 목록 |
| POST | `/api/host/spaces` | 호스트: 공간 등록 |
| POST | `/api/host/spaces/{id}/photos` | 호스트: 사진 업로드(필드 `photo`, JPG, PNG, WEBP, GIF, 5MB 이하) |
| GET | `/api/host/reservations` | 호스트: 예약 현황. `spaceId`, `from` |

호스트 API는 `X-Host-Code` 헤더로 코드를 받습니다. 오류 응답은 `{"error": "..."}` 형식입니다.

예약 생성 예시:

```bash
curl -X POST http://localhost:8080/api/reservations \
  -H 'Content-Type: application/json' \
  -d '{"spaceId":1,"date":"2026-11-20","startHour":14,"endHour":16,"headcount":3,"guestName":"홍길동","phone":"010-1234-5678"}'
```

## 배포

### 이미지 빌드

```bash
docker build -t ghcr.io/2026-softbank-hackathon-charlie/moimroom:0.1.0 .
```

멀티 스테이지 빌드로 SPA와 정적 Go 바이너리를 만들고, distroless 이미지에서 uid 65532로 실행합니다.

### platform.yaml

저장소 루트의 `platform.yaml`은 이 서비스를 빌드하고 띄우는 방법을 적은 배포 명세입니다. 루트의 `Dockerfile`로 이미지를 만들고, 8080 포트에서 받으며, `/healthz`로 헬스 체크를 합니다. PostgreSQL 데이터베이스(`db`)와 S3 버킷(`photos`)이 필요하고, 이 명세로 배포하는 환경이 `PORT`, `DATABASE_URL`, `S3_ENDPOINT`, `S3_BUCKET`, `S3_REGION`, `S3_PATH_STYLE`, `S3_ACCESS_KEY`, `S3_SECRET_KEY`를 넣어 줍니다. `HOST_CODE`와 `ADMIN_TOKEN`은 비밀값이라 명세에 넣지 않으므로, 이렇게 배포하면 호스트 API와 운영 API는 꺼져 있습니다.

### Helm

차트는 이미 있는 Secret을 읽습니다. 먼저 S3 키와 (선택) 호스트 코드, 운영 토큰을 넣어 둡니다.

```bash
kubectl create secret generic moimroom-s3 \
  --from-literal=accessKey=... --from-literal=secretKey=...
kubectl create secret generic moimroom-app \
  --from-literal=hostCode=... --from-literal=adminToken=...
```

데이터베이스는 `deploy/cnpg/cluster.yaml`처럼 CloudNativePG로 만들면 `moimroom-db-app` Secret의 `uri` 키를 그대로 씁니다. 다른 Secret을 쓰려면 `database.secretName`, `database.secretKey`를 지정합니다.

```bash
kubectl apply -f deploy/cnpg/cluster.yaml
helm install moimroom deploy/helm/moimroom \
  --set image.tag=0.1.0 \
  --set s3.endpoint=https://s3.ap-northeast-2.amazonaws.com \
  --set s3.region=ap-northeast-2 \
  --set s3.bucket=moimroom-photos
```

| 값 | 기본값 | 설명 |
| --- | --- | --- |
| `replicaCount` | `2` | 레플리카 수 |
| `image.repository`, `image.tag` | `ghcr.io/2026-softbank-hackathon-charlie/moimroom`, 차트 appVersion | 이미지 |
| `database.secretName`, `database.secretKey` | `<release>-db-app`, `uri` | DB URI Secret |
| `s3.endpoint`, `s3.region`, `s3.bucket`, `s3.pathStyle` | | S3 설정. `endpoint`와 `bucket`은 필수 |
| `s3.secretName` | `<release>-s3` | 키 이름은 `accessKey`, `secretKey` |
| `appSecret.name` | `<release>-app` | 키 이름은 `hostCode`, `adminToken`. 없으면 해당 기능이 꺼집니다 |
| `siteRegionLabel` | | `X-Region` 헤더 값 |
| `seedSpaces` | `true` | 빈 DB에 초기 공간 등록 |
| `service.type`, `service.port` | `ClusterIP`, `80` | Service |

파드는 비루트, 읽기 전용 루트 파일시스템으로 실행되고, `/healthz`(생존)와 `/readyz`(DB와 저장소 연결) 프로브를 씁니다. 운영 관련 내용은 [docs/operations.md](docs/operations.md)를 보세요.

## 라이선스

MIT
