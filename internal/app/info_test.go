package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"tasker/internal/notes"
	"tasker/internal/vault"
)

// infoService поднимает настоящий сервис поверх временного vault.
func infoService(t *testing.T) (*Info, *notes.Service) {
	t.Helper()
	root := t.TempDir()
	service, err := notes.Open(context.Background(), root, notes.Options{Origin: vault.OriginUser})
	if err != nil {
		t.Fatalf("open service: %v", err)
	}
	t.Cleanup(func() { service.Close() })

	home := t.TempDir()
	keymap, err := NewKeymap(home)
	if err != nil {
		t.Fatal(err)
	}
	vaults, err := NewVaults(home, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	return NewInfo(service, keymap, vaults), service
}

// agentService открывает тот же vault от лица агента: origin задаётся при
// открытии сервиса, а не у каждой заметки, потому что один процесс всегда
// пишет от одного лица.
func agentService(t *testing.T, of *notes.Service) *notes.Service {
	t.Helper()
	service, err := notes.Open(context.Background(), of.Vault().Root(), notes.Options{
		Origin: vault.OriginAgent,
	})
	if err != nil {
		t.Fatalf("open agent service: %v", err)
	}
	t.Cleanup(func() { service.Close() })
	return service
}

func TestStatsCountsWhatIsThere(t *testing.T) {
	info, service := infoService(t)
	ctx := context.Background()

	if _, err := service.Create(ctx, notes.CreateParams{Title: "Своя", Notebook: "Работа"}); err != nil {
		t.Fatal(err)
	}
	agent, err := agentService(t, service).Create(ctx, notes.CreateParams{Title: "От агента"})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.Trash(ctx, agent.ID); err != nil {
		t.Fatal(err)
	}

	got, err := info.Stats(ctx)
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if got.All != 1 {
		t.Errorf("All = %d, ожидалась 1: удалённая в счёт не идёт", got.All)
	}
	if got.Trashed != 1 {
		t.Errorf("Trashed = %d, ожидалась 1", got.Trashed)
	}
	// Агентская уехала в корзину — значит и в счётчике агента её быть не должно.
	if got.Agent != 0 {
		t.Errorf("Agent = %d, ожидался 0", got.Agent)
	}
	if got.IndexSize <= 0 {
		t.Errorf("IndexSize = %d, ожидался ненулевой размер файла", got.IndexSize)
	}
}

func TestStatsAgentLastEmptyUntilAgentWrites(t *testing.T) {
	info, service := infoService(t)
	ctx := context.Background()

	if _, err := service.Create(ctx, notes.CreateParams{Title: "Своя"}); err != nil {
		t.Fatal(err)
	}
	got, err := info.Stats(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !got.AgentLast.IsZero() {
		t.Errorf("AgentLast = %v, ожидалось нулевое время", got.AgentLast)
	}

	if _, err := agentService(t, service).Create(ctx, notes.CreateParams{Title: "От агента"}); err != nil {
		t.Fatal(err)
	}
	got, err = info.Stats(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got.AgentLast.IsZero() {
		t.Error("AgentLast остался нулевым после записи агента")
	}
}

func TestRebuildRestoresIndexFromFiles(t *testing.T) {
	info, service := infoService(t)
	ctx := context.Background()

	if _, err := service.Create(ctx, notes.CreateParams{Title: "Первая", Tags: []string{"баг"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Create(ctx, notes.CreateParams{Title: "Вторая"}); err != nil {
		t.Fatal(err)
	}

	before, err := info.Stats(ctx)
	if err != nil {
		t.Fatal(err)
	}

	after, err := info.Rebuild(ctx)
	if err != nil {
		t.Fatalf("Rebuild: %v", err)
	}
	if after.All != before.All {
		t.Errorf("после пересборки All = %d, было %d", after.All, before.All)
	}
	// Теги живут в отдельном файле и обязаны пережить пересборку.
	if after.Tags != before.Tags {
		t.Errorf("после пересборки тегов %d, было %d", after.Tags, before.Tags)
	}
}

func TestRebuildPicksUpFilesWrittenPastTheApp(t *testing.T) {
	info, service := infoService(t)
	ctx := context.Background()
	root := service.Vault().Root()

	// Файл, положенный мимо приложения: индекс о нём не знает.
	raw := "---\nid: 01K3QF8ZN7X2WPBV4YHMC6TDA9\ntitle: Снаружи\n---\n\nтело\n"
	if err := os.WriteFile(filepath.Join(root, "snaruzhi.md"), []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := info.Rebuild(ctx)
	if err != nil {
		t.Fatalf("Rebuild: %v", err)
	}
	if got.All != 1 {
		t.Errorf("All = %d, ожидалась 1: пересборка обязана увидеть файл с диска", got.All)
	}
}

func TestPathsPointAtRealPlaces(t *testing.T) {
	info, service := infoService(t)
	paths := info.Paths()

	if paths.Vault != service.Vault().Root() {
		t.Errorf("Vault = %q, ожидалось %q", paths.Vault, service.Vault().Root())
	}
	for name, path := range map[string]string{
		"Config": paths.Config,
		"Keymap": paths.Keymap,
		"Vaults": paths.Vaults,
		"Index":  paths.Index,
	} {
		if !filepath.IsAbs(path) {
			t.Errorf("%s = %q, ожидался абсолютный путь", name, path)
		}
	}
	// Индекс уже создан открытием сервиса — путь обязан вести в него.
	if _, err := os.Stat(paths.Index); err != nil {
		t.Errorf("индекса нет по пути %q: %v", paths.Index, err)
	}
}

func TestBuildReportsToolchain(t *testing.T) {
	info, _ := infoService(t)
	got := info.Build()

	if got.Go == "" {
		t.Error("версия Go пустая")
	}
	// Wails попадает в зависимости только у настоящей сборки; под go test
	// модуль тестируется отдельно, поэтому проверяем мягко.
	if got.Wails != "" && got.Wails[0] != 'v' {
		t.Errorf("версия Wails = %q, ожидалась начинающаяся с v", got.Wails)
	}
}
