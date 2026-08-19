package vault

import (
	"strings"
	"testing"
)

// Главная гарантия слоя: документ, который мы не меняли, записывается обратно
// байт в байт. Всё остальное в этом пакете держится на ней.
func TestParseDocumentRoundTrip(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{"заметка из SPEC §4.2", `---
id: 01K3QF8ZN7X2WPBV4YHMC6TDAE
title: Перерасчёт значений свойств
created: 2026-08-18T13:12:03+03:00
updated: 2026-08-18T14:40:11+03:00
status: active
tags: [работа, armz, баг]
pinned: false
origin: agent
context:
  repo: armz-frontend
  branch: feature/recalc
  commit: 3f9a1c2
---

# Заголовок в теле

Текст заметки.
`},
		{"неизвестные поля и комментарии", `---
id: 01K3QF8ZN7X2WPBV4YHMC6TDAE   # ULID, неизменяемый
title: Заметка
# чужой комментарий целой строкой
obsidian_plugin_field: значение
nested_unknown:
  a: 1
  b: [x, y]
---
Тело.
`},
		{"без frontmatter", "# Просто markdown\n\nБез всякого заголовка.\n"},
		{"пустой frontmatter", "---\n---\nТело.\n"},
		{"тело содержит горизонтальную линию", `---
title: Заметка
---
Первый абзац.

---

Второй абзац после линии.
`},
		{"незакрытый frontmatter — это тело", "---\ntitle: не закрыт\n\nи дальше текст\n"},
		{"пустое тело", "---\ntitle: Пусто\n---\n"},
		{"CRLF", "---\r\ntitle: Windows\r\n---\r\nТело.\r\n"},
		{"тело без завершающего перевода строки", "---\ntitle: A\n---\nхвост без \\n"},
		{"совсем пусто", ""},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d, err := ParseDocument([]byte(c.src))
			if err != nil {
				t.Fatalf("ParseDocument: %v", err)
			}
			got := string(d.Bytes())
			if got != c.src {
				t.Errorf("round-trip разошёлся\n--- было ---\n%s\n--- стало ---\n%s", c.src, got)
			}
		})
	}
}

func TestParseDocumentSplit(t *testing.T) {
	cases := []struct {
		name     string
		src      string
		wantMeta bool
		wantBody string
	}{
		{"обычная", "---\ntitle: A\n---\nТело.\n", true, "Тело.\n"},
		{"без frontmatter", "Тело.\n", false, "Тело.\n"},
		{"пустой frontmatter", "---\n---\nТело.\n", true, "Тело." + "\n"},
		{"тело с линией", "---\ntitle: A\n---\nдо\n\n---\n\nпосле\n", true, "до\n\n---\n\nпосле\n"},
		{"разделитель с пробелами в хвосте", "---  \ntitle: A\n---  \nТело.\n", true, "Тело.\n"},
		{"незакрытый", "---\ntitle: A\n", false, "---\ntitle: A\n"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d, err := ParseDocument([]byte(c.src))
			if err != nil {
				t.Fatalf("ParseDocument: %v", err)
			}
			if d.HasFrontmatter() != c.wantMeta {
				t.Errorf("HasFrontmatter = %v, ожидалось %v", d.HasFrontmatter(), c.wantMeta)
			}
			if d.Body != c.wantBody {
				t.Errorf("Body = %q, ожидалось %q", d.Body, c.wantBody)
			}
		})
	}
}

// Битый frontmatter не проглатывается молча: заметка, которую человек поправил
// руками и сломал отступ, должна дать ошибку, а не превратиться в тело.
func TestParseDocumentBrokenFrontmatter(t *testing.T) {
	src := "---\ntitle: A\n  сдвинуто: и сломано\n\ttab: сюда нельзя\n---\nТело.\n"
	if _, err := ParseDocument([]byte(src)); err == nil {
		t.Fatal("ожидалась ошибка разбора, получено nil")
	}
}

func TestDocumentBytesAfterEdit(t *testing.T) {
	src := `---
id: 01K3
title: Старый
# комментарий должен уцелеть
чужое_поле: не трогать
tags: [работа, баг]
---
Тело не меняем.
`
	d, err := ParseDocument([]byte(src))
	if err != nil {
		t.Fatalf("ParseDocument: %v", err)
	}
	if err := d.Meta.SetTitle("Новый"); err != nil {
		t.Fatalf("SetTitle: %v", err)
	}

	want := `---
id: 01K3
title: Новый
# комментарий должен уцелеть
чужое_поле: не трогать
tags: [работа, баг]
---
Тело не меняем.
`
	if got := string(d.Bytes()); got != want {
		t.Errorf("после правки заголовка\n--- ожидалось ---\n%s\n--- получено ---\n%s", want, got)
	}
}

func TestDocumentAddFrontmatterToPlainFile(t *testing.T) {
	// SPEC §4.1: отсутствующий frontmatter достраивается при первом открытии.
	d, err := ParseDocument([]byte("# Просто файл\n"))
	if err != nil {
		t.Fatalf("ParseDocument: %v", err)
	}
	if err := d.Meta.SetTitle("Просто файл"); err != nil {
		t.Fatalf("SetTitle: %v", err)
	}
	got := string(d.Bytes())
	want := "---\ntitle: Просто файл\n---\n# Просто файл\n"
	if got != want {
		t.Errorf("ожидалось %q, получено %q", want, got)
	}
	if !strings.HasPrefix(got, "---\n") {
		t.Error("frontmatter не дописан в начало")
	}
}
