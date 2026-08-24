package notes

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"tasker/internal/vault"
)

// Новое хранилище — это просто папка с файлами.
//
// Главное требование: приложение не заводит .git без спросу. Заметки при этом
// пишутся и читаются как обычно — история вторична, файлы первичны.
func TestNewVaultHasNoHistory(t *testing.T) {
	root := t.TempDir()
	s := openService(t, root, vault.OriginUser)
	ctx := context.Background()

	if s.GitEnabled() {
		t.Error("в новой папке история включена")
	}
	created, err := s.Create(ctx, CreateParams{Title: "Заметка"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(created.Path))); err != nil {
		t.Errorf("файла заметки нет: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".git")); !os.IsNotExist(err) {
		t.Errorf(".git завёлся сам: %v", err)
	}
	if s.Git() != nil {
		t.Error("Git() отдал репозиторий при выключенной истории")
	}
}

// Включение заводит репозиторий и начинает коммитить.
func TestEnablingHistoryStartsCommitting(t *testing.T) {
	root := t.TempDir()
	s := openService(t, root, vault.OriginUser)
	ctx := context.Background()

	// Заметка, заведённая до включения, в историю не попадёт — и это честно:
	// коммитить то, чего никто не просил записывать, задним числом не надо.
	if _, err := s.Create(ctx, CreateParams{Title: "До"}); err != nil {
		t.Fatal(err)
	}
	if err := s.SetGitEnabled(ctx, true); err != nil {
		t.Fatalf("SetGitEnabled: %v", err)
	}
	if !s.GitEnabled() {
		t.Fatal("история не включилась")
	}
	if _, err := os.Stat(filepath.Join(root, ".git")); err != nil {
		t.Fatalf(".git не завёлся: %v", err)
	}

	before := commits(t, root)
	if _, err := s.Create(ctx, CreateParams{Title: "После"}); err != nil {
		t.Fatal(err)
	}
	if added := commits(t, root) - before; added != 1 {
		t.Errorf("коммитов добавилось %d, ожидался один", added)
	}
}

// Выключение перестаёт писать, но ничего не удаляет.
//
// Это главное обещание тумблера: репозиторий с коммитами остаётся на диске, и
// включённый обратно, он продолжает ту же историю, а не начинает новую.
func TestDisablingHistoryKeepsRepository(t *testing.T) {
	s, root := testService(t, vault.OriginUser)
	ctx := context.Background()

	if _, err := s.Create(ctx, CreateParams{Title: "Первая"}); err != nil {
		t.Fatal(err)
	}
	before := commits(t, root)

	if err := s.SetGitEnabled(ctx, false); err != nil {
		t.Fatalf("SetGitEnabled: %v", err)
	}
	if s.GitEnabled() {
		t.Error("история не выключилась")
	}
	if _, err := os.Stat(filepath.Join(root, ".git")); err != nil {
		t.Fatalf("репозиторий удалён при выключении: %v", err)
	}

	if _, err := s.Create(ctx, CreateParams{Title: "Вторая"}); err != nil {
		t.Fatal(err)
	}
	if got := commits(t, root); got != before {
		t.Errorf("коммитов %d, было %d — выключенная история продолжает писать", got, before)
	}

	// Включаем обратно: прежние коммиты на месте, новые ложатся сверху.
	if err := s.SetGitEnabled(ctx, true); err != nil {
		t.Fatalf("SetGitEnabled: %v", err)
	}
	if _, err := s.Create(ctx, CreateParams{Title: "Третья"}); err != nil {
		t.Fatal(err)
	}
	if got := commits(t, root); got != before+1 {
		t.Errorf("коммитов %d, ожидалось %d — история не продолжилась", got, before+1)
	}
}

// Решение переживает перезапуск.
func TestHistoryChoiceSurvivesReopen(t *testing.T) {
	root := t.TempDir()
	ctx := context.Background()

	first := openService(t, root, vault.OriginUser)
	if err := first.SetGitEnabled(ctx, true); err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}

	second := openService(t, root, vault.OriginUser)
	if !second.GitEnabled() {
		t.Error("включённая история не пережила перезапуск")
	}
	if err := second.SetGitEnabled(ctx, false); err != nil {
		t.Fatal(err)
	}
	if err := second.Close(); err != nil {
		t.Fatal(err)
	}

	// Репозиторий на диске остался, но выключенным он и должен открыться:
	// записанное решение сильнее наличия .git.
	third := openService(t, root, vault.OriginUser)
	if third.GitEnabled() {
		t.Error("выключенная история включилась сама, потому что .git на месте")
	}
}

// Папка, в которой репозиторий уже есть, историю уже ведёт.
//
// Умолчание «без гита» относится к новым хранилищам. Молча перестать писать в
// существующий репозиторий значит на глазах у человека прекратить делать то,
// что он видел вчера.
func TestExistingRepositoryKeepsHistoryOn(t *testing.T) {
	root := t.TempDir()

	// Заводим репозиторий, как это сделал бы прошлый запуск приложения.
	first := openService(t, root, vault.OriginUser)
	if err := first.SetGitEnabled(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	// Убираем записанное решение: так выглядит хранилище, заведённое версией,
	// в которой тумблера ещё не было.
	if err := os.Remove(filepath.Join(root, configDirName, gitConfigFile)); err != nil {
		t.Fatal(err)
	}

	second := openService(t, root, vault.OriginUser)
	if !second.GitEnabled() {
		t.Error("в папке с готовым .git история выключена")
	}
}

// Испорченный файл не должен ни ронять запуск, ни заводить репозиторий молча.
func TestBrokenGitConfigFallsBackToRepoPresence(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, configDirName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, gitConfigFile), []byte("{не json"), 0o644); err != nil {
		t.Fatal(err)
	}

	s := openService(t, root, vault.OriginUser)
	if s.GitEnabled() {
		t.Error("испорченный файл включил историю в папке без .git")
	}
}
