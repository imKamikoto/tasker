package vault

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSameSlug(t *testing.T) {
	cases := []struct {
		base string
		slug string
		want bool
	}{
		{"plany.md", "plany", true},
		{"plany.md", "plany-na-osen", false},
		// Суффикс коллизии — то же имя: перекладывать plany-2 в plany-3 незачем.
		{"plany-2.md", "plany", true},
		{"plany-17.md", "plany", true},
		// А это уже другой слаг, просто с общим началом.
		{"plany-b.md", "plany", false},
		{"plany-na-osen.md", "plany", false},
		{"plany-.md", "plany", false},
		{"plan.md", "plany", false},
	}
	for _, c := range cases {
		if got := sameSlug(c.base, c.slug); got != c.want {
			t.Errorf("sameSlug(%q, %q) = %v, ожидалось %v", c.base, c.slug, got, c.want)
		}
	}
}

// Имя файла обязано следовать за заголовком.
func TestRenameFollowsTitle(t *testing.T) {
	v, root := testVault(t)

	n, err := v.Create(NewNote{Title: "Старый заголовок"})
	if err != nil {
		t.Fatal(err)
	}
	before := n.Path
	id := n.Doc.Meta.ID()

	if err := n.Doc.Meta.SetTitle("Планы на осень"); err != nil {
		t.Fatal(err)
	}
	if err := v.Save(n); err != nil {
		t.Fatal(err)
	}
	if err := v.Rename(n); err != nil {
		t.Fatalf("Rename: %v", err)
	}

	if filepath.Base(n.Path) != "plany-na-osen.md" {
		t.Errorf("имя файла %q, ожидалось plany-na-osen.md", filepath.Base(n.Path))
	}
	if _, err := os.Stat(before); !os.IsNotExist(err) {
		t.Errorf("старый файл остался: %v", err)
	}
	// id держит ссылки между заметками, и переименование его трогать не должно.
	loaded, err := v.Load(n.Path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Doc.Meta.ID() != id {
		t.Errorf("id поменялся: %s → %s", id, loaded.Doc.Meta.ID())
	}
	if got, _ := loaded.Doc.Meta.Title(); got != "Планы на осень" {
		t.Errorf("заголовок в файле %q", got)
	}
	_ = root
}

// Тот же заголовок — файл на месте, лишних переездов нет.
func TestRenameKeepsMatchingName(t *testing.T) {
	v, _ := testVault(t)

	n, err := v.Create(NewNote{Title: "Планы"})
	if err != nil {
		t.Fatal(err)
	}
	before := n.Path
	if err := v.Rename(n); err != nil {
		t.Fatal(err)
	}
	if n.Path != before {
		t.Errorf("файл переехал без причины: %s → %s", before, n.Path)
	}
}

// Занятое имя разрешается суффиксом, как и при создании.
func TestRenameResolvesCollision(t *testing.T) {
	v, _ := testVault(t)

	first, err := v.Create(NewNote{Title: "Планы"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := v.Create(NewNote{Title: "Другое"})
	if err != nil {
		t.Fatal(err)
	}

	if err := second.Doc.Meta.SetTitle("Планы"); err != nil {
		t.Fatal(err)
	}
	if err := v.Save(second); err != nil {
		t.Fatal(err)
	}
	if err := v.Rename(second); err != nil {
		t.Fatalf("Rename: %v", err)
	}

	if filepath.Base(second.Path) != "plany-2.md" {
		t.Errorf("имя файла %q, ожидалось plany-2.md", filepath.Base(second.Path))
	}
	if _, err := os.Stat(first.Path); err != nil {
		t.Errorf("первая заметка пострадала: %v", err)
	}
}

// Заголовок, из которого слаг не получается, оставляет имя как есть.
//
// Пустое имя файла хуже устаревшего, а «переименовать в ничего» — не то, чего
// человек просил, меняя заголовок на эмодзи.
func TestRenameKeepsNameWhenSlugIsEmpty(t *testing.T) {
	v, _ := testVault(t)

	n, err := v.Create(NewNote{Title: "Планы"})
	if err != nil {
		t.Fatal(err)
	}
	before := n.Path

	if err := n.Doc.Meta.SetTitle("🌿🌿🌿"); err != nil {
		t.Fatal(err)
	}
	if err := v.Save(n); err != nil {
		t.Fatal(err)
	}
	if err := v.Rename(n); err != nil {
		t.Fatalf("Rename: %v", err)
	}
	if n.Path != before {
		t.Errorf("файл переехал в %s", n.Path)
	}
}

// Переименование не выдаёт себя за правку содержимого.
func TestRenameKeepsUpdated(t *testing.T) {
	v, _ := testVault(t)

	n, err := v.Create(NewNote{Title: "Планы"})
	if err != nil {
		t.Fatal(err)
	}
	if err := n.Doc.Meta.SetTitle("Совсем другое"); err != nil {
		t.Fatal(err)
	}
	if err := v.Save(n); err != nil {
		t.Fatal(err)
	}
	before, err := n.Doc.Meta.Updated()
	if err != nil {
		t.Fatal(err)
	}
	if err := v.Rename(n); err != nil {
		t.Fatal(err)
	}
	after, err := n.Doc.Meta.Updated()
	if err != nil {
		t.Fatal(err)
	}
	if !before.Equal(after) {
		t.Errorf("updated поменялся при переименовании: %s → %s", before, after)
	}
}
