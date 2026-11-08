package main

import (
	"context"
	"os"
	"sync"
	"testing"
)

func newTestStore(t *testing.T) *pgStore {
	t.Helper()

	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("DATABASE_URL not set; skipping integration test")
	}

	if err := migrate(url); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	pool, err := openDB(context.Background(), url)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(pool.Close)

	return &pgStore{pool: pool}
}

func TestPgStoreAddVisit(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)

	before, err := store.Visits(ctx)
	if err != nil {
		t.Fatalf("visits: %v", err)
	}
	after, err := store.AddVisit(ctx)
	if err != nil {
		t.Fatalf("add visit: %v", err)
	}
	if after != before+1 {
		t.Errorf("after = %d, want %d", after, before+1)
	}
}

func TestPgStoreConcurrentAddVisit(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)

	before, err := store.Visits(ctx)
	if err != nil {
		t.Fatalf("visits: %v", err)
	}

	const n = 20
	var wg sync.WaitGroup
	for range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := store.AddVisit(ctx); err != nil {
				t.Errorf("add visit: %v", err)
			}
		}()
	}
	wg.Wait()

	after, err := store.Visits(ctx)
	if err != nil {
		t.Fatalf("visits: %v", err)
	}
	if after != before+n {
		t.Errorf("after = %d, want %d: visits were lost", after, before+n)
	}
}
