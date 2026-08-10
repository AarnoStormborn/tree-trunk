package state

import (
	"context"
	"runtime"
	"testing"
	"time"

	"github.com/AarnoStormborn/tree-trunk/internal/model"
)

// TestNoGoroutineLeaks runs a full scan + refresh cycle and asserts the
// goroutine count returns to baseline (02-go-suitability §7.3: leak
// discipline; runs under -race in CI).
func TestNoGoroutineLeaks(t *testing.T) {
	if testing.Short() {
		t.Skip("leak check skipped in -short mode")
	}

	baseline := runtime.NumGoroutine()

	store := NewStore()
	runner := &fakeStatusRunner{porcelain: []byte("## main\x00"), fp: "refs"}
	rf := NewRefresher(runner, store, 4)
	for i := 0; i < 20; i++ {
		store.Upsert(&model.Repo{ID: "/r" + itoa(i), Name: "r" + itoa(i), Path: "/r" + itoa(i)})
	}

	rf.RefreshAll(context.Background())
	for _, r := range store.List() {
		rf.RefreshOne(context.Background(), r.ID)
		rf.LoadWorktrees(context.Background(), r.ID, true)
	}

	// Give any finalizers a moment, then assert we're back at baseline.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		runtime.GC()
		if runtime.NumGoroutine() <= baseline+2 {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("goroutines leaked: baseline=%d now=%d", baseline, runtime.NumGoroutine())
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
