package gate

import (
	"fmt"
	"sync"
	"time"
)

// Job represents a scheduled recurring task.
type Job struct {
	ID      string
	CronExpr string
}

// JobStore persists job metadata. Implementations can be in-memory or persistent.
type JobStore interface {
	Save(job Job) error
	Delete(id string) error
	List() ([]Job, error)
}

// MemoryJobStore is an in-memory implementation of JobStore.
type MemoryJobStore struct {
	mu   sync.RWMutex
	jobs map[string]Job
}

// NewMemoryJobStore creates a new in-memory job store.
func NewMemoryJobStore() *MemoryJobStore {
	return &MemoryJobStore{jobs: make(map[string]Job)}
}

func (s *MemoryJobStore) Save(job Job) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.jobs[job.ID] = job
	return nil
}

func (s *MemoryJobStore) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.jobs, id)
	return nil
}

func (s *MemoryJobStore) List() ([]Job, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	jobs := make([]Job, 0, len(s.jobs))
	for _, j := range s.jobs {
		jobs = append(jobs, j)
	}
	return jobs, nil
}

// scheduledJob is the internal representation tracking a running schedule.
type scheduledJob struct {
	job    Job
	cancel chan struct{}
}

// Scheduler manages periodic job execution using cron-like expressions.
// Supports standard cron fields and the @every <duration> shorthand.
type Scheduler struct {
	store   JobStore
	mu      sync.Mutex
	running map[string]*scheduledJob
	counter uint64
	done    chan struct{}
}

// NewScheduler creates a Scheduler backed by the given store.
func NewScheduler(store JobStore) *Scheduler {
	return &Scheduler{
		store:   store,
		running: make(map[string]*scheduledJob),
		done:    make(chan struct{}),
	}
}

// Schedule registers and starts a new recurring job.
// expr supports @every <duration> (e.g. "@every 1m", "@every 500ms")
// and a subset of cron syntax.
// Returns the created Job and any parse error.
func (s *Scheduler) Schedule(expr string, action func()) (Job, error) {
	dur, err := parseCronExpr(expr)
	if err != nil {
		return Job{}, fmt.Errorf("parse cron expr %q: %w", expr, err)
	}

	s.mu.Lock()
	s.counter++
	id := fmt.Sprintf("job-%d", s.counter)
	s.mu.Unlock()

	job := Job{ID: id, CronExpr: expr}
	if err := s.store.Save(job); err != nil {
		return Job{}, fmt.Errorf("save job: %w", err)
	}

	sj := &scheduledJob{
		job:    job,
		cancel: make(chan struct{}),
	}

	s.mu.Lock()
	s.running[id] = sj
	s.mu.Unlock()

	go func() {
		ticker := time.NewTicker(dur)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				action()
			case <-sj.cancel:
				return
			case <-s.done:
				return
			}
		}
	}()

	return job, nil
}

// Cancel stops and removes a job by ID.
func (s *Scheduler) Cancel(id string) {
	s.mu.Lock()
	sj, ok := s.running[id]
	if ok {
		delete(s.running, id)
	}
	s.mu.Unlock()

	if ok {
		close(sj.cancel)
		_ = s.store.Delete(id)
	}
}

// Stop halts all running jobs.
func (s *Scheduler) Stop() {
	close(s.done)
}

// parseCronExpr parses @every <duration> shorthand.
// Returns an error for unsupported expressions.
func parseCronExpr(expr string) (time.Duration, error) {
	if len(expr) > 7 && expr[:7] == "@every " {
		d, err := time.ParseDuration(expr[7:])
		if err != nil {
			return 0, fmt.Errorf("invalid @every duration: %w", err)
		}
		if d <= 0 {
			return 0, fmt.Errorf("duration must be positive, got %v", d)
		}
		return d, nil
	}
	return 0, fmt.Errorf("unsupported cron expression %q; only @every <duration> is supported", expr)
}
