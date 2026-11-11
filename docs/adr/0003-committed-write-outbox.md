# 0003. 예약 커밋의 실제 PK를 append-only outbox로 보고한다

- 상태: 채택
- 날짜: 2026-11-07

## 상황

이전 검증의 쓰기 누락을 계산하려면 성공한 쓰기의 실제 테이블/PK가 필요하다.
HTTP 성공 횟수만으로 DB 커밋이나 클라이언트 성공 응답 수신을 증명할 수 없다.

## 결정

예약 INSERT와 같은 트랜잭션에 app.written_record_outbox를 기록한다. 실제 app.reservations
PK를 참조하며 예약·시간대·idempotency 응답·outbox가 함께 커밋되거나 rollback된다.
성공한 동일 키 재시도는 새 기록을 만들지 않는다. 기록 시각은 outbox INSERT 시각이며
커밋 완료 시각이 아니다. application_commits/reservation_inserts 범위만 보고한다.
이름·전화번호·요청 본문·DSN·자격증명은 수집 payload에 넣지 않는다.

앱 전용 stream append token만 사용한다. operator/agent token이나 프론트 설정을 사용하지
않는다. reporter는 primary에서 SELECT만 수행한다. sweep 시작에 upper seq를 고정하고
100건씩 읽어 완료 후 0부터 재시작한다. seq 배정과 커밋 순서가 달라도 지속 유입 중에
늦은 낮은 seq를 다시 수집한다. 중앙 collector가 안정적인 eventId로 중복을 제거한다.
DB pool/SQL/HTTP 전체 3초 제한과 1초 간격, 전체 accepted 응답 확인, redirect 거부를 적용한다.
실패나 재시작은 재전송하며 gateway fencing 밖에서 전달 완료 상태를 DB에 쓰지 않는다.

app.write_collection_identity의 무작위 DB generation은 WAL로 보존한다. 동일 dataset의
복원/이전은 identity를 유지하고 새 DB는 새 identity를 갖는다. 중앙은 기존 stream의
변경된 generation 재사용을 거부한다. 실제 배포 시 source primary에서 새 두 테이블을
먼저 생성하고 WAL 동기화 및 target 존재 확인 후 target collector를 켜야 한다.
standby의 MigrateSkip은 테이블을 생성하지 않는다.

## 검토한 대안

- gateway의 HTTP 성공 횟수: 실제 DB PK와 연결되지 않아 배제했다.
- 전달 완료 행을 source DB에서 UPDATE: gateway 밖 쓰기를 만들어 배제했다.
- seq cursor만 영구 증가: 늦은 낮은 seq 커밋을 놓치므로 고정 범위 재스캔을 선택했다.

## 결과

설정된 예약마다 outbox INSERT와 인덱스 비용이 추가된다. append-only 이력에는 자동
retention/compaction이 없고 저장량·재스캔 비용·보고 지연이 증가한다. 늦은 보고는 이전
검증의 중앙 cutoff 밖으로 빠질 수 있다. 측정값은 수집된 동결 입력에 대한 PK 존재 검증이며
전체 커밋/클라이언트 응답 수신, 전체 CRUD·행 내용·세션 정합성은 증명하지 않는다.
outbox 행도 앱 schema 검증과 WAL 복제에 포함된다. 운영 활성화는 별도 배포가 필요하다.

정상 예약 취소는 행을 보존한다. 수동 DELETE/TRUNCATE, reseeding, PK 재사용 전에는 승인된
정비 과정에서 기존 stream을 폐기하고 identity/stream을 교체한다. 정상 이전 중 identity를
바꾸지 않는다. 새 설치는 새 stream/token을 사용한다. 실제 운영 변경은 수행하지 않았다.

기존 MOIMROOM_TEST_DATABASE_URL로 outbox 원자성, 실패 rollback, 재시도 중복,
역순 커밋 및 지속 유입 중 고정 sweep 범위를 검증한다. CI도 PostgreSQL을 제공한다.
