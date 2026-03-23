package apps

import _ "embed"

// DashboardHTML is the embedded dashboard widget HTML content.
// Served as the ui://cq/dashboard resource via MCP resources/read.
//
//go:embed widgets/dashboard.html
var DashboardHTML string
