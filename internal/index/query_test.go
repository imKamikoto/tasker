package index

import (
	"errors"
	"reflect"
	"testing"
)

func text(v string) Term             { return Term{Kind: TermText, Value: v} }
func field(f, v string) Term         { return Term{Kind: TermText, Field: f, Value: v} }
func neg(t Term) Term                { t.Negated = true; return t }
func kind(k TermKind, v string) Term { return Term{Kind: k, Value: v} }

// Каждая форма из SPEC §8.5.
func TestParseQueryForms(t *testing.T) {
	cases := []struct {
		in   string
		want []Term
	}{
		{"слово", []Term{text("слово")}},
		{`"точная фраза"`, []Term{text("точная фраза")}},
		{"book:Работа", []Term{kind(TermNotebook, "Работа")}},
		// Корень vault: у него нет имени, и назвать его иначе как отдельным
		// знаком нечем. Пустое значение занято под опечатку.
		{"book:/", []Term{kind(TermNotebook, "")}},
		{"tag:баг", []Term{kind(TermTag, "баг")}},
		{"status:active", []Term{kind(TermStatus, "active")}},
		{"title:счётчик", []Term{field("title", "счётчик")}},
		{"body:счётчик", []Term{field("body", "счётчик")}},
		{"is:pinned", []Term{kind(TermPinned, "pinned")}},
		{"is:agent", []Term{kind(TermAgent, "agent")}},
		{"has:task", []Term{kind(TermTask, "task")}},
		{"слово -tag:черновик", []Term{text("слово"), neg(kind(TermTag, "черновик"))}},
		{`слово -"точная фраза"`, []Term{text("слово"), neg(text("точная фраза"))}},

		// Пробел — это И.
		{"счётчик tag:баг status:active", []Term{
			text("счётчик"), kind(TermTag, "баг"), kind(TermStatus, "active"),
		}},

		// Значение в кавычках после префикса.
		{`book:"Работа/Проект X"`, []Term{kind(TermNotebook, "Работа/Проект X")}},
		{`tag:"два слова"`, []Term{kind(TermTag, "два слова")}},

		// Несколько слов без кавычек — несколько термов, а не одна фраза.
		{"два слова", []Term{text("два"), text("слова")}},

		// Регистр префикса не важен, а значения — важен.
		{"TAG:Баг", []Term{kind(TermTag, "Баг")}},
		{"Book:Работа", []Term{kind(TermNotebook, "Работа")}},
		{"BOOK:/", []Term{kind(TermNotebook, "")}},

		// Ведущая косая черта читается как «от корня» и просто снимается.
		// Проверять имя ноутбука парсер не берётся нигде — book:Рабта тоже
		// разбирается и просто ничего не находит, — и здесь не начинает.
		{"book:/Работа", []Term{kind(TermNotebook, "Работа")}},

		// Неизвестный префикс — обычный текст: двоеточие в поиске по коду
		// встречается чаще, чем опечатка в фильтре.
		{"foo:bar", []Term{text("foo:bar")}},
		{"http://example.com", []Term{text("http://example.com")}},

		// Незакрытая кавычка тянется до конца ввода.
		{`"незакрытая фраза`, []Term{text("незакрытая фраза")}},

		// Лишние пробелы.
		{"  слово   tag:баг  ", []Term{text("слово"), kind(TermTag, "баг")}},

		// Пустой запрос — это «всё», а не ошибка.
		{"", nil},
		{"   ", nil},
	}

	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			q, err := ParseQuery(c.in)
			if err != nil {
				t.Fatalf("ParseQuery(%q): %v", c.in, err)
			}
			if !reflect.DeepEqual(q.Terms, c.want) {
				t.Errorf("термы = %+v, ожидались %+v", q.Terms, c.want)
			}
		})
	}
}

func TestParseQueryErrors(t *testing.T) {
	cases := []struct {
		in   string
		want error
	}{
		// SPEC §8.5: запрос только из отрицаний — понятная ошибка.
		{"-слово", ErrOnlyNegations},
		{"-tag:черновик", ErrOnlyNegations},
		{"-tag:черновик -status:completed", ErrOnlyNegations},

		// Закрытые перечисления: опечатку здесь надо показать, а не искать текст.
		{"status:почтиГотово", ErrUnknownValue},
		{"is:красивая", ErrUnknownValue},
		{"has:настроение", ErrUnknownValue},

		// Пустое значение фильтра.
		{"tag:", ErrEmptyValue},
		{"book:", ErrEmptyValue},
		{`tag:""`, ErrEmptyValue},
		{"-tag:", ErrEmptyValue},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			_, err := ParseQuery(c.in)
			if !errors.Is(err, c.want) {
				t.Errorf("ParseQuery(%q) = %v, ожидалась %v", c.in, err, c.want)
			}
		})
	}
}

// status: принимает ровно то же перечисление, что и frontmatter.
func TestParseQueryStatusValues(t *testing.T) {
	for _, s := range []string{"none", "active", "onHold", "completed", "dropped"} {
		q, err := ParseQuery("status:" + s)
		if err != nil {
			t.Errorf("status:%s: %v", s, err)
			continue
		}
		if len(q.Terms) != 1 || q.Terms[0].Value != s {
			t.Errorf("status:%s разобрался в %+v", s, q.Terms)
		}
	}
}

// Минус — отрицание только снаружи кавычек. Внутри он часть искомого текста.
func TestParseQueryMinusPlacement(t *testing.T) {
	cases := []struct {
		in   string
		want []Term
	}{
		{`слово -"точная фраза"`, []Term{text("слово"), neg(text("точная фраза"))}},
		{`"-дефис внутри"`, []Term{text("-дефис внутри")}},
		{`"-"`, []Term{text("-")}},
		{"слово-через-дефис", []Term{text("слово-через-дефис")}},
		{"-", []Term{text("-")}},
		{"слово -", []Term{text("слово"), text("-")}},
		{`tag:"-минус-в-теге"`, []Term{kind(TermTag, "-минус-в-теге")}},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			q, err := ParseQuery(c.in)
			if err != nil {
				t.Fatalf("ParseQuery(%q): %v", c.in, err)
			}
			if !reflect.DeepEqual(q.Terms, c.want) {
				t.Errorf("термы = %+v, ожидались %+v", q.Terms, c.want)
			}
		})
	}
}
