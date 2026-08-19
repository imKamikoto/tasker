package vault

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
)

// maxNameAttempts ограничивает перебор суффиксов. Тысяча заметок с одинаковым
// заголовком в одном ноутбуке — это уже не коллизия, а что-то сломалось.
const maxNameAttempts = 1000

// ErrNameCollision — свободное имя файла не нашлось.
var ErrNameCollision = errors.New("no free file name")

// createUnique пишет data в первый свободный файл вида <slug>.md, <slug>-2.md,
// <slug>-3.md (SPEC §4.1) и возвращает выбранный путь.
//
// Имя занимается через os.Link, а не связкой «проверить, что файла нет, и
// создать»: приложение и tasker-mcp — разные процессы, и между проверкой и
// созданием второй успевает занять имя. Link возвращает ошибку, если цель уже
// существует, поэтому имя достаётся ровно одному, а чужой файл не может быть
// затёрт даже теоретически.
func createUnique(dir, slug string, data []byte, perm os.FileMode) (path string, err error) {
	tmp, err := os.CreateTemp(dir, "."+slug+".tmp*")
	if err != nil {
		return "", fmt.Errorf("create temp in %s: %w", dir, err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if _, err = tmp.Write(data); err != nil {
		tmp.Close()
		return "", fmt.Errorf("write temp in %s: %w", dir, err)
	}
	if err = tmp.Chmod(perm); err != nil {
		tmp.Close()
		return "", fmt.Errorf("chmod temp in %s: %w", dir, err)
	}
	if err = tmp.Sync(); err != nil {
		tmp.Close()
		return "", fmt.Errorf("fsync temp in %s: %w", dir, err)
	}
	if err = tmp.Close(); err != nil {
		return "", fmt.Errorf("close temp in %s: %w", dir, err)
	}

	path, err = linkFirstFree(dir, slug, tmpName, os.Link)
	if err != nil {
		return "", err
	}
	if err = syncDir(dir); err != nil {
		return "", fmt.Errorf("fsync dir %s: %w", dir, err)
	}
	return path, nil
}

// linkFirstFree перебирает имена, пока link не согласится создать ссылку.
//
// Занятое имя — единственная ошибка, которую можно пропустить и попробовать
// дальше. Всё остальное (кончилось место, ошибка ввода-вывода) отдаётся
// вызывающему как есть: превратить переполненный диск в «нет свободного имени»
// значит отправить человека искать проблему не там. Функция link передаётся
// параметром, чтобы это поведение можно было проверить тестом.
func linkFirstFree(dir, slug, src string, link func(oldname, newname string) error) (string, error) {
	for n := 1; n <= maxNameAttempts; n++ {
		candidate := filepath.Join(dir, noteFileName(slug, n))
		err := link(src, candidate)
		if err == nil {
			return candidate, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return "", fmt.Errorf("link %s: %w", candidate, err)
		}
	}
	return "", fmt.Errorf("create note %q in %s: %w", slug, dir, ErrNameCollision)
}

// noteFileName даёт имя файла для n-й попытки: первая без суффикса.
func noteFileName(slug string, n int) string {
	if n <= 1 {
		return slug + ".md"
	}
	return slug + "-" + strconv.Itoa(n) + ".md"
}
