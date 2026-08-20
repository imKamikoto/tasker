package app

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func testSettings(t *testing.T) (*Settings, string) {
	t.Helper()
	dir := t.TempDir()
	return NewSettings(dir), filepath.Join(dir, settingsFile)
}

func TestSettingsRoundTrip(t *testing.T) {
	s, path := testSettings(t)

	if got, err := s.Load(); err != nil || got != "" {
		t.Fatalf("до первой записи: %q, %v", got, err)
	}

	const raw = `{"sidebarWidth":240,"sort":{"field":"title","reversed":true},"expanded":["Работа"]}`
	if err := s.Save(raw); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	if got != raw {
		t.Errorf("прочитано %q", got)
	}

	// Файл лежит там, где обещает SPEC §4.1.
	if _, err := os.Stat(path); err != nil {
		t.Errorf("файла нет: %v", err)
	}
}

// Испорченный файл не должен мешать запуску: интерфейс начнёт с умолчаний.
func TestSettingsIgnoresCorruptFile(t *testing.T) {
	s, path := testSettings(t)
	if err := os.WriteFile(path, []byte("{это не json"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := s.Load()
	if err != nil {
		t.Errorf("испорченный файл вернул ошибку: %v", err)
	}
	if got != "" {
		t.Errorf("прочитано %q, ожидалась пустота", got)
	}
	// И его можно перезаписать.
	if err := s.Save(`{"ok":true}`); err != nil {
		t.Fatalf("Save поверх испорченного: %v", err)
	}
}

func TestSettingsRejectsGarbage(t *testing.T) {
	s, _ := testSettings(t)

	if err := s.Save("не json"); err == nil {
		t.Error("не-JSON принят")
	}
	if err := s.Save(`{"x":"` + strings.Repeat("я", maxSettings) + `"}`); !errors.Is(err, ErrSettingsTooBig) {
		t.Errorf("ошибка = %v, ожидалась ErrSettingsTooBig", err)
	}
	// После отказа файла быть не должно.
	if got, _ := s.Load(); got != "" {
		t.Errorf("отвергнутое всё-таки записалось: %q", got)
	}
}

// Настройки пишутся из обработчиков интерфейса, которые приходят параллельно.
func TestSettingsConcurrent(t *testing.T) {
	s, _ := testSettings(t)

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if err := s.Save(`{"n":` + string(rune('0'+i%10)) + `}`); err != nil {
				t.Errorf("Save: %v", err)
			}
			if _, err := s.Load(); err != nil {
				t.Errorf("Load: %v", err)
			}
		}(i)
	}
	wg.Wait()

	got, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	if got == "" {
		t.Error("после гонки настроек не осталось")
	}
}
