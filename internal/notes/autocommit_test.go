package notes

import (
	"context"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"tasker/internal/gitstore"
	"tasker/internal/vault"
)

// commits считает коммиты в vault. Своя функция, а не commitCount из
// service_test.go: та считает строки лога и на пустой истории даёт единицу,
// а здесь важно поймать именно «коммитов пока ноль».
func commits(t *testing.T, root string) int {
	t.Helper()
	out, err := exec.Command("git", "-C", root, "rev-list", "--count", "HEAD").Output()
	if err != nil {
		// Пустая история — HEAD ещё не существует.
		return 0
	}
	n, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil {
		t.Fatalf("разбор %q: %v", out, err)
	}
	return n
}

// batchService открывает сервис с включённой пачкой.
func batchService(t *testing.T, window time.Duration) *Service {
	t.Helper()
	root := t.TempDir()

	// Автокоммиту нужен собственный store: сервис свой открывает сам, но
	// снаружи до него не добраться до открытия.
	store, err := gitstore.Open(root)
	if err != nil {
		t.Fatalf("gitstore.Open: %v", err)
	}
	auto := gitstore.NewAutocommit(store, window, func(err error) { t.Logf("автокоммит: %v", err) })

	service, err := Open(context.Background(), root, Options{
		Origin:     vault.OriginUser,
		Autocommit: auto,
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { service.Close() })
	service.SetCommitWindow(window)
	return service
}

func TestCommitWindowZeroCommitsEveryTime(t *testing.T) {
	service := batchService(t, time.Hour)
	// Ноль возвращает поведение по умолчанию: коммит на каждое сохранение.
	service.SetCommitWindow(0)
	ctx := context.Background()
	root := service.Vault().Root()

	before := commits(t, root)
	for _, title := range []string{"Раз", "Два", "Три"} {
		if _, err := service.Create(ctx, CreateParams{Title: title}); err != nil {
			t.Fatal(err)
		}
	}
	if got := commits(t, root) - before; got != 3 {
		t.Errorf("коммитов %d, ожидалось 3", got)
	}
	if service.CommitWindow() != 0 {
		t.Errorf("CommitWindow = %v, ожидался ноль", service.CommitWindow())
	}
}

func TestCommitWindowBatchesUntilFlush(t *testing.T) {
	// Окно заведомо больше теста: коммит должен случиться только по Flush.
	service := batchService(t, time.Hour)
	ctx := context.Background()
	root := service.Vault().Root()

	before := commits(t, root)
	for _, title := range []string{"Раз", "Два", "Три"} {
		if _, err := service.Create(ctx, CreateParams{Title: title}); err != nil {
			t.Fatal(err)
		}
	}
	if got := commits(t, root); got != before {
		t.Fatalf("коммитов %d, было %d: пачка не должна коммитить сама", got, before)
	}

	if err := service.FlushCommits(ctx); err != nil {
		t.Fatalf("FlushCommits: %v", err)
	}
	if got := commits(t, root) - before; got != 1 {
		t.Errorf("после сброса коммитов %d, ожидался ровно один на всю пачку", got)
	}
}

func TestBatchedFilesAreOnDiskBeforeCommit(t *testing.T) {
	service := batchService(t, time.Hour)
	ctx := context.Background()

	rec, err := service.Create(ctx, CreateParams{Title: "Не потеряется", Body: "тело"})
	if err != nil {
		t.Fatal(err)
	}
	// Главное свойство пачки: файл уже на диске, ждёт только история.
	// Иначе обещание из SPEC §10 перестало бы выполняться.
	got, err := service.Get(ctx, rec.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !strings.Contains(got.Body, "тело") {
		t.Errorf("тело = %q, ожидалось записанное на диск", got.Body)
	}
}

func TestCommitWindowReportsWhatWasSet(t *testing.T) {
	service := batchService(t, time.Hour)
	service.SetCommitWindow(45 * time.Second)
	if got := service.CommitWindow(); got != 45*time.Second {
		t.Errorf("CommitWindow = %v, ожидалось 45s", got)
	}
}

func TestServiceWithoutAutocommitIgnoresWindow(t *testing.T) {
	// tasker-mcp открывается именно так: разовый процесс, копить нечего.
	root := t.TempDir()
	service, err := Open(context.Background(), root, Options{Origin: vault.OriginAgent})
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()

	service.SetCommitWindow(time.Hour)
	if got := service.CommitWindow(); got != 0 {
		t.Errorf("CommitWindow = %v, без автокоммита ожидался ноль", got)
	}
	if err := service.FlushCommits(context.Background()); err != nil {
		t.Errorf("FlushCommits без автокоммита вернул ошибку: %v", err)
	}

	ctx := context.Background()
	before := commits(t, root)
	if _, err := service.Create(ctx, CreateParams{Title: "Заметка"}); err != nil {
		t.Fatal(err)
	}
	if got := commits(t, root) - before; got != 1 {
		t.Errorf("коммитов %d, ожидался 1", got)
	}
}
