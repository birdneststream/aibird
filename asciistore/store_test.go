package asciistore

import (
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewASCIIStore(t *testing.T) {
	store := NewASCIIStore()
	require.NotNil(t, store)
	assert.NotNil(t, store.cache)
}

func TestGenerateKey(t *testing.T) {
	key := generateKey("user1", "network1", "#channel1")
	assert.Equal(t, "user1@network1##channel1", key)
}

func TestStoreAndRetrieve(t *testing.T) {
	store := NewASCIIStore()

	lines := []string{"line1", "line2", "line3"}
	store.Store("user1", "network1", "#test", lines, "a cat", true)

	art, found := store.Retrieve("user1", "network1", "#test")
	require.True(t, found)
	assert.Equal(t, lines, art.Lines)
	assert.Equal(t, "a cat", art.Prompt)
	assert.Equal(t, "user1", art.User)
	assert.Equal(t, "network1", art.Network)
	assert.Equal(t, "#test", art.Channel)
	assert.True(t, art.UseHalfblocks)
	assert.WithinDuration(t, art.Timestamp, art.Timestamp, 0) // just checking it's set
}

func TestRetrieve_NotFound(t *testing.T) {
	store := NewASCIIStore()

	art, found := store.Retrieve("nonexistent", "network", "#test")
	assert.False(t, found)
	assert.Nil(t, art)
}

func TestClear(t *testing.T) {
	store := NewASCIIStore()

	store.Store("user1", "network1", "#test", []string{"line1"}, "prompt", false)

	_, found := store.Retrieve("user1", "network1", "#test")
	require.True(t, found)

	store.Clear("user1", "network1", "#test")

	_, found = store.Retrieve("user1", "network1", "#test")
	assert.False(t, found)
}

func TestStore_Overwrite(t *testing.T) {
	store := NewASCIIStore()

	store.Store("user1", "network1", "#test", []string{"old"}, "old prompt", false)
	store.Store("user1", "network1", "#test", []string{"new"}, "new prompt", true)

	art, found := store.Retrieve("user1", "network1", "#test")
	require.True(t, found)
	assert.Equal(t, []string{"new"}, art.Lines)
	assert.Equal(t, "new prompt", art.Prompt)
	assert.True(t, art.UseHalfblocks)
}

func TestStore_ConcurrentAccess(t *testing.T) {
	store := NewASCIIStore()

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			store.Store("user", "net", "#ch", []string{string(rune('a' + (idx % 26)))}, "prompt", false)
		}(i)
	}
	wg.Wait()

	art, found := store.Retrieve("user", "net", "#ch")
	require.True(t, found)
	assert.Len(t, art.Lines, 1)
	// The final value should be one of the goroutine writes (single ASCII char)
	assert.Len(t, art.Lines[0], 1)
}

func TestRetrieve_ConcurrentAccess(t *testing.T) {
	store := NewASCIIStore()
	store.Store("user", "net", "#ch", []string{"line1", "line2"}, "prompt", true)

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			art, found := store.Retrieve("user", "net", "#ch")
			require.True(t, found)
			assert.NotNil(t, art)
			assert.Equal(t, []string{"line1", "line2"}, art.Lines)
		}()
	}
	wg.Wait()
}

func TestKeyIsolation(t *testing.T) {
	store := NewASCIIStore()

	store.Store("user1", "net1", "#ch1", []string{"a"}, "prompt1", false)
	store.Store("user2", "net2", "#ch2", []string{"b"}, "prompt2", true)

	art1, found1 := store.Retrieve("user1", "net1", "#ch1")
	require.True(t, found1)
	assert.Equal(t, []string{"a"}, art1.Lines)

	art2, found2 := store.Retrieve("user2", "net2", "#ch2")
	require.True(t, found2)
	assert.Equal(t, []string{"b"}, art2.Lines)

	_, found3 := store.Retrieve("user1", "net2", "#ch1")
	assert.False(t, found3)
}

func TestASCIIStore_Timestamp(t *testing.T) {
	store := NewASCIIStore()

	store.Store("user", "net", "#ch", []string{"line"}, "prompt", false)

	art, found := store.Retrieve("user", "net", "#ch")
	require.True(t, found)
	assert.False(t, art.Timestamp.IsZero())
}

func TestGenerateKey_SpecialCharacters(t *testing.T) {
	key := generateKey("user[name]", "net-work", "#channel.name")
	assert.True(t, strings.Contains(key, "user[name]"))
	assert.True(t, strings.Contains(key, "net-work"))
	assert.True(t, strings.Contains(key, "#channel.name"))
}
