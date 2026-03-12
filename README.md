# Time Recording System

A production-grade REST API for employee time tracking, built in Go using clean architecture principles — no web framework, no ORM, just the standard library + `lib/pq`.

## Architecture

```
cmd/api/             → entrypoint (wiring, server startup)
internal/
  domain/            → models, interfaces, sentinel errors
  repository/        → PostgreSQL implementations
  service/           → business logic (clock state machine, overtime calc, reporting)
  handler/           → HTTP routing & request/response marshalling
  middleware/        → request logging, request-ID injection
  db/                → connection pool + embedded migrations
```

**Key design decisions**

| Decision | Rationale |
|---|---|
| Standard `net/http` mux | No framework overhead; sufficient for a REST API |
| Per-user `sync.Mutex` via `sync.Map` | Prevents race conditions on clock-in/out without locking every user |
| Soft deletes (`deleted_at`) | Preserves audit trail for time records |
| Embedded migrations in Go | No external migration tool needed; runs on startup |
| Interface-driven repository | Enables full unit testing without a database |

---

## Prerequisites

- Go 1.22+
- Docker & Docker Compose (recommended)

---

## Quick Start (Docker)

```bash
git clone <repo-url>
cd time-recording
docker compose up --build
```

The API will be available at `http://localhost:8080`.

---

## Local Development

```bash
# 1. Start Postgres
docker compose up postgres -d

# 2. Set connection string
export DATABASE_URL="postgres://postgres:postgres@localhost:5432/timerecording?sslmode=disable"

# 3. Run
go run ./cmd/api

# 4. Test
go test ./... -race
```

---

## API Reference

All request/response bodies are JSON. Timestamps use **RFC3339** format (e.g. `2024-01-08T09:00:00Z`).  
Dates in query params use `YYYY-MM-DD`.

### Health

```
GET /health
→ 200  {"status":"ok"}
```

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
```json
{"error": "user is already clocked in"}
```

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
  -d '{"user_id":"alice","clock_in":"2024-01-09T09:00:00Z","clock_out":"2024-01-09T17:30:00Z"}'
```

#### Get Record

```
GET /records/{id}
```

```bash
curl http://localhost:8080/records/1
```

#### Update Record

```
PUT /records/{id}
```

```bash
curl -X PUT http://localhost:8080/records/1 \
  -H "Content-Type: application/json" \
  -d '{"clock_in":"2024-01-09T09:00:00Z","clock_out":"2024-01-09T18:00:00Z","note":"corrected"}'
```

#### Delete Record (soft delete)

```
DELETE /records/{id}
→ 204 No Content
```

```bash
curl -X DELETE http://localhost:8080/records/1
```

---

### Report

```
GET /report?user_id=alice&from=2024-01-01&to=2024-01-31
```

```bash
curl "http://localhost:8080/report?user_id=alice&from=2024-01-08&to=2024-01-12"
```

**Response 200**
```json
{
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

1. **User identity** — `user_id` is a plain string (e.g. UUID or email). Authentication/authorisation is out of scope; the API assumes a trusted internal caller or an upstream gateway handles it.

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
