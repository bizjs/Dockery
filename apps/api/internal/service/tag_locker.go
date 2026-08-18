package service

import (
	"sort"
	"sync"
)

type tagLockEntry struct {
	mu   sync.Mutex
	refs int
}

// tagLocker serializes check+put for the same repository/tag while allowing
// unrelated pushes to proceed concurrently.
type tagLocker struct {
	mu      sync.Mutex
	entries map[string]*tagLockEntry
}

func newTagLocker() *tagLocker {
	return &tagLocker{entries: make(map[string]*tagLockEntry)}
}

func (l *tagLocker) lockMany(keys []string) func() {
	keys = uniqueSorted(keys)
	entries := make([]*tagLockEntry, len(keys))

	l.mu.Lock()
	for i, key := range keys {
		entry := l.entries[key]
		if entry == nil {
			entry = &tagLockEntry{}
			l.entries[key] = entry
		}
		entry.refs++
		entries[i] = entry
	}
	l.mu.Unlock()

	for _, entry := range entries {
		entry.mu.Lock()
	}

	return func() {
		for i := len(entries) - 1; i >= 0; i-- {
			entries[i].mu.Unlock()
		}
		l.mu.Lock()
		defer l.mu.Unlock()
		for i, key := range keys {
			entries[i].refs--
			if entries[i].refs == 0 {
				delete(l.entries, key)
			}
		}
	}
}

func uniqueSorted(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}
