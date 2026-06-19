package storage

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// mockStore is a test double for ObjectStore.
type mockStore struct {
	mu      sync.Mutex
	objects map[string][]byte
}

func newMockStore() *mockStore {
	return &mockStore{objects: make(map[string][]byte)}
}

func (m *mockStore) ListObjects(ctx context.Context, prefix string) ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var keys []string
	for k := range m.objects {
		keys = append(keys, k)
	}
	return keys, nil
}

func (m *mockStore) GetObject(ctx context.Context, key string) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	data, ok := m.objects[key]
	if !ok {
		return nil, fmt.Errorf("not found: %s", key)
	}
	return data, nil
}

func (m *mockStore) PutObject(ctx context.Context, key string, data []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.objects[key] = make([]byte, len(data))
	copy(m.objects[key], data)
	return nil
}

func (m *mockStore) getObject(key string) ([]byte, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	data, ok := m.objects[key]
	return data, ok
}

func TestPull(t *testing.T) {
	store := newMockStore()
	store.objects["clients/a.md"] = []byte("---\ntype: Note\n---\nContent A")
	store.objects["runbooks/deploy.md"] = []byte("---\ntype: Runbook\n---\nDeploy steps")

	dir := t.TempDir()
	syncer := NewSyncer(store, dir)
	defer syncer.Close()

	ctx := context.Background()
	if err := syncer.Pull(ctx); err != nil {
		t.Fatal(err)
	}

	// Verify files were written
	data, err := os.ReadFile(filepath.Join(dir, "clients/a.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "---\ntype: Note\n---\nContent A" {
		t.Errorf("unexpected content: %s", data)
	}

	data, err = os.ReadFile(filepath.Join(dir, "runbooks/deploy.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "---\ntype: Runbook\n---\nDeploy steps" {
		t.Errorf("unexpected content: %s", data)
	}
}

func TestPullEmpty(t *testing.T) {
	store := newMockStore()
	dir := t.TempDir()
	syncer := NewSyncer(store, dir)
	defer syncer.Close()

	if err := syncer.Pull(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestPushFile(t *testing.T) {
	store := newMockStore()
	dir := t.TempDir()

	// Write a local file
	os.MkdirAll(filepath.Join(dir, "clients"), 0o755)
	os.WriteFile(filepath.Join(dir, "clients/test.md"), []byte("test content"), 0o644)

	syncer := NewSyncer(store, dir)
	syncer.PushFile("clients/test.md")

	// Wait for async push
	time.Sleep(100 * time.Millisecond)
	syncer.Close()

	data, ok := store.getObject("clients/test.md")
	if !ok {
		t.Fatal("file not pushed to store")
	}
	if string(data) != "test content" {
		t.Errorf("unexpected pushed content: %s", data)
	}
}

func TestPushMultipleFiles(t *testing.T) {
	store := newMockStore()
	dir := t.TempDir()

	os.MkdirAll(filepath.Join(dir, "notes"), 0o755)
	for i := 0; i < 5; i++ {
		path := fmt.Sprintf("notes/%d.md", i)
		os.WriteFile(filepath.Join(dir, path), []byte(fmt.Sprintf("note %d", i)), 0o644)
	}

	syncer := NewSyncer(store, dir)
	for i := 0; i < 5; i++ {
		syncer.PushFile(fmt.Sprintf("notes/%d.md", i))
	}

	time.Sleep(200 * time.Millisecond)
	syncer.Close()

	for i := 0; i < 5; i++ {
		key := fmt.Sprintf("notes/%d.md", i)
		data, ok := store.getObject(key)
		if !ok {
			t.Errorf("missing %s", key)
			continue
		}
		if string(data) != fmt.Sprintf("note %d", i) {
			t.Errorf("wrong content for %s: %s", key, data)
		}
	}
}

func TestPushMissingFile(t *testing.T) {
	store := newMockStore()
	dir := t.TempDir()
	syncer := NewSyncer(store, dir)

	// Should not panic, just log warning
	syncer.PushFile("nonexistent.md")

	time.Sleep(50 * time.Millisecond)
	syncer.Close()
}

func TestRoundTrip(t *testing.T) {
	store := newMockStore()

	// Simulate: push from one syncer, pull from another
	dir1 := t.TempDir()
	os.MkdirAll(filepath.Join(dir1, "clients"), 0o755)
	os.WriteFile(filepath.Join(dir1, "clients/a.md"), []byte("round trip data"), 0o644)

	syncer1 := NewSyncer(store, dir1)
	syncer1.PushFile("clients/a.md")
	time.Sleep(100 * time.Millisecond)
	syncer1.Close()

	dir2 := t.TempDir()
	syncer2 := NewSyncer(store, dir2)
	defer syncer2.Close()

	if err := syncer2.Pull(context.Background()); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(dir2, "clients/a.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "round trip data" {
		t.Errorf("round trip failed: %s", data)
	}
}
