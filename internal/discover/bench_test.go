package discover

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// BenchmarkScan measures the discovery walk over a synthetic tree.
func BenchmarkScan(b *testing.B) {
	root := b.TempDir()
	// Build a tree with 200 dirs, 50 of which are repos (with node_modules
	// and .git to exercise the skip + detection paths).
	for i := 0; i < 50; i++ {
		repo := filepath.Join(root, "repo"+itoa(i), "src")
		if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
			b.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(repo, "node_modules", "pkg"), 0o755); err != nil {
			b.Fatal(err)
		}
		for j := 0; j < 3; j++ {
			if err := os.MkdirAll(filepath.Join(root, "plain"+itoa(i), "dir"+itoa(j)), 0o755); err != nil {
				b.Fatal(err)
			}
		}
	}

	opts := Options{Roots: []string{root}, MaxDepth: 8, Ignore: DefaultIgnore, Hidden: HiddenPeek, HiddenPeekDepth: 2}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Scanner(context.Background(), opts, func(Hit) error { return nil })
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var tmp [20]byte
	i := len(tmp)
	for n > 0 {
		i--
		tmp[i] = byte('0' + n%10)
		n /= 10
	}
	return string(tmp[i:])
}
