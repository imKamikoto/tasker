package vault

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Путь вложения из SPEC §4.4: attachments/<год>/<месяц>/<8 base32>.<ext>.
var attachmentPath = regexp.MustCompile(`^attachments/2026/08/[0-9A-HJKMNP-TV-Z]{8}\.png$`)

func TestSaveAttachment(t *testing.T) {
	v, root := testVault(t)

	got, err := v.SaveAttachment("Снимок экрана 2026-08-19.PNG", []byte("картинка"))
	if err != nil {
		t.Fatalf("SaveAttachment: %v", err)
	}
	if !attachmentPath.MatchString(got.Path) {
		t.Errorf("путь %q не подходит под attachments/<год>/<месяц>/<имя>.png", got.Path)
	}
	if !got.Image {
		t.Error("png не опознан картинкой")
	}

	raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(got.Path)))
	if err != nil {
		t.Fatalf("файла нет: %v", err)
	}
	if string(raw) != "картинка" {
		t.Errorf("содержимое %q", raw)
	}
}

// Имя случайное: снимки экрана из буфера называются одинаково, и раскладывать
// их по уникальным именам дешевле, чем разрешать коллизии осмысленных.
func TestSaveAttachmentNamesAreUnique(t *testing.T) {
	v, _ := testVault(t)

	seen := make(map[string]bool)
	for i := 0; i < 20; i++ {
		got, err := v.SaveAttachment("снимок.png", []byte("данные"))
		if err != nil {
			t.Fatal(err)
		}
		if seen[got.Path] {
			t.Fatalf("путь повторился: %s", got.Path)
		}
		seen[got.Path] = true
	}
}

func TestSaveAttachmentExtensions(t *testing.T) {
	v, _ := testVault(t)

	cases := []struct {
		filename string
		suffix   string
		image    bool
	}{
		{"снимок.png", ".png", true},
		{"фото.JPEG", ".jpeg", true},
		{"схема.svg", ".svg", true},
		{"договор.pdf", ".pdf", false},
		// Расширения нет — файл сохраняется без него.
		{"безымянный", "", false},
		// Мусор после точки расширением не считается: имя приходит из вебвью.
		{"файл.э/../..", "", false},
		{"файл.оченьдлинноерасширение", "", false},
	}
	for _, c := range cases {
		got, err := v.SaveAttachment(c.filename, []byte("данные"))
		if err != nil {
			t.Fatalf("%s: %v", c.filename, err)
		}
		if !strings.HasSuffix(got.Path, c.suffix) {
			t.Errorf("%s → %q, ожидался суффикс %q", c.filename, got.Path, c.suffix)
		}
		if got.Image != c.image {
			t.Errorf("%s: Image = %v", c.filename, got.Image)
		}
		// Что бы ни пришло в имени, файл обязан остаться внутри attachments.
		if strings.Contains(got.Path, "..") {
			t.Errorf("%s дал путь наружу: %s", c.filename, got.Path)
		}
	}
}

func TestSaveAttachmentRejectsEmpty(t *testing.T) {
	v, _ := testVault(t)
	if _, err := v.SaveAttachment("пусто.png", nil); err == nil {
		t.Error("пустое вложение сохранилось")
	}
}

func TestAttachmentMarkdown(t *testing.T) {
	image := Attachment{Path: "attachments/2026/08/ABCDEFGH.png", Image: true}
	if got := AttachmentMarkdown(image, ""); got != "![](attachments/2026/08/ABCDEFGH.png)" {
		t.Errorf("картинка: %q", got)
	}
	if got := AttachmentMarkdown(image, "схема"); got != "![схема](attachments/2026/08/ABCDEFGH.png)" {
		t.Errorf("подпись: %q", got)
	}

	// Не-картинка вставляется обычной ссылкой, и без подписи она была бы
	// пустой — то есть выглядела бы как пропажа.
	file := Attachment{Path: "attachments/2026/08/ABCDEFGH.pdf"}
	if got := AttachmentMarkdown(file, ""); got != "[ABCDEFGH.pdf](attachments/2026/08/ABCDEFGH.pdf)" {
		t.Errorf("файл: %q", got)
	}
}
