package index

import (
	"context"
	"strings"
	"testing"
	"time"
)

// Набор заметок, покрывающий все формы запроса разом.
func seed(t *testing.T, ix *Index) {
	t.Helper()
	ctx := context.Background()
	base := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)

	records := []Record{
		{
			ID: "01K3QF8ZN7X2WPBV4YHMC6TDA1", Path: "Работа/Баги/schetchik.md",
			Notebook: "Работа/Баги", Title: "Счётчик перерасчёта", Status: "active", Pinned: false,
			NumTasks: 2, NumDone: 1, Tags: []string{"баг", "bug"},
			Body: "Счётчик не пересчитывается после ручной правки значения",
		},
		{
			ID: "01K3QF8ZN7X2WPBV4YHMC6TDA2", Path: "Работа/plan.md",
			Notebook: "Работа", Title: "План на квартал", Status: "onHold", Pinned: true,
			NumTasks: 3, NumDone: 3, Tags: []string{"планирование"},
			Body: "Разобрать миграции и почистить индексы",
		},
		{
			ID: "01K3QF8ZN7X2WPBV4YHMC6TDA3", Path: "Личное/pokupki.md",
			Notebook: "Личное", Title: "Покупки", Status: "none", Pinned: false,
			Origin: "agent", Tags: []string{"черновик"},
			Body: "Купить миграции чего-нибудь к чаю",
		},
		{
			ID: "01K3QF8ZN7X2WPBV4YHMC6TDA4", Path: "Работа-старая/arhiv.md",
			Notebook: "Работа-старая", Title: "Архив", Status: "completed",
			Body: "Старые записи",
		},
		{
			ID: "01K3QF8ZN7X2WPBV4YHMC6TDA5", Path: ".trash/udalennaya.md",
			Notebook: ".trash", Title: "Удалённая", Status: "none", Trashed: true,
			Body: "Счётчик тоже упоминается здесь",
		},
	}
	for i, r := range records {
		r.Created = base
		r.Updated = base.Add(time.Duration(i) * time.Hour)
		r.ModTime = r.Updated
		r.Size = 100
		if err := ix.Put(ctx, r); err != nil {
			t.Fatalf("Put %d: %v", i, err)
		}
	}
}

func search(t *testing.T, ix *Index, query string) []Record {
	t.Helper()
	q, err := ParseQuery(query)
	if err != nil {
		t.Fatalf("ParseQuery(%q): %v", query, err)
	}
	res, err := ix.Search(context.Background(), q, SearchOptions{})
	if err != nil {
		t.Fatalf("Search(%q): %v", query, err)
	}
	return res
}

func titles(rs []Record) []string {
	out := make([]string, len(rs))
	for i, r := range rs {
		out[i] = r.Title
	}
	return out
}

func hasTitle(rs []Record, title string) bool {
	for _, r := range rs {
		if r.Title == title {
			return true
		}
	}
	return false
}

func TestSearchForms(t *testing.T) {
	ix, _ := testIndex(t)
	seed(t, ix)

	cases := []struct {
		query string
		want  []string
	}{
		{"", []string{"План на квартал", "Архив", "Покупки", "Счётчик перерасчёта"}},
		{"перерасч", []string{"Счётчик перерасчёта"}},
		{`"ручной правки"`, []string{"Счётчик перерасчёта"}},
		{"миграци", []string{"План на квартал", "Покупки"}},
		{"title:перерасч", []string{"Счётчик перерасчёта"}},
		{"body:миграци", []string{"План на квартал", "Покупки"}},
		{"book:Работа", []string{"План на квартал", "Счётчик перерасчёта"}},
		{"book:Работа/Баги", []string{"Счётчик перерасчёта"}},
		{"book:Личное", []string{"Покупки"}},
		{"tag:баг", []string{"Счётчик перерасчёта"}},
		{"status:active", []string{"Счётчик перерасчёта"}},
		{"status:onHold", []string{"План на квартал"}},
		{"is:pinned", []string{"План на квартал"}},
		{"is:agent", []string{"Покупки"}},
		{"миграци -is:agent", []string{"План на квартал"}},
		{"has:task", []string{"Счётчик перерасчёта"}},
		{"миграци -tag:черновик", []string{"План на квартал"}},
		{"миграци -book:Личное", []string{"План на квартал"}},
		{"book:Работа status:active", []string{"Счётчик перерасчёта"}},
		{"миграци индекс", []string{"План на квартал"}},
	}

	for _, c := range cases {
		t.Run(c.query, func(t *testing.T) {
			got := titles(search(t, ix, c.query))
			if len(got) != len(c.want) {
				t.Fatalf("найдено %v, ожидалось %v", got, c.want)
			}
			for i := range c.want {
				if got[i] != c.want[i] {
					t.Errorf("позиция %d: %q, ожидалось %q (всё: %v)", i, got[i], c.want[i], got)
				}
			}
		})
	}
}

// book:Работа не должен цеплять «Работа-старая»: это префикс строки, но не
// вложенный ноутбук.
func TestSearchNotebookIsNotStringPrefix(t *testing.T) {
	ix, _ := testIndex(t)
	seed(t, ix)

	got := search(t, ix, "book:Работа")
	if hasTitle(got, "Архив") {
		t.Errorf("book:Работа зацепил соседний ноутбук: %v", titles(got))
	}
}

// Символы шаблона LIKE в значении не должны превращать его в шаблон.
func TestSearchNotebookEscapesLikeWildcards(t *testing.T) {
	ix, _ := testIndex(t)
	ctx := context.Background()
	seed(t, ix)

	if err := ix.Put(ctx, Record{
		ID: "01K3QF8ZN7X2WPBV4YHMC6TDA6", Path: "100%/note.md", Notebook: "100%",
		Title: "Сто процентов", Status: "none", Body: "тело",
		Created: time.Now(), Updated: time.Now(), ModTime: time.Now(), Size: 10,
	}); err != nil {
		t.Fatal(err)
	}

	got := search(t, ix, "book:%")
	if len(got) != 0 {
		t.Errorf("book:%% нашёл %v — процент сработал как шаблон", titles(got))
	}
	got = search(t, ix, "book:100%")
	if len(got) != 1 || got[0].Title != "Сто процентов" {
		t.Errorf("book:100%% нашёл %v", titles(got))
	}
}

// Корзина не участвует в поиске, пока её не попросят явно.
func TestSearchExcludesTrash(t *testing.T) {
	ix, _ := testIndex(t)
	seed(t, ix)
	ctx := context.Background()

	if got := search(t, ix, "Счётчик"); hasTitle(got, "Удалённая") {
		t.Errorf("заметка из корзины попала в выдачу: %v", titles(got))
	}

	q, _ := ParseQuery("Счётчик")
	got, err := ix.Search(ctx, q, SearchOptions{Trash: TrashIncluded})
	if err != nil {
		t.Fatal(err)
	}
	if !hasTitle(got, "Удалённая") {
		t.Errorf("с TrashIncluded корзина не вернулась: %v", titles(got))
	}
}

// Закреплённые сверху, дальше по дате изменения по убыванию.
func TestSearchOrder(t *testing.T) {
	ix, _ := testIndex(t)
	seed(t, ix)

	got := search(t, ix, "")
	if len(got) == 0 || !got[0].Pinned {
		t.Fatalf("первой должна идти закреплённая: %v", titles(got))
	}
	for i := 1; i < len(got); i++ {
		if got[i-1].Pinned == got[i].Pinned && got[i-1].Updated.Before(got[i].Updated) {
			t.Errorf("порядок по дате нарушен на позиции %d: %v", i, titles(got))
		}
	}
}

func TestSearchLimit(t *testing.T) {
	ix, _ := testIndex(t)
	seed(t, ix)

	q, _ := ParseQuery("")
	got, err := ix.Search(context.Background(), q, SearchOptions{Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Errorf("вернулось %d, ожидалось 2", len(got))
	}
}

// Теги должны приезжать вместе с результатами: список их показывает.
func TestSearchFillsTags(t *testing.T) {
	ix, _ := testIndex(t)
	seed(t, ix)

	got := search(t, ix, "перерасч")
	if len(got) != 1 {
		t.Fatalf("найдено %v", titles(got))
	}
	if len(got[0].Tags) != 2 || got[0].Tags[0] != "баг" && got[0].Tags[1] != "баг" {
		t.Errorf("теги = %v", got[0].Tags)
	}
}

// Латиница в тегах регистронезависима (COLLATE NOCASE в схеме SPEC §5.1).
// Для кириллицы NOCASE в SQLite не работает — это ограничение схемы, а не
// недосмотр; см. отчёт.
func TestSearchTagCaseFolding(t *testing.T) {
	ix, _ := testIndex(t)
	seed(t, ix)

	if got := search(t, ix, "tag:BUG"); len(got) != 1 {
		t.Errorf("tag:BUG нашёл %v, ожидалась одна заметка", titles(got))
	}
	if got := search(t, ix, "tag:БАГ"); len(got) != 0 {
		t.Logf("tag:БАГ нашёл %v — кириллица в NOCASE неожиданно заработала", titles(got))
	}
}

// title: и body: обязаны действительно сужать поиск до своей колонки. Проверять
// это на слове, которое и так встречается только в одном месте, бессмысленно:
// результат совпадёт и без фильтра колонки.
func TestSearchColumnFilterIsReal(t *testing.T) {
	ix, _ := testIndex(t)
	ctx := context.Background()
	now := time.Now()

	if err := ix.Put(ctx, Record{
		ID: "01K3QF8ZN7X2WPBV4YHMC6TDB1", Path: "note.md", Notebook: "",
		Title: "зверобойзаголовок", Status: "none",
		Body:    "коромыслотело и больше ничего",
		Created: now, Updated: now, ModTime: now, Size: 10,
	}); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		query string
		want  int
	}{
		{"зверобой", 1},
		{"коромысло", 1},
		{"title:зверобой", 1},
		{"title:коромысло", 0},
		{"body:коромысло", 1},
		{"body:зверобой", 0},
		{"title:зверобой body:коромысло", 1},
		{"title:коромысло body:зверобой", 0},
	}
	for _, c := range cases {
		t.Run(c.query, func(t *testing.T) {
			if got := search(t, ix, c.query); len(got) != c.want {
				t.Errorf("найдено %d (%v), ожидалось %d", len(got), titles(got), c.want)
			}
		})
	}
}

// Список ноутбука по умолчанию не показывает завершённое и брошенное
// (SPEC §8.3). Через язык запросов это не выразить: там только И, а запрос из
// одних отрицаний отвергается.
func TestSearchHidesCompleted(t *testing.T) {
	ix, _ := testIndex(t)
	seed(t, ix)

	q, _ := ParseQuery("")
	got, err := ix.Search(context.Background(), q, SearchOptions{HideCompleted: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range got {
		if r.Status == "completed" || r.Status == "dropped" {
			t.Errorf("в выдаче %q со статусом %q", r.Title, r.Status)
		}
	}
	if hasTitle(got, "Архив") {
		t.Errorf("завершённая заметка осталась: %v", titles(got))
	}
	// Остальные на месте.
	if !hasTitle(got, "Счётчик перерасчёта") || !hasTitle(got, "Покупки") {
		t.Errorf("выдача = %v", titles(got))
	}
}

// Экран корзины: только удалённые и ничего кроме.
func TestSearchTrashOnly(t *testing.T) {
	ix, _ := testIndex(t)
	seed(t, ix)

	q, _ := ParseQuery("")
	got, err := ix.Search(context.Background(), q, SearchOptions{Trash: TrashOnly})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Title != "Удалённая" {
		t.Fatalf("выдача = %v, ожидалась только удалённая", titles(got))
	}
	for _, r := range got {
		if !r.Trashed {
			t.Errorf("живая заметка на экране корзины: %q", r.Title)
		}
	}
}

// Сортировки из SPEC §8.4: заголовок, создано, изменено, обе стороны.
func TestSearchSort(t *testing.T) {
	ix, _ := testIndex(t)
	ctx := context.Background()
	base := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)

	for i, title := range []string{"Вторая", "Первая", "Третья"} {
		if err := ix.Put(ctx, Record{
			ID:   "01K3QF8ZN7X2WPBV4YHMC6TD" + string(rune('A'+i)) + "1",
			Path: title + ".md", Title: title, Status: "none", Body: "тело",
			Created: base.Add(time.Duration(i) * time.Hour),
			Updated: base.Add(time.Duration(2-i) * time.Hour),
			ModTime: base, Size: 10,
		}); err != nil {
			t.Fatal(err)
		}
	}

	cases := []struct {
		name string
		sort Sort
		want []string
	}{
		{"по изменению, свежее сверху", Sort{Field: SortUpdated}, []string{"Вторая", "Первая", "Третья"}},
		{"по изменению наоборот", Sort{Field: SortUpdated, Reversed: true}, []string{"Третья", "Первая", "Вторая"}},
		{"по созданию, свежее сверху", Sort{Field: SortCreated}, []string{"Третья", "Первая", "Вторая"}},
		{"по созданию наоборот", Sort{Field: SortCreated, Reversed: true}, []string{"Вторая", "Первая", "Третья"}},
		{"по заголовку от А к Я", Sort{Field: SortTitle}, []string{"Вторая", "Первая", "Третья"}},
		{"по заголовку наоборот", Sort{Field: SortTitle, Reversed: true}, []string{"Третья", "Первая", "Вторая"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			q, _ := ParseQuery("")
			got, err := ix.Search(ctx, q, SearchOptions{Sort: c.sort})
			if err != nil {
				t.Fatal(err)
			}
			if strings.Join(titles(got), ",") != strings.Join(c.want, ",") {
				t.Errorf("порядок = %v, ожидался %v", titles(got), c.want)
			}
		})
	}
}

// Закреплённые сверху при любой сортировке: это группировка, а не порядок.
func TestSearchPinnedStayOnTop(t *testing.T) {
	ix, _ := testIndex(t)
	seed(t, ix)

	for _, sort := range []Sort{
		{Field: SortUpdated}, {Field: SortCreated, Reversed: true}, {Field: SortTitle},
	} {
		q, _ := ParseQuery("")
		got, err := ix.Search(context.Background(), q, SearchOptions{Sort: sort})
		if err != nil {
			t.Fatal(err)
		}
		if len(got) == 0 || !got[0].Pinned {
			t.Errorf("при сортировке %+v первой идёт %v", sort, titles(got))
		}
	}
}
