package gitstore

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func testAutocommit(t *testing.T) (*Autocommit, *Store, string, chan error) {
	t.Helper()
	s, root := testStore(t)
	errs := make(chan error, 4)
	a := NewAutocommit(s, 60*time.Millisecond, func(err error) {
		select {
		case errs <- err:
		default:
		}
	})
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go a.Run(ctx)
	return a, s, root, errs
}

// log терпим к отсутствию коммитов: его зовут в цикле ожидания, и пустая
// история — нормальное промежуточное состояние, а не повод падать.
func log(t *testing.T, root string) string {
	t.Helper()
	c := exec.Command("git", "-c", "core.quotepath=false", "log", "--format=%s")
	c.Dir = root
	out, err := c.CombinedOutput()
	if err != nil {
		return ""
	}
	return string(out)
}

func waitFor(t *testing.T, d time.Duration, cond func() bool) bool {
	t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(5 * time.Millisecond)
	}
	return cond()
}

func TestAutocommitCommitsAfterDelay(t *testing.T) {
	a, _, root, errs := testAutocommit(t)

	write(t, root, "заметка.md", "тело\n")
	a.Touch("Счётчик перерасчёта")

	if !waitFor(t, 2*time.Second, func() bool {
		return strings.Contains(log(t, root), "notes: Счётчик перерасчёта")
	}) {
		t.Errorf("коммита нет:\n%s", log(t, root))
	}
	select {
	case err := <-errs:
		t.Errorf("ошибка: %v", err)
	default:
	}
}

func TestAutocommitBatchesSeveralNotes(t *testing.T) {
	a, _, root, _ := testAutocommit(t)

	write(t, root, "первая.md", "тело\n")
	a.Touch("Первая")
	write(t, root, "вторая.md", "тело\n")
	a.Touch("Вторая")
	write(t, root, "третья.md", "тело\n")
	a.Touch("Третья")

	if !waitFor(t, 2*time.Second, func() bool {
		return strings.Contains(log(t, root), "notes: 3 изменено")
	}) {
		t.Errorf("правки не собрались в один коммит:\n%s", log(t, root))
	}
	if n := strings.Count(log(t, root), "notes:"); n != 1 {
		t.Errorf("коммитов %d, ожидался один:\n%s", n, log(t, root))
	}
}

// Одна и та же заметка, правленная несколько раз, — это одна заметка.
func TestAutocommitDeduplicatesTitles(t *testing.T) {
	a, _, root, _ := testAutocommit(t)

	write(t, root, "заметка.md", "раз\n")
	a.Touch("Счётчик")
	write(t, root, "заметка.md", "два\n")
	a.Touch("Счётчик")

	if !waitFor(t, 2*time.Second, func() bool {
		return strings.Contains(log(t, root), "notes: Счётчик")
	}) {
		t.Errorf("лог:\n%s", log(t, root))
	}
	if strings.Contains(log(t, root), "изменено") {
		t.Errorf("одна заметка посчитана как несколько:\n%s", log(t, root))
	}
}

// Таймер срабатывает и вхолостую: пустых коммитов быть не должно.
func TestAutocommitStaysQuietWithoutChanges(t *testing.T) {
	_, _, root, _ := testAutocommit(t)

	time.Sleep(200 * time.Millisecond)
	if out := log(t, root); strings.TrimSpace(out) != "" {
		t.Errorf("появились коммиты на пустом месте:\n%s", out)
	}
}

// Перед массовой операцией нужно зафиксировать текущее состояние немедленно,
// не дожидаясь таймера (SPEC §4.5).
func TestAutocommitFlushIsImmediate(t *testing.T) {
	a, _, root, _ := testAutocommit(t)

	write(t, root, "заметка.md", "тело\n")
	a.Touch("Счётчик")

	if err := a.Flush(context.Background()); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	if !strings.Contains(log(t, root), "notes: Счётчик") {
		t.Errorf("Flush не закоммитил сразу:\n%s", log(t, root))
	}
}

func TestAutocommitFlushWithoutChanges(t *testing.T) {
	a, _, _, _ := testAutocommit(t)
	if err := a.Flush(context.Background()); err != nil {
		t.Errorf("Flush на пустом месте: %v", err)
	}
}

// При выходе несохранённое обязано уехать в историю: это и есть обещание не
// потерять больше 90 секунд работы (SPEC §10).
func TestAutocommitFlushesOnShutdown(t *testing.T) {
	s, root := testStore(t)
	a := NewAutocommit(s, time.Hour, func(error) {})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { a.Run(ctx); close(done) }()

	write(t, root, "заметка.md", "тело\n")
	a.Touch("Последняя правка")
	time.Sleep(50 * time.Millisecond)

	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Run не завершился")
	}

	if !strings.Contains(log(t, root), "notes: Последняя правка") {
		t.Errorf("правка потеряна при выходе:\n%s", log(t, root))
	}
}

// Окно отсчитывается от первой правки, а не от последней. Иначе непрерывная
// работа откладывала бы коммит бесконечно, и обещание «не потерять больше 90
// секунд» перестало бы выполняться.
func TestAutocommitWindowDoesNotSlide(t *testing.T) {
	s, root := testStore(t)
	a := NewAutocommit(s, 150*time.Millisecond, func(error) {})
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go a.Run(ctx)

	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; ; i++ {
			select {
			case <-stop:
				return
			default:
			}
			os.WriteFile(filepath.Join(root, "заметка.md"),
				[]byte(strings.Repeat("x", i%40+1)+"\n"), 0o644)
			a.Touch("Счётчик")
			time.Sleep(20 * time.Millisecond)
		}
	}()
	defer func() { close(stop); <-done }()

	if !waitFor(t, 700*time.Millisecond, func() bool {
		return strings.Contains(log(t, root), "notes: Счётчик")
	}) {
		t.Error("за 700 мс непрерывной правки не случилось ни одного коммита")
	}
}

func TestAutocommitReportsErrors(t *testing.T) {
	a, _, root, errs := testAutocommit(t)

	write(t, root, "заметка.md", "тело\n")
	a.Touch("Счётчик")
	// Ломаем репозиторий так, чтобы коммит не прошёл.
	if err := os.RemoveAll(filepath.Join(root, ".git")); err != nil {
		t.Fatal(err)
	}

	select {
	case err := <-errs:
		if err == nil {
			t.Error("пустая ошибка")
		}
	case <-time.After(3 * time.Second):
		t.Error("о неудавшемся коммите не сообщили")
	}
}
