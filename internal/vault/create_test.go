package vault

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
)

func TestCreateUniqueFirstName(t *testing.T) {
	dir := t.TempDir()

	path, err := createUnique(dir, "zametka", []byte("содержимое\n"), 0o644)
	if err != nil {
		t.Fatalf("createUnique: %v", err)
	}
	if filepath.Base(path) != "zametka.md" {
		t.Errorf("имя %q, ожидалось zametka.md", filepath.Base(path))
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "содержимое\n" {
		t.Errorf("содержимое %q", got)
	}
}

// SPEC §4.1: коллизии разрешаются суффиксом -2, -3.
func TestCreateUniqueCollisions(t *testing.T) {
	dir := t.TempDir()

	want := []string{"zametka.md", "zametka-2.md", "zametka-3.md", "zametka-4.md"}
	for i, w := range want {
		path, err := createUnique(dir, "zametka", []byte("n\n"), 0o644)
		if err != nil {
			t.Fatalf("создание %d: %v", i, err)
		}
		if got := filepath.Base(path); got != w {
			t.Errorf("создание %d: имя %q, ожидалось %q", i, got, w)
		}
	}
}

// Занятый номер посередине не должен ломать нумерацию.
func TestCreateUniqueSkipsExisting(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"zametka.md", "zametka-2.md", "zametka-4.md"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	path, err := createUnique(dir, "zametka", []byte("n\n"), 0o644)
	if err != nil {
		t.Fatalf("createUnique: %v", err)
	}
	if got := filepath.Base(path); got != "zametka-3.md" {
		t.Errorf("имя %q, ожидалось zametka-3.md", got)
	}
}

// Существующий файл не должен быть затёрт ни при каких обстоятельствах.
func TestCreateUniqueNeverOverwrites(t *testing.T) {
	dir := t.TempDir()
	existing := filepath.Join(dir, "zametka.md")
	if err := os.WriteFile(existing, []byte("ЧУЖОЕ СОДЕРЖИМОЕ\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := createUnique(dir, "zametka", []byte("новое\n"), 0o644); err != nil {
		t.Fatalf("createUnique: %v", err)
	}

	got, err := os.ReadFile(existing)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "ЧУЖОЕ СОДЕРЖИМОЕ\n" {
		t.Fatalf("существующий файл затёрт: %q", got)
	}
}

// Приложение и tasker-mcp пишут в один vault одновременно. Имя должно
// доставаться ровно одному процессу, иначе заметки затрут друг друга.
func TestCreateUniqueConcurrent(t *testing.T) {
	dir := t.TempDir()
	const n = 24

	var wg sync.WaitGroup
	paths := make([]string, n)
	errs := make([]error, n)
	start := make(chan struct{})

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			paths[i], errs[i] = createUnique(dir, "zametka", []byte("n\n"), 0o644)
		}(i)
	}
	close(start)
	wg.Wait()

	seen := map[string]int{}
	for i, err := range errs {
		if err != nil {
			t.Fatalf("горутина %d: %v", i, err)
		}
		seen[paths[i]]++
	}
	for path, count := range seen {
		if count > 1 {
			t.Errorf("имя %q досталось %d раз", path, count)
		}
	}
	if len(seen) != n {
		t.Errorf("уникальных имён %d, ожидалось %d", len(seen), n)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)
	if len(names) != n {
		t.Errorf("файлов в каталоге %d (%v), ожидалось %d", len(names), names, n)
	}
	for _, name := range names {
		if strings.HasPrefix(name, ".") {
			t.Errorf("остался временный файл %q", name)
		}
	}
}

func TestCreateUniqueMissingDir(t *testing.T) {
	_, err := createUnique(filepath.Join(t.TempDir(), "нет"), "zametka", []byte("x"), 0o644)
	if err == nil {
		t.Fatal("ожидалась ошибка для несуществующего каталога")
	}
}

func TestCreateUniqueTooManyCollisions(t *testing.T) {
	dir := t.TempDir()
	for i := 1; i <= maxNameAttempts; i++ {
		name := noteFileName("zametka", i)
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := createUnique(dir, "zametka", []byte("x"), 0o644); err == nil {
		t.Fatal("ожидалась ошибка после исчерпания попыток")
	}
}

// Ошибка, не связанная с занятым именем, обязана дойти до вызывающего, а не
// превратиться в «нет свободного имени» после тысячи бессмысленных попыток.
func TestLinkFirstFreeSurfacesRealErrors(t *testing.T) {
	diskFull := errors.New("на устройстве не осталось места")

	calls := 0
	_, err := linkFirstFree(t.TempDir(), "zametka", "/tmp/src", func(_, _ string) error {
		calls++
		return diskFull
	})

	if !errors.Is(err, diskFull) {
		t.Errorf("ошибка = %v, ожидалась обёртка вокруг %v", err, diskFull)
	}
	if errors.Is(err, ErrNameCollision) {
		t.Error("настоящая ошибка подменена на ErrNameCollision")
	}
	if calls != 1 {
		t.Errorf("попыток %d, ожидалась одна: перебирать имена после такой ошибки бессмысленно", calls)
	}
}

// Занятое имя, наоборот, пропускается — и перебор идёт дальше.
func TestLinkFirstFreeSkipsTakenNames(t *testing.T) {
	taken := map[string]bool{"zametka.md": true, "zametka-2.md": true}

	path, err := linkFirstFree(t.TempDir(), "zametka", "/tmp/src", func(_, newname string) error {
		if taken[filepath.Base(newname)] {
			return &os.LinkError{Op: "link", New: newname, Err: os.ErrExist}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("linkFirstFree: %v", err)
	}
	if got := filepath.Base(path); got != "zametka-3.md" {
		t.Errorf("имя %q, ожидалось zametka-3.md", got)
	}
}
