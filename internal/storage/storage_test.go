package storage

import (
	"path/filepath"
	"testing"
)

func setupTestDB(t *testing.T) *Storage {
	t.Helper()
	tmpFile := filepath.Join(t.TempDir(), "test.db")
	store, err := New(tmpFile)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

func createTestService(t *testing.T, store *Storage, name, url string) *Service {
	t.Helper()
	svc := &Service{
		Name:        name,
		URL:         url,
		Method:      "GET",
		Headers:     map[string]string{"Accept": "application/json"},
		Concurrency: 10,
		Duration:    "10s",
		Timeout:     "30s",
	}
	if err := store.CreateService(svc); err != nil {
		t.Fatal(err)
	}
	return svc
}

func TestCreateAndGetService(t *testing.T) {
	t.Parallel()
	store := setupTestDB(t)

	svc := createTestService(t, store, "Test Service", "http://example.com")

	if svc.ID == 0 {
		t.Fatal("expected non-zero ID after create")
	}

	got, err := store.GetService(svc.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Fatal("expected service, got nil")
	}
	if got.Name != "Test Service" {
		t.Fatalf("expected name 'Test Service', got %q", got.Name)
	}
	if got.URL != "http://example.com" {
		t.Fatalf("expected url 'http://example.com', got %q", got.URL)
	}
	if got.Method != "GET" {
		t.Fatalf("expected method 'GET', got %q", got.Method)
	}
	if got.Headers["Accept"] != "application/json" {
		t.Fatalf("expected Accept header, got %v", got.Headers)
	}
}

func TestUpdateService(t *testing.T) {
	t.Parallel()
	store := setupTestDB(t)

	svc := createTestService(t, store, "Original", "http://original.com")

	svc.Name = "Updated"
	svc.URL = "http://updated.com"
	if err := store.UpdateService(svc); err != nil {
		t.Fatal(err)
	}

	got, err := store.GetService(svc.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "Updated" {
		t.Fatalf("expected name 'Updated', got %q", got.Name)
	}
	if got.URL != "http://updated.com" {
		t.Fatalf("expected url 'http://updated.com', got %q", got.URL)
	}
}

func TestDeleteService(t *testing.T) {
	t.Parallel()
	store := setupTestDB(t)

	svc := createTestService(t, store, "ToDelete", "http://delete.com")

	if err := store.DeleteService(svc.ID); err != nil {
		t.Fatal(err)
	}

	got, err := store.GetService(svc.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Fatal("expected nil after delete, got service")
	}
}

func TestListServices(t *testing.T) {
	t.Parallel()
	store := setupTestDB(t)

	createTestService(t, store, "Svc1", "http://1.com")
	createTestService(t, store, "Svc2", "http://2.com")
	createTestService(t, store, "Svc3", "http://3.com")

	list, err := store.ListServices()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 3 {
		t.Fatalf("expected 3 services, got %d", len(list))
	}
}

func TestSaveAndGetTestResult(t *testing.T) {
	t.Parallel()
	store := setupTestDB(t)

	svc := createTestService(t, store, "ResultSvc", "http://result.com")

	result := &TestResult{
		DurationMs:   5000,
		TotalReqs:    100,
		Errors:       2,
		RPS:          20,
		AvgLatencyMs: 50,
		P50LatencyMs: 45,
		P95LatencyMs: 90,
		P99LatencyMs: 98,
		MinLatencyMs: 5,
		MaxLatencyMs: 120,
		StatusCodes:  map[int]int{200: 98, 500: 2},
		Note:         "initial test",
	}
	if err := store.SaveTestResult(svc.ID, result); err != nil {
		t.Fatal(err)
	}
	if result.ID == 0 {
		t.Fatal("expected non-zero result ID")
	}

	got, err := store.GetLastResult(svc.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Fatal("expected result, got nil")
	}
	if got.TotalReqs != 100 {
		t.Fatalf("expected 100 total reqs, got %d", got.TotalReqs)
	}
	if got.RPS != 20 {
		t.Fatalf("expected RPS 20, got %f", got.RPS)
	}
	if got.StatusCodes[200] != 98 {
		t.Fatalf("expected 98 status 200, got %d", got.StatusCodes[200])
	}
	if got.StatusCodes[500] != 2 {
		t.Fatalf("expected 2 status 500, got %d", got.StatusCodes[500])
	}
}

func TestListResults(t *testing.T) {
	t.Parallel()
	store := setupTestDB(t)

	svc := createTestService(t, store, "ListResultsSvc", "http://list.com")

	for i := 0; i < 5; i++ {
		r := &TestResult{
			TotalReqs:   (i + 1) * 10,
			StatusCodes: map[int]int{200: (i + 1) * 10},
		}
		if err := store.SaveTestResult(svc.ID, r); err != nil {
			t.Fatal(err)
		}
	}

	results, err := store.ListResults(svc.ID, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}

	// Most recent first: TotalReqs should be 50, 40, 30
	if results[0].TotalReqs != 50 {
		t.Fatalf("expected first result TotalReqs=50, got %d", results[0].TotalReqs)
	}
	if results[1].TotalReqs != 40 {
		t.Fatalf("expected second result TotalReqs=40, got %d", results[1].TotalReqs)
	}
	if results[2].TotalReqs != 30 {
		t.Fatalf("expected third result TotalReqs=30, got %d", results[2].TotalReqs)
	}
}

func TestCascadeDelete(t *testing.T) {
	t.Parallel()
	store := setupTestDB(t)

	svc := createTestService(t, store, "CascadeSvc", "http://cascade.com")

	for i := 0; i < 3; i++ {
		r := &TestResult{
			TotalReqs:   10,
			StatusCodes: map[int]int{200: 10},
		}
		if err := store.SaveTestResult(svc.ID, r); err != nil {
			t.Fatal(err)
		}
	}

	if err := store.DeleteService(svc.ID); err != nil {
		t.Fatal(err)
	}

	results, err := store.ListResults(svc.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Fatalf("expected 0 results after cascade delete, got %d", len(results))
	}
}

func TestSettings(t *testing.T) {
	t.Parallel()
	store := setupTestDB(t)

	// Set and get
	if err := store.SetSetting("theme", "dark"); err != nil {
		t.Fatal(err)
	}
	val, err := store.GetSetting("theme")
	if err != nil {
		t.Fatal(err)
	}
	if val != "dark" {
		t.Fatalf("expected 'dark', got %q", val)
	}

	// Upsert
	if err := store.SetSetting("theme", "light"); err != nil {
		t.Fatal(err)
	}
	val, err = store.GetSetting("theme")
	if err != nil {
		t.Fatal(err)
	}
	if val != "light" {
		t.Fatalf("expected 'light' after upsert, got %q", val)
	}

	// Non-existent key
	val, err = store.GetSetting("nonexistent")
	if err != nil {
		t.Fatal(err)
	}
	if val != "" {
		t.Fatalf("expected empty string for missing key, got %q", val)
	}

	// GetAllSettings
	if err := store.SetSetting("port", "8080"); err != nil {
		t.Fatal(err)
	}
	all, err := store.GetAllSettings()
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Fatalf("expected 2 settings, got %d", len(all))
	}
	if all["theme"] != "light" {
		t.Fatalf("expected theme=light, got %q", all["theme"])
	}
	if all["port"] != "8080" {
		t.Fatalf("expected port=8080, got %q", all["port"])
	}
}

func TestCloneService(t *testing.T) {
	t.Parallel()
	store := setupTestDB(t)

	svc := createTestService(t, store, "Original", "http://original.com")

	clone, err := store.CloneService(svc.ID)
	if err != nil {
		t.Fatal(err)
	}
	if clone.ID == svc.ID {
		t.Fatal("clone should have different ID")
	}
	if clone.Name != "Original (copy)" {
		t.Fatalf("expected name 'Original (copy)', got %q", clone.Name)
	}
	if clone.URL != svc.URL {
		t.Fatalf("expected same URL, got %q", clone.URL)
	}
	if clone.Method != svc.Method {
		t.Fatalf("expected same method, got %q", clone.Method)
	}
}

func TestUpdateTestNote(t *testing.T) {
	t.Parallel()
	store := setupTestDB(t)

	svc := createTestService(t, store, "NoteSvc", "http://note.com")

	result := &TestResult{
		TotalReqs:   10,
		StatusCodes: map[int]int{200: 10},
	}
	if err := store.SaveTestResult(svc.ID, result); err != nil {
		t.Fatal(err)
	}

	if err := store.UpdateTestNote(result.ID, "updated note"); err != nil {
		t.Fatal(err)
	}

	got, err := store.GetLastResult(svc.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Note != "updated note" {
		t.Fatalf("expected note 'updated note', got %q", got.Note)
	}
}

func TestScheduleCRUD(t *testing.T) {
	t.Parallel()
	store := setupTestDB(t)

	svc := createTestService(t, store, "SchedSvc", "http://sched.com")

	// Create
	sched := &Schedule{
		ServiceID: svc.ID,
		CronExpr:  "*/5 * * * *",
		Enabled:   true,
	}
	if err := store.CreateSchedule(sched); err != nil {
		t.Fatal(err)
	}
	if sched.ID == 0 {
		t.Fatal("expected non-zero schedule ID")
	}

	// List
	list, err := store.ListSchedules()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 schedule, got %d", len(list))
	}

	// Get
	got, err := store.GetSchedule(sched.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Fatal("expected schedule, got nil")
	}
	if got.CronExpr != "*/5 * * * *" {
		t.Fatalf("expected cron '*/5 * * * *', got %q", got.CronExpr)
	}
	if !got.Enabled {
		t.Fatal("expected enabled=true")
	}

	// Update enabled
	if err := store.UpdateScheduleEnabled(sched.ID, false); err != nil {
		t.Fatal(err)
	}
	got, err = store.GetSchedule(sched.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Enabled {
		t.Fatal("expected enabled=false after update")
	}

	// Delete
	if err := store.DeleteSchedule(sched.ID); err != nil {
		t.Fatal(err)
	}
	got, err = store.GetSchedule(sched.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Fatal("expected nil after delete")
	}
}
