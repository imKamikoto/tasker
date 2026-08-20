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
}

// Service — операции над заметками уровня пользователя.
type Service struct {
	vault  *vault.Vault
	index  *index.Index
	git    *gitstore.Store
	lock   *vaultLock
	origin vault.Origin
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
	return &Service{vault: v, index: ix, git: git, lock: newVaultLock(dir), origin: origin}, nil
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
	return s.index.Scan(ctx, s.vault)
}

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

// Notebooks и Tags отдают дерево ноутбуков и список тегов.
func (s *Service) Notebooks(ctx context.Context) ([]index.Notebook, error) {
	return s.index.Notebooks(ctx)
}

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
