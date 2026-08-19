package vault

import (
	"strings"
	"testing"
)

func TestSlug(t *testing.T) {
	cases := []struct {
		title string
		want  string
	}{
		// Основной случай: русский заголовок целиком.
		{"Счётчик перерасчёта не обновляется", "schetchik-pererascheta-ne-obnovlyaetsya"},
		{"в парсере ломается экранирование кавычек", "v-parsere-lomaetsya-ekranirovanie-kavychek"},

		// Латиница и цифры проходят как есть, регистр опускается.
		{"Bug: RequestHeader counter", "bug-requestheader-counter"},
		{"Release 2.1.0", "release-2-1-0"},
		{"Тест Test 123", "test-test-123"},

		// Каждая буква алфавита — таблица транслита не должна разъезжаться.
		{"абвгдеёжзийклмнопрстуфхцчшщъыьэюя", "abvgdeezhziyklmnoprstufhcchshschyeyuya"},
		{"АБВГДЕЁЖЗИЙКЛМНОПРСТУФХЦЧШЩЪЫЬЭЮЯ", "abvgdeezhziyklmnoprstufhcchshschyeyuya"},

		// Отдельные буквы, на которых обычно спорят.
		{"подъезд", "podezd"},
		{"мышь", "mysh"},
		{"цена", "cena"},
		{"щука", "schuka"},
		{"хорошо", "horosho"},
		{"юла", "yula"},
		{"ёжик", "ezhik"},
		{"йогурт", "yogurt"},

		// Пунктуация схлопывается в один дефис, края обрезаются.
		{"  Заметка!  ", "zametka"},
		{"Что?! Опять...", "chto-opyat"},
		{"a---b", "a-b"},
		{"-начало и конец-", "nachalo-i-konec"},
		{"путь/к/файлу", "put-k-faylu"},
		{"Работа: Баги", "rabota-bagi"},

		// Уже готовый slug остаётся собой.
		{"note-2", "note-2"},

		// Ничего пригодного — пустой результат, решать вызывающему.
		{"", ""},
		{"   ", ""},
		{"🎉🎉🎉", ""},
		{"!!!", ""},
		{"中文标题", ""},
	}

	for _, c := range cases {
		t.Run(c.title, func(t *testing.T) {
			if got := Slug(c.title); got != c.want {
				t.Errorf("Slug(%q) = %q, ожидалось %q", c.title, got, c.want)
			}
		})
	}
}

func TestSlugLength(t *testing.T) {
	t.Run("длинный заголовок обрезается по границе слова", func(t *testing.T) {
		title := "Очень длинный заголовок заметки который заведомо не помещается в отведённые шестьдесят символов"
		// Точное ожидание, а не «не длиннее шестидесяти»: обрезка обязана
		// пройти по дефису, а не посреди слова "pomeschaetsya".
		const want = "ochen-dlinnyy-zagolovok-zametki-kotoryy-zavedomo-ne"
		got := Slug(title)
		if got != want {
			t.Errorf("Slug = %q, ожидалось %q", got, want)
		}
		if len(got) > maxSlugLen {
			t.Errorf("длина %d > %d", len(got), maxSlugLen)
		}
	})

	t.Run("слово без пробелов режется жёстко и не тянет дефис", func(t *testing.T) {
		title := "Заголовокбезпробеловкоторыйдлиннеешестидесятисимволовиегонекударезать"
		const want = "zagolovokbezprobelovkotoryydlinneeshestidesyatisimvoloviegon"
		got := Slug(title)
		if got != want {
			t.Errorf("Slug = %q, ожидалось %q", got, want)
		}
		if len(got) != maxSlugLen {
			t.Errorf("длина %d, ожидалось %d", len(got), maxSlugLen)
		}
	})

	t.Run("одно длинное слово режется жёстко", func(t *testing.T) {
		got := Slug(strings.Repeat("a", 100))
		if len(got) != maxSlugLen {
			t.Errorf("длина %d, ожидалось %d", len(got), maxSlugLen)
		}
	})

	t.Run("ровно на границе", func(t *testing.T) {
		got := Slug(strings.Repeat("a", maxSlugLen))
		if len(got) != maxSlugLen {
			t.Errorf("длина %d, ожидалось %d", len(got), maxSlugLen)
		}
	})
}

// Slug обязан давать имя, безопасное для файловой системы и для git.
func TestSlugIsFilesystemSafe(t *testing.T) {
	titles := []string{
		"a/b\\c",
		"file:name",
		"звёздочка * и вопрос ?",
		"кавычки \"и\" 'апострофы'",
		"перевод\nстроки\tи таб",
		"..",
		".",
		"CON",
		"нулевой\x00байт",
	}
	for _, title := range titles {
		got := Slug(title)
		for _, r := range got {
			ok := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-'
			if !ok {
				t.Errorf("Slug(%q) = %q — недопустимый символ %q", title, got, r)
			}
		}
		if got == "." || got == ".." {
			t.Errorf("Slug(%q) = %q — опасное имя", title, got)
		}
	}
}
