package birdbase

import (
	"aibird/logger"
	"strconv"
	"time"

	"git.mills.io/prologic/bitcask"
)

var (
	Data *bitcask.Bitcask
)

func Init() {
	// Increase the maximum value size to 10MB (from the default 65KB)
	var err error
	Data, err = bitcask.Open("bird.db", bitcask.WithMaxValueSize(10*1024*1024))
	if err != nil {
		logger.Fatal("Failed to open database", "error", err)
	}

	go func() {
		for {
			time.Sleep(24 * time.Hour)
			Merge()
		}
	}()
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
