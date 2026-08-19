package index

import (
	"reflect"
	"strings"
	"testing"
)

func TestExcerpt(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{
		{"обычный текст", "Первый абзац.\n\nВторой абзац.\n", "Первый абзац. Второй абзац."},
		{"заголовок снимается", "# Заголовок\n\nТекст под ним.\n", "Заголовок Текст под ним."},
		{"маркеры списка снимаются", "- пункт один\n- пункт два\n", "пункт один пункт два"},
		{"чекбоксы снимаются", "- [ ] задача\n- [x] сделано\n", "задача сделано"},
		{"нумерация снимается", "1. раз\n2. два\n", "раз два"},
		{"цитата снимается", "> цитата\n", "цитата"},
		{"забор кода выбрасывается", "текст\n\n```go\nfunc main() {}\n```\n\nещё текст\n", "текст func main() {} ещё текст"},
		{"пробелы схлопываются", "а   б\n\n\n\tв\n", "а б в"},
		{"пусто", "", ""},
		{"только пробелы", "   \n\n  \n", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Excerpt(c.body); got != c.want {
				t.Errorf("Excerpt = %q, ожидалось %q", got, c.want)
			}
		})
	}

	t.Run("длина ограничена", func(t *testing.T) {
		got := Excerpt(strings.Repeat("слово ", 200))
		if len([]rune(got)) > maxExcerptLen {
			t.Errorf("длина %d рун > %d", len([]rune(got)), maxExcerptLen)
		}
		if strings.HasSuffix(got, " ") {
			t.Errorf("пробел в хвосте: %q", got)
		}
	})

	t.Run("режется по границе руны", func(t *testing.T) {
		got := Excerpt(strings.Repeat("ё", 500))
		if !strings.HasPrefix(got, "ё") {
			t.Errorf("многобайтная руна разрезана: %q", got[:10])
		}
		if len([]rune(got)) != maxExcerptLen {
			t.Errorf("рун %d, ожидалось %d", len([]rune(got)), maxExcerptLen)
		}
	})
}

func TestCountTasks(t *testing.T) {
	cases := []struct {
		name              string
		body              string
		wantAll, wantDone int
	}{
		{"пусто", "", 0, 0},
		{"дефисы", "- [ ] раз\n- [x] два\n", 2, 1},
		{"звёздочки и плюсы", "* [ ] раз\n+ [x] два\n", 2, 1},
		{"большая X", "- [X] сделано\n", 1, 1},
		{"с отступом", "  - [ ] вложенная\n    - [x] глубже\n", 2, 1},
		{"без пробела после маркера не считается", "-[ ] раз\n", 0, 0},
		{"скобки в тексте не считаются", "просто [ ] в строке\n", 0, 0},
		{"обычный список не считается", "- пункт\n- ещё\n", 0, 0},
		{"смешанное", "текст\n- [ ] a\nещё\n- [x] b\n- [ ] c\n", 3, 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			all, done := CountTasks(c.body)
			if all != c.wantAll || done != c.wantDone {
				t.Errorf("CountTasks = %d/%d, ожидалось %d/%d", done, all, c.wantDone, c.wantAll)
			}
		})
	}
}

func TestExtractLinks(t *testing.T) {
	const a = "01K3QF8ZN7X2WPBV4YHMC6TDAE"
	const b = "01K3QF8ZN7X2WPBV4YHMC6TDAF"

	cases := []struct {
		name string
		body string
		want []string
	}{
		{"одна ссылка", "см. [Заголовок](tasker://note/" + a + ")", []string{a}},
		{"две ссылки", "[A](tasker://note/" + a + ") и [B](tasker://note/" + b + ")", []string{a, b}},
		{"дубли схлопываются", "[A](tasker://note/" + a + ") [A](tasker://note/" + a + ")", []string{a}},
		{"голый url", "tasker://note/" + a, []string{a}},
		{"чужая схема не берётся", "[A](oak://note/" + a + ")", nil},
		{"нижний регистр не берётся", "tasker://note/" + strings.ToLower(a), nil},
		{"короткий id не берётся", "tasker://note/01K3QF8", nil},
		{"буквы вне Crockford не берутся", "tasker://note/01K3QF8ZN7X2WPBV4YHMC6TDAI", nil},
		{"пусто", "", nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ExtractLinks(c.body)
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("ExtractLinks = %v, ожидалось %v", got, c.want)
			}
		})
	}
}
