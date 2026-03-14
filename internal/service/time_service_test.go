package service_test

import (
	"errors"
	"testing"
	"time"

	"github.com/rinaldypasya/time-recording/internal/domain"
	"github.com/rinaldypasya/time-recording/internal/service"
)

// ---- in-memory mock repository ----

type mockRecordRepo struct {
	records   map[int64]*domain.TimeRecord
	nextID    int64
	activeRec *domain.TimeRecord // simulates an open record
}

func newMockRepo() *mockRecordRepo {
	return &mockRecordRepo{records: make(map[int64]*domain.TimeRecord), nextID: 1}
}

func (m *mockRecordRepo) Create(rec *domain.TimeRecord) (*domain.TimeRecord, error) {
	rec.ID = m.nextID
	m.nextID++
	rec.CreatedAt = time.Now()
	rec.UpdatedAt = time.Now()
	m.records[rec.ID] = rec
	if rec.ClockOut == nil {
		m.activeRec = rec
	}
	return rec, nil
}

func (m *mockRecordRepo) GetByID(id int64) (*domain.TimeRecord, error) {
	if r, ok := m.records[id]; ok {
		return r, nil
	}
	return nil, domain.ErrRecordNotFound
}

func (m *mockRecordRepo) GetActiveRecord(userID string) (*domain.TimeRecord, error) {
	if m.activeRec != nil && m.activeRec.UserID == userID {
		return m.activeRec, nil
	}
	return nil, nil
}

func (m *mockRecordRepo) GetByUserAndDateRange(userID string, from, to time.Time) ([]*domain.TimeRecord, error) {
	var result []*domain.TimeRecord
	for _, r := range m.records {
		if r.UserID == userID && !r.ClockIn.Before(from) && r.ClockIn.Before(to) {
			result = append(result, r)
		}
	}
	return result, nil
}

func (m *mockRecordRepo) Update(rec *domain.TimeRecord) (*domain.TimeRecord, error) {
	if _, ok := m.records[rec.ID]; !ok {
		return nil, domain.ErrRecordNotFound
	}
	rec.UpdatedAt = time.Now()
	m.records[rec.ID] = rec
	if rec.ClockOut != nil && m.activeRec != nil && m.activeRec.ID == rec.ID {
		m.activeRec = nil
	}
	return rec, nil
}

func (m *mockRecordRepo) Delete(id int64) error {
	if _, ok := m.records[id]; !ok {
		return domain.ErrRecordNotFound
	}
	delete(m.records, id)
	if m.activeRec != nil && m.activeRec.ID == id {
		m.activeRec = nil
	}
	return nil
}

func (m *mockRecordRepo) CheckOverlap(userID string, ci, co time.Time, excludeID int64) (bool, error) {
	return false, nil
}

// ---- mock calendar repo ----

type mockCalRepo struct{}

func (m *mockCalRepo) GetDefault() (*domain.WorkCalendar, error) {
	return &domain.WorkCalendar{
		ID:                1,
		Name:              "Default",
		NormalHoursPerDay: 8.0,
		WorkingDays:       []int{1, 2, 3, 4, 5}, // Mon-Fri
	}, nil
}
func (m *mockCalRepo) GetByID(id int64) (*domain.WorkCalendar, error) { return m.GetDefault() }
func (m *mockCalRepo) Upsert(cal *domain.WorkCalendar) (*domain.WorkCalendar, error) {
	return cal, nil
}

// ---- tests ----

func newSvc() *service.TimeService {
	return service.NewTimeService(newMockRepo(), &mockCalRepo{})
}

func TestClockIn_Success(t *testing.T) {
	svc := newSvc()
	rec, err := svc.ClockIn("user1", "morning shift", time.Now())
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if rec.ID == 0 {
		t.Fatal("expected non-zero ID")
	}
	if rec.ClockOut != nil {
		t.Fatal("expected clock_out to be nil after clock-in")
	}
}

func TestClockIn_AlreadyClockedIn(t *testing.T) {
	svc := newSvc()
	if _, err := svc.ClockIn("user1", "", time.Now()); err != nil {
		t.Fatal(err)
	}
	_, err := svc.ClockIn("user1", "", time.Now())
	if !errors.Is(err, domain.ErrAlreadyClockedIn) {
		t.Fatalf("expected ErrAlreadyClockedIn, got %v", err)
	}
}

func TestClockOut_Success(t *testing.T) {
	svc := newSvc()
	now := time.Now()
	if _, err := svc.ClockIn("user1", "", now); err != nil {
		t.Fatal(err)
	}
	rec, err := svc.ClockOut("user1", "done", now.Add(8*time.Hour))
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if rec.ClockOut == nil {
		t.Fatal("expected clock_out to be set")
	}
}

func TestClockOut_NotClockedIn(t *testing.T) {
	svc := newSvc()
	_, err := svc.ClockOut("user1", "", time.Now())
	if !errors.Is(err, domain.ErrNotClockedIn) {
		t.Fatalf("expected ErrNotClockedIn, got %v", err)
	}
}

func TestClockOut_InvalidTime(t *testing.T) {
	svc := newSvc()
	now := time.Now()
	if _, err := svc.ClockIn("user1", "", now); err != nil {
		t.Fatal(err)
	}
	_, err := svc.ClockOut("user1", "", now.Add(-1*time.Hour))
	if !errors.Is(err, domain.ErrInvalidTimeRange) {
		t.Fatalf("expected ErrInvalidTimeRange, got %v", err)
	}
}

func TestDeleteRecord(t *testing.T) {
	svc := newSvc()
	rec, _ := svc.ClockIn("user1", "", time.Now())
	if err := svc.DeleteRecord(rec.ID); err != nil {
		t.Fatalf("delete failed: %v", err)
	}
	if err := svc.DeleteRecord(rec.ID); !errors.Is(err, domain.ErrRecordNotFound) {
		t.Fatalf("expected ErrRecordNotFound, got %v", err)
	}
}

func TestGenerateReport_Overtime(t *testing.T) {
	repo := newMockRepo()
	svc := service.NewTimeService(repo, &mockCalRepo{})

	// Monday with 10 hours of work → 2 hrs overtime
	monday := time.Date(2024, 1, 8, 9, 0, 0, 0, time.UTC)
	clockOut := monday.Add(10 * time.Hour)
	repo.Create(&domain.TimeRecord{
		UserID:   "user1",
		ClockIn:  monday,
		ClockOut: &clockOut,
	})

	report, err := svc.GenerateReport("user1", monday, monday)
	if err != nil {
		t.Fatalf("report error: %v", err)
	}
	if report.TotalWorked != 10.0 {
		t.Errorf("expected 10 worked hours, got %.2f", report.TotalWorked)
	}
	if report.TotalOvertime != 2.0 {
		t.Errorf("expected 2 overtime hours, got %.2f", report.TotalOvertime)
	}
}

func TestGenerateReport_Weekend_NoOvertime(t *testing.T) {
	repo := newMockRepo()
	svc := service.NewTimeService(repo, &mockCalRepo{})

	// Sunday = not a working day → no overtime even with 10hrs
	sunday := time.Date(2024, 1, 7, 9, 0, 0, 0, time.UTC) // Sunday
	clockOut := sunday.Add(10 * time.Hour)
	repo.Create(&domain.TimeRecord{
		UserID:   "user1",
		ClockIn:  sunday,
		ClockOut: &clockOut,
	})

	report, err := svc.GenerateReport("user1", sunday, sunday)
	if err != nil {
		t.Fatal(err)
	}
	if report.TotalOvertime != 0 {
		t.Errorf("expected 0 overtime on weekend, got %.2f", report.TotalOvertime)
	}
}
