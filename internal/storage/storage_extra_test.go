package storage

import (
	"testing"
	"time"
)

func seedResult(t *testing.T, store *Storage, serviceID int64, reqs int) *TestResult {
	t.Helper()
	r := &TestResult{
		DurationMs: 1000, TotalReqs: reqs, Errors: 1, RPS: float64(reqs),
		AvgLatencyMs: 10, P50LatencyMs: 9, P95LatencyMs: 20, P99LatencyMs: 30,
		MinLatencyMs: 1, MaxLatencyMs: 40, StatusCodes: map[int]int{200: reqs},
		Status: "pass",
	}
	if err := store.SaveTestResult(serviceID, r); err != nil {
		t.Fatal(err)
	}
	return r
}

func TestWorkspaces(t *testing.T) {
	t.Parallel()
	store := setupTestDB(t)

	// A "Default" workspace always exists after init.
	defID, err := store.DefaultWorkspaceID()
	if err != nil || defID == 0 {
		t.Fatalf("DefaultWorkspaceID = %d, err %v", defID, err)
	}

	ws := &Workspace{Name: "Prod", Slug: "prod", Description: "production"}
	if err := store.CreateWorkspace(ws); err != nil {
		t.Fatal(err)
	}
	if ws.ID == 0 {
		t.Fatal("expected workspace ID")
	}

	got, err := store.GetWorkspace(ws.ID)
	if err != nil || got == nil || got.Slug != "prod" {
		t.Fatalf("GetWorkspace = %+v, err %v", got, err)
	}
	bySlug, err := store.GetWorkspaceBySlug("prod")
	if err != nil || bySlug == nil || bySlug.ID != ws.ID {
		t.Fatalf("GetWorkspaceBySlug = %+v, err %v", bySlug, err)
	}
	if missing, _ := store.GetWorkspaceBySlug("nope"); missing != nil {
		t.Error("expected nil for unknown slug")
	}

	list, err := store.ListWorkspaces()
	if err != nil || len(list) < 2 {
		t.Fatalf("ListWorkspaces returned %d (want >=2 incl Default)", len(list))
	}

	// A service assigned to the workspace shows up in ListServicesByWorkspace.
	svc := &Service{Name: "in-prod", URL: "http://x", Method: "GET", Concurrency: 1, Duration: "1s", Timeout: "5s", WorkspaceID: ws.ID}
	if err := store.CreateService(svc); err != nil {
		t.Fatal(err)
	}
	inWs, err := store.ListServicesByWorkspace(ws.ID)
	if err != nil || len(inWs) != 1 || inWs[0].ID != svc.ID {
		t.Fatalf("ListServicesByWorkspace = %+v, err %v", inWs, err)
	}

	if err := store.DeleteWorkspace(ws.ID); err != nil {
		t.Fatal(err)
	}
	if gone, _ := store.GetWorkspace(ws.ID); gone != nil {
		t.Error("workspace should be deleted")
	}
}

func TestQueue(t *testing.T) {
	t.Parallel()
	store := setupTestDB(t)
	s1 := createTestService(t, store, "q1", "http://1")
	s2 := createTestService(t, store, "q2", "http://2")
	s3 := createTestService(t, store, "q3", "http://3")

	for _, s := range []*Service{s1, s2, s3} {
		if _, err := store.EnqueueTest(s.ID); err != nil {
			t.Fatal(err)
		}
	}

	q, err := store.ListQueue()
	if err != nil || len(q) != 3 {
		t.Fatalf("ListQueue len = %d, err %v", len(q), err)
	}
	if n, _ := store.CountQueueForService(s1.ID); n != 1 {
		t.Errorf("CountQueueForService = %d, want 1", n)
	}

	// Reorder: put s3 first.
	if err := store.ReorderQueue([]int64{q[2].ID, q[0].ID, q[1].ID}); err != nil {
		t.Fatal(err)
	}
	reordered, _ := store.ListQueue()
	if reordered[0].ServiceID != s3.ID {
		t.Errorf("after reorder, first = %d, want %d", reordered[0].ServiceID, s3.ID)
	}

	// Pop returns the front item.
	popped, err := store.PopQueue()
	if err != nil || popped == nil || popped.ServiceID != s3.ID {
		t.Fatalf("PopQueue = %+v, err %v", popped, err)
	}

	// Remove one, then clear.
	rest, _ := store.ListQueue()
	if err := store.RemoveQueueItem(rest[0].ID); err != nil {
		t.Fatal(err)
	}
	if err := store.ClearQueue(); err != nil {
		t.Fatal(err)
	}
	empty, _ := store.ListQueue()
	if len(empty) != 0 {
		t.Errorf("queue should be empty, got %d", len(empty))
	}
	if none, _ := store.PopQueue(); none != nil {
		t.Error("PopQueue on empty should be nil")
	}
}

func TestCapacityRuns(t *testing.T) {
	t.Parallel()
	store := setupTestDB(t)
	svc := createTestService(t, store, "cap", "http://cap")

	if err := store.SaveCapacityResult(svc.ID, `{"max_rps":100,"knee_concurrency":8}`); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveCapacityResult(svc.ID, `{"max_rps":200,"knee_concurrency":16}`); err != nil {
		t.Fatal(err)
	}

	runs, err := store.ListCapacityRuns(svc.ID)
	if err != nil || len(runs) != 2 {
		t.Fatalf("ListCapacityRuns len = %d, err %v", len(runs), err)
	}
	// Newest first.
	if runs[0].ID < runs[1].ID {
		t.Error("expected newest-first ordering")
	}

	badge, err := store.GetCapacityResults([]int64{svc.ID})
	if err != nil || badge[svc.ID] == "" {
		t.Fatalf("GetCapacityResults = %v, err %v", badge, err)
	}

	if err := store.DeleteCapacityRun(svc.ID, runs[0].ID); err != nil {
		t.Fatal(err)
	}
	after, _ := store.ListCapacityRuns(svc.ID)
	if len(after) != 1 {
		t.Errorf("after delete, runs = %d, want 1", len(after))
	}
}

func TestResultLifecycle(t *testing.T) {
	t.Parallel()
	store := setupTestDB(t)
	svc := createTestService(t, store, "res", "http://res")

	r1 := seedResult(t, store, svc.ID, 100)
	r2 := seedResult(t, store, svc.ID, 200)

	// GetResult by id.
	got, err := store.GetResult(r2.ID)
	if err != nil || got == nil || got.TotalReqs != 200 {
		t.Fatalf("GetResult = %+v, err %v", got, err)
	}

	// GetLastResults (batch) returns the latest per service.
	last, err := store.GetLastResults([]int64{svc.ID})
	if err != nil || last[svc.ID] == nil || last[svc.ID].TotalReqs != 200 {
		t.Fatalf("GetLastResults = %+v, err %v", last, err)
	}

	// Delete a single result.
	if err := store.DeleteResult(svc.ID, r1.ID); err != nil {
		t.Fatal(err)
	}
	remaining, _ := store.ListResults(svc.ID, 10)
	if len(remaining) != 1 {
		t.Fatalf("after DeleteResult, %d remain, want 1", len(remaining))
	}

	// Purge everything older than an hour from now.
	n, err := store.PurgeOldResults(time.Now().Add(time.Hour))
	if err != nil || n != 1 {
		t.Fatalf("PurgeOldResults deleted %d (err %v), want 1", n, err)
	}

	// DeleteResultsByService on a fresh batch.
	seedResult(t, store, svc.ID, 50)
	if err := store.DeleteResultsByService(svc.ID); err != nil {
		t.Fatal(err)
	}
	if left, _ := store.ListResults(svc.ID, 10); len(left) != 0 {
		t.Errorf("DeleteResultsByService left %d", len(left))
	}
}

func TestStatsAndPing(t *testing.T) {
	t.Parallel()
	store := setupTestDB(t)
	if err := store.Ping(); err != nil {
		t.Fatalf("Ping: %v", err)
	}
	svc := createTestService(t, store, "s", "http://s")
	seedResult(t, store, svc.ID, 10)

	st, err := store.Stats()
	if err != nil || st == nil {
		t.Fatalf("Stats err %v", err)
	}
	if st.ServiceCount != 1 || st.TestResultCount != 1 {
		t.Errorf("Stats = %+v, want 1 service / 1 result", st)
	}
}

func TestListServicesPaged(t *testing.T) {
	t.Parallel()
	store := setupTestDB(t)
	defID, _ := store.DefaultWorkspaceID()
	for _, n := range []string{"alpha", "beta", "gamma"} {
		svc := &Service{Name: n, URL: "http://" + n, Method: "GET", Concurrency: 1, Duration: "1s", Timeout: "5s", WorkspaceID: defID}
		if err := store.CreateService(svc); err != nil {
			t.Fatal(err)
		}
	}

	// Page 1, 2 per page.
	page, err := store.ListServicesPaged(1, 2, "", "name", "asc", defID)
	if err != nil || page == nil {
		t.Fatalf("ListServicesPaged err %v", err)
	}
	if len(page.Services) != 2 {
		t.Errorf("page size = %d, want 2", len(page.Services))
	}
	if page.Total != 3 {
		t.Errorf("Total = %d, want 3", page.Total)
	}

	// Search narrows results.
	found, err := store.ListServicesPaged(1, 10, "beta", "name", "asc", defID)
	if err != nil {
		t.Fatal(err)
	}
	if found.Total != 1 || found.Services[0].Name != "beta" {
		t.Errorf("search 'beta' = %+v", found.Services)
	}
}
