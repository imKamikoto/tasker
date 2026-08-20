package index

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	"tasker/internal/vault"
)

// Причины, по которым заметка не попадает в индекс.
var (
	// ErrMissingID — в заметке лежит id, который не является ULID. Пустой id
	// скан дописывает сам, а этот трогать нельзя: неизвестно, чем он был.
	ErrMissingID = errors.New("note has no valid id")
	// ErrDuplicateID — тот же id встретился второй раз за скан. Обычно это
	// копия файла заметки.
	ErrDuplicateID = errors.New("duplicate note id")
)

// ScanResult — что сделал скан.
type ScanResult struct {
	Added     int
	Updated   int
	Removed   int
	Unchanged int

	// Backfilled — сколько заметок скан дописал на диске. Это единственное, чем
	// он меняет vault, и молчать об этом нельзя.
	Backfilled int

	// Failed — заметки, которые не удалось проиндексировать. Одна заметка со
	// сломанным заголовком не должна оставлять пользователя без индекса, поэтому
	// скан их собирает и идёт дальше.
	Failed []ScanError
}

// ScanError — заметка и причина, по которой она не попала в индекс.
type ScanError struct {
	Path string
	Err  error
}

func (e ScanError) Error() string { return fmt.Sprintf("%s: %v", e.Path, e.Err) }
func (e ScanError) Unwrap() error { return e.Err }

// Scan приводит индекс в соответствие с содержимым vault.
//
// Инкрементальность держится на паре (mtime, size): совпали — файл не
// перечитывается (SPEC §5.2). Полный скан — это тот же проход по пустому
// индексу, отдельного пути для него нет.
func (ix *Index) Scan(ctx context.Context, v *vault.Vault) (ScanResult, error) {
	var res ScanResult

	// Снимок индекса. По ходу обхода отсюда вычёркивается всё, что нашлось на
	// диске; остаток — то, чего больше нет.
	states, err := ix.States(ctx)
	if err != nil {
		return res, fmt.Errorf("scan vault: %w", err)
	}

	root := v.Root()
	seen := make(map[string]string, len(states))

	walkErr := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}

		name := d.Name()
		if d.IsDir() {
			// Скрытые каталоги vault игнорирует, кроме корзины (SPEC §4.1).
			if p != root && strings.HasPrefix(name, ".") && name != trashDir {
				return fs.SkipDir
			}
			return nil
		}
		if strings.HasPrefix(name, ".") || !strings.EqualFold(filepath.Ext(name), ".md") {
			return nil
		}

		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)

		info, err := d.Info()
		if err != nil {
			res.Failed = append(res.Failed, ScanError{Path: rel, Err: err})
			return nil
		}

		st, known := states[rel]
		delete(states, rel)

		if known && st.Size == info.Size() && st.ModTime.Equal(info.ModTime()) {
			res.Unchanged++
			seen[st.ID] = rel
			return nil
		}

		r, filled, err := readRecord(v, p)
		if err != nil {
			res.Failed = append(res.Failed, ScanError{Path: rel, Err: err})
			return nil
		}
		if filled {
			res.Backfilled++
		}
		if other, dup := seen[r.ID]; dup {
			res.Failed = append(res.Failed, ScanError{
				Path: rel,
				Err:  fmt.Errorf("%w: тот же id уже у %s", ErrDuplicateID, other),
			})
			return nil
		}
		if err := ix.Put(ctx, r); err != nil {
			res.Failed = append(res.Failed, ScanError{Path: rel, Err: err})
			return nil
		}

		seen[r.ID] = rel
		if known {
			res.Updated++
		} else {
			res.Added++
		}
		return nil
	})
	if walkErr != nil {
		return res, fmt.Errorf("scan vault %s: %w", root, walkErr)
	}

	for rel, st := range states {
		// Заметка могла не пропасть, а переехать: её id уже встретился под
		// другим путём, и строка в индексе теперь описывает новое место.
		// Удалять её означало бы оборвать ссылки на неё.
		if _, moved := seen[st.ID]; moved {
			continue
		}
		if err := ix.Delete(ctx, rel); err != nil {
			return res, fmt.Errorf("scan vault %s: %w", root, err)
		}
		res.Removed++
	}

	return res, nil
}

// readRecord читает заметку и готовит строку индекса.
//
// Файлу, попавшему в vault снаружи, по дороге дописывается frontmatter: скан и
// есть то самое «первое открытие» из SPEC §4.1, а без id заметку нельзя ни
// положить в индекс, ни сослаться на неё.
func readRecord(v *vault.Vault, abs string) (Record, bool, error) {
	n, err := v.Load(abs)
	if err != nil {
		return Record{}, false, err
	}
	filled, err := v.Backfill(n)
	if err != nil {
		return Record{}, false, err
	}
	r, err := RecordFrom(n)
	if err != nil {
		return Record{}, filled, err
	}
	// Backfill не трогает id, который уже есть, даже негодный: мы не знаем, чем
	// он был и кто на него ссылается. Такую заметку остаётся только показать.
	if !vault.ValidID(r.ID) {
		return Record{}, filled, ErrMissingID
	}
	return r, filled, nil
}
