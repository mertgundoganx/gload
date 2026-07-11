package storage

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// Service represents a saved load test configuration.
type Service struct {
	ID                   int64             `json:"id"`
	Name                 string            `json:"name"`
	URL                  string            `json:"url"`
	Method               string            `json:"method"`
	Headers              map[string]string `json:"headers"`
	Body                 string            `json:"body"`
	Concurrency          int               `json:"concurrency"`
	Duration             string            `json:"duration"`
	Timeout              string            `json:"timeout"`
	Tags                 string            `json:"tags"`        // comma-separated
	GroupName            string            `json:"group_name"`  // group/category
	Steps                string            `json:"steps"`       // JSON array of steps
	DataSource           string            `json:"data_source"` // JSON array of {key: value} objects
	Assertions           string            `json:"assertions"`  // JSON array of assertions
	Profiles             string            `json:"profiles"`    // JSON array of test profiles
	CookieJar            int               `json:"cookie_jar"`  // 1 = enable per-worker cookie jar
	WorkspaceID          int64             `json:"workspace_id"`
	HTTP2                int               `json:"http2"`              // 1=enabled (default)
	DisableKeepAlive     int               `json:"disable_keep_alive"` // 0=enabled
	MaxIdleConns         int               `json:"max_idle_conns"`
	DNSCache             int               `json:"dns_cache"`
	WarmupSeconds        int               `json:"warmup_seconds"`
	ThinkTimeMs          int               `json:"think_time_ms"`
	ThinkTimeMaxMs       int               `json:"think_time_max_ms"`
	ArrivalRate          int               `json:"arrival_rate"`
	Validations          string            `json:"validations"`
	ContentType          string            `json:"content_type"`
	FormFields           string            `json:"form_fields"`
	Protocol             string            `json:"protocol"`        // "http", "websocket", "graphql", "grpc", "tcp"
	ProtocolConfig       string            `json:"protocol_config"` // JSON map of plugin-specific config
	WarmupConns          int               `json:"warmup_conns"`
	AdaptiveConcurrency  int               `json:"adaptive_concurrency"`
	AdaptiveTargetMs     float64           `json:"adaptive_target_ms"`
	RequestsPerIteration int               `json:"requests_per_iteration"`
	CreatedAt            time.Time         `json:"created_at"`
	UpdatedAt            time.Time         `json:"updated_at"`
}

// Workspace represents a team workspace for multi-tenancy.
type Workspace struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	Slug        string    `json:"slug"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
}

// TestResult represents the outcome of a single load test run.
type TestResult struct {
	ID               int64       `json:"id"`
	ServiceID        int64       `json:"service_id"`
	DurationMs       float64     `json:"duration_ms"`
	TotalReqs        int         `json:"total_reqs"`
	Errors           int         `json:"errors"`
	RPS              float64     `json:"rps"`
	AvgLatencyMs     float64     `json:"avg_latency_ms"`
	P50LatencyMs     float64     `json:"p50_latency_ms"`
	P95LatencyMs     float64     `json:"p95_latency_ms"`
	P99LatencyMs     float64     `json:"p99_latency_ms"`
	MinLatencyMs     float64     `json:"min_latency_ms"`
	MaxLatencyMs     float64     `json:"max_latency_ms"`
	StatusCodes      map[int]int `json:"status_codes"`
	Note             string      `json:"note"`
	Timeline         string      `json:"timeline"`          // JSON string of timeline points
	Status           string      `json:"status"`            // "pass" or "fail"
	AssertionResults string      `json:"assertion_results"` // JSON array of {metric, operator, value, actual, passed}
	TLSInfo          string      `json:"tls_info"`          // JSON object with TLS handshake details
	RateLimitInfo    string      `json:"rate_limit_info"`   // JSON object with rate limit analysis
	RunConfig        string      `json:"run_config"`        // JSON object with test run configuration
	CreatedAt        time.Time   `json:"created_at"`
}

// Storage provides persistent storage backed by SQLite.
type Storage struct {
	db   *sql.DB
	path string
}

// DefaultDBPath returns the default database path (~/.gload/gload.db).
func DefaultDBPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("get home dir: %w", err)
	}
	return filepath.Join(home, ".gload", "gload.db"), nil
}

// New opens (or creates) a SQLite database at dbPath and initializes tables.
func New(dbPath string) (*Storage, error) {
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create db directory: %w", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}

	// Enable WAL mode, foreign keys, and a busy timeout so concurrent
	// writers wait for the lock instead of failing with "database is locked".
	for _, pragma := range []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA foreign_keys=ON",
		"PRAGMA busy_timeout=5000",
		"PRAGMA synchronous=NORMAL",
	} {
		if _, err := db.Exec(pragma); err != nil {
			db.Close()
			return nil, fmt.Errorf("exec %s: %w", pragma, err)
		}
	}

	// SQLite allows a single writer at a time; cap the pool to avoid
	// contention while still allowing concurrent readers under WAL.
	db.SetMaxOpenConns(1)
	db.SetConnMaxLifetime(0)

	if err := createTables(db); err != nil {
		db.Close()
		return nil, err
	}

	return &Storage{db: db, path: dbPath}, nil
}

func createTables(db *sql.DB) error {
	const servicesTable = `
CREATE TABLE IF NOT EXISTS services (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    url TEXT NOT NULL,
    method TEXT NOT NULL DEFAULT 'GET',
    headers TEXT DEFAULT '{}',
    body TEXT DEFAULT '',
    concurrency INTEGER DEFAULT 10,
    duration TEXT DEFAULT '10s',
    timeout TEXT DEFAULT '30s',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);`

	const resultsTable = `
CREATE TABLE IF NOT EXISTS test_results (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    service_id INTEGER NOT NULL,
    duration_ms REAL,
    total_reqs INTEGER,
    errors INTEGER,
    rps REAL,
    avg_latency_ms REAL,
    p50_latency_ms REAL,
    p95_latency_ms REAL,
    p99_latency_ms REAL,
    min_latency_ms REAL,
    max_latency_ms REAL,
    status_codes TEXT DEFAULT '{}',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (service_id) REFERENCES services(id) ON DELETE CASCADE
);`

	const settingsTable = `
CREATE TABLE IF NOT EXISTS settings (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL
);`

	const schedulesTable = `
CREATE TABLE IF NOT EXISTS schedules (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    service_id INTEGER NOT NULL,
    cron_expr TEXT NOT NULL,
    enabled INTEGER DEFAULT 1,
    last_run DATETIME,
    next_run DATETIME,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (service_id) REFERENCES services(id) ON DELETE CASCADE
);`

	const workspacesTable = `
CREATE TABLE IF NOT EXISTS workspaces (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL UNIQUE,
    slug TEXT NOT NULL UNIQUE,
    description TEXT DEFAULT '',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);`

	// Persistent test queue so pending items survive a restart. Each row has a
	// stable id (used for removal/reordering) and a position for ordering.
	const queueTable = `
CREATE TABLE IF NOT EXISTS queue (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    service_id INTEGER NOT NULL,
    position INTEGER NOT NULL DEFAULT 0,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (service_id) REFERENCES services(id) ON DELETE CASCADE
);`

	// Capacity-probe run history — one row per run, kept so users can browse
	// past capacity tests.
	const capacityTable = `
CREATE TABLE IF NOT EXISTS capacity_runs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    service_id INTEGER NOT NULL,
    result TEXT NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (service_id) REFERENCES services(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_capacity_runs_service ON capacity_runs(service_id, id DESC);`

	for _, ddl := range []string{servicesTable, resultsTable, settingsTable, schedulesTable, workspacesTable, queueTable, capacityTable} {
		if _, err := db.Exec(ddl); err != nil {
			return fmt.Errorf("create table: %w", err)
		}
	}

	// Run migrations for columns added after initial schema.
	// Errors from duplicate columns are silently ignored.
	migrations := []string{
		// Old single-result capacity cache, superseded by capacity_runs history.
		"DROP TABLE IF EXISTS capacity_results",
		"ALTER TABLE services ADD COLUMN tags TEXT DEFAULT ''",
		"ALTER TABLE services ADD COLUMN group_name TEXT DEFAULT ''",
		"ALTER TABLE test_results ADD COLUMN note TEXT DEFAULT ''",
		"ALTER TABLE test_results ADD COLUMN timeline TEXT DEFAULT '[]'",
		"ALTER TABLE services ADD COLUMN steps TEXT DEFAULT '[]'",
		"ALTER TABLE services ADD COLUMN data_source TEXT DEFAULT '[]'",
		"ALTER TABLE services ADD COLUMN assertions TEXT DEFAULT '[]'",
		"ALTER TABLE services ADD COLUMN profiles TEXT DEFAULT '[]'",
		"ALTER TABLE services ADD COLUMN cookie_jar INTEGER DEFAULT 0",
		"ALTER TABLE test_results ADD COLUMN status TEXT DEFAULT 'pass'",
		"ALTER TABLE test_results ADD COLUMN assertion_results TEXT DEFAULT '[]'",
		"ALTER TABLE services DROP COLUMN sla_config", // removed in v1.0.0; no-ops on fresh DBs
		"ALTER TABLE test_results ADD COLUMN tls_info TEXT DEFAULT '{}'",
		"ALTER TABLE services ADD COLUMN workspace_id INTEGER DEFAULT 0",
		"ALTER TABLE test_results ADD COLUMN rate_limit_info TEXT DEFAULT '{}'",
		"ALTER TABLE services ADD COLUMN http2 INTEGER DEFAULT 1",
		"ALTER TABLE services ADD COLUMN disable_keep_alive INTEGER DEFAULT 0",
		"ALTER TABLE services ADD COLUMN max_idle_conns INTEGER DEFAULT 100",
		"ALTER TABLE services ADD COLUMN dns_cache INTEGER DEFAULT 0",
		"ALTER TABLE services ADD COLUMN warmup_seconds INTEGER DEFAULT 0",
		"ALTER TABLE services ADD COLUMN think_time_ms INTEGER DEFAULT 0",
		"ALTER TABLE services ADD COLUMN think_time_max_ms INTEGER DEFAULT 0",
		"ALTER TABLE services ADD COLUMN arrival_rate INTEGER DEFAULT 0",
		"ALTER TABLE services ADD COLUMN validations TEXT DEFAULT '[]'",
		"ALTER TABLE services ADD COLUMN content_type TEXT DEFAULT 'json'",
		"ALTER TABLE services ADD COLUMN form_fields TEXT DEFAULT '[]'",
		"ALTER TABLE services ADD COLUMN protocol TEXT DEFAULT 'http'",
		"ALTER TABLE services ADD COLUMN protocol_config TEXT DEFAULT '{}'",
		"ALTER TABLE services ADD COLUMN warmup_conns INTEGER DEFAULT 0",
		"ALTER TABLE services ADD COLUMN adaptive_concurrency INTEGER DEFAULT 0",
		"ALTER TABLE services ADD COLUMN adaptive_target_ms REAL DEFAULT 500",
		"ALTER TABLE services ADD COLUMN requests_per_iteration INTEGER DEFAULT 1",
		"ALTER TABLE test_results ADD COLUMN run_config TEXT DEFAULT '{}'",
	}
	for _, m := range migrations {
		_, _ = db.Exec(m) // ignore "duplicate column" errors
	}

	// Indexes for the most common lookups (foreign-key joins and ordering).
	indexes := []string{
		"CREATE INDEX IF NOT EXISTS idx_test_results_service_id ON test_results(service_id, id DESC)",
		"CREATE INDEX IF NOT EXISTS idx_test_results_created_at ON test_results(created_at)",
		"CREATE INDEX IF NOT EXISTS idx_schedules_service_id ON schedules(service_id)",
		"CREATE INDEX IF NOT EXISTS idx_services_workspace_id ON services(workspace_id)",
	}
	for _, idx := range indexes {
		if _, err := db.Exec(idx); err != nil {
			return fmt.Errorf("create index: %w", err)
		}
	}

	// Ensure "Default" workspace always exists.
	_, _ = db.Exec(`INSERT OR IGNORE INTO workspaces (name, slug, description) VALUES ('Default', 'default', 'Default workspace')`)

	// Migrate legacy services (workspace_id=0) to the default workspace.
	var defaultWSID int64
	row := db.QueryRow(`SELECT id FROM workspaces WHERE slug = 'default'`)
	if err := row.Scan(&defaultWSID); err == nil && defaultWSID > 0 {
		_, _ = db.Exec(`UPDATE services SET workspace_id = ? WHERE workspace_id = 0`, defaultWSID)
	}

	return nil
}

// Close closes the database connection.
func (s *Storage) Close() error {
	return s.db.Close()
}

// Ping checks database connectivity.
func (s *Storage) Ping() error {
	return s.db.Ping()
}

// DBStats returns basic database statistics for health checks.
type DBStats struct {
	ServiceCount    int    `json:"service_count"`
	TestResultCount int    `json:"test_result_count"`
	DBSizeBytes     int64  `json:"db_size_bytes"`
	DBPath          string `json:"db_path"`
}

func (s *Storage) Stats() (*DBStats, error) {
	stats := &DBStats{DBPath: s.path}
	if err := s.db.QueryRow("SELECT COUNT(*) FROM services").Scan(&stats.ServiceCount); err != nil {
		return nil, err
	}
	if err := s.db.QueryRow("SELECT COUNT(*) FROM test_results").Scan(&stats.TestResultCount); err != nil {
		return nil, err
	}
	// page_count * page_size gives the database file size.
	var pageCount, pageSize int64
	if err := s.db.QueryRow("PRAGMA page_count").Scan(&pageCount); err == nil {
		if err := s.db.QueryRow("PRAGMA page_size").Scan(&pageSize); err == nil {
			stats.DBSizeBytes = pageCount * pageSize
		}
	}
	return stats, nil
}

// ---------- Settings ----------

// GetSetting returns the value for a setting key, or "" if not found.
func (s *Storage) GetSetting(key string) (string, error) {
	var value string
	err := s.db.QueryRow(`SELECT value FROM settings WHERE key = ?`, key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return value, nil
}

// SetSetting upserts a setting key/value pair.
func (s *Storage) SetSetting(key, value string) error {
	_, err := s.db.Exec(
		`INSERT INTO settings (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
		key, value,
	)
	return err
}

// GetAllSettings returns all settings as a map.
func (s *Storage) GetAllSettings() (map[string]string, error) {
	rows, err := s.db.Query(`SELECT key, value FROM settings`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	settings := make(map[string]string)
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		settings[k] = v
	}
	return settings, rows.Err()
}

// ---------- Services ----------

// ListServices returns all services ordered by id.
func (s *Storage) ListServices() ([]Service, error) {
	rows, err := s.db.Query(`SELECT id, name, url, method, headers, body, concurrency, duration, timeout, tags, group_name, steps, data_source, assertions, profiles, cookie_jar, workspace_id, http2, disable_keep_alive, max_idle_conns, dns_cache, warmup_seconds, think_time_ms, think_time_max_ms, arrival_rate, validations, content_type, form_fields, protocol, protocol_config, warmup_conns, adaptive_concurrency, adaptive_target_ms, requests_per_iteration, created_at, updated_at FROM services ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var services []Service
	for rows.Next() {
		svc, err := scanService(rows)
		if err != nil {
			return nil, err
		}
		services = append(services, svc)
	}
	return services, rows.Err()
}

// ServicePage holds a paginated list of services.
type ServicePage struct {
	Services []Service `json:"services"`
	Total    int       `json:"total"`
	Page     int       `json:"page"`
	PerPage  int       `json:"per_page"`
	Pages    int       `json:"pages"`
}

// ListServicesPaged returns a paginated list of services.
// search is an optional name/url filter; sortBy can be "name", "created_at", "updated_at".
func (s *Storage) ListServicesPaged(page, perPage int, search, sortBy, sortDir string, workspaceID int64) (*ServicePage, error) {
	if page < 1 {
		page = 1
	}
	if perPage < 1 || perPage > 100 {
		perPage = 20
	}

	// Validate sort parameters to prevent SQL injection.
	allowedSort := map[string]bool{"id": true, "name": true, "created_at": true, "updated_at": true}
	if !allowedSort[sortBy] {
		sortBy = "id"
	}
	if sortDir != "asc" && sortDir != "desc" {
		sortDir = "asc"
	}

	where := "1=1"
	args := []interface{}{}
	if workspaceID > 0 {
		where += " AND workspace_id = ?"
		args = append(args, workspaceID)
	}
	if search != "" {
		where += " AND (name LIKE ? OR url LIKE ?)"
		like := "%" + search + "%"
		args = append(args, like, like)
	}

	// Count total matching.
	var total int
	countQ := "SELECT COUNT(*) FROM services WHERE " + where
	if err := s.db.QueryRow(countQ, args...).Scan(&total); err != nil {
		return nil, err
	}

	pages := (total + perPage - 1) / perPage
	if page > pages && pages > 0 {
		page = pages
	}
	offset := (page - 1) * perPage

	query := fmt.Sprintf(
		`SELECT id, name, url, method, headers, body, concurrency, duration, timeout, tags, group_name, steps, data_source, assertions, profiles, cookie_jar, workspace_id, http2, disable_keep_alive, max_idle_conns, dns_cache, warmup_seconds, think_time_ms, think_time_max_ms, arrival_rate, validations, content_type, form_fields, protocol, protocol_config, warmup_conns, adaptive_concurrency, adaptive_target_ms, requests_per_iteration, created_at, updated_at FROM services WHERE %s ORDER BY %s %s LIMIT ? OFFSET ?`,
		where, sortBy, sortDir,
	)
	args = append(args, perPage, offset)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	services := make([]Service, 0, perPage)
	for rows.Next() {
		svc, err := scanService(rows)
		if err != nil {
			return nil, err
		}
		services = append(services, svc)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return &ServicePage{
		Services: services,
		Total:    total,
		Page:     page,
		PerPage:  perPage,
		Pages:    pages,
	}, nil
}

// GetService returns a single service by id, or nil if not found.
func (s *Storage) GetService(id int64) (*Service, error) {
	row := s.db.QueryRow(`SELECT id, name, url, method, headers, body, concurrency, duration, timeout, tags, group_name, steps, data_source, assertions, profiles, cookie_jar, workspace_id, http2, disable_keep_alive, max_idle_conns, dns_cache, warmup_seconds, think_time_ms, think_time_max_ms, arrival_rate, validations, content_type, form_fields, protocol, protocol_config, warmup_conns, adaptive_concurrency, adaptive_target_ms, requests_per_iteration, created_at, updated_at FROM services WHERE id = ?`, id)
	svc, err := scanServiceRow(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &svc, nil
}

// CreateService inserts a new service and sets svc.ID.
func (s *Storage) CreateService(svc *Service) error {
	headersJSON, err := json.Marshal(svc.Headers)
	if err != nil {
		return fmt.Errorf("marshal headers: %w", err)
	}

	stepsStr := svc.Steps
	if stepsStr == "" {
		stepsStr = "[]"
	}
	dataSourceStr := svc.DataSource
	if dataSourceStr == "" {
		dataSourceStr = "[]"
	}
	assertionsStr := svc.Assertions
	if assertionsStr == "" {
		assertionsStr = "[]"
	}
	profilesStr := svc.Profiles
	if profilesStr == "" {
		profilesStr = "[]"
	}

	validationsStr := svc.Validations
	if validationsStr == "" {
		validationsStr = "[]"
	}

	contentTypeStr := svc.ContentType
	if contentTypeStr == "" {
		contentTypeStr = "json"
	}
	formFieldsStr := svc.FormFields
	if formFieldsStr == "" {
		formFieldsStr = "[]"
	}

	protocolStr := svc.Protocol
	if protocolStr == "" {
		protocolStr = "http"
	}
	protocolConfigStr := svc.ProtocolConfig
	if protocolConfigStr == "" {
		protocolConfigStr = "{}"
	}

	res, err := s.db.Exec(`INSERT INTO services (name, url, method, headers, body, concurrency, duration, timeout, tags, group_name, steps, data_source, assertions, profiles, cookie_jar, workspace_id, http2, disable_keep_alive, max_idle_conns, dns_cache, warmup_seconds, think_time_ms, think_time_max_ms, arrival_rate, validations, content_type, form_fields, protocol, protocol_config, warmup_conns, adaptive_concurrency, adaptive_target_ms, requests_per_iteration) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		svc.Name, svc.URL, svc.Method, string(headersJSON), svc.Body, svc.Concurrency, svc.Duration, svc.Timeout, svc.Tags, svc.GroupName, stepsStr, dataSourceStr, assertionsStr, profilesStr, svc.CookieJar, svc.WorkspaceID, svc.HTTP2, svc.DisableKeepAlive, svc.MaxIdleConns, svc.DNSCache, svc.WarmupSeconds, svc.ThinkTimeMs, svc.ThinkTimeMaxMs, svc.ArrivalRate, validationsStr, contentTypeStr, formFieldsStr, protocolStr, protocolConfigStr, svc.WarmupConns, svc.AdaptiveConcurrency, svc.AdaptiveTargetMs, svc.RequestsPerIteration)
	if err != nil {
		return err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	svc.ID = id

	// Read back timestamps.
	row := s.db.QueryRow(`SELECT created_at, updated_at FROM services WHERE id = ?`, id)
	return row.Scan(&svc.CreatedAt, &svc.UpdatedAt)
}

// UpdateService updates an existing service.
func (s *Storage) UpdateService(svc *Service) error {
	headersJSON, err := json.Marshal(svc.Headers)
	if err != nil {
		return fmt.Errorf("marshal headers: %w", err)
	}

	stepsStr := svc.Steps
	if stepsStr == "" {
		stepsStr = "[]"
	}
	dataSourceStr := svc.DataSource
	if dataSourceStr == "" {
		dataSourceStr = "[]"
	}
	assertionsStr := svc.Assertions
	if assertionsStr == "" {
		assertionsStr = "[]"
	}
	profilesStr := svc.Profiles
	if profilesStr == "" {
		profilesStr = "[]"
	}

	validationsStr := svc.Validations
	if validationsStr == "" {
		validationsStr = "[]"
	}

	contentTypeStr := svc.ContentType
	if contentTypeStr == "" {
		contentTypeStr = "json"
	}
	formFieldsStr := svc.FormFields
	if formFieldsStr == "" {
		formFieldsStr = "[]"
	}

	protocolStr := svc.Protocol
	if protocolStr == "" {
		protocolStr = "http"
	}
	protocolConfigStr := svc.ProtocolConfig
	if protocolConfigStr == "" {
		protocolConfigStr = "{}"
	}

	res, err := s.db.Exec(`UPDATE services SET name=?, url=?, method=?, headers=?, body=?, concurrency=?, duration=?, timeout=?, tags=?, group_name=?, steps=?, data_source=?, assertions=?, profiles=?, cookie_jar=?, workspace_id=?, http2=?, disable_keep_alive=?, max_idle_conns=?, dns_cache=?, warmup_seconds=?, think_time_ms=?, think_time_max_ms=?, arrival_rate=?, validations=?, content_type=?, form_fields=?, protocol=?, protocol_config=?, warmup_conns=?, adaptive_concurrency=?, adaptive_target_ms=?, requests_per_iteration=?, updated_at=CURRENT_TIMESTAMP WHERE id=?`,
		svc.Name, svc.URL, svc.Method, string(headersJSON), svc.Body, svc.Concurrency, svc.Duration, svc.Timeout, svc.Tags, svc.GroupName, stepsStr, dataSourceStr, assertionsStr, profilesStr, svc.CookieJar, svc.WorkspaceID, svc.HTTP2, svc.DisableKeepAlive, svc.MaxIdleConns, svc.DNSCache, svc.WarmupSeconds, svc.ThinkTimeMs, svc.ThinkTimeMaxMs, svc.ArrivalRate, validationsStr, contentTypeStr, formFieldsStr, protocolStr, protocolConfigStr, svc.WarmupConns, svc.AdaptiveConcurrency, svc.AdaptiveTargetMs, svc.RequestsPerIteration, svc.ID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}

	row := s.db.QueryRow(`SELECT updated_at FROM services WHERE id = ?`, svc.ID)
	return row.Scan(&svc.UpdatedAt)
}

// DeleteService removes a service by id.
func (s *Storage) DeleteService(id int64) error {
	res, err := s.db.Exec(`DELETE FROM services WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// CloneService copies a service with " (copy)" appended to the name.
func (s *Storage) CloneService(id int64) (*Service, error) {
	src, err := s.GetService(id)
	if err != nil {
		return nil, err
	}
	if src == nil {
		return nil, sql.ErrNoRows
	}

	clone := *src
	clone.ID = 0
	clone.Name = src.Name + " (copy)"
	if err := s.CreateService(&clone); err != nil {
		return nil, err
	}
	return &clone, nil
}

// ---------- Test Results ----------

// SaveTestResult inserts a test result and sets result.ID.
func (s *Storage) SaveTestResult(serviceID int64, result *TestResult) error {
	codesJSON, err := json.Marshal(result.StatusCodes)
	if err != nil {
		return fmt.Errorf("marshal status_codes: %w", err)
	}

	timelineStr := result.Timeline
	if timelineStr == "" {
		timelineStr = "[]"
	}
	statusStr := result.Status
	if statusStr == "" {
		statusStr = "pass"
	}
	assertionResultsStr := result.AssertionResults
	if assertionResultsStr == "" {
		assertionResultsStr = "[]"
	}
	tlsInfoStr := result.TLSInfo
	if tlsInfoStr == "" {
		tlsInfoStr = "{}"
	}
	rateLimitInfoStr := result.RateLimitInfo
	if rateLimitInfoStr == "" {
		rateLimitInfoStr = "{}"
	}
	runConfigStr := result.RunConfig
	if runConfigStr == "" {
		runConfigStr = "{}"
	}

	res, err := s.db.Exec(`INSERT INTO test_results (service_id, duration_ms, total_reqs, errors, rps, avg_latency_ms, p50_latency_ms, p95_latency_ms, p99_latency_ms, min_latency_ms, max_latency_ms, status_codes, note, timeline, status, assertion_results, tls_info, rate_limit_info, run_config) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		serviceID, result.DurationMs, result.TotalReqs, result.Errors, result.RPS,
		result.AvgLatencyMs, result.P50LatencyMs, result.P95LatencyMs, result.P99LatencyMs,
		result.MinLatencyMs, result.MaxLatencyMs, string(codesJSON), result.Note, timelineStr, statusStr, assertionResultsStr, tlsInfoStr, rateLimitInfoStr, runConfigStr)
	if err != nil {
		return err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	result.ID = id
	result.ServiceID = serviceID

	row := s.db.QueryRow(`SELECT created_at FROM test_results WHERE id = ?`, id)
	return row.Scan(&result.CreatedAt)
}

// UpdateTestNote updates the note for a test result.
func (s *Storage) UpdateTestNote(resultID int64, note string) error {
	res, err := s.db.Exec(`UPDATE test_results SET note = ? WHERE id = ?`, note, resultID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// GetLastResult returns the most recent test result for a service, or nil,nil if none.
func (s *Storage) GetLastResult(serviceID int64) (*TestResult, error) {
	row := s.db.QueryRow(`SELECT id, service_id, duration_ms, total_reqs, errors, rps, avg_latency_ms, p50_latency_ms, p95_latency_ms, p99_latency_ms, min_latency_ms, max_latency_ms, status_codes, note, timeline, status, assertion_results, tls_info, rate_limit_info, run_config, created_at FROM test_results WHERE service_id = ? ORDER BY id DESC LIMIT 1`, serviceID)

	tr, err := scanResultRow(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &tr, nil
}

// GetLastResults returns the most recent test result for each of the given
// service IDs in a single query, keyed by service ID. Services with no results
// are simply absent from the map. This avoids the N+1 query pattern when
// building service lists.
func (s *Storage) GetLastResults(serviceIDs []int64) (map[int64]*TestResult, error) {
	out := make(map[int64]*TestResult, len(serviceIDs))
	if len(serviceIDs) == 0 {
		return out, nil
	}

	placeholders := make([]string, len(serviceIDs))
	args := make([]interface{}, len(serviceIDs))
	for i, id := range serviceIDs {
		placeholders[i] = "?"
		args[i] = id
	}
	in := strings.Join(placeholders, ",")

	query := `SELECT id, service_id, duration_ms, total_reqs, errors, rps, avg_latency_ms, p50_latency_ms, p95_latency_ms, p99_latency_ms, min_latency_ms, max_latency_ms, status_codes, note, timeline, status, assertion_results, tls_info, rate_limit_info, run_config, created_at
		FROM test_results
		WHERE id IN (SELECT MAX(id) FROM test_results WHERE service_id IN (` + in + `) GROUP BY service_id)`

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		tr, err := scanResult(rows)
		if err != nil {
			return nil, err
		}
		trCopy := tr
		out[tr.ServiceID] = &trCopy
	}
	return out, rows.Err()
}

// ListResults returns the most recent test results for a service.
func (s *Storage) ListResults(serviceID int64, limit int) ([]TestResult, error) {
	rows, err := s.db.Query(`SELECT id, service_id, duration_ms, total_reqs, errors, rps, avg_latency_ms, p50_latency_ms, p95_latency_ms, p99_latency_ms, min_latency_ms, max_latency_ms, status_codes, note, timeline, status, assertion_results, tls_info, rate_limit_info, run_config, created_at FROM test_results WHERE service_id = ? ORDER BY id DESC LIMIT ?`, serviceID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []TestResult
	for rows.Next() {
		tr, err := scanResult(rows)
		if err != nil {
			return nil, err
		}
		results = append(results, tr)
	}
	return results, rows.Err()
}

// GetResult returns a single test result by its ID.
func (s *Storage) GetResult(resultID int64) (*TestResult, error) {
	row := s.db.QueryRow(`SELECT id, service_id, duration_ms, total_reqs, errors, rps, avg_latency_ms, p50_latency_ms, p95_latency_ms, p99_latency_ms, min_latency_ms, max_latency_ms, status_codes, note, timeline, status, assertion_results, tls_info, rate_limit_info, run_config, created_at FROM test_results WHERE id = ?`, resultID)
	tr, err := scanResultRow(row)
	if err != nil {
		return nil, err
	}
	return &tr, nil
}

// DeleteResultsByService removes all test results for a service.
func (s *Storage) DeleteResultsByService(serviceID int64) error {
	_, err := s.db.Exec(`DELETE FROM test_results WHERE service_id = ?`, serviceID)
	return err
}

// DeleteResult removes a single test result belonging to a service.
func (s *Storage) DeleteResult(serviceID, resultID int64) error {
	_, err := s.db.Exec(`DELETE FROM test_results WHERE id = ? AND service_id = ?`, resultID, serviceID)
	return err
}

// PurgeOldResults deletes test results older than the given time.
// Returns the number of deleted rows.
func (s *Storage) PurgeOldResults(before time.Time) (int64, error) {
	res, err := s.db.Exec(`DELETE FROM test_results WHERE created_at < ?`, before)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// ---------- scan helpers ----------

func scanService(rows *sql.Rows) (Service, error) {
	var svc Service
	var headersStr string
	if err := rows.Scan(&svc.ID, &svc.Name, &svc.URL, &svc.Method, &headersStr, &svc.Body, &svc.Concurrency, &svc.Duration, &svc.Timeout, &svc.Tags, &svc.GroupName, &svc.Steps, &svc.DataSource, &svc.Assertions, &svc.Profiles, &svc.CookieJar, &svc.WorkspaceID, &svc.HTTP2, &svc.DisableKeepAlive, &svc.MaxIdleConns, &svc.DNSCache, &svc.WarmupSeconds, &svc.ThinkTimeMs, &svc.ThinkTimeMaxMs, &svc.ArrivalRate, &svc.Validations, &svc.ContentType, &svc.FormFields, &svc.Protocol, &svc.ProtocolConfig, &svc.WarmupConns, &svc.AdaptiveConcurrency, &svc.AdaptiveTargetMs, &svc.RequestsPerIteration, &svc.CreatedAt, &svc.UpdatedAt); err != nil {
		return svc, err
	}
	svc.Headers = make(map[string]string)
	if headersStr != "" {
		_ = json.Unmarshal([]byte(headersStr), &svc.Headers)
	}
	return svc, nil
}

func scanServiceRow(row *sql.Row) (Service, error) {
	var svc Service
	var headersStr string
	if err := row.Scan(&svc.ID, &svc.Name, &svc.URL, &svc.Method, &headersStr, &svc.Body, &svc.Concurrency, &svc.Duration, &svc.Timeout, &svc.Tags, &svc.GroupName, &svc.Steps, &svc.DataSource, &svc.Assertions, &svc.Profiles, &svc.CookieJar, &svc.WorkspaceID, &svc.HTTP2, &svc.DisableKeepAlive, &svc.MaxIdleConns, &svc.DNSCache, &svc.WarmupSeconds, &svc.ThinkTimeMs, &svc.ThinkTimeMaxMs, &svc.ArrivalRate, &svc.Validations, &svc.ContentType, &svc.FormFields, &svc.Protocol, &svc.ProtocolConfig, &svc.WarmupConns, &svc.AdaptiveConcurrency, &svc.AdaptiveTargetMs, &svc.RequestsPerIteration, &svc.CreatedAt, &svc.UpdatedAt); err != nil {
		return svc, err
	}
	svc.Headers = make(map[string]string)
	if headersStr != "" {
		_ = json.Unmarshal([]byte(headersStr), &svc.Headers)
	}
	return svc, nil
}

func scanResult(rows *sql.Rows) (TestResult, error) {
	var tr TestResult
	var codesStr string
	if err := rows.Scan(&tr.ID, &tr.ServiceID, &tr.DurationMs, &tr.TotalReqs, &tr.Errors, &tr.RPS, &tr.AvgLatencyMs, &tr.P50LatencyMs, &tr.P95LatencyMs, &tr.P99LatencyMs, &tr.MinLatencyMs, &tr.MaxLatencyMs, &codesStr, &tr.Note, &tr.Timeline, &tr.Status, &tr.AssertionResults, &tr.TLSInfo, &tr.RateLimitInfo, &tr.RunConfig, &tr.CreatedAt); err != nil {
		return tr, err
	}
	tr.StatusCodes = make(map[int]int)
	if codesStr != "" {
		_ = json.Unmarshal([]byte(codesStr), &tr.StatusCodes)
	}
	return tr, nil
}

func scanResultRow(row *sql.Row) (TestResult, error) {
	var tr TestResult
	var codesStr string
	if err := row.Scan(&tr.ID, &tr.ServiceID, &tr.DurationMs, &tr.TotalReqs, &tr.Errors, &tr.RPS, &tr.AvgLatencyMs, &tr.P50LatencyMs, &tr.P95LatencyMs, &tr.P99LatencyMs, &tr.MinLatencyMs, &tr.MaxLatencyMs, &codesStr, &tr.Note, &tr.Timeline, &tr.Status, &tr.AssertionResults, &tr.TLSInfo, &tr.RateLimitInfo, &tr.RunConfig, &tr.CreatedAt); err != nil {
		return tr, err
	}
	tr.StatusCodes = make(map[int]int)
	if codesStr != "" {
		_ = json.Unmarshal([]byte(codesStr), &tr.StatusCodes)
	}
	return tr, nil
}

// ---------- Schedules ----------

// Schedule represents a recurring load test schedule.
type Schedule struct {
	ID        int64      `json:"id"`
	ServiceID int64      `json:"service_id"`
	CronExpr  string     `json:"cron_expr"`
	Enabled   bool       `json:"enabled"`
	LastRun   *time.Time `json:"last_run"`
	NextRun   *time.Time `json:"next_run"`
	CreatedAt time.Time  `json:"created_at"`
}

// CreateSchedule inserts a new schedule and sets sched.ID.
func (s *Storage) CreateSchedule(sched *Schedule) error {
	enabled := 0
	if sched.Enabled {
		enabled = 1
	}
	var nextRun interface{}
	if sched.NextRun != nil {
		nextRun = *sched.NextRun
	}
	res, err := s.db.Exec(
		`INSERT INTO schedules (service_id, cron_expr, enabled, next_run) VALUES (?, ?, ?, ?)`,
		sched.ServiceID, sched.CronExpr, enabled, nextRun,
	)
	if err != nil {
		return err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	sched.ID = id

	row := s.db.QueryRow(`SELECT created_at FROM schedules WHERE id = ?`, id)
	return row.Scan(&sched.CreatedAt)
}

// ListSchedules returns all schedules ordered by id.
func (s *Storage) ListSchedules() ([]Schedule, error) {
	rows, err := s.db.Query(`SELECT id, service_id, cron_expr, enabled, last_run, next_run, created_at FROM schedules ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var schedules []Schedule
	for rows.Next() {
		var sched Schedule
		var enabled int
		var lastRun, nextRun sql.NullTime
		if err := rows.Scan(&sched.ID, &sched.ServiceID, &sched.CronExpr, &enabled, &lastRun, &nextRun, &sched.CreatedAt); err != nil {
			return nil, err
		}
		sched.Enabled = enabled == 1
		if lastRun.Valid {
			sched.LastRun = &lastRun.Time
		}
		if nextRun.Valid {
			sched.NextRun = &nextRun.Time
		}
		schedules = append(schedules, sched)
	}
	return schedules, rows.Err()
}

// GetSchedule returns a single schedule by id, or nil if not found.
func (s *Storage) GetSchedule(id int64) (*Schedule, error) {
	var sched Schedule
	var enabled int
	var lastRun, nextRun sql.NullTime
	err := s.db.QueryRow(`SELECT id, service_id, cron_expr, enabled, last_run, next_run, created_at FROM schedules WHERE id = ?`, id).
		Scan(&sched.ID, &sched.ServiceID, &sched.CronExpr, &enabled, &lastRun, &nextRun, &sched.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	sched.Enabled = enabled == 1
	if lastRun.Valid {
		sched.LastRun = &lastRun.Time
	}
	if nextRun.Valid {
		sched.NextRun = &nextRun.Time
	}
	return &sched, nil
}

// UpdateScheduleEnabled enables or disables a schedule.
func (s *Storage) UpdateScheduleEnabled(id int64, enabled bool) error {
	val := 0
	if enabled {
		val = 1
	}
	res, err := s.db.Exec(`UPDATE schedules SET enabled = ? WHERE id = ?`, val, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// UpdateScheduleCron updates a schedule's cron expression and next-run time.
func (s *Storage) UpdateScheduleCron(id int64, cronExpr string, nextRun *time.Time) error {
	var nr interface{}
	if nextRun != nil {
		nr = *nextRun
	}
	res, err := s.db.Exec(`UPDATE schedules SET cron_expr = ?, next_run = ? WHERE id = ?`, cronExpr, nr, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// SetScheduleNextRun updates only the next_run time for a schedule.
func (s *Storage) SetScheduleNextRun(id int64, nextRun *time.Time) error {
	var nr interface{}
	if nextRun != nil {
		nr = *nextRun
	}
	_, err := s.db.Exec(`UPDATE schedules SET next_run = ? WHERE id = ?`, nr, id)
	return err
}

// DeleteSchedule removes a schedule by id.
func (s *Storage) DeleteSchedule(id int64) error {
	res, err := s.db.Exec(`DELETE FROM schedules WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// GetDueSchedules returns enabled schedules whose next_run is NULL or <= now.
func (s *Storage) GetDueSchedules(now time.Time) ([]Schedule, error) {
	rows, err := s.db.Query(
		`SELECT id, service_id, cron_expr, enabled, last_run, next_run, created_at FROM schedules WHERE enabled = 1 AND (next_run IS NULL OR next_run <= ?)`,
		now,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var schedules []Schedule
	for rows.Next() {
		var sched Schedule
		var enabled int
		var lastRun, nextRun sql.NullTime
		if err := rows.Scan(&sched.ID, &sched.ServiceID, &sched.CronExpr, &enabled, &lastRun, &nextRun, &sched.CreatedAt); err != nil {
			return nil, err
		}
		sched.Enabled = enabled == 1
		if lastRun.Valid {
			sched.LastRun = &lastRun.Time
		}
		if nextRun.Valid {
			sched.NextRun = &nextRun.Time
		}
		schedules = append(schedules, sched)
	}
	return schedules, rows.Err()
}

// UpdateScheduleRun updates the last_run and next_run for a schedule.
func (s *Storage) UpdateScheduleRun(id int64, lastRun, nextRun time.Time) error {
	_, err := s.db.Exec(`UPDATE schedules SET last_run = ?, next_run = ? WHERE id = ?`, lastRun, nextRun, id)
	return err
}

// ---------- Workspaces ----------

// CreateWorkspace inserts a new workspace and sets ws.ID.
func (s *Storage) CreateWorkspace(ws *Workspace) error {
	res, err := s.db.Exec(
		`INSERT INTO workspaces (name, slug, description) VALUES (?, ?, ?)`,
		ws.Name, ws.Slug, ws.Description,
	)
	if err != nil {
		return err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	ws.ID = id

	row := s.db.QueryRow(`SELECT created_at FROM workspaces WHERE id = ?`, id)
	return row.Scan(&ws.CreatedAt)
}

// ListWorkspaces returns all workspaces ordered by id.
func (s *Storage) ListWorkspaces() ([]Workspace, error) {
	rows, err := s.db.Query(`SELECT id, name, slug, description, created_at FROM workspaces ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var workspaces []Workspace
	for rows.Next() {
		var ws Workspace
		if err := rows.Scan(&ws.ID, &ws.Name, &ws.Slug, &ws.Description, &ws.CreatedAt); err != nil {
			return nil, err
		}
		workspaces = append(workspaces, ws)
	}
	if workspaces == nil {
		workspaces = []Workspace{}
	}
	return workspaces, rows.Err()
}

// GetWorkspace returns a single workspace by id, or nil if not found.
func (s *Storage) GetWorkspace(id int64) (*Workspace, error) {
	var ws Workspace
	err := s.db.QueryRow(`SELECT id, name, slug, description, created_at FROM workspaces WHERE id = ?`, id).
		Scan(&ws.ID, &ws.Name, &ws.Slug, &ws.Description, &ws.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &ws, nil
}

// GetWorkspaceBySlug returns a single workspace by slug, or nil if not found.
func (s *Storage) GetWorkspaceBySlug(slug string) (*Workspace, error) {
	var ws Workspace
	err := s.db.QueryRow(`SELECT id, name, slug, description, created_at FROM workspaces WHERE slug = ?`, slug).
		Scan(&ws.ID, &ws.Name, &ws.Slug, &ws.Description, &ws.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &ws, nil
}

// CapacityRun is one stored capacity-probe run.
type CapacityRun struct {
	ID        int64     `json:"id"`
	Result    string    `json:"result"` // raw CapacityResult JSON
	CreatedAt time.Time `json:"created_at"`
}

// SaveCapacityResult appends a capacity-probe run to a service's history.
func (s *Storage) SaveCapacityResult(serviceID int64, resultJSON string) error {
	_, err := s.db.Exec(
		`INSERT INTO capacity_runs (service_id, result, created_at) VALUES (?, ?, CURRENT_TIMESTAMP)`,
		serviceID, resultJSON)
	return err
}

// DeleteCapacityRun removes a single capacity-probe run belonging to a service.
func (s *Storage) DeleteCapacityRun(serviceID, runID int64) error {
	_, err := s.db.Exec(`DELETE FROM capacity_runs WHERE id = ? AND service_id = ?`, runID, serviceID)
	return err
}

// ListCapacityRuns returns a service's capacity-probe runs, newest first.
func (s *Storage) ListCapacityRuns(serviceID int64) ([]CapacityRun, error) {
	rows, err := s.db.Query(
		`SELECT id, result, created_at FROM capacity_runs WHERE service_id = ? ORDER BY id DESC`, serviceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var runs []CapacityRun
	for rows.Next() {
		var r CapacityRun
		if err := rows.Scan(&r.ID, &r.Result, &r.CreatedAt); err != nil {
			return nil, err
		}
		runs = append(runs, r)
	}
	return runs, rows.Err()
}

// GetCapacityResults returns the stored capacity-probe result JSON keyed by
// service id, for the given services. Missing services are simply absent.
func (s *Storage) GetCapacityResults(ids []int64) (map[int64]string, error) {
	out := make(map[int64]string, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	placeholders := make([]string, len(ids))
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}
	// Latest run per service (the newest id in each service's history).
	q := `SELECT cr.service_id, cr.result FROM capacity_runs cr
	      JOIN (SELECT service_id, MAX(id) AS mid FROM capacity_runs
	            WHERE service_id IN (` + strings.Join(placeholders, ",") + `) GROUP BY service_id) m
	      ON cr.id = m.mid`
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var result string
		if err := rows.Scan(&id, &result); err != nil {
			return nil, err
		}
		out[id] = result
	}
	return out, rows.Err()
}

// DefaultWorkspaceID returns the id of the built-in "default" workspace.
func (s *Storage) DefaultWorkspaceID() (int64, error) {
	ws, err := s.GetWorkspaceBySlug("default")
	if err != nil {
		return 0, err
	}
	if ws == nil {
		return 0, sql.ErrNoRows
	}
	return ws.ID, nil
}

// DeleteWorkspace removes a workspace by id and reassigns its services to the
// default workspace. Refuses to delete the default workspace itself.
func (s *Storage) DeleteWorkspace(id int64) error {
	defID, err := s.DefaultWorkspaceID()
	if err != nil {
		return err
	}
	if id == defID {
		return fmt.Errorf("cannot delete the default workspace")
	}
	// Reassign services to the default workspace before deleting.
	if _, err := s.db.Exec(`UPDATE services SET workspace_id = ? WHERE workspace_id = ?`, defID, id); err != nil {
		return err
	}
	res, err := s.db.Exec(`DELETE FROM workspaces WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// ListServicesByWorkspace returns services filtered by workspace id.
func (s *Storage) ListServicesByWorkspace(workspaceID int64) ([]Service, error) {
	rows, err := s.db.Query(`SELECT id, name, url, method, headers, body, concurrency, duration, timeout, tags, group_name, steps, data_source, assertions, profiles, cookie_jar, workspace_id, http2, disable_keep_alive, max_idle_conns, dns_cache, warmup_seconds, think_time_ms, think_time_max_ms, arrival_rate, validations, content_type, form_fields, protocol, protocol_config, warmup_conns, adaptive_concurrency, adaptive_target_ms, requests_per_iteration, created_at, updated_at FROM services WHERE workspace_id = ? ORDER BY id`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var services []Service
	for rows.Next() {
		svc, err := scanService(rows)
		if err != nil {
			return nil, err
		}
		services = append(services, svc)
	}
	if services == nil {
		services = []Service{}
	}
	return services, rows.Err()
}

// ---------- Test Queue ----------

// QueueItem is a pending queued test with a stable id.
type QueueItem struct {
	ID        int64 `json:"id"`
	ServiceID int64 `json:"service_id"`
	Position  int   `json:"position"`
}

// EnqueueTest appends a service to the queue and returns the new item id.
func (s *Storage) EnqueueTest(serviceID int64) (int64, error) {
	var maxPos sql.NullInt64
	if err := s.db.QueryRow(`SELECT MAX(position) FROM queue`).Scan(&maxPos); err != nil {
		return 0, err
	}
	pos := int64(0)
	if maxPos.Valid {
		pos = maxPos.Int64 + 1
	}
	res, err := s.db.Exec(`INSERT INTO queue (service_id, position) VALUES (?, ?)`, serviceID, pos)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// ListQueue returns the pending queue ordered by position.
func (s *Storage) ListQueue() ([]QueueItem, error) {
	rows, err := s.db.Query(`SELECT id, service_id, position FROM queue ORDER BY position ASC, id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]QueueItem, 0)
	for rows.Next() {
		var q QueueItem
		if err := rows.Scan(&q.ID, &q.ServiceID, &q.Position); err != nil {
			return nil, err
		}
		items = append(items, q)
	}
	return items, rows.Err()
}

// PopQueue removes and returns the head of the queue, or nil if empty.
func (s *Storage) PopQueue() (*QueueItem, error) {
	var q QueueItem
	row := s.db.QueryRow(`SELECT id, service_id, position FROM queue ORDER BY position ASC, id ASC LIMIT 1`)
	if err := row.Scan(&q.ID, &q.ServiceID, &q.Position); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	if _, err := s.db.Exec(`DELETE FROM queue WHERE id = ?`, q.ID); err != nil {
		return nil, err
	}
	return &q, nil
}

// RemoveQueueItem deletes a queue item by its stable id.
func (s *Storage) RemoveQueueItem(id int64) error {
	res, err := s.db.Exec(`DELETE FROM queue WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// ClearQueue removes all pending items.
func (s *Storage) ClearQueue() error {
	_, err := s.db.Exec(`DELETE FROM queue`)
	return err
}

// ReorderQueue sets positions to match the given order of item ids.
func (s *Storage) ReorderQueue(ids []int64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for i, id := range ids {
		if _, err := tx.Exec(`UPDATE queue SET position = ? WHERE id = ?`, i, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// CountQueueForService returns how many times a service is already queued.
func (s *Storage) CountQueueForService(serviceID int64) (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM queue WHERE service_id = ?`, serviceID).Scan(&n)
	return n, err
}
