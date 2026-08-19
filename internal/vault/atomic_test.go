package vault

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteFileAtomicCreates(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "note.md")

	if err := writeFileAtomic(path, []byte("содержимое\n"), 0o644); err != nil {
		t.Fatalf("writeFileAtomic: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != "содержимое\n" {
		t.Errorf("прочитано %q", got)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o644 {
		t.Errorf("права %o, ожидалось 644", perm)
	}
}

// После успешной записи временных файлов рядом остаться не должно: иначе
// watcher будет натыкаться на мусор, а vault обрастёт хвостами.
func TestWriteFileAtomicLeavesNoTempFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "note.md")

	for i := 0; i < 3; i++ {
		if err := writeFileAtomic(path, []byte("версия\n"), 0o644); err != nil {
			t.Fatalf("writeFileAtomic: %v", err)
		}
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "note.md" {
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Errorf("в каталоге %v, ожидался только note.md", names)
	}
}

func TestWriteFileAtomicOverwrites(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "note.md")

	if err := os.WriteFile(path, []byte("старое содержимое, длинное\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := writeFileAtomic(path, []byte("новое\n"), 0o644); err != nil {
		t.Fatalf("writeFileAtomic: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	// Хвост старого файла не должен просвечивать: rename заменяет файл целиком.
	if string(got) != "новое\n" {
		t.Errorf("прочитано %q", got)
	}
}

// Временный файл создаётся рядом с целевым, а не в os.TempDir(): rename
// атомарен только в пределах одной файловой системы.
func TestWriteFileAtomicTempIsSiblingAndHidden(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "note.md")

	seen := make(chan []string, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		var names []string
		for i := 0; i < 2000; i++ {
			entries, err := os.ReadDir(dir)
			if err != nil {
				continue
			}
			for _, e := range entries {
				if e.Name() != "note.md" {
					names = append(names, e.Name())
				}
			}
			if len(names) > 0 {
				break
			}
		}
		seen <- names
	}()

	if err := writeFileAtomic(path, []byte(strings.Repeat("данные\n", 5000)), 0o644); err != nil {
		t.Fatalf("writeFileAtomic: %v", err)
	}
	<-done
	names := <-seen

	// Гонку можно и не застать — это нормально, тест проверяет только то,
	// что если временный файл виден, он лежит рядом и начинается с точки.
	for _, n := range names {
		if !strings.HasPrefix(n, ".") {
			t.Errorf("временный файл %q не скрытый: watcher его увидит", n)
		}
	}
}

func TestWriteFileAtomicMissingDir(t *testing.T) {
	path := filepath.Join(t.TempDir(), "нет-такого-каталога", "note.md")
	err := writeFileAtomic(path, []byte("x"), 0o644)
	if err == nil {
		t.Fatal("ожидалась ошибка для несуществующего каталога")
	}
	if !strings.Contains(err.Error(), "note.md") {
		t.Errorf("в ошибке нет пути: %v", err)
	}
}
