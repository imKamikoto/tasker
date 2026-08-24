// Package notes соединяет файлы, индекс и историю в операции уровня заметки.
//
// Слой существует потому, что «создать заметку» — это не один вызов: файл,
// строка индекса и коммит должны обновиться вместе, и ровно та же связка нужна
// и приложению, и tasker-mcp. Держать её в internal/app нельзя — там живёт
// Wails, а MCP-сервер обязан оставаться отдельным бинарником без него
// (docs/MCP.md §2).
//
// Пакет не импортирует Wails — см. CLAUDE.md, инвариант 4.
package notes

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"time"

	"tasker/internal/gitstore"
	"tasker/internal/index"
	"tasker/internal/vault"
)

// configDirName — служебный каталог vault (SPEC §4.1).
const configDirName = ".tasker"

// ErrConflictingParams — во входных данных взаимоисключающие поля.
var ErrConflictingParams = errors.New("conflicting parameters")

// Options — настройки сервиса.
type Options struct {
	// Origin помечает, кто вносит изменения. От него же зависит текст коммита
	// (SPEC §4.2, §4.5).
	Origin vault.Origin

	// Autocommit собирает правки в пачку вместо коммита на каждое сохранение.
	// Нулевое значение означает коммит сразу — так работает tasker-mcp: это
	// разовый процесс, копить ему нечего и некогда.
	//
	// Пачка включается отдельно, через SetCommitWindow: до того как интерфейс
	// прочитал настройки, безопаснее коммитить сразу.
	Autocommit *gitstore.Autocommit
}

// Service — операции над заметками уровня пользователя.
type Service struct {
	vault  *vault.Vault
	index  *index.Index
	git    *gitstore.Store
	lock   *vaultLock
	colors *colorStore
	origin vault.Origin

	auto *gitstore.Autocommit
	// batching читается из вызовов вебвью, которые приходят параллельно,
	// поэтому atomic, а не обычный bool под мьютексом сервиса.
	batching atomic.Bool
}

// Open открывает vault, индекс и историю, создавая недостающее.
func Open(ctx context.Context, root string, opts Options) (*Service, error) {
	v, err := vault.Open(root)
	if err != nil {
		return nil, err
	}

	dir := filepath.Join(v.Root(), configDirName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("open notes service: %w", err)
	}

	git, err := gitstore.Open(v.Root())
	if err != nil {
		return nil, err
	}
	ix, err := index.Open(ctx, filepath.Join(dir, "index.sqlite"))
	if err != nil {
		return nil, err
	}

	origin := opts.Origin
	if origin == "" {
		origin = vault.OriginUser
	}
	return &Service{
		vault:  v,
		index:  ix,
		git:    git,
		lock:   newVaultLock(dir),
		colors: newColorStore(dir),
		origin: origin,
		auto:   opts.Autocommit,
	}, nil
}

// SetCommitWindow задаёт, как часто правки уезжают в историю.
//
// Ноль — коммит на каждое сохранение, как было всегда. Больше нуля — правки
// собираются в пачку с таким окном. Терять при этом нечего: файл уже на диске,
// git здесь история, а не хранилище (SPEC §2).
func (s *Service) SetCommitWindow(d time.Duration) {
	if s.auto == nil {
		return
	}
	if d <= 0 {
		s.batching.Store(false)
		return
	}
	s.auto.SetDelay(d)
	s.batching.Store(true)
}

// CommitWindow возвращает текущее окно. Ноль — коммит сразу.
func (s *Service) CommitWindow() time.Duration {
	if s.auto == nil || !s.batching.Load() {
		return 0
	}
	return s.auto.Delay()
}

// FlushCommits немедленно фиксирует накопленное. Без пачки делать нечего:
// всё уже закоммичено.
func (s *Service) FlushCommits(ctx context.Context) error {
	if s.auto == nil {
		return nil
	}
	return s.auto.Flush(ctx)
}

func (s *Service) Close() error         { return s.index.Close() }
func (s *Service) Vault() *vault.Vault  { return s.vault }
func (s *Service) Index() *index.Index  { return s.index }
func (s *Service) Git() *gitstore.Store { return s.git }

// Sync приводит индекс в соответствие с содержимым vault.
func (s *Service) Sync(ctx context.Context) (index.ScanResult, error) {
	release, err := s.lock.acquire(ctx)
	if err != nil {
		return index.ScanResult{}, err
	}
	defer release()

	result, err := s.index.Scan(ctx, s.vault)
	if err != nil {
		return result, err
	}
	// Цвета живут в файле, а индекс мог быть только что пересобран с нуля.
	// Возвращаем их на место здесь, чтобы list_tags отвечал как раньше.
	if err := s.index.ApplyTagColors(ctx, s.colors.load()); err != nil {
		return result, err
	}
	return result, nil
}

// SetTagColor выбирает цвет тега из палитры или снимает выбор (AutoColor).
func (s *Service) SetTagColor(ctx context.Context, name string, color int) error {
	if err := s.colors.set(name, color); err != nil {
		return err
	}
	return s.index.ApplyTagColors(ctx, s.colors.load())
}

// TagColors возвращает выбранные вручную цвета.
func (s *Service) TagColors() map[string]int { return s.colors.load() }

// CreateParams — что нужно для новой заметки (docs/MCP.md §3).
type CreateParams struct {
	Title    string
	Body     string
	Notebook string
	Tags     []string
	Status   vault.Status
	Pinned   bool
	Context  *vault.Context

	// LinkTo — id заметки, с которой связать новую. Ссылки взаимные.
	LinkTo string
}

// Create заводит заметку: файл, строка индекса и коммит.
func (s *Service) Create(ctx context.Context, p CreateParams) (index.Record, error) {
	release, err := s.lock.acquire(ctx)
	if err != nil {
		return index.Record{}, err
	}
	defer release()

	// Цель проверяем до создания файла: иначе при неверном id на диске остаётся
	// заметка, которую никто не просил.
	var target index.Record
	if p.LinkTo != "" {
		target, err = s.index.Get(ctx, p.LinkTo)
		if err != nil {
			return index.Record{}, err
		}
	}

	n, err := s.vault.Create(vault.NewNote{
		Title:    p.Title,
		Body:     p.Body,
		Notebook: p.Notebook,
		Tags:     p.Tags,
		Status:   p.Status,
		Pinned:   p.Pinned,
		Origin:   s.origin,
		Context:  p.Context,
	})
	if err != nil {
		return index.Record{}, err
	}

	if p.LinkTo != "" {
		if err := s.linkBoth(ctx, n, target); err != nil {
			return index.Record{}, err
		}
	}

	rec, err := s.reindex(ctx, n)
	if err != nil {
		return index.Record{}, err
	}
	if err := s.commit(ctx, "create", rec.Title); err != nil {
		return index.Record{}, err
	}
	return rec, nil
}

// linkBoth дописывает в обе заметки ссылку друг на друга.
//
// Ссылка идёт отдельным абзацем в конце тела: формат ссылок задан спекой
// (`[Заголовок](tasker://note/<ULID>)`), а место — нет, и конец текста —
// единственное место, где дописывание не ломает уже написанное.
func (s *Service) linkBoth(ctx context.Context, n *vault.Note, target index.Record) error {
	title, err := n.Doc.Meta.Title()
	if err != nil {
		return err
	}

	n.Doc.Body = appendBlock(n.Doc.Body, noteLink(target.Title, target.ID))
	if err := s.vault.Save(n); err != nil {
		return err
	}

	other, err := s.loadByPath(target.Path)
	if err != nil {
		return err
	}
	other.Doc.Body = appendBlock(other.Doc.Body, noteLink(title, n.Doc.Meta.ID()))
	if err := s.vault.Save(other); err != nil {
		return err
	}
	if _, err := s.reindex(ctx, other); err != nil {
		return err
	}
	return nil
}

func noteLink(title, id string) string {
	return fmt.Sprintf("[%s](tasker://note/%s)", title, id)
}

// UpdateParams — правка заметки.
//
// Указатели, а не значения: иначе не отличить «не передано» от «передан ноль»,
// и любое обновление сбрасывало бы всё, о чём вызывающий не упомянул
// (docs/MCP.md §3).
type UpdateParams struct {
	ID         string
	Title      *string
	Body       *string
	Append     *string
	Prepend    *string
	Status     *vault.Status
	AddTags    []string
	RemoveTags []string
	Pinned     *bool
	Notebook   *string
}

// Update правит заметку по частям.
func (s *Service) Update(ctx context.Context, p UpdateParams) (index.Record, error) {
	if p.Body != nil && (p.Append != nil || p.Prepend != nil) {
		return index.Record{}, fmt.Errorf("update %s: body и append/prepend: %w", p.ID, ErrConflictingParams)
	}

	release, err := s.lock.acquire(ctx)
	if err != nil {
		return index.Record{}, err
	}
	defer release()

	rec, err := s.index.Get(ctx, p.ID)
	if err != nil {
		return index.Record{}, err
	}
	n, err := s.loadByPath(rec.Path)
	if err != nil {
		return index.Record{}, err
	}

	if err := applyUpdate(n, p); err != nil {
		return index.Record{}, err
	}
	if err := s.vault.Save(n); err != nil {
		return index.Record{}, err
	}
	if p.Notebook != nil {
		if err := s.vault.Move(n, *p.Notebook); err != nil {
			return index.Record{}, err
		}
	}

	updated, err := s.reindex(ctx, n)
	if err != nil {
		return index.Record{}, err
	}
	if err := s.commit(ctx, "update", updated.Title); err != nil {
		return index.Record{}, err
	}
	return updated, nil
}

// applyUpdate накладывает переданные поля на заметку.
func applyUpdate(n *vault.Note, p UpdateParams) error {
	meta := n.Doc.Meta

	if p.Title != nil {
		if err := meta.SetTitle(*p.Title); err != nil {
			return err
		}
	}
	if p.Status != nil {
		if err := meta.SetStatus(*p.Status); err != nil {
			return err
		}
	}
	if p.Pinned != nil {
		if err := meta.SetPinned(*p.Pinned); err != nil {
			return err
		}
	}
	if len(p.AddTags) > 0 || len(p.RemoveTags) > 0 {
		tags, err := meta.Tags()
		if err != nil {
			return err
		}
		for _, tag := range p.AddTags {
			if !slices.Contains(tags, tag) {
				tags = append(tags, tag)
			}
		}
		tags = slices.DeleteFunc(tags, func(tag string) bool {
			return slices.Contains(p.RemoveTags, tag)
		})
		if err := meta.SetTags(tags); err != nil {
			return err
		}
	}

	switch {
	case p.Body != nil:
		n.Doc.Body = *p.Body
	default:
		if p.Prepend != nil {
			n.Doc.Body = prependBlock(n.Doc.Body, *p.Prepend)
		}
		if p.Append != nil {
			n.Doc.Body = appendBlock(n.Doc.Body, *p.Append)
		}
	}
	return nil
}

// SetStatus меняет только статус.
//
// Отдельно от Update, потому что это самая частая точечная операция, и дёргать
// ради неё общий путь значит рисковать задеть что-то ещё (docs/MCP.md §3).
func (s *Service) SetStatus(ctx context.Context, id string, status vault.Status) (index.Record, error) {
	if _, err := vault.ParseStatus(string(status)); err != nil {
		return index.Record{}, err
	}
	return s.Update(ctx, UpdateParams{ID: id, Status: &status})
}

// Trash переносит заметку в корзину. Удалить насовсем отсюда нельзя.
func (s *Service) Trash(ctx context.Context, id string) error {
	release, err := s.lock.acquire(ctx)
	if err != nil {
		return err
	}
	defer release()

	rec, err := s.index.Get(ctx, id)
	if err != nil {
		return err
	}
	n, err := s.loadByPath(rec.Path)
	if err != nil {
		return err
	}
	if err := s.vault.Trash(n); err != nil {
		return err
	}
	if _, err := s.reindex(ctx, n); err != nil {
		return err
	}
	return s.commit(ctx, "trash", rec.Title)
}

// duplicateSuffix дописывается к заголовку копии.
//
// Заголовок — единственный источник имени файла, и без пометки копия сядет
// рядом с оригиналом как «note-2.md»: по списку их будет не различить.
const duplicateSuffix = " (копия)"

// Duplicate создаёт копию заметки рядом с оригиналом.
//
// Новый id, новое время создания, тот же ноутбук, теги и статус. Ссылки в теле
// копируются как есть: они ведут на те же заметки, что и в оригинале, и
// переписывать их значило бы решать за человека.
func (s *Service) Duplicate(ctx context.Context, id string) (index.Record, error) {
	rec, err := s.index.Get(ctx, id)
	if err != nil {
		return index.Record{}, err
	}
	n, err := s.loadByPath(rec.Path)
	if err != nil {
		return index.Record{}, err
	}

	status, err := n.Doc.Meta.Status()
	if err != nil {
		return index.Record{}, err
	}
	tags, err := n.Doc.Meta.Tags()
	if err != nil {
		return index.Record{}, err
	}
	context, err := n.Doc.Meta.Context()
	if err != nil {
		return index.Record{}, err
	}

	return s.Create(ctx, CreateParams{
		Title:    rec.Title + duplicateSuffix,
		Body:     n.Doc.Body,
		Notebook: rec.Notebook,
		Tags:     tags,
		Status:   status,
		Pinned:   rec.Pinned,
		Context:  context,
	})
}

// SetTags заменяет набор тегов заметки целиком.
//
// Не через AddTags и RemoveTags: поле под заголовком правится как одно целое,
// и вычислять разницу на стороне интерфейса значит поручать ему логику.
func (s *Service) SetTags(ctx context.Context, id string, tags []string) (index.Record, error) {
	release, err := s.lock.acquire(ctx)
	if err != nil {
		return index.Record{}, err
	}
	defer release()

	rec, err := s.index.Get(ctx, id)
	if err != nil {
		return index.Record{}, err
	}
	n, err := s.loadByPath(rec.Path)
	if err != nil {
		return index.Record{}, err
	}

	clean := make([]string, 0, len(tags))
	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		if tag != "" && !slices.ContainsFunc(clean, func(other string) bool {
			return strings.EqualFold(other, tag)
		}) {
			clean = append(clean, tag)
		}
	}
	if err := n.Doc.Meta.SetTags(clean); err != nil {
		return index.Record{}, err
	}
	if err := s.vault.Save(n); err != nil {
		return index.Record{}, err
	}

	updated, err := s.reindex(ctx, n)
	if err != nil {
		return index.Record{}, err
	}
	if err := s.commit(ctx, "tags", updated.Title); err != nil {
		return index.Record{}, err
	}
	return updated, nil
}

// SetPinned закрепляет заметку или снимает закрепление.
func (s *Service) SetPinned(ctx context.Context, id string, pinned bool) (index.Record, error) {
	return s.Update(ctx, UpdateParams{ID: id, Pinned: &pinned})
}

// Restore возвращает заметку из корзины туда, откуда она уехала.
func (s *Service) Restore(ctx context.Context, id string) (index.Record, error) {
	return s.fromTrash(ctx, id, "restore", func(n *vault.Note) error { return s.vault.Restore(n) })
}

// Delete удаляет заметку насовсем.
//
// Только из корзины и только отсюда: агенту это не дано вовсе (docs/MCP.md §4),
// а вернуть после этого можно лишь из истории git.
func (s *Service) Delete(ctx context.Context, id string) error {
	release, err := s.lock.acquire(ctx)
	if err != nil {
		return err
	}
	defer release()

	rec, err := s.index.Get(ctx, id)
	if err != nil {
		return err
	}
	n, err := s.loadByPath(rec.Path)
	if err != nil {
		return err
	}
	if err := s.vault.Delete(n); err != nil {
		return err
	}
	if err := s.index.Delete(ctx, rec.Path); err != nil {
		return err
	}
	return s.commit(ctx, "delete", rec.Title)
}

// fromTrash — общая обвязка для операций над заметкой в корзине.
func (s *Service) fromTrash(
	ctx context.Context, id, action string, apply func(*vault.Note) error,
) (index.Record, error) {
	release, err := s.lock.acquire(ctx)
	if err != nil {
		return index.Record{}, err
	}
	defer release()

	rec, err := s.index.Get(ctx, id)
	if err != nil {
		return index.Record{}, err
	}
	n, err := s.loadByPath(rec.Path)
	if err != nil {
		return index.Record{}, err
	}
	if err := apply(n); err != nil {
		return index.Record{}, err
	}

	// Путь заметки изменился, но убирать старую строку не нужно: Put находит
	// её по id и переносит на новый путь той же записью.
	updated, err := s.reindex(ctx, n)
	if err != nil {
		return index.Record{}, err
	}
	if err := s.commit(ctx, action, updated.Title); err != nil {
		return index.Record{}, err
	}
	return updated, nil
}

// applyMany выполняет одно и то же действие над несколькими заметками.
//
// Одна блокировка и один коммит на всю пачку, а не на каждую заметку: иначе
// перенос двадцати заметок даёт двадцать коммитов в истории и двадцать
// захватов блокировки, сквозь которые агент в это время не пролезет.
//
// Заметка, на которой действие сорвалось, не отменяет остальные: остановиться
// посередине значило бы оставить пачку в непонятном состоянии. Ошибки
// собираются и возвращаются вместе.
func (s *Service) applyMany(
	ctx context.Context, ids []string, action string, apply func(*vault.Note) error,
) ([]index.Record, error) {
	if len(ids) == 0 {
		return nil, nil
	}

	release, err := s.lock.acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer release()

	var (
		updated []index.Record
		titles  []string
		failed  []error
	)
	for _, id := range ids {
		rec, err := s.index.Get(ctx, id)
		if err != nil {
			failed = append(failed, err)
			continue
		}
		n, err := s.loadByPath(rec.Path)
		if err != nil {
			failed = append(failed, err)
			continue
		}
		if err := apply(n); err != nil {
			failed = append(failed, fmt.Errorf("%s %q: %w", action, rec.Title, err))
			continue
		}
		result, err := s.reindex(ctx, n)
		if err != nil {
			failed = append(failed, err)
			continue
		}
		updated = append(updated, result)
		titles = append(titles, result.Title)
	}

	if len(titles) > 0 {
		message := gitstore.NotesMessage(titles)
		if s.origin == vault.OriginAgent {
			message = gitstore.AgentMessage(action, fmt.Sprintf("%d заметок", len(titles)))
		}
		if _, err := s.git.Commit(ctx, message); err != nil {
			failed = append(failed, err)
		}
	}
	return updated, errors.Join(failed...)
}

// TrashMany переносит в корзину пачку заметок.
func (s *Service) TrashMany(ctx context.Context, ids []string) ([]index.Record, error) {
	return s.applyMany(ctx, ids, "trash", s.vault.Trash)
}

// MoveMany переносит пачку заметок в один ноутбук.
func (s *Service) MoveMany(ctx context.Context, ids []string, notebook string) ([]index.Record, error) {
	return s.applyMany(ctx, ids, "move", func(n *vault.Note) error {
		return s.vault.Move(n, notebook)
	})
}

// SetStatusMany проставляет один статус пачке заметок.
func (s *Service) SetStatusMany(ctx context.Context, ids []string, status vault.Status) ([]index.Record, error) {
	if _, err := vault.ParseStatus(string(status)); err != nil {
		return nil, err
	}
	return s.applyMany(ctx, ids, "status", func(n *vault.Note) error {
		if err := n.Doc.Meta.SetStatus(status); err != nil {
			return err
		}
		return s.vault.Save(n)
	})
}

// SetPinnedMany закрепляет или открепляет пачку заметок.
func (s *Service) SetPinnedMany(ctx context.Context, ids []string, pinned bool) ([]index.Record, error) {
	return s.applyMany(ctx, ids, "pin", func(n *vault.Note) error {
		if err := n.Doc.Meta.SetPinned(pinned); err != nil {
			return err
		}
		return s.vault.Save(n)
	})
}

// ErrEmptyTag — пустое имя тега.
var ErrEmptyTag = errors.New("empty tag")

// RenameTag переименовывает тег во всех заметках.
//
// Одной операцией и одним коммитом — это критерий приёмки из SPEC §8.2.
// Заметки находятся запросом к индексу, а не обходом всех файлов: тег с двумя
// заметками не должен стоить перечитывания vault целиком.
func (s *Service) RenameTag(ctx context.Context, from, to string) ([]index.Record, error) {
	from, to = strings.TrimSpace(from), strings.TrimSpace(to)
	if from == "" || to == "" {
		return nil, fmt.Errorf("rename tag %q: %w", from, ErrEmptyTag)
	}
	if from == to {
		return nil, nil
	}

	found, err := s.Search(ctx, "tag:"+quoteTerm(from), SearchOptions{})
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(found))
	for _, note := range found {
		ids = append(ids, note.ID)
	}

	return s.applyMany(ctx, ids, "rename tag", func(n *vault.Note) error {
		tags, err := n.Doc.Meta.Tags()
		if err != nil {
			return err
		}
		next := make([]string, 0, len(tags))
		for _, tag := range tags {
			// EqualFold, а не точное сравнение: поиск по тегу в SQLite
			// регистронезависим для латиницы, значит сюда доедет и заметка
			// с тегом BUG при переименовании bug.
			replacement := tag
			if strings.EqualFold(tag, from) {
				replacement = to
			}
			// Заметка могла нести и старый, и новый тег: после переименования
			// он должен остаться один, а не задвоиться.
			if !slices.Contains(next, replacement) {
				next = append(next, replacement)
			}
		}
		if err := n.Doc.Meta.SetTags(next); err != nil {
			return err
		}
		return s.vault.Save(n)
	})
}

// DeleteTag убирает тег из всех заметок одним коммитом.
//
// Отдельного списка тегов в vault нет: тег существует ровно постольку,
// поскольку стоит хотя бы в одной заметке (файлы — источник правды). Поэтому
// «удалить тег» — это снять его со всех, после чего он пропадает из сайдбара
// сам, а не остаётся строкой с нулевым счётчиком.
func (s *Service) DeleteTag(ctx context.Context, name string) ([]index.Record, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("delete tag %q: %w", name, ErrEmptyTag)
	}

	// Вместе с корзиной, в отличие от переименования: тег снимается насовсем,
	// и оставить его на удалённых заметках значит получить две неприятности —
	// он воскреснет в сайдбаре ближайшей сверкой (файлы источник правды) и
	// вернётся вместе с восстановленной заметкой.
	found, err := s.Search(ctx, "tag:"+quoteTerm(name), SearchOptions{Trash: index.TrashIncluded})
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(found))
	for _, note := range found {
		ids = append(ids, note.ID)
	}

	updated, err := s.applyMany(ctx, ids, "delete tag", func(n *vault.Note) error {
		tags, err := n.Doc.Meta.Tags()
		if err != nil {
			return err
		}
		// EqualFold по той же причине, что и в RenameTag: поиск по тегу в
		// SQLite регистронезависим для латиницы, значит по запросу bug сюда
		// доедет и заметка с тегом BUG.
		next := slices.DeleteFunc(slices.Clone(tags), func(tag string) bool {
			return strings.EqualFold(tag, name)
		})
		if err := n.Doc.Meta.SetTags(next); err != nil {
			return err
		}
		return s.vault.Save(n)
	})
	if err != nil {
		return nil, err
	}

	// Цвет выбирался руками и удаление тега пережил бы: заведённый заново тег
	// с тем же именем оказался бы прежнего цвета, которого для него никто не
	// выбирал.
	if err := s.SetTagColor(ctx, name, AutoColor); err != nil {
		return nil, err
	}
	// Файлы уже без тега — теперь его можно забыть и в индексе. Порядок
	// обязателен: сначала диск, потом производное от него (CLAUDE.md, инвариант 1).
	if err := s.index.ForgetTag(ctx, name); err != nil {
		return nil, err
	}
	return updated, nil
}

// Note — заметка целиком: строка индекса, тело и связи.
type Note struct {
	index.Record
	Body      string
	Links     []index.Record
	Backlinks []index.Record
}

// Get читает заметку вместе с телом и связями.
func (s *Service) Get(ctx context.Context, id string) (Note, error) {
	rec, err := s.index.Get(ctx, id)
	if err != nil {
		return Note{}, err
	}
	n, err := s.loadByPath(rec.Path)
	if err != nil {
		return Note{}, err
	}

	out := Note{Record: rec, Body: n.Doc.Body}
	for _, dst := range rec.Links {
		// Ссылка может вести в никуда: заметку удалили, а текст остался.
		if linked, err := s.index.Get(ctx, dst); err == nil {
			out.Links = append(out.Links, linked)
		}
	}
	if out.Backlinks, err = s.index.Backlinks(ctx, id); err != nil {
		return Note{}, err
	}
	return out, nil
}

// SearchOptions — что вернуть кроме самих записей.
type SearchOptions struct {
	Limit         int
	IncludeBody   bool
	Trash         index.Trash
	HideCompleted bool
	Sort          index.Sort
}

// Search выполняет запрос на языке из SPEC §8.5.
func (s *Service) Search(ctx context.Context, query string, opts SearchOptions) ([]Note, error) {
	q, err := index.ParseQuery(query)
	if err != nil {
		return nil, err
	}
	records, err := s.index.Search(ctx, q, index.SearchOptions{
		Limit:         opts.Limit,
		Trash:         opts.Trash,
		Sort:          opts.Sort,
		HideCompleted: opts.HideCompleted,
	})
	if err != nil {
		return nil, err
	}

	out := make([]Note, 0, len(records))
	for _, rec := range records {
		note := Note{Record: rec}
		if opts.IncludeBody {
			n, err := s.loadByPath(rec.Path)
			if err != nil {
				return nil, err
			}
			note.Body = n.Doc.Body
		}
		out = append(out, note)
	}
	return out, nil
}

// TasksParams — «что у меня в работе» (docs/MCP.md §3).
type TasksParams struct {
	Status   []vault.Status
	Notebook string
	Tag      string
	Limit    int
}

// Tasks — сахар над поиском с явной семантикой.
func (s *Service) Tasks(ctx context.Context, p TasksParams) ([]Note, error) {
	statuses := p.Status
	if len(statuses) == 0 {
		statuses = []vault.Status{vault.StatusActive, vault.StatusOnHold}
	}

	// Несколько статусов — это ИЛИ, а язык запросов соединяет условия через И.
	// Поэтому запрос строится по одному статусу за раз, а результаты сливаются.
	seen := make(map[string]struct{})
	var out []Note
	for _, status := range statuses {
		var parts []string
		parts = append(parts, "status:"+string(status))
		if p.Notebook != "" {
			parts = append(parts, "book:"+quoteTerm(p.Notebook))
		}
		if p.Tag != "" {
			parts = append(parts, "tag:"+quoteTerm(p.Tag))
		}

		found, err := s.Search(ctx, strings.Join(parts, " "), SearchOptions{})
		if err != nil {
			return nil, err
		}
		for _, note := range found {
			if _, dup := seen[note.ID]; dup {
				continue
			}
			seen[note.ID] = struct{}{}
			out = append(out, note)
		}
	}

	slices.SortFunc(out, func(a, b Note) int {
		if a.Pinned != b.Pinned {
			if a.Pinned {
				return -1
			}
			return 1
		}
		return b.Updated.Compare(a.Updated)
	})
	if p.Limit > 0 && len(out) > p.Limit {
		out = out[:p.Limit]
	}
	return out, nil
}

// quoteTerm берёт значение в кавычки, если в нём есть пробелы.
func quoteTerm(v string) string {
	if strings.ContainsAny(v, " \t") {
		return `"` + v + `"`
	}
	return v
}

// Tags отдаёт список тегов со счётчиками.
func (s *Service) Tags(ctx context.Context) ([]index.Tag, error) {
	return s.index.Tags(ctx)
}

// loadByPath читает заметку по пути относительно корня vault.
func (s *Service) loadByPath(rel string) (*vault.Note, error) {
	return s.vault.Load(filepath.FromSlash(rel))
}

// reindex перечитывает заметку в индекс.
func (s *Service) reindex(ctx context.Context, n *vault.Note) (index.Record, error) {
	rec, err := index.RecordFrom(n)
	if err != nil {
		return index.Record{}, err
	}
	if err := s.index.Put(ctx, rec); err != nil {
		return index.Record{}, err
	}
	return rec, nil
}

// commit фиксирует изменения. Текст сообщения зависит от того, кто их внёс
// (SPEC §4.5).
func (s *Service) commit(ctx context.Context, action, title string) error {
	// Пачка включена — правка только отмечается, а коммит сделает цикл.
	// Агент сюда не попадает: у него свой текст сообщения, который в общей
	// пачке собрать не из чего, да и живёт он один вызов.
	if s.auto != nil && s.batching.Load() && s.origin != vault.OriginAgent {
		s.auto.Touch(title)
		return nil
	}

	message := gitstore.NotesMessage([]string{title})
	if s.origin == vault.OriginAgent {
		message = gitstore.AgentMessage(action, title)
	}
	_, err := s.git.Commit(ctx, message)
	return err
}

// appendBlock и prependBlock дописывают абзац, отделяя его пустой строкой.
func appendBlock(body, block string) string {
	block = strings.TrimRight(block, "\n")
	if strings.TrimSpace(body) == "" {
		return block + "\n"
	}
	return strings.TrimRight(body, "\n") + "\n\n" + block + "\n"
}

func prependBlock(body, block string) string {
	block = strings.TrimRight(block, "\n")
	if strings.TrimSpace(body) == "" {
		return block + "\n"
	}
	return block + "\n\n" + strings.TrimLeft(body, "\n")
}
