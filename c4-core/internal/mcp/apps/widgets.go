package apps

import _ "embed"

// DashboardHTML is the embedded dashboard widget HTML content.
// Served as the ui://cq/dashboard resource via MCP resources/read.
//
//go:embed widgets/dashboard.html
var DashboardHTML string

// JobProgressHTML is the embedded job progress widget HTML content.
// Served as the ui://cq/job-progress resource via MCP resources/read.
//
//go:embed widgets/job_progress.html
var JobProgressHTML string

// JobResultHTML is the embedded job result widget HTML content.
// Served as the ui://cq/job-result resource via MCP resources/read.
//
//go:embed widgets/job_result.html
var JobResultHTML string

// ExperimentCompareHTML is the embedded experiment compare widget HTML content.
// Served as the ui://cq/experiment-compare resource via MCP resources/read.
//
//go:embed widgets/experiment_compare.html
var ExperimentCompareHTML string
