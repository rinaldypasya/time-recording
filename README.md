# Time Recording System

A production-grade REST API for employee time tracking, built in Go using clean architecture principles — no web framework, no ORM, just the standard library + `lib/pq`.

## Architecture

```
cmd/api/             → entrypoint (wiring, server startup, graceful shutdown)
internal/
  domain/            → models, interfaces, sentinel errors
  repository/        → PostgreSQL implementations
  service/           → business logic (clock state machine, overtime calc, reporting)
  handler/           → HTTP routing, request/response marshalling, input validation
  middleware/        → structured logging, request-ID, rate limiting, API key auth
  db/                → connection pool + embedded migrations
web/                 → embedded frontend (single-page HTML/CSS/JS, no build step)
```

**Key design decisions**

| Decision | Rationale |
|---|---|
| Standard `net/http` mux | No framework overhead; sufficient for a REST API |
| Per-user `sync.Mutex` via `sync.Map` | Prevents race conditions on clock-in/out without locking every user |
| Soft deletes (`deleted_at`) | Preserves audit trail for time records |
| Embedded migrations in Go | No external migration tool needed; runs on startup |
| Interface-driven repository | Enables full unit testing without a database |
| Graceful shutdown | Handles SIGINT/SIGTERM; drains in-flight requests with a 15s timeout |
| Structured logging (`log/slog`) | JSON output with key-value pairs for observability tooling |
| Per-IP token bucket rate limiting | Prevents resource exhaustion without global bottleneck |
| Optional API key auth | Bearer token via `API_KEY` env var; disabled when unset |

---

## Prerequisites

- Go 1.24+
- Docker & Docker Compose (recommended)

---

## Configuration

| Variable | Default | Description |
|---|---|---|
| `DATABASE_URL` | `postgres://postgres:postgres@localhost:5432/timerecording?sslmode=disable` | PostgreSQL connection string |
| `PORT` | `8080` | Server listen port |
| `API_KEY` | _(empty — auth disabled)_ | When set, all endpoints (except `/health`) require `Authorization: Bearer <key>` |

---

## Quick Start (Docker)

```bash
git clone <repo-url>
cd time-recording
docker compose up --build
```

The API will be available at `http://localhost:8080`.
Open `http://localhost:8080` in your browser to access the frontend UI.

To enable authentication:

```bash
API_KEY=my-secret-key docker compose up --build
```

---

## Local Development

```bash
# 1. Start Postgres
docker compose up postgres -d

# 2. Set connection string
export DATABASE_URL="postgres://postgres:postgres@localhost:5432/timerecording?sslmode=disable"

# 3. (Optional) Enable API key auth
export API_KEY="my-secret-key"

# 4. Run
go run ./cmd/api

# 5. Test
go test ./... -race
```

---

## Frontend UI

A built-in web interface is available at `http://localhost:8080` (redirects to `/ui/`). The frontend is a single-page application with no external dependencies — vanilla HTML, CSS, and JavaScript embedded directly into the Go binary via `go:embed`.

### Features

- **Clock In/Out** — One-click clock-in and clock-out with real-time status indicator and optional notes
- **Records Management** — View, create, edit, and delete time records for the current month
- **Monthly Report** — Aggregated summary with total hours, overtime breakdown, and expandable daily details
- **User Switching** — Change user ID to view different users' data

The frontend is exempt from API key authentication, so it is always accessible. API calls from the browser go directly to the same server — no CORS configuration needed.

---

## API Reference

All request/response bodies are JSON. Timestamps use **RFC3339** format (e.g. `2024-01-08T09:00:00Z`).
Dates in query params use `YYYY-MM-DD`.

### Authentication

When `API_KEY` is set, include the header on every request (except `/health`):

```
Authorization: Bearer <your-api-key>
```

Unauthenticated requests receive:

```json
{"error": "unauthorized"}
```

### Input Validation

All endpoints enforce the following constraints:

| Field | Constraint |
|---|---|
| `user_id` | 1–128 characters, alphanumeric plus `-`, `_`, `.` |
| `note` | Max 1024 characters |
| Request body | Max 1 MB |

### Error Responses

All error responses include the `request_id` for log correlation:

```json
{"error": "user is already clocked in", "request_id": "a1b2c3d4e5f6g7h8"}
```

### Rate Limiting

The API enforces per-IP rate limiting (10 requests/second, burst of 20). Exceeding the limit returns:

```
429 Too Many Requests
Retry-After: 1
```

---

### Health

```
GET /health
→ 200  {"status":"ok"}
```

This endpoint is exempt from authentication and rate limiting.

---

### Clock Events

#### Clock In

```
POST /clock-in
```

| Field | Type | Required | Description |
|---|---|---|---|
| `user_id` | string | ✓ | Unique user identifier |
| `at` | RFC3339 | — | Clock-in time (defaults to now) |
| `note` | string | — | Optional note |

```bash
curl -X POST http://localhost:8080/clock-in \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer my-secret-key" \
  -d '{"user_id":"alice","at":"2024-01-08T09:00:00Z","note":"morning shift"}'
```

**Response 201**
```json
{
  "id": 1,
  "user_id": "alice",
  "clock_in": "2024-01-08T09:00:00Z",
  "clock_out": null,
  "note": "morning shift",
  "created_at": "2024-01-08T09:00:01Z",
  "updated_at": "2024-01-08T09:00:01Z"
}
```

**Error 409** – already clocked in

---

#### Clock Out

```
POST /clock-out
```

| Field | Type | Required |
|---|---|---|
| `user_id` | string | ✓ |
| `at` | RFC3339 | — |
| `note` | string | — |

```bash
curl -X POST http://localhost:8080/clock-out \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer my-secret-key" \
  -d '{"user_id":"alice","at":"2024-01-08T18:00:00Z"}'
```

**Response 200** – same shape as clock-in, with `clock_out` populated.

**Error 409** – not clocked in
**Error 400** – clock-out time is before clock-in time

---

### Time Records (CRUD)

#### Create Record (manual / admin correction)

```
POST /records
```

| Field | Type | Required |
|---|---|---|
| `user_id` | string | ✓ |
| `clock_in` | RFC3339 | ✓ |
| `clock_out` | RFC3339 | — |
| `note` | string | — |

```bash
curl -X POST http://localhost:8080/records \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer my-secret-key" \
  -d '{"user_id":"alice","clock_in":"2024-01-09T09:00:00Z","clock_out":"2024-01-09T17:30:00Z"}'
```

#### Get Record

```
GET /records/{id}
```

```bash
curl -H "Authorization: Bearer my-secret-key" \
  http://localhost:8080/records/1
```

#### Update Record

```
PUT /records/{id}
```

```bash
curl -X PUT http://localhost:8080/records/1 \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer my-secret-key" \
  -d '{"clock_in":"2024-01-09T09:00:00Z","clock_out":"2024-01-09T18:00:00Z","note":"corrected"}'
```

#### Delete Record (soft delete)

```
DELETE /records/{id}
→ 204 No Content
```

```bash
curl -X DELETE -H "Authorization: Bearer my-secret-key" \
  http://localhost:8080/records/1
```

---

### Report

```
GET /report?user_id=alice&from=2024-01-01&to=2024-01-31
```

| Param | Type | Required | Default | Description |
|---|---|---|---|---|
| `user_id` | string | ✓ | — | User to report on |
| `from` | YYYY-MM-DD | ✓ | — | Start date (inclusive) |
| `to` | YYYY-MM-DD | ✓ | — | End date (inclusive) |
| `page` | int | — | `1` | Page number |
| `page_size` | int | — | `31` | Days per page (max 366) |

```bash
curl -H "Authorization: Bearer my-secret-key" \
  "http://localhost:8080/report?user_id=alice&from=2024-01-08&to=2024-01-12"
```

**Response 200**
```json
{
  "page": 1,
  "page_size": 31,
  "total_days": 5,
  "report": {
    "user_id": "alice",
    "from": "2024-01-08T00:00:00Z",
    "to": "2024-01-12T23:59:59Z",
    "total_worked_hours": 42.5,
    "total_overtime_hours": 2.5,
    "days": [
      {
        "date": "2024-01-08T00:00:00Z",
        "is_working_day": true,
        "worked_seconds": 32400,
        "worked_hours": 9.0,
        "overtime_hours": 1.0,
        "records": [...]
      }
    ]
  }
}
```

---

## Database Schema

```sql
-- time_records: stores each clock-in/clock-out session
CREATE TABLE time_records (
    id         BIGSERIAL PRIMARY KEY,
    user_id    TEXT        NOT NULL,
    clock_in   TIMESTAMPTZ NOT NULL,
    clock_out  TIMESTAMPTZ,             -- NULL means still clocked in
    note       TEXT        NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ             -- soft delete
);

-- Indexes for common query patterns
CREATE INDEX idx_time_records_user_id  ON time_records(user_id);
CREATE INDEX idx_time_records_clock_in ON time_records(clock_in);
CREATE INDEX idx_time_records_active   ON time_records(user_id, clock_out)
    WHERE deleted_at IS NULL;          -- partial index for active sessions

-- work_calendars: defines normal hours and working days
CREATE TABLE work_calendars (
    id                   BIGSERIAL PRIMARY KEY,
    name                 TEXT         NOT NULL DEFAULT 'Default',
    normal_hours_per_day NUMERIC(5,2) NOT NULL DEFAULT 8.0,
    working_days         JSONB        NOT NULL DEFAULT '[1,2,3,4,5]'
    -- working_days: JSON array of weekday integers (0=Sun, 1=Mon … 6=Sat)
);
```

Migrations run automatically on startup via the embedded runner in `internal/db/db.go`.

---

## Assumptions

1. **User identity** — `user_id` is a plain string (1–128 alphanumeric, dash, underscore, or dot characters). Optional API key authentication is available via the `API_KEY` environment variable; when unset, the API assumes a trusted internal caller or an upstream gateway.

2. **Single active record per user** — A user can have at most one open (no `clock_out`) record at a time. Attempting a second clock-in returns HTTP 409.

3. **Time zones** — All timestamps are stored as `TIMESTAMPTZ` in UTC. Clients are responsible for converting to/from their local timezone before calling the API.

4. **Overtime applies only on working days** — Hours worked on non-working days (weekends, per the calendar) count toward `worked_hours` but not `overtime_hours`.

5. **No break deduction** — The system records raw clock-in to clock-out duration. Break deductions are not implemented; this is noted as a potential enhancement.

6. **Work calendar** — A single default calendar (Mon–Fri, 8 hrs/day) is seeded on first run. A future `/calendar` endpoint can expose CRUD for this.

7. **Overlap detection** — Manual record creation and updates check for overlapping records for the same user. Active (no clock-out) records are excluded from the overlap check.

8. **Concurrency** — Per-user mutexes (held in a `sync.Map`) prevent double clock-in from concurrent requests. Database-level uniqueness constraints (`idx_time_records_active`) provide a second line of defence.

---

## Running Tests

```bash
go test ./... -race -v
```

Test coverage includes:
- Clock-in / clock-out happy path
- Invalid state transitions (double clock-in, clock-out when idle)
- Invalid time ranges (clock-out before clock-in)
- Delete + not-found handling
- Overtime calculation on a working day
- Weekend worked time with zero overtime
