package vault

import (
	"strings"
	"sync"
	"testing"
)

func TestNewID(t *testing.T) {
	id := NewID()
	if len(id) != 26 {
		t.Errorf("длина %d, ожидалось 26: %q", len(id), id)
	}
	if id != strings.ToUpper(id) {
		t.Errorf("ULID должен быть в верхнем регистре: %q", id)
	}
	if !ValidID(id) {
		t.Errorf("свежий ULID не проходит собственную проверку: %q", id)
	}
}

// ULID монотонны по времени: имена и сортировка по id должны идти по возрастанию.
func TestNewIDIsOrdered(t *testing.T) {
	prev := NewID()
	for i := 0; i < 100; i++ {
		next := NewID()
		if next <= prev {
			t.Fatalf("ULID не возрастает: %q после %q", next, prev)
		}
		prev = next
	}
}

// Заметки создают одновременно приложение и tasker-mcp, поэтому генератор
// обязан быть потокобезопасным и не выдавать дублей.
func TestNewIDConcurrent(t *testing.T) {
	const workers, perWorker = 16, 200

	var mu sync.Mutex
	seen := make(map[string]struct{}, workers*perWorker)

	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ids := make([]string, 0, perWorker)
			for j := 0; j < perWorker; j++ {
				ids = append(ids, NewID())
			}
			mu.Lock()
			defer mu.Unlock()
			for _, id := range ids {
				if _, dup := seen[id]; dup {
					t.Errorf("дубль ULID: %q", id)
				}
				seen[id] = struct{}{}
			}
		}()
	}
	wg.Wait()

	if len(seen) != workers*perWorker {
		t.Errorf("уникальных %d, ожидалось %d", len(seen), workers*perWorker)
	}
}

func TestValidID(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"01K3QF8ZN7X2WPBV4YHMC6TDAE", true},
		{"", false},
		{"01K3QF8ZN7X2WPBV4YHMC6TDA", false},   // 25 символов
		{"01K3QF8ZN7X2WPBV4YHMC6TDAEE", false}, // 27
		{"01k3qf8zn7x2wpbv4yhmc6tdae", false},  // нижний регистр
		{"01K3QF8ZN7X2WPBV4YHMC6TDAI", false},  // I не входит в Crockford base32
		{"01K3QF8ZN7X2WPBV4YHMC6TDAU", false},  // U тоже
		{"не-ulid-вообще", false},
	}
	for _, c := range cases {
		if got := ValidID(c.in); got != c.want {
			t.Errorf("ValidID(%q) = %v, ожидалось %v", c.in, got, c.want)
		}
	}
}
