package birdbase

import (
	"aibird/logger"
	"context"
	"strconv"
	"time"

	"git.mills.io/prologic/bitcask"
)

var (
	Data              *bitcask.Bitcask
	maintenanceCancel context.CancelFunc
)

func Init() {
	// Increase the maximum value size to 10MB (from the default 65KB)
	var err error
	Data, err = bitcask.Open("bird.db", bitcask.WithMaxValueSize(10*1024*1024))
	if err != nil {
		logger.Fatal("Failed to open database", "error", err)
	}

	// Start maintenance loop with context cancellation
	ctx, cancel := context.WithCancel(context.Background())
	maintenanceCancel = cancel
	go maintenanceLoop(ctx)
}

func maintenanceLoop(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			logger.Info("Database maintenance loop stopped")
			return
		case <-ticker.C:
			// Run cleanup and merge in separate goroutines to avoid blocking
			go func() {
				cleanupExpiredKeys(ctx)
			}()
			go func() {
				Merge()
			}()
		}
	}
}

func Close() {
	logger.Info("Closing database...")

	// Cancel maintenance loop
	if maintenanceCancel != nil {
		maintenanceCancel()
	}

	// Use a timeout context for shutdown operations to prevent hanging
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Final cleanup before shutdown with timeout
	done := make(chan struct{})
	go func() {
		defer close(done)
		cleanupExpiredKeys(ctx)
		Merge()
	}()

	select {
	case <-done:
		logger.Info("Database cleanup completed successfully")
	case <-ctx.Done():
		logger.Warn("Database cleanup timed out during shutdown")
	}

	// Close database with a separate timeout
	closeCtx, closeCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer closeCancel()

	closeDone := make(chan error, 1)
	go func() {
		closeDone <- Data.Close()
	}()

	select {
	case err := <-closeDone:
		if err != nil {
			logger.Error("Error closing database", "error", err)
		} else {
			logger.Info("Database closed successfully")
		}
	case <-closeCtx.Done():
		logger.Warn("Database close operation timed out")
	}
}

func Merge() {
	logger.Info("Merging database to reclaim space...")
	err := Data.Merge()
	if err != nil {
		logger.Error("Error merging database", "error", err)
	} else {
		logger.Info("Database merge complete.")
	}
}

func PutString(key string, value string) error {
	compressedValue, err := compress([]byte(value))
	if err != nil {
		return err
	}
	return Data.Put(CacheKey(key), compressedValue)
}

func PutInt(key string, value int) error {
	compressedValue, err := compress([]byte(strconv.Itoa(value)))
	if err != nil {
		return err
	}
	return Data.Put(CacheKey(key), compressedValue)
}

func PutBytes(key string, value []byte) error {
	compressedValue, err := compress(value)
	if err != nil {
		return err
	}
	return Data.Put(CacheKey(key), compressedValue)
}

func PutBytesExpireHours(key string, value []byte, expire int) error {
	compressedValue, err := compress(value)
	if err != nil {
		return err
	}
	return Data.PutWithTTL(CacheKey(key), compressedValue, time.Hour*time.Duration(expire))
}

func PutStringExpireSeconds(key string, value string, expire int) error {
	compressedValue, err := compress([]byte(value))
	if err != nil {
		return err
	}
	return Data.PutWithTTL(CacheKey(key), compressedValue, time.Second*time.Duration(expire))
}

func PutIntExpireHours(key string, value int, expire int) error {
	compressedValue, err := compress([]byte(strconv.Itoa(value)))
	if err != nil {
		return err
	}
	return Data.PutWithTTL(CacheKey(key), compressedValue, time.Hour*time.Duration(expire))
}

func Get(key string) ([]byte, error) {
	compressedValue, err := Data.Get(CacheKey(key))
	if err != nil {
		return nil, err
	}
	return decompress(compressedValue)
}

func Has(key string) bool {
	return Data.Has(CacheKey(key))
}

func Delete(key string) error {
	return Data.Delete(CacheKey(key))
}

func cleanupExpiredKeys(ctx context.Context) {
	logger.Info("Cleaning up expired keys...")
	deletedCount := 0
	const batchSize = 100
	const maxWorkers = 4

	// Channel to collect keys that need processing
	keysChan := make(chan []byte, batchSize*maxWorkers)
	deleteChan := make(chan bool, batchSize*maxWorkers)

	// Start worker goroutines
	for range maxWorkers {
		go func() {
			for key := range keysChan {
				select {
				case <-ctx.Done():
					return
				default:
					// Try to get the key - if it's expired, Bitcask returns ErrKeyExpired
					_, err := Data.Get(key)
					if err == bitcask.ErrKeyExpired {
						// Delete the expired key
						if delErr := Data.Delete(key); delErr != nil {
							logger.Debug("Failed to delete expired key", "error", delErr)
						} else {
							deleteChan <- true
						}
					}
				}
			}
		}()
	}

	// Scan keys and distribute to workers
	go func() {
		defer close(keysChan)
		batch := make([][]byte, 0, batchSize)

		Data.Scan([]byte(""), func(key []byte) error {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
				// Make a copy of the key since Bitcask reuses the slice
				keyCopy := make([]byte, len(key))
				copy(keyCopy, key)
				batch = append(batch, keyCopy)

				// Send batch when full
				if len(batch) >= batchSize {
					for _, k := range batch {
						select {
						case keysChan <- k:
						case <-ctx.Done():
							return ctx.Err()
						}
					}
					batch = batch[:0] // Reset slice
				}
				return nil
			}
		})

		// Send remaining keys
		for _, k := range batch {
			select {
			case keysChan <- k:
			case <-ctx.Done():
				return
			}
		}
	}()

	// Count deletions with timeout
	timeout := time.After(55 * time.Minute) // Leave 5 minutes buffer before next run

	for {
		select {
		case <-ctx.Done():
			logger.Info("Expired key cleanup cancelled", "deleted", deletedCount)
			return
		case <-timeout:
			logger.Info("Expired key cleanup timed out", "deleted", deletedCount)
			return
		case deleted, ok := <-deleteChan:
			if !ok {
				logger.Info("Expired key cleanup complete", "deleted", deletedCount)
				return
			}
			if deleted {
				deletedCount++
				// Log progress every 1000 deletions
				if deletedCount%1000 == 0 {
					logger.Debug("Cleanup progress", "deleted", deletedCount)
				}
			}
		}
	}
}

func GetDatabaseStats() (map[string]any, error) {
	stats, err := Data.Stats()
	if err != nil {
		return nil, err
	}

	result := map[string]any{
		"keys":      stats.Keys,
		"datafiles": stats.Datafiles,
		"size":      stats.Size,
	}

	return result, nil
}
