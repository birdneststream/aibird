package birdbase

import (
	"context"
	"time"

	"aibird/logger"
)

const vacuumThreshold = 1000

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

	totalDeleted := int64(0)

	// Cleanup chat_history
	chatResult, err := s.db.Exec("DELETE FROM chat_history WHERE expires_at < datetime('now')")
	if err != nil {
		logger.Error("Failed to cleanup table", "table", "chat_history", "error", err)
	} else {
		deleted, _ := chatResult.RowsAffected()
		totalDeleted += deleted
	}

	// Cleanup key_value_store
	kvResult, err := s.db.Exec("DELETE FROM key_value_store WHERE expires_at IS NOT NULL AND expires_at < datetime('now')")
	if err != nil {
		logger.Error("Failed to cleanup table", "table", "key_value_store", "error", err)
	} else {
		deleted, _ := kvResult.RowsAffected()
		totalDeleted += deleted
	}

	if totalDeleted > 0 {
		logger.Info("Cleaned up expired entries", "total", totalDeleted)

		// Run VACUUM if significant deletions (runs inside lock for exclusive SQLite access)
		if totalDeleted > vacuumThreshold {
			if vacuumErr := s.vacuum(); vacuumErr != nil {
				logger.Error("Failed to vacuum database", "error", vacuumErr)
			}
		}
	}

	// Cleanup old channel messages
	retentionDays := 7                                           // default
	msgDeleted, err := s.cleanupOldMessagesLocked(retentionDays) // caller already holds lock
	if err != nil {
		logger.Error("Failed to cleanup old channel messages", "error", err)
	} else if msgDeleted > 0 {
		logger.Debug("Cleaned up old channel messages", "deleted", msgDeleted)
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
