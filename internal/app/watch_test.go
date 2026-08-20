package app

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"tasker/internal/notes"
	"tasker/internal/vault"
	"tasker/internal/watcher"
)

type captured struct {
	name string
	data any
}

// recorder собирает события вместо окна.
type recorder struct {
	mu     sync.Mutex
	events []captured
	errs   []error
}

func (r *recorder) emit(name string, data any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, captured{name, data})
}

func (r *recorder) fail(err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.errs = append(r.errs, err)
}

func (r *recorder) snapshot() []captured {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]captured(nil), r.events...)
}

func (r *recorder) waitFor(t *testing.T, name string, d time.Duration) captured {
	t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		for _, e := range r.snapshot() {
			if e.name == name {
				return e
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("событие %s не пришло за %v: %+v", name, d, r.snapshot())
	return captured{}
}

func testWatch(t *testing.T) (*notes.Service, *recorder, chan watcher.Batch, string) {
	t.Helper()
	root := t.TempDir()
	service, err := notes.Open(context.Background(), root, notes.Options{Origin: vault.OriginUser})
	if err != nil {
		t.Fatalf("notes.Open: %v", err)
	}
	t.Cleanup(func() { service.Close() })

	rec := &recorder{}
	batches := make(chan watcher.Batch, 4)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { NewWatch(service, rec.emit, rec.fail).Run(ctx, batches); close(done) }()
	t.Cleanup(func() { cancel(); <-done })

	return service, rec, batches, service.Vault().Root()
}

// Главный сценарий: чужой процесс положил файл, приложение узнало об этом само.
func TestWatchReportsNewNote(t *testing.T) {
	_, rec, batches, root := testWatch(t)

	path := filepath.Join(root, "снаружи.md")
	if err := os.WriteFile(path, []byte("---\nid: 01K3QF8ZN7X2WPBV4YHMC6TDAE\ntitle: Извне\n---\nтело\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	batches <- watcher.Batch{Paths: []string{path}}

	rec.waitFor(t, EventNotesChanged, 3*time.Second)

	note := rec.waitFor(t, EventNoteChanged, 3*time.Second)
	single, ok := note.data.(NoteChanged)
	if !ok || single.ID != "01K3QF8ZN7X2WPBV4YHMC6TDAE" || single.Path != "снаружи.md" {
		t.Errorf("нагрузка = %+v", note.data)
	}
}

// Пакет, за которым ничего не стоит, не должен дёргать интерфейс.
func TestWatchStaysQuietWhenNothingChanged(t *testing.T) {
	service, rec, batches, _ := testWatch(t)
	if _, err := service.Create(context.Background(), notes.CreateParams{Title: "Заметка"}); err != nil {
		t.Fatal(err)
	}

	batches <- watcher.Batch{Full: true}
	time.Sleep(400 * time.Millisecond)

	for _, e := range rec.snapshot() {
		if e.name == EventNotesChanged {
			t.Errorf("событие пришло на пустом месте: %+v", e.data)
		}
	}
}

// Удалённый файл: список обновить надо, а отдельной заметки уже нет.
func TestWatchOnRemovedFile(t *testing.T) {
	service, rec, batches, root := testWatch(t)
	created, err := service.Create(context.Background(), notes.CreateParams{Title: "Ненужная"})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, filepath.FromSlash(created.Path))
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}

	batches <- watcher.Batch{Paths: []string{path}}
	rec.waitFor(t, EventNotesChanged, 3*time.Second)
	for _, e := range rec.snapshot() {
		if e.name == EventNoteChanged {
			t.Errorf("событие про отдельную заметку для удалённого файла: %+v", e.data)
		}
	}
}

// Битая заметка не молчит: о ней сообщают вызывающему, а остальное работает.
func TestWatchReportsUnreadableNotes(t *testing.T) {
	_, rec, batches, root := testWatch(t)

	path := filepath.Join(root, "битая.md")
	if err := os.WriteFile(path, []byte("---\ntitle: A\n  сдвиг: сломано\n\tтаб: нельзя\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	batches <- watcher.Batch{Paths: []string{path}}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		rec.mu.Lock()
		n := len(rec.errs)
		rec.mu.Unlock()
		if n > 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Error("о нечитаемой заметке не сообщили")
}

// Вся цепочка целиком, с настоящим watcher'ом: чужой процесс пишет файл →
// watcher замечает → индекс обновляется → событие уходит в окно. Это шаг 5
// сценария приёмки из docs/MCP.md §6, самый важный в нём.
func TestWatchEndToEnd(t *testing.T) {
	root := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	service, err := notes.Open(ctx, root, notes.Options{Origin: vault.OriginUser})
	if err != nil {
		t.Fatalf("notes.Open: %v", err)
	}
	t.Cleanup(func() { service.Close() })
	if _, err := service.Sync(ctx); err != nil {
		t.Fatal(err)
	}

	files, err := watcher.Start(ctx, service.Vault().Root(), watcher.Options{
		Debounce: 40 * time.Millisecond,
		Sweep:    time.Hour,
		OnError:  func(error) {},
	})
	if err != nil {
		t.Fatalf("watcher.Start: %v", err)
	}
	service.Vault().OnWrite(files.Ignore)

	rec := &recorder{}
	done := make(chan struct{})
	go func() { NewWatch(service, rec.emit, rec.fail).Run(ctx, files.Events()); close(done) }()
	t.Cleanup(func() { cancel(); <-done })
	time.Sleep(80 * time.Millisecond)

	// Так это выглядит со стороны tasker-mcp: отдельный процесс кладёт файл.
	outside, err := notes.Open(ctx, root, notes.Options{Origin: vault.OriginAgent})
	if err != nil {
		t.Fatal(err)
	}
	defer outside.Close()
	created, err := outside.Create(ctx, notes.CreateParams{
		Title: "Заведено агентом", Notebook: "Работа/Баги", Body: "описание\n",
	})
	if err != nil {
		t.Fatalf("Create из другого процесса: %v", err)
	}

	rec.waitFor(t, EventNotesChanged, 5*time.Second)
	note := rec.waitFor(t, EventNoteChanged, 5*time.Second)
	if single, ok := note.data.(NoteChanged); !ok || single.ID != created.ID {
		t.Errorf("нагрузка = %+v, ожидался id %s", note.data, created.ID)
	}
}

// Собственная запись наружу не выходит: иначе редактор перечитывал бы свой же
// буфер после каждого сохранения (SPEC §5.3).
func TestWatchIgnoresOwnWrites(t *testing.T) {
	root := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	service, err := notes.Open(ctx, root, notes.Options{Origin: vault.OriginUser})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { service.Close() })
	created, err := service.Create(ctx, notes.CreateParams{Title: "Своя", Body: "тело\n"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Sync(ctx); err != nil {
		t.Fatal(err)
	}

	files, err := watcher.Start(ctx, service.Vault().Root(), watcher.Options{
		Debounce: 40 * time.Millisecond,
		Sweep:    time.Hour,
		OnError:  func(error) {},
	})
	if err != nil {
		t.Fatal(err)
	}
	service.Vault().OnWrite(files.Ignore)

	rec := &recorder{}
	done := make(chan struct{})
	go func() { NewWatch(service, rec.emit, rec.fail).Run(ctx, files.Events()); close(done) }()
	t.Cleanup(func() { cancel(); <-done })
	time.Sleep(80 * time.Millisecond)

	body := "правка из редактора\n"
	if _, err := service.Update(ctx, notes.UpdateParams{ID: created.ID, Body: &body}); err != nil {
		t.Fatal(err)
	}

	time.Sleep(700 * time.Millisecond)
	for _, e := range rec.snapshot() {
		if e.name == EventNoteChanged {
			t.Errorf("своя запись вышла наружу как событие: %+v", e.data)
		}
	}
}
