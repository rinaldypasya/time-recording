package db

import (
	"database/sql"
	"fmt"
	"log"
	"time"

	_ "github.com/lib/pq"
)

// Connect opens and verifies a PostgreSQL connection with retries
func Connect(dsn string) (*sql.DB, error) {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(10)
	db.SetConnMaxLifetime(5 * time.Minute)

	// Retry ping up to 10 times (useful when postgres starts after the app)
	for i := 0; i < 10; i++ {
		if err = db.Ping(); err == nil {
			log.Println("database connected")
			return db, nil
		}
		log.Printf("waiting for database... (%d/10)", i+1)
		time.Sleep(2 * time.Second)
	}
	return nil, fmt.Errorf("database not reachable: %w", err)
}

// RunMigrations applies all SQL migration files in order
func RunMigrations(db *sql.DB) error {
	migrations := []struct {
		name string
		sql  string
	}{
		{
			name: "create_time_records",
			sql: `
				CREATE TABLE IF NOT EXISTS time_records (
					id          BIGSERIAL PRIMARY KEY,
					user_id     TEXT        NOT NULL,
					clock_in    TIMESTAMPTZ NOT NULL,
					clock_out   TIMESTAMPTZ,
					note        TEXT        NOT NULL DEFAULT '',
					created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					deleted_at  TIMESTAMPTZ
				);
				CREATE INDEX IF NOT EXISTS idx_time_records_user_id     ON time_records(user_id);
				CREATE INDEX IF NOT EXISTS idx_time_records_clock_in    ON time_records(clock_in);
				CREATE INDEX IF NOT EXISTS idx_time_records_active      ON time_records(user_id, clock_out) WHERE deleted_at IS NULL;
			`,
		},
		{
			name: "create_work_calendars",
			sql: `
				CREATE TABLE IF NOT EXISTS work_calendars (
					id                   BIGSERIAL PRIMARY KEY,
					name                 TEXT    NOT NULL DEFAULT 'Default',
					normal_hours_per_day NUMERIC(5,2) NOT NULL DEFAULT 8.0,
					working_days         JSONB   NOT NULL DEFAULT '[1,2,3,4,5]'
				);
				INSERT INTO work_calendars (id, name, normal_hours_per_day, working_days)
				VALUES (1, 'Default', 8.0, '[1,2,3,4,5]')
				ON CONFLICT (id) DO NOTHING;
			`,
		},
		{
			name: "create_schema_migrations",
			sql: `
				CREATE TABLE IF NOT EXISTS schema_migrations (
					name       TEXT PRIMARY KEY,
					applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
				);
			`,
		},
	}

	// Ensure schema_migrations table exists first
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS schema_migrations (
			name       TEXT PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	for _, m := range migrations {
		var exists bool
		_ = db.QueryRow(`SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE name=$1)`, m.name).Scan(&exists)
		if exists {
			continue
		}
		if _, err := db.Exec(m.sql); err != nil {
			return fmt.Errorf("migration %s: %w", m.name, err)
		}
		if _, err := db.Exec(`INSERT INTO schema_migrations(name) VALUES($1)`, m.name); err != nil {
			return fmt.Errorf("record migration %s: %w", m.name, err)
		}
		log.Printf("applied migration: %s", m.name)
	}
	return nil
}
