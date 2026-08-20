package index

import (
	"context"
	"reflect"
	"testing"
	"time"
)

func put(t *testing.T, ix *Index, id, path, notebook string, trashed bool, tags []string, links []string) {
	t.Helper()
	now := time.Now()
	if err := ix.Put(context.Background(), Record{
		ID: id, Path: path, Notebook: notebook, Title: "Заметка " + id,
		Status: "none", Trashed: trashed, Tags: tags, Links: links,
		Created: now, Updated: now, ModTime: now, Size: 10, Body: "тело",
	}); err != nil {
		t.Fatal(err)
	}
}

func seedTree(t *testing.T, ix *Index) {
	t.Helper()
	put(t, ix, "01K3QF8ZN7X2WPBV4YHMC6TDA1", "Работа/Баги/a.md", "Работа/Баги", false, []string{"баг", "работа"}, nil)
	put(t, ix, "01K3QF8ZN7X2WPBV4YHMC6TDA2", "Работа/Баги/b.md", "Работа/Баги", false, []string{"баг"}, nil)
	put(t, ix, "01K3QF8ZN7X2WPBV4YHMC6TDA3", "Работа/план.md", "Работа", false, []string{"работа"}, nil)
	put(t, ix, "01K3QF8ZN7X2WPBV4YHMC6TDA4", "Личное/Покупки/c.md", "Личное/Покупки", false, nil, nil)
	put(t, ix, "01K3QF8ZN7X2WPBV4YHMC6TDA5", "корневая.md", "", false, []string{"работа"}, nil)
	put(t, ix, "01K3QF8ZN7X2WPBV4YHMC6TDA6", ".trash/удалённая.md", ".trash", true, []string{"баг"}, nil)
}

func TestNotebooks(t *testing.T) {
	ix, _ := testIndex(t)
	seedTree(t, ix)

	books, err := ix.Notebooks(context.Background())
	if err != nil {
		t.Fatalf("Notebooks: %v", err)
	}

	byPath := map[string]Notebook{}
	var paths []string
	for _, b := range books {
		byPath[b.Path] = b
		paths = append(paths, b.Path)
	}

	// Корзина ноутбуком не считается, а промежуточное «Личное» существует,
	// хотя своих заметок в нём нет.
	want := []string{"", "Личное", "Личное/Покупки", "Работа", "Работа/Баги"}
	if !reflect.DeepEqual(paths, want) {
		t.Fatalf("ноутбуки = %v, ожидались %v", paths, want)
	}

	// Счётчик — только свои заметки: вложенные считает интерфейс, когда
	// ноутбук свёрнут (SPEC §8.1).
	for path, count := range map[string]int{
		"": 1, "Работа": 1, "Работа/Баги": 2, "Личное": 0, "Личное/Покупки": 1,
	} {
		if got := byPath[path].Count; got != count {
			t.Errorf("в %q заметок %d, ожидалось %d", path, got, count)
		}
	}

	if kids := byPath["Работа"].Children; !reflect.DeepEqual(kids, []string{"Работа/Баги"}) {
		t.Errorf("дети «Работа» = %v", kids)
	}
	if kids := byPath["Работа/Баги"].Children; len(kids) != 0 {
		t.Errorf("у «Работа/Баги» не должно быть детей: %v", kids)
	}
	if kids := byPath[""].Children; !reflect.DeepEqual(kids, []string{"Личное", "Работа"}) {
		t.Errorf("дети корня = %v", kids)
	}
}

func TestTags(t *testing.T) {
	ix, _ := testIndex(t)
	seedTree(t, ix)

	tags, err := ix.Tags(context.Background())
	if err != nil {
		t.Fatalf("Tags: %v", err)
	}

	// Тег есть только у удалённой заметки — считаться он не должен, но и
	// исчезать из списка тоже: цвет и имя переживают удаление заметки.
	counts := map[string]int{}
	for _, tag := range tags {
		counts[tag.Name] = tag.Count
		if tag.Color == "" {
			t.Errorf("у тега %q пустой цвет", tag.Name)
		}
	}
	if counts["баг"] != 2 {
		t.Errorf("тег «баг» посчитан %d раз, ожидалось 2 (удалённая не в счёт)", counts["баг"])
	}
	if counts["работа"] != 3 {
		t.Errorf("тег «работа» посчитан %d раз, ожидалось 3", counts["работа"])
	}

	var names []string
	for _, tag := range tags {
		names = append(names, tag.Name)
	}
	if !reflect.DeepEqual(names, []string{"баг", "работа"}) {
		t.Errorf("теги = %v, ожидались отсортированные [баг работа]", names)
	}
}

func TestBacklinks(t *testing.T) {
	ix, _ := testIndex(t)
	const target = "01K3QF8ZN7X2WPBV4YHMC6TDA9"

	put(t, ix, "01K3QF8ZN7X2WPBV4YHMC6TDA1", "a.md", "", false, nil, []string{target})
	put(t, ix, "01K3QF8ZN7X2WPBV4YHMC6TDA2", "b.md", "", false, nil, []string{target, "01K3QF8ZN7X2WPBV4YHMC6TDAX"})
	put(t, ix, "01K3QF8ZN7X2WPBV4YHMC6TDA3", "c.md", "", false, nil, nil)
	put(t, ix, "01K3QF8ZN7X2WPBV4YHMC6TDA4", ".trash/d.md", ".trash", true, nil, []string{target})
	put(t, ix, target, "цель.md", "", false, nil, nil)

	got, err := ix.Backlinks(context.Background(), target)
	if err != nil {
		t.Fatalf("Backlinks: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("бэклинков %d, ожидалось 2 (удалённая не в счёт): %v", len(got), titles(got))
	}
	for _, r := range got {
		if r.ID != "01K3QF8ZN7X2WPBV4YHMC6TDA1" && r.ID != "01K3QF8ZN7X2WPBV4YHMC6TDA2" {
			t.Errorf("лишний бэклинк %s", r.ID)
		}
		if r.Path == "" || r.Title == "" {
			t.Errorf("бэклинк заполнен не полностью: %+v", r)
		}
	}
}

func TestBacklinksNone(t *testing.T) {
	ix, _ := testIndex(t)
	seedTree(t, ix)

	got, err := ix.Backlinks(context.Background(), "01K3QF8ZN7X2WPBV4YHMC6TDAZ")
	if err != nil {
		t.Fatalf("Backlinks: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("бэклинков %d, ожидалось 0", len(got))
	}
}

func TestNotebooksAndTagsOnEmptyIndex(t *testing.T) {
	ix, _ := testIndex(t)
	ctx := context.Background()

	books, err := ix.Notebooks(ctx)
	if err != nil || len(books) != 0 {
		t.Errorf("Notebooks = %v, %v", books, err)
	}
	tags, err := ix.Tags(ctx)
	if err != nil || len(tags) != 0 {
		t.Errorf("Tags = %v, %v", tags, err)
	}
}
