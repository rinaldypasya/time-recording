package domain

import "time"

// TimeRecordRepository defines persistence operations for time records
type TimeRecordRepository interface {
	// Create inserts a new clock-in record (no clock-out yet)
	Create(record *TimeRecord) (*TimeRecord, error)

	// GetByID fetches a single record by ID
	GetByID(id int64) (*TimeRecord, error)

	// GetActiveRecord returns the open (no clock-out) record for a user, if any
	GetActiveRecord(userID string) (*TimeRecord, error)

	// GetByUserAndDateRange returns all records for a user within [from, to]
	GetByUserAndDateRange(userID string, from, to time.Time) ([]*TimeRecord, error)

	// Update saves changes to an existing record (used for clock-out & edits)
	Update(record *TimeRecord) (*TimeRecord, error)

	// Delete removes a record by ID
	Delete(id int64) error

	// CheckOverlap returns true if [clockIn, clockOut] overlaps any existing record for the user
	CheckOverlap(userID string, clockIn time.Time, clockOut time.Time, excludeID int64) (bool, error)
}

// WorkCalendarRepository defines persistence for work calendar configuration
type WorkCalendarRepository interface {
	GetDefault() (*WorkCalendar, error)
	GetByID(id int64) (*WorkCalendar, error)
	Upsert(cal *WorkCalendar) (*WorkCalendar, error)
}
