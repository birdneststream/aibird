package birdbase

import (
	"bytes"
	"compress/gzip"
	"encoding/hex"
	"golang.org/x/crypto/sha3"
	"io"
)

func CacheKey(key string) []byte {
	hash := sha3.Sum224([]byte(key))
	hashString := hex.EncodeToString(hash[:])

	return []byte(hashString)
}

func compress(data []byte) ([]byte, error) {
	var b bytes.Buffer
	gz, err := gzip.NewWriterLevel(&b, gzip.BestCompression)
	if err != nil {
		return nil, err
	}
	if _, err := gz.Write(data); err != nil {
		_ = gz.Close()
		return nil, err
	}
	if err := gz.Close(); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

func decompress(data []byte) ([]byte, error) {
	b := bytes.NewReader(data)
	gz, err := gzip.NewReader(b)
	if err != nil {
		return nil, err
	}
	defer gz.Close()
	return io.ReadAll(gz)
}


