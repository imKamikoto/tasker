package notes

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

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

// batchService открывает сервис с историей и включённой пачкой.
func batchService(t *testing.T, window time.Duration) *Service {
	t.Helper()
	service, _ := testService(t, vault.OriginUser)
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

// Папка без истории: окно настраивать нечего, коммитить некуда.
//
// Именно так открывается новое хранилище — и приложением, и tasker-mcp.
func TestWithoutHistoryWindowIsIgnored(t *testing.T) {
	root := t.TempDir()
	service := openService(t, root, vault.OriginAgent)

	service.SetCommitWindow(time.Hour)
	if got := service.CommitWindow(); got != 0 {
		t.Errorf("CommitWindow = %v, без истории ожидался ноль", got)
	}
	if err := service.FlushCommits(context.Background()); err != nil {
		t.Errorf("FlushCommits без истории вернул ошибку: %v", err)
	}

	if _, err := service.Create(context.Background(), CreateParams{Title: "Заметка"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, ".git")); !os.IsNotExist(err) {
		t.Errorf(".git завёлся сам: %v", err)
	}
}

// С включённой историей разовый процесс коммитит каждое сохранение сразу:
// пачку он не включает, копить ему нечего и некогда.
func TestAgentCommitsEverySaveWithHistory(t *testing.T) {
	service, root := testService(t, vault.OriginAgent)

	before := commits(t, root)
	for _, title := range []string{"Раз", "Два"} {
		if _, err := service.Create(context.Background(), CreateParams{Title: title}); err != nil {
			t.Fatal(err)
		}
	}
	if got := commits(t, root) - before; got != 2 {
		t.Errorf("коммитов %d, ожидалось 2", got)
	}
}
