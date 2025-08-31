package birdbase

import (
	"context"
	"database/sql"
	"time"

	"aibird/logger"
)

// maintenanceLoop runs periodic cleanup
func maintenanceLoop(ctx context.Context) {
	ticker := time.NewTicker(15 * time.Minute) // Clean up every 15 minutes
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			logger.Info("Database maintenance loop stopped")
			return
		case <-ticker.C:
			go func() {
				if err := Data.Cleanup(); err != nil {
					logger.Error("Failed to cleanup expired entries", "error", err)
				}
			}()
		}
	}
}

// Cleanup removes expired entries
func (s *SQLiteDB) Cleanup() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Cleanup all tables with TTL data
	tables := []string{"chat_history", "key_value_store"}
	totalDeleted := int64(0)

	for _, table := range tables {
		var result sql.Result
		var err error

		if table == "chat_history" {
			result, err = s.db.Exec("DELETE FROM chat_history WHERE expires_at < datetime('now')")
		} else {
			result, err = s.db.Exec("DELETE FROM key_value_store WHERE expires_at IS NOT NULL AND expires_at < datetime('now')")
		}

		if err != nil {
			logger.Error("Failed to cleanup table", "table", table, "error", err)
			continue
		}

		deleted, _ := result.RowsAffected()
		totalDeleted += deleted
	}

	if totalDeleted > 0 {
		logger.Info("Cleaned up expired entries", "total", totalDeleted)

		// Run VACUUM if significant deletions (once daily)
		if totalDeleted > 1000 {
			go func() {
				if err := s.vacuum(); err != nil {
					logger.Error("Failed to vacuum database", "error", err)
				}
			}()
		}
	}

	return nil
}

// vacuum reclaims space
func (s *SQLiteDB) vacuum() error {
	_, err := s.db.Exec("VACUUM")
	if err != nil {
		return err
	}
	logger.Info("Database vacuum completed")
	return nil
}
