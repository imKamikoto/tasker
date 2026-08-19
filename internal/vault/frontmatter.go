package vault

import (
	"bytes"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/goccy/go-yaml"
	"github.com/goccy/go-yaml/ast"
	"github.com/goccy/go-yaml/parser"
)

// ErrInvalidFrontmatter — заголовок разобрался как YAML, но это не отображение
// ключ-значение, а, например, список или скаляр.
var ErrInvalidFrontmatter = errors.New("frontmatter is not a mapping")

// Frontmatter — YAML-заголовок заметки.
//
// Внутри лежит разобранный AST, а не структура и не map. Это принципиально:
// декодирование в map[string]any или в типизированную структуру с последующим
// кодированием обратно молча убивает чужие поля, порядок ключей, комментарии и
// стиль кавычек — то есть ровно то, что SPEC §4.2 требует сохранять.
//
// Пока ничего не меняли, Bytes отдаёт исходные байты без изменений; рендер из
// AST включается только после первой правки.
type Frontmatter struct {
	raw     []byte
	mapping *ast.MappingNode // nil, если заголовок пустой
	dirty   bool
}

// ParseFrontmatter разбирает сырой YAML заголовка. Пустой ввод — не ошибка.
func ParseFrontmatter(raw []byte) (*Frontmatter, error) {
	f := &Frontmatter{raw: slices.Clone(raw)}
	if len(bytes.TrimSpace(raw)) == 0 {
		return f, nil
	}

	file, err := parser.ParseBytes(raw, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("parse frontmatter: %w", err)
	}
	if len(file.Docs) == 0 || file.Docs[0].Body == nil {
		return f, nil
	}

	switch body := file.Docs[0].Body.(type) {
	case *ast.MappingNode:
		f.mapping = body
	case *ast.MappingValueNode:
		// Заголовок из одного ключа парсер отдаёт не отображением, а самой парой.
		f.mapping = ast.Mapping(body.GetToken(), false, body)
	default:
		return nil, fmt.Errorf("parse frontmatter: %w", ErrInvalidFrontmatter)
	}
	return f, nil
}

// Bytes отдаёт YAML заголовка с завершающим переводом строки.
// Пустой заголовок — пустой результат, без разделителей.
func (f *Frontmatter) Bytes() []byte {
	if !f.dirty {
		return slices.Clone(f.raw)
	}
	if f.mapping == nil || len(f.mapping.Values) == 0 {
		return nil
	}
	s := f.mapping.String()
	if !strings.HasSuffix(s, "\n") {
		s += "\n"
	}
	return []byte(s)
}

// Keys возвращает ключи в том порядке, в котором они лежат в файле.
func (f *Frontmatter) Keys() []string {
	if f.mapping == nil {
		return nil
	}
	keys := make([]string, 0, len(f.mapping.Values))
	for _, v := range f.mapping.Values {
		keys = append(keys, keyName(v))
	}
	return keys
}

// Get разбирает значение ключа в out. Ключа нет — false без ошибки.
func (f *Frontmatter) Get(key string, out any) (bool, error) {
	v := f.find(key)
	if v == nil {
		return false, nil
	}
	if err := yaml.NodeToValue(v.Value, out); err != nil {
		return true, fmt.Errorf("read frontmatter key %s: %w", key, err)
	}
	return true, nil
}

// Set записывает значение. Существующий ключ правится на месте, сохраняя
// позицию и хвостовой комментарий; новый дописывается в конец.
func (f *Frontmatter) Set(key string, value any) error {
	entry, err := newEntry(key, value)
	if err != nil {
		return fmt.Errorf("set frontmatter key %s: %w", key, err)
	}

	f.put(key, entry)
	return nil
}

// put кладёт готовую пару в отображение: существующий ключ правится на месте,
// новый дописывается в конец.
func (f *Frontmatter) put(key string, entry *ast.MappingValueNode) {
	if existing := f.find(key); existing != nil {
		// Стиль последовательности сохраняем: превращение tags: [a, b] в
		// блочный список переписало бы строку в git-диффе без причины.
		if old, ok := existing.Value.(*ast.SequenceNode); ok {
			if fresh, ok := entry.Value.(*ast.SequenceNode); ok {
				fresh.SetIsFlowStyle(old.IsFlowStyle)
			}
		}
		existing.Value = entry.Value
		f.dirty = true
		return
	}

	if f.mapping == nil {
		f.mapping = ast.Mapping(entry.GetToken(), false)
	}
	f.mapping.Values = append(f.mapping.Values, entry)
	f.dirty = true
}

// setScalarRaw записывает значение как есть, без кавычек.
//
// Кодировщик YAML берёт строку вида 2026-08-19T13:12:03+03:00 в кавычки, чтобы
// она гарантированно осталась строкой, — а SPEC §4.2 показывает её без кавычек,
// и лишние кавычки в каждой заметке шумят в git-диффах. Поэтому такие значения
// пишутся напрямую, но только после проверки набора символов: всё, что могло бы
// потребовать экранирования, уходит обычным путём через Set.
func (f *Frontmatter) setScalarRaw(key, raw string) error {
	if !isPlainScalar(raw) {
		return f.Set(key, raw)
	}
	entry, err := parseEntry(key + ": " + raw + "\n")
	if err != nil {
		return fmt.Errorf("set frontmatter key %s: %w", key, err)
	}
	f.put(key, entry)
	return nil
}

// isPlainScalar — значение, которое YAML прочитает как обычную строку без
// кавычек и без сюрпризов. Набор намеренно узкий: сюда попадают даты, ULID и
// хеши коммитов, а всё остальное пусть экранирует кодировщик.
func isPlainScalar(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9',
			r >= 'a' && r <= 'z',
			r >= 'A' && r <= 'Z',
			r == '-', r == '+', r == '.', r == ':':
		default:
			return false
		}
	}
	// Ведущий символ не должен превращать значение в что-то другое.
	return s[0] != '-' && s[0] != '+' && s[0] != '.' && s[0] != ':'
}

// Remove удаляет ключ. Возвращает false, если его и не было.
func (f *Frontmatter) Remove(key string) bool {
	if f.mapping == nil {
		return false
	}
	for i, v := range f.mapping.Values {
		if keyName(v) == key {
			f.mapping.Values = slices.Delete(f.mapping.Values, i, i+1)
			f.dirty = true
			return true
		}
	}
	return false
}

func (f *Frontmatter) find(key string) *ast.MappingValueNode {
	if f.mapping == nil {
		return nil
	}
	for _, v := range f.mapping.Values {
		if keyName(v) == key {
			return v
		}
	}
	return nil
}

// keyName берёт имя ключа из токена, а не из String(): у ключа в кавычках
// String() вернёт его вместе с кавычками.
func keyName(v *ast.MappingValueNode) string {
	if tk := v.Key.GetToken(); tk != nil {
		return tk.Value
	}
	return v.Key.String()
}

// newEntry собирает пару ключ-значение, прогоняя её через кодировщик YAML.
//
// Через Marshal, а не сборкой токенов руками: кодировщик сам решит, что надо
// заэкранировать, и сам расставит отступы во вложенных структурах. Заголовок с
// двоеточием внутри, начинающийся с дефиса или с решётки, — обычное дело, и
// ошибка здесь означает файл, который перестанет разбираться.
func newEntry(key string, value any) (*ast.MappingValueNode, error) {
	encoded, err := yaml.Marshal(map[string]any{key: value})
	if err != nil {
		return nil, fmt.Errorf("encode value: %w", err)
	}
	return parseEntry(string(encoded))
}

// parseEntry разбирает YAML из одной пары ключ-значение.
func parseEntry(src string) (*ast.MappingValueNode, error) {
	file, err := parser.ParseBytes([]byte(src), 0)
	if err != nil {
		return nil, fmt.Errorf("reparse encoded value: %w", err)
	}
	if len(file.Docs) == 0 || file.Docs[0].Body == nil {
		return nil, fmt.Errorf("reparse encoded value: %w", ErrInvalidFrontmatter)
	}
	switch body := file.Docs[0].Body.(type) {
	case *ast.MappingNode:
		return body.Values[0], nil
	case *ast.MappingValueNode:
		return body, nil
	default:
		return nil, fmt.Errorf("reparse encoded value: %w", ErrInvalidFrontmatter)
	}
}
