# Graylog 대량 로그 AI 분석용 축약 분석기 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development
> (recommended) or superpowers:executing-plans to implement this plan task-by-task.
> Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Graylog API에서 대량 로그를 조회해 AI 입력용 Markdown 사건 브리프로 1-3%
수준 축약하는 `analyze` CLI를 추가한다.

**Architecture:** Graylog 조회는 기존 `pkg/graylog` 흐름을 재사용하고, 축약 로직은
`pkg/analyze` 순수 함수 패키지로 분리한다. CLI는 시간창 분할 조회, 메시지 추출,
가명화, 정규화, 패턴 집계, Markdown 렌더링을 순서대로 연결한다.

**Tech Stack:** Go 1.22, Cobra, 기존 Graylog View Search API client, 표준 라이브러리만 사용.

## Global Constraints

- 기준 문서: `docs/2026-06-22-graylog-ai-log-reduction-design.md`
- AI 입력에는 원본 IP, UUID, VPC/ENI/ARN/account-like 값을 넣지 않고 일관 가명화한다.
- 기본 분석 시간창은 장애 시각 기준 `-30m`부터 `+30m`까지다.
- 절대 `--from/--to`, 장애 시각 기반 창, 상대 `--range` 입력을 모두 지원한다.
- 로그가 많을 수 있으므로 Graylog 조회는 1-5분 단위 chunk로 분할한다.
- 최종 출력은 Markdown 사건 브리프다.
- 반복적인 정상 `info/debug`는 카운트 중심으로 축약하고, 기준선 샘플만 최소 보존한다.
- `error/warn/fatal/panic`, 신규/희귀/급증/호스트 편중 패턴은 원인 후보 우선순위로 보존한다.
- 일부 조회, 파싱, 처리 상한 도달은 부분 결과와 품질 경고로 브리프에 표시한다.

---

## Public Interfaces

- 새 CLI: `graylog-cli analyze`
- 주요 flags:
  - `--query, -q string`: Graylog query, default `*`
  - `--incident-time string`: RFC3339/RFC3339Nano 장애 기준 시각
  - `--from string`, `--to string`: 절대 시간 범위. 지정되면 `--incident-time` 기반 범위보다 우선
  - `--range string`: 상대 시간 범위. `--from/--to`와 `--incident-time`이 없을 때 사용
  - `--window-before string`: default `30m`
  - `--window-after string`: default `30m`
  - `--chunk-size string`: default `5m`
  - `--limit int`: page limit, default `500`
  - `--max-messages int`: 전체 처리 상한, default `20000`
  - `--fields stringSlice`: default
    `timestamp,level,application,hostname,spc_service,spc_tier,part,pid,message,source,gl2_message_id`
  - `--redact-fields stringSlice`: default `message,hostname,application,service,part,source,fields`
  - `--normalize-fields stringSlice`: default `message`
  - `--redact-pattern stringSlice`: custom sensitive pattern, format `NAME=REGEX`, default empty
  - `--out string`: 비어 있으면 stdout, 지정되면 파일 출력

- 새 package `pkg/analyze`:
  - `type LogEntry struct`
    - `Timestamp time.Time`
    - `Level string`: normalized uppercase value such as `ERROR`, `WARN`, `INFO`, `DEBUG`, `UNKNOWN`
    - `Application string`
    - `Hostname string`
    - `Service string`
    - `Tier string`
    - `Part string`
    - `PID string`
    - `Message string`
    - `Source string`
    - `MessageID string`
    - `Fields map[string]string`
  - `type Options struct`
    - `IncidentTime time.Time`
    - `MaxRepresentative int`
    - `RareThreshold int`
    - `BurstWindow time.Duration`
    - `TopN int`
    - `RedactFields []string`
    - `NormalizeFields []string`
    - `CustomRedactPatterns map[string]string`
    - `NormalSampleLimit int`
  - `func ExtractMessage(raw map[string]any, fields []string) (LogEntry, []QualityWarning)`
  - `func RedactEntries(entries []LogEntry, opts Options) ([]LogEntry, []QualityWarning)`
  - `func NormalizeMessage(message string) string`
  - `func PatternKey(entry LogEntry, opts Options) string`
  - `func BuildBrief(entries []LogEntry, opts Options, warnings []QualityWarning) Brief`
  - `func RenderMarkdown(brief Brief) string`
  - `type Pattern struct`
    - `Key string`
    - `Count int`
    - `FirstSeen time.Time`
    - `LastSeen time.Time`
    - `Priority int`
    - `Flags []string`
    - `Samples []LogEntry`
  - `type Brief struct`
    - `TotalEntries int`
    - `StartedAt time.Time`
    - `EndedAt time.Time`
    - `Patterns []Pattern`
    - `QualityWarnings []QualityWarning`
  - `type QualityWarning struct`
    - `Kind string`
    - `Message string`
    - `Count int`

---

## Task 1: 분석 데이터 모델과 Graylog 메시지 추출

**Files:**
- Create: `pkg/analyze/model.go`
- Create: `pkg/analyze/extract.go`
- Test: `pkg/analyze/extract_test.go`

**Implementation:**
- `LogEntry`, `Options`, `Pattern`, `Brief` 타입을 정의한다.
- Task 1의 `Pattern`과 `Brief`는 Task 3 집계를 위한 최소 골격만 둔다.
  - `Pattern`: `Key`, `Count`, `FirstSeen`, `LastSeen`, `Priority`, `Flags`, `Samples`
  - `Brief`: `TotalEntries`, `StartedAt`, `EndedAt`, `Patterns`, `QualityWarnings`
- `QualityWarning` 타입은 `Kind`, `Message`, `Count`를 갖는다.
- Task 1에서는 `QualityWarning`을 집계하지 않고, 이후 Task가 채울 수 있는 모델만 만든다.
- `Options`에는 `CustomRedactPatterns`와 `NormalSampleLimit`을 포함한다.
- `CustomRedactPatterns`는 key가 정책 이름이고 value가 regexp 문자열인 map이다.
- `ExtractMessage`는 Graylog `map[string]any`에서 allowlist 필드를 문자열로 안전 추출한다.
- `ExtractMessage`는 error를 반환하지 않고, 파싱 품질 문제를 `QualityWarning`으로 반환한다.
- allowlist에 없는 raw field는 `LogEntry.Fields`에 넣지 않는다.
- 필드별 fallback 후보를 사용한다.
  - `Timestamp`: `timestamp`
  - `Level`: `level`
  - `Application`: `application`
  - `Hostname`: `hostname`, fallback `source`
  - `Service`: `spc_service`
  - `Tier`: `spc_tier`
  - `Part`: `part`
  - `PID`: `pid`
  - `Message`: `message`, fallback `full_message`
  - `Source`: `source`
  - `MessageID`: `gl2_message_id`, fallback `_id`
- 필드가 누락되거나 타입이 예상과 달라도 error를 반환하지 않는다.
- 누락/타입 불일치 필드는 zero time 또는 빈 문자열로 유지한다.
- `timestamp`는 `timeutil.Parse`를 사용하고 실패하면 zero time으로 두고 `parse_failed` warning을 반환한다.
- `level`은 문자열/숫자 모두 처리하고 대문자 문자열로 표준화한다.
- 숫자 `3`은 `ERROR`, `4`는 `WARN`, `6`은 `INFO`, `7`은 `DEBUG`로 변환한다.
- 알 수 없는 숫자 또는 문자열은 `UNKNOWN`으로 둔다.
- `gl2_message_id`가 없으면 `_id`를 `MessageID` fallback으로 사용한다.

**Tests:**
- 숫자 level, 문자열 level, 누락 필드, timestamp parse 성공/실패를 fixture로 검증한다.
- timestamp parse 실패가 `QualityWarning{Kind:"parse_failed"}`를 반환하는지 검증한다.
- `hostname` 누락 시 `source`가 fallback으로 들어가는지 검증한다.
- `message` 누락 시 `full_message`가 fallback으로 들어가는지 검증한다.
- allowlist 밖 raw field가 `Fields`에 들어가지 않는지 검증한다.
- 잘못된 타입의 필드가 panic 없이 빈 문자열 또는 zero time으로 남는지 검증한다.
- `QualityWarning`과 확장된 `Pattern`/`Brief` 필드가 컴파일되는지 검증한다.
- `Options.CustomRedactPatterns`와 `Options.NormalSampleLimit`을 설정할 수 있는지 검증한다.
- Run: `go test ./pkg/analyze -run 'TestExtractMessage' -v`

**Commit:** `feat: add analyze log extraction model`

---

## Task 2: 보안 가명화와 패턴 정규화

**Files:**
- Create: `pkg/analyze/redact.go`
- Create: `pkg/analyze/normalize.go`
- Test: `pkg/analyze/redact_test.go`
- Test: `pkg/analyze/normalize_test.go`

**Implementation:**
- `Redactor`는 문서 전체에서 동일 원문을 동일 alias로 치환한다.
- Alias prefix:
  - IPv4/IPv6: `IP_001`
  - UUID: `UUID_001`
  - AWS ARN: `ARN_001`
  - VPC ID: `VPC_001`
  - ENI ID: `ENI_001`
  - 10-12자리 account-like 숫자: `ACCOUNT_001`
  - custom sensitive pattern: `<NAME>_001`
- `Options.CustomRedactPatterns`에 있는 regexp를 기본 민감값 regexp 뒤에 적용한다.
- custom pattern 이름은 대문자 alias prefix로 쓰며, `TENANT=tenant-[a-z0-9]+`는 `TENANT_001`로 치환한다.
- custom pattern regexp가 컴파일되지 않으면 해당 pattern은 무시하고 품질 경고 대상으로 남긴다.
- 지원 가능한 치환 대상은 `Message`, `Hostname`, `Application`, `Service`, `Part`, `Source`, `Fields` 값이다.
- 실제 치환 대상은 `Options.RedactFields` 또는 CLI `--redact-fields`로 결정한다.
- redaction field 이름은 `message`, `hostname`, `application`, `service`, `part`, `source`, `fields`를 지원한다.
- `fields`는 `LogEntry.Fields` map 전체를 의미한다.
- 기본 redaction field는 `message,hostname,application,service,part,source,fields`다.
- `Timestamp`, `Level`, `Tier`, `PID`, `MessageID`는 기본 redaction 대상이 아니다.
- `MessageID`는 Graylog 재조회 키로 쓰기 위해 redaction field로 지정해도 치환하지 않는다.
- `NormalizeMessage`는 redacted message 기준으로 숫자, 포트, duration, hex id,
  request/order/id-like token을 placeholder로 바꿔 같은 원인 로그가 같은 pattern key를 갖게 한다.
- `PatternKey(entry, opts)`는 `Options.NormalizeFields`에 지정된 필드를 정규화한 뒤 `|`로 연결한다.
- normalization field 이름은 `message`, `hostname`, `application`, `service`, `part`, `source`, `fields`를 지원한다.
- 기본 normalization field는 `message`다.
- `NormalizeMessage`는 단일 문자열 정규화 helper로 유지하고, field 선택은 `PatternKey`에서 처리한다.
- 정규화는 소문자화하지 않는다. 운영자가 보는 대표 로그는 redacted 원문을 사용하고, grouping key만 normalized string을 사용한다.

**Tests:**
- 같은 IP가 여러 필드와 여러 로그에서 같은 alias가 되는지 검증한다.
- UUID/ARN/VPC/ENI/account-like 값이 원문 없이 alias로 바뀌는지 검증한다.
- `failed request id=123 elapsed=45ms`와 `failed request id=456 elapsed=91ms`가 같은 key로 묶이는지 검증한다.
- `RedactFields`에서 `hostname`을 제외하면 hostname 원문이 유지되는지 검증한다.
- `RedactFields`에 `message`만 있으면 message만 치환되고 source/application은 유지되는지 검증한다.
- `NormalizeFields`가 `message`일 때 hostname 차이는 pattern key에 반영되지 않는지 검증한다.
- `NormalizeFields`가 `message,hostname`일 때 hostname 차이가 pattern key에 반영되는지 검증한다.
- `MessageID`는 redaction field로 지정해도 치환되지 않는지 검증한다.
- custom pattern `TENANT=tenant-[0-9]+`가 `TENANT_001`로 치환되는지 검증한다.
- 잘못된 custom regexp가 panic 없이 무시되고 경고 후보로 반환되는지 검증한다.

**Commit:** `feat: add deterministic redaction and normalization`

---

## Task 3: 패턴 집계와 이상 신호 보존

**Files:**
- Create: `pkg/analyze/aggregate.go`
- Test: `pkg/analyze/aggregate_test.go`

**Implementation:**
- `BuildBrief(entries, opts, warnings)`는 timestamp 오름차순으로 처리한다.
- pattern key는 `PatternKey(entry, opts)`다.
- 각 pattern은 다음을 보존한다:
  - 총 건수
  - 최초/최후 시각
  - level 분포
  - application/hostname/part 상위 분포
  - 대표 로그 최대 3개
  - 장애 시각과 가장 가까운 샘플 1개
  - 최초 발생 샘플 1개
- anomaly flags:
  - `priority_level`: level이 `ERROR`, `WARN`, `FATAL`, `PANIC`, `EMERG`, `ALERT`, `CRITI`
  - `rare`: count <= `RareThreshold`, default `3`
  - `new_near_incident`: 최초 발생이 incident time 전후 5분 안
  - `host_skew`: 한 hostname이 pattern count의 80% 이상이고 count >= 3
  - `burst`: `BurstWindow` default 1분 안에 pattern 전체 count의 50% 이상 발생하고 count >= 5
- pattern priority는 `priority_level`, `new_near_incident`, `burst`, `host_skew`, `rare` 순으로 계산한다.
- 같은 priority 안에서는 count 내림차순, first seen 오름차순으로 정렬한다.
- 정상 `INFO`/`DEBUG` pattern은 기본적으로 count와 분포만 유지한다.
- 정상 pattern의 대표 샘플은 `Options.NormalSampleLimit` 전체 예산 안에서만 보존한다.
- `Brief`에는 top error/warn patterns, rare/new patterns, host skew patterns, timeline events,
  total counts, quality warnings가 포함된다.

**Tests:**
- error/warn 패턴이 info보다 적어도 top section에 보존되는지 검증한다.
- incident 근처 최초 발생 패턴이 `new_near_incident`로 표시되는지 검증한다.
- 특정 hostname에 몰린 패턴이 `host_skew`로 표시되는지 검증한다.
- burst 조건이 1분 윈도우에서 검출되는지 검증한다.
- 여러 anomaly가 있을 때 priority 순서로 정렬되는지 검증한다.
- 반복 `INFO`/`DEBUG` pattern은 대표 샘플이 `NormalSampleLimit`을 넘지 않는지 검증한다.
- `QualityWarnings`가 입력되면 `Brief`에 유지되는지 검증한다.

**Commit:** `feat: aggregate log patterns for incident briefs`

---

## Task 4: Markdown 사건 브리프 렌더링

**Files:**
- Create: `pkg/analyze/render.go`
- Test: `pkg/analyze/render_test.go`

**Implementation:**
- `RenderMarkdown(brief Brief)`는 다음 섹션을 고정 순서로 출력한다:
  1. `# Graylog Incident Brief`
  2. `## Summary`
  3. `## Timeline`
  4. `## Top Error and Warning Patterns`
  5. `## Rare or New Patterns`
  6. `## Host and Service Skew`
  7. `## Representative Logs`
  8. `## Questions for AI Analysis`
  9. `## Graylog Follow-up Queries`
  10. `## Quality Warnings`
- 대표 로그에는 timestamp, level, application, hostname, message id, redacted message를 포함한다.
- Follow-up query는 pattern normalized key의 핵심 단어, message id, first/last time,
  incident-near time을 사람이 Graylog에서 재조회할 수 있게 짧게 출력한다.
- Markdown에는 redaction 전 원문 값이 들어가지 않는다.
- `Quality Warnings`에는 누락 chunk, 파싱 실패, 처리 상한 도달, 잘못된 custom regexp를 표시한다.

**Tests:**
- 모든 필수 섹션이 순서대로 존재하는지 검증한다.
- 대표 로그가 3개를 넘지 않는지 검증한다.
- fixture에 포함된 원본 IP/UUID가 출력에 없는지 검증한다.
- Follow-up query에 message id와 시간 단서가 포함되는지 검증한다.
- quality warning이 있을 때 `## Quality Warnings`에 표시되는지 검증한다.

**Commit:** `feat: render markdown incident briefs`

---

## Task 5: Graylog windowed 조회와 fields allowlist 지원

**Files:**
- Modify: `pkg/graylog/client/types.go`
- Modify: `pkg/graylog/client/query.go`
- Modify: `pkg/graylog/search.go`
- Test: `pkg/graylog/client/query_test.go`

**Implementation:**
- `SearchTypeMessage`에 `Fields []string 'json:"fields,omitempty"'`를 추가한다.
- `QueryRequest`에 `Fields []string`을 추가한다.
- `QueryRequest`에는 조회 chunk를 식별할 수 있는 `ChunkLabel string`을 추가한다.
- `AppendSearchMessage`는 기존 호출 호환성을 유지하고, 내부에서 `AppendSearchMessageWithFields(id, limit, offset, sort, nil)`을 호출한다.
- 새 함수 `AppendSearchMessageWithFields(id string, limit int, offset int, sort string, fields []string)`를 추가한다.
- `Search`에서 message query 생성 시 `qreq.Fields`를 넘긴다.
- sort parsing은 기존 `timestamp:DESC` 형식을 유지하되, `:`가 없으면 `timestamp:DESC`로 fallback한다.
- chunk 조회 자체는 Task 6에서 담당하고, Task 5는 단일 query request가 chunk label을 보존하게만 한다.

**Tests:**
- fields가 비어 있으면 JSON에 `fields`가 없음을 검증한다.
- fields가 있으면 message search type JSON에 allowlist가 들어가는지 검증한다.
- sort에 `:`가 없는 입력이 panic 없이 기본 정렬로 처리되는지 검증한다.
- `QueryRequest.ChunkLabel`을 설정해도 기존 Graylog 요청 JSON에는 불필요한 필드가 섞이지 않는지 검증한다.

**Commit:** `feat: support message field allowlists`

---

## Task 6: `analyze` CLI 연결

**Files:**
- Create: `cmd/analyze.go`
- Test: `cmd/analyze_test.go`

**Implementation:**
- `analyzeCmd`를 root command에 등록한다.
- config resolution은 `cmd/search.go`와 동일하게 endpoint, username, password, tier를 처리한다.
- 시간 범위 결정:
  - `--from`과 `--to`가 모두 있으면 그대로 사용한다.
  - 아니면 `--incident-time`이 있으면 incident mode를 사용한다.
  - 아니면 `--range` 상대 범위를 사용한다.
  - 세 입력이 모두 없으면 기존 global `--range` 값 또는 기본 `8h`를 사용한다.
  - incident mode에서는 `incident - window-before`부터 `incident + window-after`까지 계산한다.
- 상대 범위 mode는 Graylog relative timerange로 요청하되, chunk 분할이 불가능하면 단일 조회로 처리한다.
- chunking:
  - default `--chunk-size=5m`
  - 전체 범위를 chunk로 나누고 각 chunk마다 offset pagination을 수행한다.
  - 각 page는 `limit`만큼 조회하고, 누적 수가 `--max-messages`에 도달하면 조회를 멈춘다.
  - chunk 조회 실패는 전체 실패로 만들지 않고 `QualityWarning{Kind:"chunk_failed"}`로 기록한다.
  - `--max-messages`에 도달하면 `QualityWarning{Kind:"limit_reached"}`를 기록한다.
- query request:
  - `QueryTypeMessage`
  - `Sort = "timestamp:ASC"`
  - `Fields = analyzeFields`
- analysis options:
  - `Options.RedactFields`는 `--redact-fields` 값을 사용한다.
  - `Options.NormalizeFields`는 `--normalize-fields` 값을 사용한다.
  - `Options.CustomRedactPatterns`는 `--redact-pattern` 값을 `NAME=REGEX` 형식으로 파싱한다.
- 처리 순서:
  - Graylog raw message 추출
  - `analyze.ExtractMessage`에서 entry와 warnings를 받는다.
  - extract warning은 redaction warning과 함께 `BuildBrief`에 전달한다.
  - `analyze.RedactEntries(entries, opts)`에서 redacted entries와 warnings를 받는다.
  - `analyze.BuildBrief(entries, opts, warnings)`
  - `analyze.RenderMarkdown`
  - stdout 또는 `--out` 파일 출력
- CLI validation 실패는 `cmd.Help()`가 아니라 명확한 error를 반환한다.

**Tests:**
- `--from/--to`가 있으면 `--incident-time` 없이 통과하는지 검증한다.
- `--incident-time`만 있으면 기본 +/-30분 범위를 계산하는지 검증한다.
- `--range 1h`가 상대 timerange query로 전달되는지 검증한다.
- `--out`이 비어 있으면 stdout writer로 쓰는지 검증한다.
- `--redact-fields message`가 `Options.RedactFields`에 전달되는지 검증한다.
- `--normalize-fields message,hostname`이 `Options.NormalizeFields`에 전달되는지 검증한다.
- `--redact-pattern TENANT=tenant-[0-9]+`가 `Options.CustomRedactPatterns`에 전달되는지 검증한다.
- 한 chunk 조회 실패가 전체 명령 실패가 아니라 quality warning으로 렌더링되는지 검증한다.
- `--max-messages` 도달 시 limit warning이 렌더링되는지 검증한다.
- 테스트 가능성을 위해 command 실행 함수는 `runAnalyze(cmd, args, fetcher, stdout)` 형태로 분리한다.

**Commit:** `feat: add graylog analyze command`

---

## Task 7: 통합 fixture와 문서 갱신

**Files:**
- Create: `pkg/analyze/testdata/incident_messages.json`
- Create: `pkg/analyze/testdata/large_incident_messages.json`
- Modify: `README.md`
- Test: `pkg/analyze/integration_test.go`

**Implementation:**
- fixture는 반복 info 80개, error burst 10개, rare warn 2개, host-skew error 6개, redaction 대상 IP/UUID/ENI/VPC/ARN을 포함한다.
- fixture에는 custom sensitive value `tenant-12345`와 정상 debug baseline 로그를 포함한다.
- integration test는 fixture를 `LogEntry`로 변환한 뒤 최종 Markdown을 생성한다.
- 원본 fixture 크기 대비 Markdown 크기가 1-3%에 가까운지 직접 고정하지 않는다. 작은 fixture는 비율 왜곡이 크므로 대신 반복 info가 대표 로그로 과다 출력되지 않는지 검증한다.
- integration test는 anomaly section이 priority 순서로 정렬되는지 검증한다.
- integration test는 quality warning fixture가 `## Quality Warnings`에 출력되는지 검증한다.
- large fixture는 반복 info/debug 5000개와 장애 신호 100개를 포함한다.
- large fixture에서 원본 대비 Markdown 크기가 1-3% 목표 범위인지 측정한다.
- README에 `analyze` 사용 예시를 추가한다:
  - incident time 기반
  - absolute from/to 기반
  - relative range 기반
  - 파일 출력
  - fields override
  - redaction/normalization field override
  - custom redaction pattern

**Tests:**
- Run: `go test ./...`
- Expected: all packages pass.
- Manual smoke:
  - `go run . analyze --incident-time 2026-06-22T01:00:00Z --query '*' --max-messages 10`
  - `go run . analyze --range 1h --query '*' --max-messages 10`
  - Graylog credential이 없으면 네트워크 에러가 명확히 출력되는지 확인한다.

**Commit:** `docs: document analyze incident brief workflow`

---

## Assumptions and Defaults

- v1은 새 `graylog-cli analyze` 명령으로 구현한다.
- AI API 호출은 하지 않는다. 결과 Markdown을 사람이 AI에 붙여넣는 흐름이다.
- 가명화 비활성화 flag는 제공하지 않는다.
- chunk size 기본값은 문서의 1-5분 범위 중 보수적인 `5m`이다.
- `streams` flag는 v1에 포함하지 않는다. 기존 `QueryRequest` stream filter wiring이 CLI에 노출되어 있지 않아 별도 후속 작업으로 둔다.
- custom redaction policy는 `NAME=REGEX` 실행 옵션으로만 제공하고, 설정 파일 연동은 후속 작업으로 둔다.
