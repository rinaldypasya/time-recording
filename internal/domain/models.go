package domain

import (
	"errors"
	"time"
)

// ClockStatus represents whether a user is clocked-in or clocked-out
type ClockStatus string

const (
	StatusClockedIn  ClockStatus = "clocked_in"
	StatusClockedOut ClockStatus = "clocked_out"
)

// EventType represents a clock-in or clock-out event
type EventType string

const (
	EventClockIn  EventType = "clock_in"
	EventClockOut EventType = "clock_out"
)

// TimeRecord represents a single clock-in/clock-out session
type TimeRecord struct {
	ID         int64      `json:"id"`
	UserID     string     `json:"user_id"`
	ClockIn    time.Time  `json:"clock_in"`
	ClockOut   *time.Time `json:"clock_out,omitempty"`
	Note       string     `json:"note,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

// Duration returns worked duration for a completed record
func (r *TimeRecord) Duration() time.Duration {
	if r.ClockOut == nil {
		return 0
	}
	return r.ClockOut.Sub(r.ClockIn)
}

// IsComplete returns true if the record has both clock-in and clock-out
func (r *TimeRecord) IsComplete() bool {
	return r.ClockOut != nil
}

// WorkCalendar defines working day rules
type WorkCalendar struct {
	ID                 int64   `json:"id"`
	Name               string  `json:"name"`
	NormalHoursPerDay  float64 `json:"normal_hours_per_day"` // e.g., 8.0
	WorkingDays        []int   `json:"working_days"`          // 0=Sun,1=Mon,...,6=Sat
}

// IsWorkingDay returns true if the given weekday is a working day
func (wc *WorkCalendar) IsWorkingDay(weekday time.Weekday) bool {
	for _, d := range wc.WorkingDays {
		if d == int(weekday) {
			return true
		}
	}
	return false
}

// DailySummary holds aggregated data for a single day
type DailySummary struct {
	Date          time.Time     `json:"date"`
	IsWorkingDay  bool          `json:"is_working_day"`
	WorkedSeconds float64       `json:"worked_seconds"`
	WorkedHours   float64       `json:"worked_hours"`
	OvertimeHours float64       `json:"overtime_hours"`
	Records       []*TimeRecord `json:"records"`
}

// Report holds aggregated data for a date range
type Report struct {
	UserID        string          `json:"user_id"`
	From          time.Time       `json:"from"`
	To            time.Time       `json:"to"`
	TotalWorked   float64         `json:"total_worked_hours"`
	TotalOvertime float64         `json:"total_overtime_hours"`
	Days          []*DailySummary `json:"days"`
}

// Sentinel errors
var (
	ErrAlreadyClockedIn  = errors.New("user is already clocked in")
	ErrNotClockedIn      = errors.New("user is not clocked in")
	ErrRecordNotFound    = errors.New("time record not found")
	ErrInvalidTimeRange  = errors.New("clock_out must be after clock_in")
	ErrOverlappingRecord = errors.New("time record overlaps with an existing record")
)
