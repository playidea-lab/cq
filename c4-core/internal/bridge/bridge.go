// Package bridge implements the Python-Go bridge for gradual migration.
//
// During the migration period, the Go core communicates with the
// existing Python C4 system via gRPC or subprocess calls.
// This allows incremental replacement of Python components with Go.
package bridge
