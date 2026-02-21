// Package state implements the C4 project state machine.
// This file provides advisory phase locking to prevent concurrent
// execution of polish/finish operations.
package state

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

const (
	// phaseLockDir is the directory under .c4 where lock files are stored.
	phaseLockDir = "phase_locks"

	// staleCrossHostThreshold is how long to wait before declaring a cross-host lock stale.
	staleCrossHostThreshold = 2 * time.Hour
)

// PhaseLockInfo is the JSON schema written into .c4/phase_locks/{phase}.lock files.
type PhaseLockInfo struct {
	PID        int       `json:"pid"`
	Hostname   string    `json:"hostname"`
	Phase      string    `json:"phase"`
	AcquiredAt time.Time `json:"acquired_at"`
	SessionID  string    `json:"session_id,omitempty"`
}

// PhaseLockResult is returned by AcquirePhaseLock.
type PhaseLockResult struct {
	Acquired bool            `json:"acquired"`
	Error    *PhaseLockError `json:"error,omitempty"`
}

// PhaseLockError describes why a lock could not be acquired.
type PhaseLockError struct {
	Code    string          `json:"code"`
	Message string          `json:"message"`
	Details *LockHolderInfo `json:"details,omitempty"`
}

// LockHolderInfo describes the current lock holder.
type LockHolderInfo struct {
	HolderPID      int       `json:"holder_pid"`
	HolderHostname string    `json:"holder_hostname"`
	AcquiredAt     time.Time `json:"acquired_at"`
	AgeMinutes     int       `json:"age_minutes"`
}

// PhaseLocker manages advisory phase locks stored as JSON files.
type PhaseLocker struct {
	rootDir string
}

// NewPhaseLocker creates a new PhaseLocker rooted at rootDir (the project root).
func NewPhaseLocker(rootDir string) *PhaseLocker {
	return &PhaseLocker{rootDir: rootDir}
}

// lockDir returns the absolute path to the phase_locks directory.
func (pl *PhaseLocker) lockDir() string {
	return filepath.Join(pl.rootDir, ".c4", phaseLockDir)
}

// lockFile returns the absolute path for the given phase's lock file.
func (pl *PhaseLocker) lockFile(phase string) string {
	return filepath.Join(pl.lockDir(), phase+".lock")
}

// Acquire attempts to acquire an advisory lock for the given phase.
// Returns (true, nil) if the lock was successfully acquired.
// Returns (false, lockError) if the lock is held by another process.
// validPhase returns true if phase is an allowed phase name.
func validPhase(phase string) bool {
	return phase == "polish" || phase == "finish"
}

func (pl *PhaseLocker) Acquire(phase string) PhaseLockResult {
	if !validPhase(phase) {
		return PhaseLockResult{
			Acquired: false,
			Error: &PhaseLockError{
				Code:    "INVALID_PHASE",
				Message: fmt.Sprintf("invalid phase %q: must be one of: polish, finish", phase),
			},
		}
	}
	if err := os.MkdirAll(pl.lockDir(), 0755); err != nil {
		return PhaseLockResult{
			Acquired: false,
			Error: &PhaseLockError{
				Code:    "INTERNAL_ERROR",
				Message: fmt.Sprintf("cannot create phase_locks directory: %v", err),
			},
		}
	}

	lockPath := pl.lockFile(phase)

	// Read existing lock file if present.
	if data, err := os.ReadFile(lockPath); err == nil {
		var existing PhaseLockInfo
		if jsonErr := json.Unmarshal(data, &existing); jsonErr == nil {
			// Determine if the existing lock is stale.
			if !pl.isStale(&existing) {
				// Lock is held and valid.
				age := time.Since(existing.AcquiredAt)
				ageMinutes := int(math.Round(age.Minutes()))
				return PhaseLockResult{
					Acquired: false,
					Error: &PhaseLockError{
						Code:    "LOCK_HELD",
						Message: fmt.Sprintf("phase %q is locked by PID %d on %s (held for %d min)", phase, existing.PID, existing.Hostname, ageMinutes),
						Details: &LockHolderInfo{
							HolderPID:      existing.PID,
							HolderHostname: existing.Hostname,
							AcquiredAt:     existing.AcquiredAt,
							AgeMinutes:     ageMinutes,
						},
					},
				}
			}
			// Stale lock: remove and proceed to acquire.
			_ = os.Remove(lockPath)
		}
	}

	// Write new lock file.
	hostname, _ := os.Hostname()
	sessionID := os.Getenv("C4_SESSION_ID")

	info := PhaseLockInfo{
		PID:        os.Getpid(),
		Hostname:   hostname,
		Phase:      phase,
		AcquiredAt: time.Now().UTC(),
		SessionID:  sessionID,
	}

	data, err := json.Marshal(info)
	if err != nil {
		return PhaseLockResult{
			Acquired: false,
			Error: &PhaseLockError{
				Code:    "INTERNAL_ERROR",
				Message: fmt.Sprintf("cannot marshal lock info: %v", err),
			},
		}
	}

	if err := os.WriteFile(lockPath, data, 0644); err != nil {
		return PhaseLockResult{
			Acquired: false,
			Error: &PhaseLockError{
				Code:    "INTERNAL_ERROR",
				Message: fmt.Sprintf("cannot write lock file: %v", err),
			},
		}
	}

	return PhaseLockResult{Acquired: true}
}

// Release removes the lock file for the given phase.
// Returns true if the file was removed (or did not exist), false on error.
func (pl *PhaseLocker) Release(phase string) bool {
	if !validPhase(phase) {
		return false
	}
	lockPath := pl.lockFile(phase)
	err := os.Remove(lockPath)
	return err == nil || os.IsNotExist(err)
}

// isStale determines whether an existing lock should be treated as stale.
//
// Stale detection follows 5 scenarios:
//  1. Same host + PID alive (kill -0 success) → NOT stale
//  2. Same host + PID dead (ESRCH)             → stale
//  3. Same host + no permission (EPERM)        → NOT stale (conservative)
//  4. Different host + age < 2h                → NOT stale (conservative)
//  5. Different host + age >= 2h               → stale
func (pl *PhaseLocker) isStale(info *PhaseLockInfo) bool {
	localHost, _ := os.Hostname()

	if info.Hostname == localHost {
		// Same host: use kill(pid, 0) to check PID liveness.
		proc, err := os.FindProcess(info.PID)
		if err != nil {
			// Cannot find process — treat as stale.
			return true
		}
		sigErr := proc.Signal(syscall.Signal(0))
		if sigErr == nil {
			// Scenario 1: PID is alive → not stale.
			return false
		}
		// sigErr is non-nil. Distinguish ESRCH from EPERM.
		if isNoSuchProcess(sigErr) {
			// Scenario 2: ESRCH — process does not exist → stale.
			return true
		}
		// Scenario 3: EPERM or other → conservative, not stale.
		return false
	}

	// Different host: use age-based heuristic.
	age := time.Since(info.AcquiredAt)
	if age >= staleCrossHostThreshold {
		// Scenario 5: old enough to be stale.
		return true
	}
	// Scenario 4: recent, assume still valid.
	return false
}

// isNoSuchProcess returns true if the error indicates the process does not exist.
// This covers:
//   - syscall.ESRCH on Linux/macOS when sending signal 0 to a dead PID
//   - "os: process already finished" from os.Process.Signal on macOS
func isNoSuchProcess(err error) bool {
	if err == nil {
		return false
	}
	// Direct syscall.ESRCH check.
	if errno, ok := err.(syscall.Errno); ok {
		return errno == syscall.ESRCH
	}
	// macOS os.Process.Signal wraps the error as a plain string when the
	// process has already been reaped by the OS.
	return err.Error() == "os: process already finished"
}
