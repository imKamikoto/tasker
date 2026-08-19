package vault

import (
	"fmt"
	"os"
	"path/filepath"
)

// writeFileAtomic пишет файл так, чтобы на диске он никогда не оказался
// записанным наполовину: временный файл → fsync → rename.
//
// Временный файл создаётся рядом с целевым, а не в os.TempDir(): rename
// атомарен только в пределах одной файловой системы, а vault может лежать на
// другом томе. Имя начинается с точки, потому что скрытые файлы vault
// игнорирует (SPEC §4.1) — иначе watcher поднимет шум на собственную запись.
func writeFileAtomic(path string, data []byte, perm os.FileMode) (err error) {
	dir := filepath.Dir(path)

	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp*")
	if err != nil {
		return fmt.Errorf("create temp for %s: %w", path, err)
	}
	tmpName := tmp.Name()
	defer func() {
		if err != nil {
			tmp.Close()
			os.Remove(tmpName)
		}
	}()

	if _, err = tmp.Write(data); err != nil {
		return fmt.Errorf("write temp for %s: %w", path, err)
	}
	if err = tmp.Chmod(perm); err != nil {
		return fmt.Errorf("chmod temp for %s: %w", path, err)
	}
	// fsync до rename: иначе после внезапной перезагрузки на месте заметки
	// может оказаться файл нужного размера, набитый нулями.
	if err = tmp.Sync(); err != nil {
		return fmt.Errorf("fsync temp for %s: %w", path, err)
	}
	if err = tmp.Close(); err != nil {
		return fmt.Errorf("close temp for %s: %w", path, err)
	}
	if err = os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("rename temp to %s: %w", path, err)
	}

	// Сам rename тоже надо зафиксировать, иначе переживёт запись, но не запись
	// имени в каталоге.
	if err = syncDir(dir); err != nil {
		return fmt.Errorf("fsync dir of %s: %w", path, err)
	}
	return nil
}

func syncDir(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}
