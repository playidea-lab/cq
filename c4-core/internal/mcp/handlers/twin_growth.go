package handlers

import (
	"database/sql"
	"fmt"
	"os"
	"time"
)

// RecordGrowthSnapshot records a weekly metric snapshot in twin_growth.
func (s *SQLiteStore) RecordGrowthSnapshot(username string) {
	now := time.Now()
	period := fmt.Sprintf("%d-W%02d", now.Year(), isoWeek(now))

	// Check if already recorded this period
	var exists int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM twin_growth WHERE username=? AND period=?", username, period).Scan(&exists); err != nil {
		fmt.Fprintf(os.Stderr, "c4: twin: RecordGrowthSnapshot exists check: %v\n", err)
		return
	}
	if exists > 0 {
		return
	}

	// Calculate current approval rate
	var total, approved int
	if err := s.db.QueryRow(`
		SELECT COUNT(*), SUM(CASE WHEN outcome='approved' THEN 1 ELSE 0 END)
		FROM persona_stats`).Scan(&total, &approved); err != nil {
		fmt.Fprintf(os.Stderr, "c4: twin: RecordGrowthSnapshot approval rate: %v\n", err)
	}

	if total > 0 {
		rate := float64(approved) / float64(total)
		s.insertGrowthMetric(username, "approval_rate", rate, period)
	}

	// Average review score
	var avgScore sql.NullFloat64
	if err := s.db.QueryRow("SELECT AVG(review_score) FROM persona_stats WHERE review_score > 0").Scan(&avgScore); err != nil {
		fmt.Fprintf(os.Stderr, "c4: twin: RecordGrowthSnapshot avg score: %v\n", err)
	}
	if avgScore.Valid {
		s.insertGrowthMetric(username, "avg_review_score", avgScore.Float64, period)
	}

	// Tasks completed this period
	var completed int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM c4_tasks WHERE status='done'").Scan(&completed); err != nil {
		fmt.Fprintf(os.Stderr, "c4: twin: RecordGrowthSnapshot tasks completed: %v\n", err)
	}
	s.insertGrowthMetric(username, "tasks_completed", float64(completed), period)
}

func (s *SQLiteStore) insertGrowthMetric(username, metric string, value float64, period string) {
	if _, err := s.db.Exec(`
		INSERT OR IGNORE INTO twin_growth (username, metric, value, period)
		VALUES (?, ?, ?, ?)`, username, metric, value, period); err != nil {
		fmt.Fprintf(os.Stderr, "c4: twin: insertGrowthMetric %s: %v\n", metric, err)
	}
}

// GetGrowthTrend retrieves recent growth metrics for a user.
func (s *SQLiteStore) GetGrowthTrend(username string) map[string]*GrowthMetric {
	metrics := map[string]*GrowthMetric{}

	rows, err := s.db.Query(`
		SELECT metric, value, period FROM twin_growth
		WHERE username = ?
		ORDER BY period DESC
		LIMIT 24`, username)
	if err != nil {
		return metrics
	}
	defer rows.Close()

	// Collect values per metric
	type mv struct {
		value  float64
		period string
	}
	byMetric := map[string][]mv{}

	for rows.Next() {
		var metric, period string
		var value float64
		if err := rows.Scan(&metric, &value, &period); err != nil {
			continue
		}
		byMetric[metric] = append(byMetric[metric], mv{value, period})
	}

	for name, values := range byMetric {
		if len(values) == 0 {
			continue
		}
		gm := &GrowthMetric{Current: values[0].value}
		if len(values) >= 5 {
			gm.PreviousMonth = values[4].value
			if gm.Current > gm.PreviousMonth+0.05 {
				gm.Trend = "improving"
			} else if gm.Current < gm.PreviousMonth-0.05 {
				gm.Trend = "declining"
			} else {
				gm.Trend = "stable"
			}
		}
		metrics[name] = gm
	}

	return metrics
}
