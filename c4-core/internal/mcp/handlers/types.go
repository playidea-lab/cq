// Package handlers implements the MCP tool handlers for C4.
//
// Each handler corresponds to a C4 MCP tool and delegates to the
// underlying Store interface for data operations. Handlers are
// responsible for JSON parsing, validation, and response formatting.
package handlers

import "github.com/changmin/c4-core/internal/store"

// Type aliases — these preserve backward compatibility so that all
// existing code referencing handlers.Store, handlers.Task, etc.
// continues to compile without changes.

type Store = store.Store
type Task = store.Task
type TaskAssignment = store.TaskAssignment
type ReviewContext = store.ReviewContext
type RequestChangesResult = store.RequestChangesResult
type WorkerConfigInfo = store.WorkerConfigInfo
type ValidationResult = store.ValidationResult
type SubmitResult = store.SubmitResult
type CheckpointResult = store.CheckpointResult
type WorkerInfo = store.WorkerInfo
type EconomicModeInfo = store.EconomicModeInfo
type ProjectStatus = store.ProjectStatus
type PersonaSummary = store.PersonaSummary
type Lighthouse = store.Lighthouse
type LighthouseContext = store.LighthouseContext
