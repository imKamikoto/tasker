package vault

import (
	"bytes"
	"fmt"
	"slices"
)

// Document — содержимое файла заметки: YAML-заголовок и тело markdown.
//
// Документ, который не меняли, записывается обратно байт в байт — включая
// переводы строк, отступы и всё, чего мы про этот файл не знаем. На этой
// гарантии держится инвариант «ничего не теряем» из CLAUDE.md.
type Document struct {
	// Meta — заголовок заметки. Не nil даже для файла без frontmatter:
	// в него можно писать, и тогда заголовок появится (SPEC §4.1).
	Meta *Frontmatter

	// Body — тело markdown как есть, вместе с завершающими переводами строк.
	Body string

	raw      []byte
	origBody string
	hadMeta  bool
}

// ParseDocument разбирает содержимое файла заметки.
//
// Файл без frontmatter — не ошибка: любой .md, положенный в vault снаружи,
// подхватывается, а заголовок достраивается при первом открытии. А вот
// frontmatter, который не разбирается как YAML, ошибка: молча превратить его в
// тело значит потерять данные пользователя.
func ParseDocument(src []byte) (*Document, error) {
	metaRaw, body, hadMeta := splitFrontmatter(src)

	meta, err := ParseFrontmatter(metaRaw)
	if err != nil {
		return nil, fmt.Errorf("parse document: %w", err)
	}

	return &Document{
		Meta:     meta,
		Body:     string(body),
		raw:      slices.Clone(src),
		origBody: string(body),
		hadMeta:  hadMeta,
	}, nil
}

// HasFrontmatter сообщает, был ли заголовок в исходном файле.
func (d *Document) HasFrontmatter() bool { return d.hadMeta }

// Bytes собирает документ обратно в содержимое файла.
func (d *Document) Bytes() []byte {
	if !d.Meta.dirty && d.Body == d.origBody {
		return slices.Clone(d.raw)
	}

	meta := d.Meta.Bytes()
	if len(meta) == 0 {
		return []byte(d.Body)
	}

	var buf bytes.Buffer
	buf.Grow(len(meta) + len(d.Body) + 8)
	buf.WriteString("---\n")
	buf.Write(meta)
	buf.WriteString("---\n")
	buf.WriteString(d.Body)
	return buf.Bytes()
}

// splitFrontmatter делит содержимое файла на сырой YAML и тело.
//
// Своё, а не библиотечное: правило простое, а любая библиотека здесь принесёт
// своё мнение о том, как разбирать YAML — нам же нужны именно сырые байты,
// чтобы ничего не потерять при обратной записи.
//
// Заголовком считается блок между строкой "---" в самом начале файла и первой
// следующей строкой "---". Нет открывающей или нет закрывающей — заголовка нет,
// весь файл тело. Это то же правило, по которому живут Obsidian и Jekyll,
// поэтому горизонтальная линия markdown в середине текста ничего не ломает.
func splitFrontmatter(src []byte) (meta, body []byte, ok bool) {
	line, rest, hasNewline := cutLine(src)
	if !hasNewline || !isDelimiter(line) {
		return nil, src, false
	}

	for i := 0; i <= len(rest); {
		line, next, hasNewline := cutLine(rest[i:])
		if isDelimiter(line) {
			return rest[:i], next, true
		}
		if !hasNewline {
			break
		}
		i += len(rest[i:]) - len(next)
	}

	return nil, src, false
}

// cutLine отрезает первую строку. line — без завершающих \n и \r,
// rest — всё после \n, hasNewline — был ли перевод строки вообще.
func cutLine(b []byte) (line, rest []byte, hasNewline bool) {
	i := bytes.IndexByte(b, '\n')
	if i < 0 {
		return b, nil, false
	}
	line = b[:i]
	line = bytes.TrimSuffix(line, []byte("\r"))
	return line, b[i+1:], true
}

// isDelimiter — строка-разделитель "---", возможно с пробелами в хвосте:
// редакторы любят их оставлять, а из-за такого пробела заметка не должна
// переставать разбираться.
func isDelimiter(line []byte) bool {
	return bytes.Equal(bytes.TrimRight(line, " \t"), []byte("---"))
}
