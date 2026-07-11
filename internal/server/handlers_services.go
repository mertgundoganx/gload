package server

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/mertgundoganx/gload/internal/logger"
	"github.com/mertgundoganx/gload/internal/prom"
	"github.com/mertgundoganx/gload/internal/storage"
)

// validateServiceInput checks required fields and returns an error message, or "" if valid.
func validateServiceInput(svc *storage.Service) string {
	if strings.TrimSpace(svc.Name) == "" {
		return "name is required"
	}
	if strings.TrimSpace(svc.URL) == "" {
		return "url is required"
	}
	u, err := url.Parse(svc.URL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https" && u.Scheme != "ws" && u.Scheme != "wss") || u.Host == "" {
		return "url must be a valid http(s) or ws(s) URL"
	}
	if svc.Concurrency < 1 || svc.Concurrency > 100000 {
		return "concurrency must be between 1 and 100,000"
	}
	if d, err := time.ParseDuration(svc.Duration); err != nil || d <= 0 {
		return "duration must be a valid positive duration (e.g. 10s, 1m)"
	}
	if t, err := time.ParseDuration(svc.Timeout); err != nil || t <= 0 {
		return "timeout must be a valid positive duration (e.g. 30s)"
	}
	return ""
}

func (s *Server) handleServices(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.listServices(w, r)
	case http.MethodPost:
		s.createService(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleServiceRoute(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/services/")
	parts := strings.SplitN(path, "/", 2)
	if len(parts) == 0 || parts[0] == "" {
		http.NotFound(w, r)
		return
	}

	id, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		http.Error(w, "invalid service id", http.StatusBadRequest)
		return
	}

	action := ""
	if len(parts) == 2 {
		action = parts[1]
	}

	switch {
	case action == "" && r.Method == http.MethodPut:
		s.updateService(w, r, id)
	case action == "" && r.Method == http.MethodDelete:
		s.deleteService(w, r, id)
	case action == "" && r.Method == http.MethodGet:
		s.getService(w, r, id)
	case action == "run" && r.Method == http.MethodPost:
		s.runService(w, r, id)
	case action == "stop" && r.Method == http.MethodPost:
		s.stopService(w, r, id)
	case action == "stream" && r.Method == http.MethodGet:
		s.streamService(w, r, id)
	case action == "ws" && r.Method == http.MethodGet:
		s.wsService(w, r, id)
	case action == "report" && r.Method == http.MethodGet:
		s.reportService(w, r, id)
	case action == "pdf" && r.Method == http.MethodGet:
		s.pdfService(w, r, id)
	case action == "results" && r.Method == http.MethodGet:
		s.resultsService(w, r, id)
	case action == "history" && r.Method == http.MethodGet:
		s.historyService(w, r, id)
	case action == "clone" && r.Method == http.MethodPost:
		s.cloneService(w, r, id)
	case action == "run-profile" && r.Method == http.MethodPost:
		s.runProfile(w, r, id)
	case action == "run-pattern" && r.Method == http.MethodPost:
		s.runPattern(w, r, id)
	case action == "capacity-probe" && r.Method == http.MethodPost:
		s.startCapacityProbe(w, r, id)
	case action == "capacity-probe" && r.Method == http.MethodGet:
		s.getCapacityProbe(w, r, id)
	case strings.HasPrefix(action, "capacity-probe/") && r.Method == http.MethodDelete:
		// capacity-probe/{runId}
		runID, err := strconv.ParseInt(strings.TrimPrefix(action, "capacity-probe/"), 10, 64)
		if err != nil {
			http.Error(w, "invalid run id", http.StatusBadRequest)
			return
		}
		s.deleteCapacityRun(w, r, id, runID)
	case action == "capacity-report" && r.Method == http.MethodGet:
		s.getCapacityReport(w, r, id)
	case strings.HasPrefix(action, "results/export"):
		s.exportResults(w, r, id)
	case action == "run-distributed" && r.Method == http.MethodPost:
		s.runDistributed(w, r, id)
	case action == "insights" && r.Method == http.MethodGet:
		s.getInsights(w, r, id)
	case action == "junit" && r.Method == http.MethodGet:
		s.junitService(w, r, id)
	case action == "capacity" && r.Method == http.MethodGet:
		s.getCapacity(w, r, id)
	case action == "github-comment" && r.Method == http.MethodPost:
		s.postGitHubComment(w, r, id)
	case strings.HasPrefix(action, "results/") && r.Method == http.MethodDelete:
		// results/{resultId}
		resultID, err := strconv.ParseInt(strings.TrimPrefix(action, "results/"), 10, 64)
		if err != nil {
			http.Error(w, "invalid result id", http.StatusBadRequest)
			return
		}
		s.deleteResult(w, r, id, resultID)
	case strings.HasPrefix(action, "results/") && r.Method == http.MethodPut:
		// results/{resultId}/note
		sub := strings.TrimPrefix(action, "results/")
		subParts := strings.SplitN(sub, "/", 2)
		if len(subParts) == 2 && subParts[1] == "note" {
			resultID, err := strconv.ParseInt(subParts[0], 10, 64)
			if err != nil {
				http.Error(w, "invalid result id", http.StatusBadRequest)
				return
			}
			s.updateTestNote(w, r, resultID)
		} else {
			http.NotFound(w, r)
		}
	case strings.HasPrefix(action, "results/") && r.Method == http.MethodGet:
		sub := strings.TrimPrefix(action, "results/")
		if strings.HasPrefix(sub, "export") {
			// already handled above, but guard anyway
			s.exportResults(w, r, id)
			return
		}
		subParts := strings.SplitN(sub, "/", 2)
		if len(subParts) == 2 && subParts[1] == "share" {
			resultID, err := strconv.ParseInt(subParts[0], 10, 64)
			if err != nil {
				http.Error(w, "invalid result id", http.StatusBadRequest)
				return
			}
			s.shareResult(w, r, id, resultID)
		} else {
			http.NotFound(w, r)
		}
	default:
		http.NotFound(w, r)
	}
}

// ---------- CRUD ----------

func (s *Server) listServices(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	// Support workspace filtering via X-Workspace header or ?workspace=slug query param.
	wsSlug := q.Get("workspace")
	if wsSlug == "" {
		wsSlug = r.Header.Get("X-Workspace")
	}

	// Paginated mode: if ?page= is set, return paginated response with metadata.
	if q.Get("page") != "" {
		s.listServicesPaged(w, r, wsSlug)
		return
	}

	// Legacy mode: return flat array (backwards-compatible for existing frontend).
	var services []storage.Service
	var err error
	if wsSlug != "" {
		ws, wsErr := s.store.GetWorkspaceBySlug(wsSlug)
		if wsErr != nil {
			dbError(w, wsErr)
			return
		}
		if ws == nil {
			http.Error(w, "workspace not found", http.StatusNotFound)
			return
		}
		services, err = s.store.ListServicesByWorkspace(ws.ID)
	} else {
		services, err = s.store.ListServices()
	}
	if err != nil {
		dbError(w, err)
		return
	}

	prom.Global.SetServices(len(services))

	ids := make([]int64, len(services))
	for i, svc := range services {
		ids[i] = svc.ID
	}
	lastResults, err := s.store.GetLastResults(ids)
	if err != nil {
		dbError(w, err)
		return
	}
	capResults, _ := s.store.GetCapacityResults(ids)

	list := make([]serviceWithStatus, 0, len(services))
	s.mu.RLock()
	for _, svc := range services {
		item := serviceWithStatus{Service: svc}
		if rs, running := s.runs[svc.ID]; running {
			item.IsRunning = true
			item.RunningKind = rs.kind
			live := snapshotJSONWithProgress(rs.runner.Metrics.Snapshot(), rs.duration)
			item.Live = &live
		}
		if last := lastResults[svc.ID]; last != nil {
			sd := testResultToSnapshot(last)
			item.LastResult = &sd
		}
		item.Capacity = parseCapacityBadge(capResults[svc.ID])
		list = append(list, item)
	}
	s.mu.RUnlock()
	writeJSON(w, http.StatusOK, list)
}

// listServicesPaged returns a paginated JSON response.
// Query params: page, per_page, search, sort, order.
func (s *Server) listServicesPaged(w http.ResponseWriter, r *http.Request, wsSlug string) {
	q := r.URL.Query()
	page, _ := strconv.Atoi(q.Get("page"))
	perPage, _ := strconv.Atoi(q.Get("per_page"))
	if perPage == 0 {
		perPage = 20
	}
	search := q.Get("search")
	sortBy := q.Get("sort")
	sortDir := q.Get("order")

	var workspaceID int64
	if wsSlug != "" {
		ws, wsErr := s.store.GetWorkspaceBySlug(wsSlug)
		if wsErr != nil {
			dbError(w, wsErr)
			return
		}
		if ws == nil {
			http.Error(w, "workspace not found", http.StatusNotFound)
			return
		}
		workspaceID = ws.ID
	}

	sp, err := s.store.ListServicesPaged(page, perPage, search, sortBy, sortDir, workspaceID)
	if err != nil {
		dbError(w, err)
		return
	}

	prom.Global.SetServices(sp.Total)

	type pagedResponse struct {
		Services []serviceWithStatus `json:"services"`
		Total    int                 `json:"total"`
		Page     int                 `json:"page"`
		PerPage  int                 `json:"per_page"`
		Pages    int                 `json:"pages"`
	}

	ids := make([]int64, len(sp.Services))
	for i, svc := range sp.Services {
		ids[i] = svc.ID
	}
	lastResults, err := s.store.GetLastResults(ids)
	if err != nil {
		dbError(w, err)
		return
	}
	capResults, _ := s.store.GetCapacityResults(ids)

	list := make([]serviceWithStatus, 0, len(sp.Services))
	s.mu.RLock()
	for _, svc := range sp.Services {
		item := serviceWithStatus{Service: svc}
		if rs, running := s.runs[svc.ID]; running {
			item.IsRunning = true
			item.RunningKind = rs.kind
			live := snapshotJSONWithProgress(rs.runner.Metrics.Snapshot(), rs.duration)
			item.Live = &live
		}
		if last := lastResults[svc.ID]; last != nil {
			sd := testResultToSnapshot(last)
			item.LastResult = &sd
		}
		item.Capacity = parseCapacityBadge(capResults[svc.ID])
		list = append(list, item)
	}
	s.mu.RUnlock()

	writeJSON(w, http.StatusOK, pagedResponse{
		Services: list,
		Total:    sp.Total,
		Page:     sp.Page,
		PerPage:  sp.PerPage,
		Pages:    sp.Pages,
	})
}

func (s *Server) getService(w http.ResponseWriter, _ *http.Request, id int64) {
	svc, err := s.store.GetService(id)
	if err != nil {
		dbError(w, err)
		return
	}
	if svc == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, svc)
}

func (s *Server) createService(w http.ResponseWriter, r *http.Request) {
	var svc storage.Service
	if err := json.NewDecoder(r.Body).Decode(&svc); err != nil {
		http.Error(w, "invalid json: "+err.Error(), http.StatusBadRequest)
		return
	}
	if svc.Method == "" {
		svc.Method = "GET"
	}
	if svc.Duration == "" {
		svc.Duration = "10s"
	}
	if svc.Timeout == "" {
		svc.Timeout = "30s"
	}
	if svc.Concurrency <= 0 {
		svc.Concurrency = 10
	}
	if svc.Headers == nil {
		svc.Headers = make(map[string]string)
	}
	if msg := validateServiceInput(&svc); msg != "" {
		http.Error(w, msg, http.StatusBadRequest)
		return
	}

	// Resolve the workspace: an explicit id must reference a real workspace,
	// otherwise (or when unset) the service goes to the default workspace.
	if svc.WorkspaceID != 0 {
		if ws, _ := s.store.GetWorkspace(svc.WorkspaceID); ws == nil {
			svc.WorkspaceID = 0
		}
	}
	if svc.WorkspaceID == 0 {
		if ws, _ := s.store.GetWorkspaceBySlug("default"); ws != nil {
			svc.WorkspaceID = ws.ID
		}
	}

	if err := s.store.CreateService(&svc); err != nil {
		dbError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, svc)
	go s.broadcast.send("service_created", svc)
}

func (s *Server) updateService(w http.ResponseWriter, r *http.Request, id int64) {
	existing, err := s.store.GetService(id)
	if err != nil {
		dbError(w, err)
		return
	}
	if existing == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	var svc storage.Service
	if err := json.NewDecoder(r.Body).Decode(&svc); err != nil {
		http.Error(w, "invalid json: "+err.Error(), http.StatusBadRequest)
		return
	}
	svc.ID = existing.ID
	if svc.Method == "" {
		svc.Method = "GET"
	}
	if svc.Duration == "" {
		svc.Duration = "10s"
	}
	if svc.Timeout == "" {
		svc.Timeout = "30s"
	}
	if svc.Concurrency <= 0 {
		svc.Concurrency = 10
	}
	if svc.Headers == nil {
		svc.Headers = make(map[string]string)
	}
	if msg := validateServiceInput(&svc); msg != "" {
		http.Error(w, msg, http.StatusBadRequest)
		return
	}

	// Preserve the workspace unless the client explicitly moves the service to
	// another existing workspace. A missing or bogus value keeps the current
	// one instead of silently orphaning the service to workspace 0.
	if svc.WorkspaceID == 0 {
		svc.WorkspaceID = existing.WorkspaceID
	} else if ws, _ := s.store.GetWorkspace(svc.WorkspaceID); ws == nil {
		svc.WorkspaceID = existing.WorkspaceID
	}
	if svc.WorkspaceID == 0 {
		if id, derr := s.store.DefaultWorkspaceID(); derr == nil {
			svc.WorkspaceID = id
		}
	}

	if err := s.store.UpdateService(&svc); err != nil {
		dbError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, svc)
	go s.broadcast.send("service_updated", svc)
}

func (s *Server) deleteService(w http.ResponseWriter, _ *http.Request, id int64) {
	// Prevent deletion while a test is running.
	s.mu.RLock()
	_, running := s.runs[id]
	s.mu.RUnlock()
	if running {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "Cannot delete service while a test is running. Stop the test first."})
		return
	}

	if err := s.store.DeleteService(id); err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
	go s.broadcast.send("service_deleted", map[string]int64{"id": id})
}

// ---------- Clone ----------

func (s *Server) cloneService(w http.ResponseWriter, _ *http.Request, id int64) {
	cloned, err := s.store.CloneService(id)
	if err != nil {
		logger.Error("clone failed", logger.Fields("error", err.Error()))
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusCreated, cloned)
}

// ---------- Export / Import ----------

func (s *Server) handleExport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	services, err := s.store.ListServices()
	if err != nil {
		dbError(w, err)
		return
	}
	w.Header().Set("Content-Disposition", `attachment; filename="gload-export.json"`)
	writeJSON(w, http.StatusOK, services)
}

func (s *Server) handleImport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var services []storage.Service
	if err := json.NewDecoder(r.Body).Decode(&services); err != nil {
		http.Error(w, "invalid json: "+err.Error(), http.StatusBadRequest)
		return
	}
	count := 0
	skipped := 0
	for i := range services {
		svc := services[i]
		svc.ID = 0
		if svc.Method == "" {
			svc.Method = "GET"
		}
		if svc.Duration == "" {
			svc.Duration = "10s"
		}
		if svc.Timeout == "" {
			svc.Timeout = "30s"
		}
		if svc.Concurrency <= 0 {
			svc.Concurrency = 10
		}
		if svc.Headers == nil {
			svc.Headers = make(map[string]string)
		}
		if msg := validateServiceInput(&svc); msg != "" {
			skipped++
			continue
		}
		// Remap the workspace: keep it only if it exists on this instance,
		// otherwise fall back to default. This prevents imports from another
		// instance (whose workspace IDs differ) from creating services that
		// are orphaned under a non-existent workspace and invisible in the UI.
		if svc.WorkspaceID != 0 {
			if ws, _ := s.store.GetWorkspace(svc.WorkspaceID); ws == nil {
				svc.WorkspaceID = 0
			}
		}
		if svc.WorkspaceID == 0 {
			if ws, _ := s.store.GetWorkspaceBySlug("default"); ws != nil {
				svc.WorkspaceID = ws.ID
			}
		}
		if err := s.store.CreateService(&svc); err != nil {
			logger.Error("import: failed to create service", logger.Fields("error", err.Error()))
			skipped++
			continue
		}
		count++
	}
	writeJSON(w, http.StatusOK, map[string]int{"imported": count, "skipped": skipped})
}

// ---------- Bulk Operations ----------

func (s *Server) handleBulkDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", 405)
		return
	}
	var body struct {
		IDs []int64 `json:"ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid json", 400)
		return
	}
	deleted := 0
	skipped := 0
	for _, id := range body.IDs {
		s.mu.RLock()
		_, running := s.runs[id]
		s.mu.RUnlock()
		if running {
			skipped++
			continue
		}
		if err := s.store.DeleteService(id); err == nil {
			deleted++
			go s.broadcast.send("service_deleted", map[string]int64{"id": id})
		}
	}
	writeJSON(w, http.StatusOK, map[string]int{"deleted": deleted})
}
