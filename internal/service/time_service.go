package service

import (
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/pasya/time-recording/internal/domain"
)

// TimeService handles all business logic for time recording
type TimeService struct {
	records  domain.TimeRecordRepository
	calendar domain.WorkCalendarRepository
	// Per-user mutex to prevent concurrent clock-in/out races
	mu    sync.Map // map[userID]*sync.Mutex
}

func NewTimeService(r domain.TimeRecordRepository, c domain.WorkCalendarRepository) *TimeService {
	return &TimeService{records: r, calendar: c}
}

func (s *TimeService) lockUser(userID string) func() {
	val, _ := s.mu.LoadOrStore(userID, &sync.Mutex{})
	mu := val.(*sync.Mutex)
	mu.Lock()
	return mu.Unlock
}

// ClockIn creates an open time record for a user
func (s *TimeService) ClockIn(userID, note string, at time.Time) (*domain.TimeRecord, error) {
	unlock := s.lockUser(userID)
	defer unlock()

	active, err := s.records.GetActiveRecord(userID)
	if err != nil {
		return nil, err
	}
	if active != nil {
		return nil, domain.ErrAlreadyClockedIn
	}

	rec := &domain.TimeRecord{
		UserID:  userID,
		ClockIn: at,
		Note:    note,
	}
	return s.records.Create(rec)
}

// ClockOut closes the open record for a user
func (s *TimeService) ClockOut(userID, note string, at time.Time) (*domain.TimeRecord, error) {
	unlock := s.lockUser(userID)
	defer unlock()

	active, err := s.records.GetActiveRecord(userID)
	if err != nil {
		return nil, err
	}
	if active == nil {
		return nil, domain.ErrNotClockedIn
	}
	if at.Before(active.ClockIn) || at.Equal(active.ClockIn) {
		return nil, domain.ErrInvalidTimeRange
	}

	active.ClockOut = &at
	if note != "" {
		active.Note = note
	}
	return s.records.Update(active)
}

// GetRecord fetches a single record by ID
func (s *TimeService) GetRecord(id int64) (*domain.TimeRecord, error) {
	return s.records.GetByID(id)
}

// CreateRecord manually creates a complete time record (admin / correction)
func (s *TimeService) CreateRecord(userID string, clockIn, clockOut time.Time, note string) (*domain.TimeRecord, error) {
	if !clockOut.IsZero() && clockOut.Before(clockIn) {
		return nil, domain.ErrInvalidTimeRange
	}

	// Check for overlaps on complete records
	if !clockOut.IsZero() {
		overlap, err := s.records.CheckOverlap(userID, clockIn, clockOut, 0)
		if err != nil {
			return nil, err
		}
		if overlap {
			return nil, domain.ErrOverlappingRecord
		}
	}

	rec := &domain.TimeRecord{
		UserID:  userID,
		ClockIn: clockIn,
		Note:    note,
	}
	if !clockOut.IsZero() {
		rec.ClockOut = &clockOut
	}
	return s.records.Create(rec)
}

// UpdateRecord modifies an existing record
func (s *TimeService) UpdateRecord(id int64, clockIn, clockOut time.Time, note string) (*domain.TimeRecord, error) {
	rec, err := s.records.GetByID(id)
	if err != nil {
		return nil, err
	}

	if !clockOut.IsZero() && clockOut.Before(clockIn) {
		return nil, domain.ErrInvalidTimeRange
	}

	if !clockOut.IsZero() {
		overlap, err := s.records.CheckOverlap(rec.UserID, clockIn, clockOut, id)
		if err != nil {
			return nil, err
		}
		if overlap {
			return nil, domain.ErrOverlappingRecord
		}
	}

	rec.ClockIn = clockIn
	if !clockOut.IsZero() {
		rec.ClockOut = &clockOut
	} else {
		rec.ClockOut = nil
	}
	rec.Note = note
	return s.records.Update(rec)
}

// DeleteRecord soft-deletes a record
func (s *TimeService) DeleteRecord(id int64) error {
	return s.records.Delete(id)
}

// GenerateReport builds a daily + aggregate report for a user over a date range
func (s *TimeService) GenerateReport(userID string, from, to time.Time) (*domain.Report, error) {
	// Normalise to start/end of day in the from/to dates
	from = startOfDay(from)
	to = endOfDay(to)

	cal, err := s.calendar.GetDefault()
	if err != nil {
		return nil, fmt.Errorf("load calendar: %w", err)
	}

	recs, err := s.records.GetByUserAndDateRange(userID, from, to.Add(24*time.Hour))
	if err != nil {
		return nil, err
	}

	// Group records by date
	byDate := map[string][]*domain.TimeRecord{}
	for _, r := range recs {
		key := r.ClockIn.Format("2006-01-02")
		byDate[key] = append(byDate[key], r)
	}

	report := &domain.Report{
		UserID: userID,
		From:   from,
		To:     to,
	}

	normalSeconds := cal.NormalHoursPerDay * 3600

	// Iterate day by day
	for d := from; !d.After(to); d = d.AddDate(0, 0, 1) {
		key := d.Format("2006-01-02")
		dayRecs := byDate[key]

		var workedSec float64
		for _, r := range dayRecs {
			workedSec += r.Duration().Seconds()
		}

		workedHours := workedSec / 3600
		isWorking := cal.IsWorkingDay(d.Weekday())
		var overtime float64
		if isWorking && workedSec > normalSeconds {
			overtime = (workedSec - normalSeconds) / 3600
		}

		report.Days = append(report.Days, &domain.DailySummary{
			Date:          d,
			IsWorkingDay:  isWorking,
			WorkedSeconds: workedSec,
			WorkedHours:   roundHours(workedHours),
			OvertimeHours: roundHours(overtime),
			Records:       dayRecs,
		})

		report.TotalWorked += workedHours
		report.TotalOvertime += overtime
	}

	report.TotalWorked = roundHours(report.TotalWorked)
	report.TotalOvertime = roundHours(report.TotalOvertime)
	return report, nil
}

func startOfDay(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, t.Location())
}

func endOfDay(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 23, 59, 59, 999999999, t.Location())
}

func roundHours(h float64) float64 {
	return math.Round(h*100) / 100
}
