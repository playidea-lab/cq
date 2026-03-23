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
