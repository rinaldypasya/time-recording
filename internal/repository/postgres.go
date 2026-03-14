package repository

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/rinaldypasya/time-recording/internal/domain"
)

type pgTimeRecordRepo struct {
	db *sql.DB
}

// NewTimeRecordRepository creates a new PostgreSQL-backed repository
func NewTimeRecordRepository(db *sql.DB) domain.TimeRecordRepository {
	return &pgTimeRecordRepo{db: db}
}

func (r *pgTimeRecordRepo) Create(record *domain.TimeRecord) (*domain.TimeRecord, error) {
	query := `
		INSERT INTO time_records (user_id, clock_in, clock_out, note, created_at, updated_at)
		VALUES ($1, $2, $3, $4, NOW(), NOW())
		RETURNING id, created_at, updated_at`

	row := r.db.QueryRow(query, record.UserID, record.ClockIn, record.ClockOut, record.Note)
	err := row.Scan(&record.ID, &record.CreatedAt, &record.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("create time record: %w", err)
	}
	return record, nil
}

func (r *pgTimeRecordRepo) GetByID(id int64) (*domain.TimeRecord, error) {
	query := `
		SELECT id, user_id, clock_in, clock_out, note, created_at, updated_at
		FROM time_records WHERE id = $1 AND deleted_at IS NULL`

	rec := &domain.TimeRecord{}
	err := r.db.QueryRow(query, id).Scan(
		&rec.ID, &rec.UserID, &rec.ClockIn, &rec.ClockOut,
		&rec.Note, &rec.CreatedAt, &rec.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, domain.ErrRecordNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get time record by id: %w", err)
	}
	return rec, nil
}

func (r *pgTimeRecordRepo) GetActiveRecord(userID string) (*domain.TimeRecord, error) {
	query := `
		SELECT id, user_id, clock_in, clock_out, note, created_at, updated_at
		FROM time_records
		WHERE user_id = $1 AND clock_out IS NULL AND deleted_at IS NULL
		ORDER BY clock_in DESC LIMIT 1`

	rec := &domain.TimeRecord{}
	err := r.db.QueryRow(query, userID).Scan(
		&rec.ID, &rec.UserID, &rec.ClockIn, &rec.ClockOut,
		&rec.Note, &rec.CreatedAt, &rec.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get active record: %w", err)
	}
	return rec, nil
}

func (r *pgTimeRecordRepo) GetByUserAndDateRange(userID string, from, to time.Time) ([]*domain.TimeRecord, error) {
	query := `
		SELECT id, user_id, clock_in, clock_out, note, created_at, updated_at
		FROM time_records
		WHERE user_id = $1
		  AND clock_in >= $2
		  AND clock_in < $3
		  AND deleted_at IS NULL
		ORDER BY clock_in ASC`

	rows, err := r.db.Query(query, userID, from, to)
	if err != nil {
		return nil, fmt.Errorf("get records by range: %w", err)
	}
	defer rows.Close()

	var records []*domain.TimeRecord
	for rows.Next() {
		rec := &domain.TimeRecord{}
		if err := rows.Scan(
			&rec.ID, &rec.UserID, &rec.ClockIn, &rec.ClockOut,
			&rec.Note, &rec.CreatedAt, &rec.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan record: %w", err)
		}
		records = append(records, rec)
	}
	return records, rows.Err()
}

func (r *pgTimeRecordRepo) Update(record *domain.TimeRecord) (*domain.TimeRecord, error) {
	query := `
		UPDATE time_records
		SET clock_in = $1, clock_out = $2, note = $3, updated_at = NOW()
		WHERE id = $4 AND deleted_at IS NULL
		RETURNING updated_at`

	err := r.db.QueryRow(query, record.ClockIn, record.ClockOut, record.Note, record.ID).
		Scan(&record.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, domain.ErrRecordNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("update time record: %w", err)
	}
	return record, nil
}

func (r *pgTimeRecordRepo) Delete(id int64) error {
	query := `UPDATE time_records SET deleted_at = NOW() WHERE id = $1 AND deleted_at IS NULL`
	res, err := r.db.Exec(query, id)
	if err != nil {
		return fmt.Errorf("delete time record: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return domain.ErrRecordNotFound
	}
	return nil
}

func (r *pgTimeRecordRepo) CheckOverlap(userID string, clockIn, clockOut time.Time, excludeID int64) (bool, error) {
	query := `
		SELECT COUNT(*) FROM time_records
		WHERE user_id = $1
		  AND id != $2
		  AND deleted_at IS NULL
		  AND clock_out IS NOT NULL
		  AND clock_in < $4
		  AND clock_out > $3`

	var count int
	err := r.db.QueryRow(query, userID, excludeID, clockIn, clockOut).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("check overlap: %w", err)
	}
	return count > 0, nil
}

// ----- Work Calendar Repository -----

type pgWorkCalendarRepo struct {
	db *sql.DB
}

func NewWorkCalendarRepository(db *sql.DB) domain.WorkCalendarRepository {
	return &pgWorkCalendarRepo{db: db}
}

func (r *pgWorkCalendarRepo) GetDefault() (*domain.WorkCalendar, error) {
	return r.GetByID(1)
}

func (r *pgWorkCalendarRepo) GetByID(id int64) (*domain.WorkCalendar, error) {
	query := `SELECT id, name, normal_hours_per_day, working_days FROM work_calendars WHERE id = $1`
	cal := &domain.WorkCalendar{}
	var workingDaysJSON []byte
	err := r.db.QueryRow(query, id).Scan(&cal.ID, &cal.Name, &cal.NormalHoursPerDay, &workingDaysJSON)
	if err == sql.ErrNoRows {
		// Return sensible default: Mon-Fri, 8h
		return &domain.WorkCalendar{
			ID:                1,
			Name:              "Default",
			NormalHoursPerDay: 8.0,
			WorkingDays:       []int{1, 2, 3, 4, 5},
		}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get calendar: %w", err)
	}
	if err := json.Unmarshal(workingDaysJSON, &cal.WorkingDays); err != nil {
		return nil, fmt.Errorf("parse working_days: %w", err)
	}
	return cal, nil
}

func (r *pgWorkCalendarRepo) Upsert(cal *domain.WorkCalendar) (*domain.WorkCalendar, error) {
	workingDaysJSON, err := json.Marshal(cal.WorkingDays)
	if err != nil {
		return nil, err
	}
	query := `
		INSERT INTO work_calendars (id, name, normal_hours_per_day, working_days)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (id) DO UPDATE
		SET name = EXCLUDED.name,
		    normal_hours_per_day = EXCLUDED.normal_hours_per_day,
		    working_days = EXCLUDED.working_days
		RETURNING id`
	err = r.db.QueryRow(query, cal.ID, cal.Name, cal.NormalHoursPerDay, workingDaysJSON).Scan(&cal.ID)
	if err != nil {
		return nil, fmt.Errorf("upsert calendar: %w", err)
	}
	return cal, nil
}
