package vault

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Rename приводит имя файла в соответствие с заголовком.
//
// Заголовок «Планы на осень» должен лежать в plany-na-osen.md, а не в имени,
// которое заметка получила при создании и с тех пор ни о чём не говорит.
// Транслит тот же, что и при создании: правила менять после появления файлов
// в vault нельзя, иначе одна и та же заметка называется по-разному до и после
// переименования.
//
// id не трогается, и ссылки между заметками не рвутся: они держатся на id, а
// не на пути (SPEC §8.9). Переезд файла индекс тоже переживает — там сверка
// идёт по id.
//
// updated не трогается: имя файла — не содержимое, и выдавать переименование
// за правку значит врать в списке, отсортированном по дате изменения.
func (v *Vault) Rename(n *Note) error {
	title, err := n.Doc.Meta.Title()
	if err != nil {
		return fmt.Errorf("rename %s: %w", n.Path, err)
	}

	slug := Slug(title)
	if slug == "" {
		// Заголовок из одних непереводимых символов — эмодзи, иероглифы.
		// Оставляем как есть: имя из пустой строки хуже устаревшего.
		return nil
	}
	if sameSlug(filepath.Base(n.Path), slug) {
		return nil
	}

	dir := filepath.Dir(n.Path)
	moved, err := moveUnique(n.Path, dir, slug)
	if err != nil {
		return fmt.Errorf("rename %s to %q: %w", n.Path, slug, err)
	}

	info, err := os.Stat(moved)
	if err != nil {
		return fmt.Errorf("rename %s to %q: %w", n.Path, slug, err)
	}

	// Про оба пути: и откуда файл исчез, и куда появился — событие watcher
	// придёт по каждому из них, и своя запись не должна вернуться в редактор.
	v.wrote(n.Path, n.ModTime)
	n.Path = moved
	n.ModTime = info.ModTime()
	n.Size = info.Size()
	v.wrote(n.Path, n.ModTime)
	return nil
}

// sameSlug отвечает, назван ли файл уже по этому слагу.
//
// Суффикс коллизии считается тем же именем: plany-2.md при заголовке «Планы» —
// это по-прежнему «Планы», и переименовывать его в plany-3.md было бы
// перекладыванием ради перекладывания.
func sameSlug(base, slug string) bool {
	name := strings.TrimSuffix(base, filepath.Ext(base))
	if name == slug {
		return true
	}
	rest, found := strings.CutPrefix(name, slug+"-")
	if !found || rest == "" {
		return false
	}
	// Хвост из одних цифр — это суффикс коллизии. «plan-2» при слаге «plan»
	// им и является; «plan-b» — уже другое имя.
	for _, r := range rest {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
