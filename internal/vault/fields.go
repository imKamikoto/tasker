package vault

import (
	"fmt"
	"time"

	"github.com/goccy/go-yaml/ast"
)

// Имена полей заголовка из SPEC §4.2 и §4.3.
const (
	fieldID          = "id"
	fieldTitle       = "title"
	fieldCreated     = "created"
	fieldUpdated     = "updated"
	fieldStatus      = "status"
	fieldTags        = "tags"
	fieldPinned      = "pinned"
	fieldOrigin      = "origin"
	fieldContext     = "context"
	fieldTrashedFrom = "trashedFrom"
	fieldTrashedAt   = "trashedAt"
)

// Геттеры ниже устроены одинаково: отсутствие поля — не ошибка, возвращается
// нулевое значение. Ошибка означает, что поле есть, но в нём лежит не то —
// файл правится руками, и такое надо показывать, а не подставлять умолчание.

// ID — ULID заметки. Проставляется один раз при создании и дальше неизменен,
// поэтому сеттера нет.
func (f *Frontmatter) ID() string {
	var s string
	if _, err := f.Get(fieldID, &s); err != nil {
		return ""
	}
	return s
}

// Title — единственный источник заголовка. H1 в теле заголовком не считается.
func (f *Frontmatter) Title() (string, error) {
	var s string
	_, err := f.Get(fieldTitle, &s)
	return s, err
}

func (f *Frontmatter) SetTitle(title string) error {
	return f.Set(fieldTitle, title)
}

// Status: отсутствие поля эквивалентно StatusNone.
func (f *Frontmatter) Status() (Status, error) {
	var s string
	if _, err := f.Get(fieldStatus, &s); err != nil {
		return "", err
	}
	return ParseStatus(s)
}

func (f *Frontmatter) SetStatus(status Status) error {
	if _, err := ParseStatus(string(status)); err != nil {
		return err
	}
	return f.Set(fieldStatus, string(status))
}

func (f *Frontmatter) Tags() ([]string, error) {
	var tags []string
	_, err := f.Get(fieldTags, &tags)
	return tags, err
}

// SetTags: у нового ключа список пишется в строку — tags: [работа, баг], как в
// SPEC §4.2. У существующего стиль, который выбрал автор файла, сохраняется.
func (f *Frontmatter) SetTags(tags []string) error {
	fresh := f.find(fieldTags) == nil
	if err := f.Set(fieldTags, tags); err != nil {
		return err
	}
	if fresh {
		if seq, ok := f.find(fieldTags).Value.(*ast.SequenceNode); ok {
			seq.SetIsFlowStyle(true)
		}
	}
	return nil
}

func (f *Frontmatter) Pinned() (bool, error) {
	var pinned bool
	_, err := f.Get(fieldPinned, &pinned)
	return pinned, err
}

func (f *Frontmatter) SetPinned(pinned bool) error {
	return f.Set(fieldPinned, pinned)
}

// Origin: отсутствие поля эквивалентно OriginUser.
func (f *Frontmatter) Origin() (Origin, error) {
	var s string
	if _, err := f.Get(fieldOrigin, &s); err != nil {
		return "", err
	}
	return ParseOrigin(s)
}

func (f *Frontmatter) SetOrigin(origin Origin) error {
	if _, err := ParseOrigin(string(origin)); err != nil {
		return err
	}
	return f.Set(fieldOrigin, string(origin))
}

// Created и Updated — RFC 3339 с таймзоной. Отсутствие поля даёт нулевое время.
func (f *Frontmatter) Created() (time.Time, error) { return f.timeField(fieldCreated) }
func (f *Frontmatter) Updated() (time.Time, error) { return f.timeField(fieldUpdated) }

func (f *Frontmatter) SetCreated(t time.Time) error { return f.setTimeField(fieldCreated, t) }
func (f *Frontmatter) SetUpdated(t time.Time) error { return f.setTimeField(fieldUpdated, t) }

func (f *Frontmatter) timeField(key string) (time.Time, error) {
	var s string
	ok, err := f.Get(key, &s)
	if err != nil || !ok || s == "" {
		return time.Time{}, err
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse frontmatter key %s: %w", key, err)
	}
	return t, nil
}

func (f *Frontmatter) setTimeField(key string, t time.Time) error {
	return f.setScalarRaw(key, t.Format(time.RFC3339))
}

// Context возвращает nil, если поля нет: пустая структура и отсутствие блока —
// разные вещи, и агент должен их различать.
func (f *Frontmatter) Context() (*Context, error) {
	var ctx Context
	ok, err := f.Get(fieldContext, &ctx)
	if err != nil || !ok {
		return nil, err
	}
	return &ctx, nil
}

func (f *Frontmatter) SetContext(ctx Context) error {
	return f.Set(fieldContext, ctx)
}

// TrashedFrom — путь, с которого заметка уехала в корзину (SPEC §4.3).
func (f *Frontmatter) TrashedFrom() (string, error) {
	var s string
	_, err := f.Get(fieldTrashedFrom, &s)
	return s, err
}

func (f *Frontmatter) TrashedAt() (time.Time, error) { return f.timeField(fieldTrashedAt) }
