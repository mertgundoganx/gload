package scheduler

import (
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/mertgundoganx/gload/internal/storage"
)

// RunFunc is called when a scheduled test should execute.
type RunFunc func(serviceID int64)

// Scheduler periodically checks for due schedules and triggers test runs.
type Scheduler struct {
	store  *storage.Storage
	runFn  RunFunc
	stopCh chan struct{}
}

// New creates a new Scheduler.
func New(store *storage.Storage, runFn RunFunc) *Scheduler {
	return &Scheduler{
		store:  store,
		runFn:  runFn,
		stopCh: make(chan struct{}),
	}
}

// Start begins the scheduler loop in a background goroutine.
func (s *Scheduler) Start() {
	go s.loop()
}

// Stop signals the scheduler loop to exit. Safe to call multiple times.
func (s *Scheduler) Stop() {
	select {
	case <-s.stopCh:
		// already closed
	default:
		close(s.stopCh)
	}
}

func (s *Scheduler) loop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-s.stopCh:
			return
		case now := <-ticker.C:
			s.checkAndRun(now)
		}
	}
}

func (s *Scheduler) checkAndRun(now time.Time) {
	due, err := s.store.GetDueSchedules(now)
	if err != nil {
		log.Printf("scheduler: error getting due schedules: %v", err)
		return
	}
	for _, sched := range due {
		log.Printf("scheduler: running scheduled test for service %d", sched.ServiceID)
		s.runFn(sched.ServiceID)
		nextRun := NextCronTime(sched.CronExpr, now)
		if err := s.store.UpdateScheduleRun(sched.ID, now, nextRun); err != nil {
			log.Printf("scheduler: error updating schedule run %d: %v", sched.ID, err)
		}
	}
}

// NextCronTime returns the next time after 'after' that matches the cron expression.
// The expression format is "minute hour day-of-month month day-of-week".
// Fields may use "*", "*/n", exact numbers, ranges ("a-b"), lists ("a,b"),
// and steps over ranges ("a-b/n").
func NextCronTime(expr string, after time.Time) time.Time {
	fields := strings.Fields(expr)
	if len(fields) != 5 {
		return after.Add(24 * time.Hour) // fallback
	}

	// Search up to ~366 days so infrequent schedules (e.g. weekdays evaluated
	// on a Friday night, or a specific month/day) still resolve correctly.
	const maxMinutes = 366 * 24 * 60
	t := after.Add(time.Minute).Truncate(time.Minute)
	for i := 0; i < maxMinutes; i++ {
		if matchesCron(fields, t) {
			return t
		}
		t = t.Add(time.Minute)
	}
	return after.Add(24 * time.Hour) // fallback (should be unreachable for valid exprs)
}

// cronBounds holds the valid [min,max] for each of the 5 cron fields.
var cronBounds = [5][2]int{
	{0, 59}, // minute
	{0, 23}, // hour
	{1, 31}, // day of month
	{1, 12}, // month
	{0, 6},  // day of week (0=Sunday)
}

func matchesCron(fields []string, t time.Time) bool {
	dow := int(t.Weekday()) // 0=Sunday..6=Saturday
	return matchField(fields[0], t.Minute(), 0, 59) &&
		matchField(fields[1], t.Hour(), 0, 23) &&
		matchField(fields[2], t.Day(), 1, 31) &&
		matchField(fields[3], int(t.Month()), 1, 12) &&
		matchDOW(fields[4], dow)
}

// matchDOW matches the day-of-week field, accepting 7 as an alias for Sunday.
func matchDOW(field string, val int) bool {
	if matchField(field, val, 0, 6) {
		return true
	}
	// Standard cron allows 7 for Sunday; match it against 0.
	if val == 0 {
		return matchField(field, 7, 0, 7)
	}
	return false
}

// matchField reports whether val satisfies a cron field that may contain a
// comma-separated list of terms, each being "*", "*/n", "a", "a-b", or "a-b/n".
func matchField(field string, val, min, max int) bool {
	for _, term := range strings.Split(field, ",") {
		if matchTerm(strings.TrimSpace(term), val, min, max) {
			return true
		}
	}
	return false
}

func matchTerm(term string, val, min, max int) bool {
	step := 1
	if i := strings.IndexByte(term, '/'); i >= 0 {
		n, err := strconv.Atoi(term[i+1:])
		if err != nil || n <= 0 {
			return false
		}
		step = n
		term = term[:i]
	}

	lo, hi := min, max
	switch {
	case term == "*":
		// full range with step
	case strings.ContainsRune(term, '-'):
		parts := strings.SplitN(term, "-", 2)
		a, err1 := strconv.Atoi(parts[0])
		b, err2 := strconv.Atoi(parts[1])
		if err1 != nil || err2 != nil || a > b {
			return false
		}
		lo, hi = a, b
	default:
		n, err := strconv.Atoi(term)
		if err != nil {
			return false
		}
		lo, hi = n, n
	}
	if val < lo || val > hi {
		return false
	}
	return (val-lo)%step == 0
}

// ValidateCron checks that expr is a well-formed 5-field cron expression whose
// values are within range. Returns a descriptive error if not.
func ValidateCron(expr string) error {
	fields := strings.Fields(expr)
	if len(fields) != 5 {
		return fmt.Errorf("cron must have 5 fields (minute hour day month weekday), got %d", len(fields))
	}
	names := []string{"minute", "hour", "day-of-month", "month", "day-of-week"}
	for i, f := range fields {
		max := cronBounds[i][1]
		if i == 4 {
			max = 7 // allow 7 for Sunday in validation
		}
		if err := validateField(f, cronBounds[i][0], max); err != nil {
			return fmt.Errorf("%s field %q: %w", names[i], f, err)
		}
	}
	return nil
}

func validateField(field string, min, max int) error {
	for _, term := range strings.Split(field, ",") {
		term = strings.TrimSpace(term)
		if term == "" {
			return fmt.Errorf("empty term")
		}
		if i := strings.IndexByte(term, '/'); i >= 0 {
			n, err := strconv.Atoi(term[i+1:])
			if err != nil || n <= 0 {
				return fmt.Errorf("invalid step %q", term[i+1:])
			}
			term = term[:i]
		}
		if term == "*" {
			continue
		}
		if strings.ContainsRune(term, '-') {
			parts := strings.SplitN(term, "-", 2)
			a, err1 := strconv.Atoi(parts[0])
			b, err2 := strconv.Atoi(parts[1])
			if err1 != nil || err2 != nil {
				return fmt.Errorf("invalid range")
			}
			if a > b || a < min || b > max {
				return fmt.Errorf("range %d-%d out of bounds [%d-%d]", a, b, min, max)
			}
			continue
		}
		n, err := strconv.Atoi(term)
		if err != nil {
			return fmt.Errorf("not a number")
		}
		if n < min || n > max {
			return fmt.Errorf("value %d out of bounds [%d-%d]", n, min, max)
		}
	}
	return nil
}
