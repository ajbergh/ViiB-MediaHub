package scanner

import (
	"fmt"
	"testing"
)

func BenchmarkCoalesceFileChanges(b *testing.B) {
	changes := make([]FileChange, 0, 10000)
	for i := 0; i < 5000; i++ {
		path := fmt.Sprintf("library/album-%04d/track.mp3", i)
		changes = append(changes,
			FileChange{Path: path, ChangeType: ChangeTypeCreated, NewMtime: 1, NewSize: 100},
			FileChange{Path: path, ChangeType: ChangeTypeModified, NewMtime: 2, NewSize: 101},
		)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		result := coalesceFileChanges(changes)
		if len(result) != 5000 { b.Fatalf("unexpected result size: %d", len(result)) }
	}
}
