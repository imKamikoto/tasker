package vault

import (
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// attachmentsDir — где лежат вложения (SPEC §4.4).
const attachmentsDir = "attachments"

// nameAlphabet — Crockford base32 без похожих друг на друга букв.
//
// Тот же алфавит, что у ULID: имя вложения человек иногда читает вслух или
// набирает руками, и «I вместо 1» здесь стоит столько же, сколько в id.
const nameAlphabet = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

// nameLength — длина имени файла вложения (SPEC §4.4).
//
// Восемь символов base32 — это 40 бит. При тысяче вложений вероятность
// совпадения около одной миллиардной, и на этот случай есть проверка ниже.
const nameLength = 8

// ErrEmptyAttachment — вложение без содержимого.
var ErrEmptyAttachment = errors.New("attachment is empty")

// Attachment — сохранённое вложение.
type Attachment struct {
	// Path — путь от корня vault, всегда через прямые слэши: он уезжает в
	// markdown, а там разделитель один.
	Path string
	// Image — можно ли показать это картинкой. Для остального вставляется
	// обычная ссылка (SPEC §4.4).
	Image bool
}

// imageExts — расширения, которые вебвью умеет показать сам.
var imageExts = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true,
	".webp": true, ".heic": true, ".avif": true, ".svg": true, ".bmp": true,
}

// SaveAttachment кладёт файл в attachments/<год>/<месяц>/<имя>.<ext>.
//
// Имя случайное, а не исходное: файлы из буфера обмена называются одинаково
// («Снимок экрана»), и раскладывать их по датам с уникальным именем дешевле,
// чем разрешать коллизии осмысленных имён. Исходное имя нигде не нужно —
// подпись к картинке живёт в markdown.
//
// Путь возвращается от корня vault, а не относительно заметки: так перенос
// заметки в другой ноутбук не ломает картинки (SPEC §4.4).
func (v *Vault) SaveAttachment(filename string, data []byte) (Attachment, error) {
	if len(data) == 0 {
		return Attachment{}, fmt.Errorf("save attachment %q: %w", filename, ErrEmptyAttachment)
	}

	ext := extensionOf(filename)
	now := v.now()
	dir := path.Join(attachmentsDir, now.Format("2006"), now.Format("01"))

	if err := os.MkdirAll(filepath.Join(v.root, filepath.FromSlash(dir)), 0o755); err != nil {
		return Attachment{}, fmt.Errorf("save attachment: %w", err)
	}

	for attempt := 0; attempt < maxNameAttempts; attempt++ {
		name, err := randomName()
		if err != nil {
			return Attachment{}, fmt.Errorf("save attachment: %w", err)
		}
		rel := path.Join(dir, name+ext)
		abs := filepath.Join(v.root, filepath.FromSlash(rel))

		// Занято — берём другое имя. Проверка есть, потому что «вероятность
		// мала» и «не бывает» — разные вещи, а перезаписать чужое вложение
		// значит потерять его молча.
		if _, err := os.Stat(abs); err == nil {
			continue
		}
		if err := WriteFileAtomic(abs, data, 0o644); err != nil {
			return Attachment{}, fmt.Errorf("save attachment %s: %w", rel, err)
		}
		// Своя запись гасится по паре путь+время, поэтому время настоящее, а
		// не нулевое: иначе watcher примет её за правку снаружи.
		if info, err := os.Stat(abs); err == nil {
			v.wrote(rel, info.ModTime())
		}
		return Attachment{Path: rel, Image: imageExts[ext]}, nil
	}
	return Attachment{}, fmt.Errorf("save attachment: %w", ErrNameCollision)
}

// extensionOf достаёт расширение из имени, приводя его к безопасному виду.
//
// Расширение попадает в имя файла на диске, поэтому всё, что не буква и не
// цифра, отбрасывается: имя приходит из вебвью, и доверять ему нельзя.
func extensionOf(filename string) string {
	ext := strings.ToLower(path.Ext(filename))
	if ext == "" {
		return ""
	}
	var b strings.Builder
	b.WriteByte('.')
	for _, r := range ext[1:] {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	if b.Len() == 1 {
		return ""
	}
	// Длинный «хвост» после точки расширением не является.
	if b.Len() > 6 {
		return ""
	}
	return b.String()
}

func randomName() (string, error) {
	raw := make([]byte, nameLength)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	out := make([]byte, nameLength)
	for i, b := range raw {
		out[i] = nameAlphabet[int(b)%len(nameAlphabet)]
	}
	return string(out), nil
}

// AttachmentMarkdown собирает то, что вставляется в текст заметки.
func AttachmentMarkdown(a Attachment, caption string) string {
	if a.Image {
		return fmt.Sprintf("![%s](%s)", caption, a.Path)
	}
	// Для не-картинки подпись нужна: пустая ссылка выглядит как пропажа.
	if caption == "" {
		caption = path.Base(a.Path)
	}
	return fmt.Sprintf("[%s](%s)", caption, a.Path)
}
