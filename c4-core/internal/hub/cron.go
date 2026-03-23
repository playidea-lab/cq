package hub

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// =========================================================================
// CronSchedule model
// =========================================================================

// CronSchedule represents a row in hub_cron_schedules.
type CronSchedule struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	CronExpr    string     `json:"cron_expr"`
	JobTemplate string     `json:"job_template"`
	DagID       string     `json:"dag_id"`
	Enabled     bool       `json:"enabled"`
	LastRun     *time.Time `json:"last_run,omitempty"`
	NextRun     *time.Time `json:"next_run,omitempty"`
	ProjectID   string     `json:"project_id"`
	CreatedAt   time.Time  `json:"created_at"`
}

// =========================================================================
// CronScheduler
// =========================================================================

// CronScheduler polls hub_cron_schedules and fires jobs/DAGs on schedule.
type CronScheduler struct {
	client *Client
	logger *slog.Logger
}

// NewCronScheduler creates a CronScheduler backed by the given Hub client.
func NewCronScheduler(client *Client, logger *slog.Logger) *CronScheduler {
	if logger == nil {
		logger = slog.Default()
	}
	return &CronScheduler{client: client, logger: logger}
}

// Start runs the cron tick loop until ctx is cancelled.
// Ticks every 60 seconds.
func (s *CronScheduler) Start(ctx context.Context) {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	// Run once immediately at startup.
	s.tick(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.tick(ctx)
		}
	}
}

// tick queries due schedules and fires jobs/DAGs for each.
func (s *CronScheduler) tick(ctx context.Context) {
	now := time.Now().UTC()
	schedules, err := s.client.listDueSchedules(ctx, now)
	if err != nil {
		s.logger.Error("cron tick: list due schedules", "err", err)
		return
	}

	for _, sched := range schedules {
		s.fire(ctx, sched, now)
	}
}

// fire dispatches a single schedule as a job or DAG run.
func (s *CronScheduler) fire(ctx context.Context, sched CronSchedule, now time.Time) {
	var fireErr error

	if sched.DagID != "" {
		_, fireErr = s.client.ExecuteDAG(sched.DagID, false)
		if fireErr != nil {
			s.logger.Error("cron fire: execute dag", "id", sched.ID, "dag", sched.DagID, "err", fireErr)
		}
	} else if sched.JobTemplate != "{}" && sched.JobTemplate != "" {
		var req JobSubmitRequest
		if err := json.Unmarshal([]byte(sched.JobTemplate), &req); err != nil {
			s.logger.Error("cron fire: unmarshal job_template", "id", sched.ID, "err", err)
			return
		}
		if req.Name == "" {
			req.Name = sched.Name
		}
		if req.ProjectID == "" {
			req.ProjectID = sched.ProjectID
		}
		_, fireErr = s.client.SubmitJob(&req)
		if fireErr != nil {
			s.logger.Error("cron fire: submit job", "id", sched.ID, "err", fireErr)
		}
	}

	// Update last_run and next_run regardless of fire outcome.
	next, err := parseCron(sched.CronExpr, now)
	if err != nil {
		s.logger.Error("cron fire: parse cron expr", "id", sched.ID, "expr", sched.CronExpr, "err", err)
		next = now.Add(60 * time.Second)
	}

	if err := s.client.updateCronRunTimes(ctx, sched.ID, now, next); err != nil {
		s.logger.Error("cron fire: update run times", "id", sched.ID, "err", err)
	}
}

// =========================================================================
// Hub Client — Cron CRUD
// =========================================================================

// CreateCron inserts a new cron schedule via Supabase PostgREST.
func (c *Client) CreateCron(sched *CronSchedule) (*CronSchedule, error) {
	if sched.ID == "" {
		sched.ID = "cron-" + newID()
	}
	if sched.JobTemplate == "" {
		sched.JobTemplate = "{}"
	}

	var rows []CronSchedule
	if err := c.supabasePost("/rest/v1/hub_cron_schedules", sched, &rows); err != nil {
		return nil, fmt.Errorf("create cron: %w", err)
	}
	if len(rows) == 0 {
		return sched, nil
	}
	return &rows[0], nil
}

// ListCrons returns cron schedules, optionally filtered by project.
func (c *Client) ListCrons(projectID string) ([]CronSchedule, error) {
	path := "/rest/v1/hub_cron_schedules?order=created_at.desc"
	if projectID != "" {
		path += "&project_id=eq." + url.QueryEscape(projectID)
	}

	var schedules []CronSchedule
	if err := c.supabaseGet(path, &schedules); err != nil {
		return nil, fmt.Errorf("list crons: %w", err)
	}
	return schedules, nil
}

// DeleteCron removes a cron schedule by ID via Supabase PostgREST.
func (c *Client) DeleteCron(id string) error {
	path := "/rest/v1/hub_cron_schedules?id=eq." + url.QueryEscape(id)
	if err := c.supabaseDelete(path); err != nil {
		return fmt.Errorf("delete cron: %w", err)
	}
	return nil
}

// listDueSchedules returns enabled schedules whose next_run <= now.
func (c *Client) listDueSchedules(ctx context.Context, now time.Time) ([]CronSchedule, error) {
	ts := now.UTC().Format(time.RFC3339)
	path := "/rest/v1/hub_cron_schedules?enabled=eq.true&next_run=lte." + url.QueryEscape(ts)

	var schedules []CronSchedule
	if err := c.supabaseGetCtx(ctx, path, &schedules); err != nil {
		return nil, fmt.Errorf("list due schedules: %w", err)
	}
	return schedules, nil
}

// updateCronRunTimes patches last_run and next_run for a cron schedule.
func (c *Client) updateCronRunTimes(ctx context.Context, id string, lastRun, nextRun time.Time) error {
	body := map[string]any{
		"last_run": lastRun.UTC().Format(time.RFC3339),
		"next_run": nextRun.UTC().Format(time.RFC3339),
	}
	path := "/rest/v1/hub_cron_schedules?id=eq." + url.QueryEscape(id)
	if err := c.supabasePatchCtx(ctx, path, body, nil); err != nil {
		return fmt.Errorf("update cron run times: %w", err)
	}
	return nil
}

// =========================================================================
// parseCron — standard 5-field cron expression
// =========================================================================

// parseCron returns the next time after 'after' matching the 5-field cron expression.
// Fields: minute hour day-of-month month day-of-week (all 1-indexed except minute/hour).
// Supports '*', single values, ranges (1-5), lists (1,3,5), and step values (*/5).
func parseCron(expr string, after time.Time) (time.Time, error) {
	fields := strings.Fields(expr)
	if len(fields) != 5 {
		return time.Time{}, fmt.Errorf("cron: expected 5 fields, got %d in %q", len(fields), expr)
	}

	minuteSet, err := parseCronField(fields[0], 0, 59)
	if err != nil {
		return time.Time{}, fmt.Errorf("cron: minute field %q: %w", fields[0], err)
	}
	hourSet, err := parseCronField(fields[1], 0, 23)
	if err != nil {
		return time.Time{}, fmt.Errorf("cron: hour field %q: %w", fields[1], err)
	}
	domSet, err := parseCronField(fields[2], 1, 31)
	if err != nil {
		return time.Time{}, fmt.Errorf("cron: dom field %q: %w", fields[2], err)
	}
	monthSet, err := parseCronField(fields[3], 1, 12)
	if err != nil {
		return time.Time{}, fmt.Errorf("cron: month field %q: %w", fields[3], err)
	}
	dowSet, err := parseCronField(fields[4], 0, 6)
	if err != nil {
		return time.Time{}, fmt.Errorf("cron: dow field %q: %w", fields[4], err)
	}

	// Start searching from the next minute after 'after'.
	t := after.UTC().Truncate(time.Minute).Add(time.Minute)

	// Safety limit: search up to 4 years worth of minutes.
	limit := t.Add(4 * 365 * 24 * time.Hour)

	for t.Before(limit) {
		if !monthSet[int(t.Month())] {
			// Advance to start of next month.
			t = time.Date(t.Year(), t.Month()+1, 1, 0, 0, 0, 0, time.UTC)
			continue
		}
		if !domSet[t.Day()] || !dowSet[int(t.Weekday())] {
			// Advance to next day.
			t = time.Date(t.Year(), t.Month(), t.Day()+1, 0, 0, 0, 0, time.UTC)
			continue
		}
		if !hourSet[t.Hour()] {
			// Advance to next hour.
			t = time.Date(t.Year(), t.Month(), t.Day(), t.Hour()+1, 0, 0, 0, time.UTC)
			continue
		}
		if !minuteSet[t.Minute()] {
			t = t.Add(time.Minute)
			continue
		}
		return t, nil
	}
	return time.Time{}, fmt.Errorf("cron: no next time found within 4 years for %q", expr)
}

// parseCronField parses a single cron field into a boolean set indexed 0..max.
// Supports: * (any), N (single), N-M (range), N,M,... (list), */step, N-M/step.
func parseCronField(field string, min, max int) ([]bool, error) {
	set := make([]bool, max+1)

	for _, part := range strings.Split(field, ",") {
		if err := applyFieldPart(part, min, max, set); err != nil {
			return nil, err
		}
	}
	return set, nil
}

func applyFieldPart(part string, min, max int, set []bool) error {
	// Handle step syntax: range/step or */step.
	step := 1
	if idx := strings.Index(part, "/"); idx >= 0 {
		s, err := strconv.Atoi(part[idx+1:])
		if err != nil || s <= 0 {
			return fmt.Errorf("invalid step %q", part[idx+1:])
		}
		step = s
		part = part[:idx]
	}

	var lo, hi int
	if part == "*" {
		lo, hi = min, max
	} else if idx := strings.Index(part, "-"); idx >= 0 {
		var err error
		lo, err = strconv.Atoi(part[:idx])
		if err != nil {
			return fmt.Errorf("invalid range start %q", part[:idx])
		}
		hi, err = strconv.Atoi(part[idx+1:])
		if err != nil {
			return fmt.Errorf("invalid range end %q", part[idx+1:])
		}
	} else {
		v, err := strconv.Atoi(part)
		if err != nil {
			return fmt.Errorf("invalid value %q", part)
		}
		lo, hi = v, v
	}

	if lo < min || hi > max || lo > hi {
		return fmt.Errorf("value %d-%d out of range [%d,%d]", lo, hi, min, max)
	}

	for v := lo; v <= hi; v += step {
		set[v] = true
	}
	return nil
}

// =========================================================================
// Additional Supabase helpers (context-aware variants)
// =========================================================================

// supabaseGetCtx performs a GET to Supabase PostgREST with context.
func (c *Client) supabaseGetCtx(ctx context.Context, path string, dest any) error {
	req, err := http.NewRequestWithContext(ctx, "GET", c.supabaseRestURL(path), nil)
	if err != nil {
		return err
	}
	c.setSupabaseHeaders(req)
	req.Header.Set("Accept", "application/json")

	resp, err := c.doWithRetry(req)
	if err != nil {
		return fmt.Errorf("GET %s: %w", path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("GET %s: %d %s", path, resp.StatusCode, string(b))
	}
	if dest != nil {
		if err := json.NewDecoder(resp.Body).Decode(dest); err != nil {
			return fmt.Errorf("decode %s: %w", path, err)
		}
	}
	return nil
}

// supabasePatchCtx performs a PATCH to Supabase PostgREST with context.
func (c *Client) supabasePatchCtx(ctx context.Context, path string, body, dest any) error {
	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, "PATCH", c.supabaseRestURL(path), strings.NewReader(string(data)))
	if err != nil {
		return err
	}
	c.setSupabaseHeaders(req)
	req.Header.Set("Prefer", "return=representation")
	req.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(strings.NewReader(string(data))), nil
	}

	resp, err := c.doWithRetry(req)
	if err != nil {
		return fmt.Errorf("PATCH %s: %w", path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("PATCH %s: %d %s", path, resp.StatusCode, string(b))
	}
	if dest != nil {
		if err := json.NewDecoder(resp.Body).Decode(dest); err != nil {
			return fmt.Errorf("decode %s: %w", path, err)
		}
	}
	return nil
}

// supabaseDelete performs a DELETE to Supabase PostgREST.
func (c *Client) supabaseDelete(path string) error {
	req, err := http.NewRequest("DELETE", c.supabaseRestURL(path), nil)
	if err != nil {
		return err
	}
	c.setSupabaseHeaders(req)

	resp, err := c.doWithRetry(req)
	if err != nil {
		return fmt.Errorf("DELETE %s: %w", path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("DELETE %s: %d %s", path, resp.StatusCode, string(b))
	}
	return nil
}
