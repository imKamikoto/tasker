package app

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func testKeymap(t *testing.T) (*Keymap, string) {
	t.Helper()
	home := t.TempDir()
	k, err := NewKeymap(home)
	if err != nil {
		t.Fatalf("NewKeymap: %v", err)
	}
	return k, filepath.Join(home, keymapDir, keymapFile)
}

// Первый запуск: файл появляется сам, чтобы человеку было что править.
func TestKeymapCreatesFileWithDefaults(t *testing.T) {
	k, path := testKeymap(t)

	loaded, err := k.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded[ContextGlobal]["cmd+n"] != "note.create" {
		t.Errorf("умолчания не те: %v", loaded[ContextGlobal])
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("файла нет: %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var stored map[string]map[string]string
	if err := json.Unmarshal(raw, &stored); err != nil {
		t.Fatalf("файл не разбирается: %v\n%s", err, raw)
	}
	if stored[ContextList]["j"] != "list.down" {
		t.Errorf("в файле не то:\n%s", raw)
	}
}

// Правка одного сочетания не должна отменять остальные.
func TestKeymapMergesWithDefaults(t *testing.T) {
	k, path := testKeymap(t)
	if err := os.WriteFile(path, []byte(`{"note-list":{"j":"list.up"}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	loaded, err := k.Load()
	if err != nil {
		t.Fatal(err)
	}
	if loaded[ContextList]["j"] != "list.up" {
		t.Errorf("правка не применилась: %v", loaded[ContextList])
	}
	if loaded[ContextList]["k"] != "list.up" {
		t.Errorf("умолчание потерялось: %v", loaded[ContextList])
	}
	if loaded[ContextGlobal]["cmd+n"] != "note.create" {
		t.Errorf("чужой контекст пострадал: %v", loaded[ContextGlobal])
	}
}

// Пустая строка снимает привязку: иначе отказаться от умолчания нечем.
func TestKeymapEmptyCommandUnbinds(t *testing.T) {
	k, path := testKeymap(t)
	if err := os.WriteFile(path, []byte(`{"note-list":{"p":""}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	loaded, err := k.Load()
	if err != nil {
		t.Fatal(err)
	}
	if _, bound := loaded[ContextList]["p"]; bound {
		t.Errorf("привязка осталась: %v", loaded[ContextList])
	}
	if loaded[ContextList]["j"] != "list.down" {
		t.Error("снятие одной привязки задело другие")
	}
}

// Свой контекст и своя команда должны доезжать как есть.
func TestKeymapAcceptsNewBindings(t *testing.T) {
	k, path := testKeymap(t)
	if err := os.WriteFile(path, []byte(`{"editor":{"cmd+shift+p":"note.pin"}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	loaded, err := k.Load()
	if err != nil {
		t.Fatal(err)
	}
	if loaded[ContextEditor]["cmd+shift+p"] != "note.pin" {
		t.Errorf("новая привязка потерялась: %v", loaded[ContextEditor])
	}
}

// Испорченный файл не должен оставлять приложение без клавиш.
func TestKeymapSurvivesCorruptFile(t *testing.T) {
	k, path := testKeymap(t)
	if err := os.WriteFile(path, []byte("{это не json"), 0o644); err != nil {
		t.Fatal(err)
	}

	loaded, err := k.Load()
	if err != nil {
		t.Errorf("испорченный файл вернул ошибку: %v", err)
	}
	if loaded[ContextGlobal]["cmd+n"] != "note.create" {
		t.Errorf("умолчания не подставились: %v", loaded[ContextGlobal])
	}
}

func TestKeymapSaveAndReset(t *testing.T) {
	k, _ := testKeymap(t)
	if _, err := k.Load(); err != nil {
		t.Fatal(err)
	}

	if err := k.Save(`{"note-list":{"g":"list.down"}}`); err != nil {
		t.Fatalf("Save: %v", err)
	}
	loaded, err := k.Load()
	if err != nil {
		t.Fatal(err)
	}
	if loaded[ContextList]["g"] != "list.down" {
		t.Errorf("сохранённое не прочиталось: %v", loaded[ContextList])
	}

	if err := k.Reset(); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	loaded, err = k.Load()
	if err != nil {
		t.Fatal(err)
	}
	if _, bound := loaded[ContextList]["g"]; bound {
		t.Error("после сброса осталась своя привязка")
	}
}

func TestKeymapRejectsGarbage(t *testing.T) {
	k, _ := testKeymap(t)
	if err := k.Save("не json"); err == nil {
		t.Error("не-JSON принят")
	}
	big := `{"global":{"a":"` + string(make([]byte, maxKeymap)) + `"}}`
	if err := k.Save(big); !errors.Is(err, ErrKeymapTooBig) {
		t.Errorf("ошибка = %v, ожидалась ErrKeymapTooBig", err)
	}
}

// Раскладка по умолчанию не должна отбирать у редактора его же клавиши.
func TestDefaultKeymapLeavesEditorAlone(t *testing.T) {
	defaults := defaultKeymap()

	// В глобальном контексте не место тому, что в тексте значит другое:
	// cmd+backspace — удаление до начала строки, cmd+d — мультикурсор.
	for _, forbidden := range []string{"cmd+backspace", "cmd+d", "j", "k", "p", "m", "enter"} {
		if command, bound := defaults[ContextGlobal][forbidden]; bound {
			t.Errorf("%q в глобальном контексте занято под %q — редактор его не получит", forbidden, command)
		}
	}

	// Контекст редактора пуст: там всё принадлежит CodeMirror и виму.
	if len(defaults[ContextEditor]) != 0 {
		t.Errorf("контекст редактора не пуст: %v", defaults[ContextEditor])
	}

	// А статусы работать должны откуда угодно.
	if defaults[ContextGlobal]["cmd+ctrl+4"] != "note.status.completed" {
		t.Errorf("статусы не глобальны: %v", defaults[ContextGlobal])
	}
}
