package vault

import (
	"errors"
	"strings"
	"testing"
	"time"
)

const sampleFM = `id: 01K3QF8ZN7X2WPBV4YHMC6TDAE
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
`

func mustParse(t *testing.T, raw string) *Frontmatter {
	t.Helper()
	f, err := ParseFrontmatter([]byte(raw))
	if err != nil {
		t.Fatalf("ParseFrontmatter: %v", err)
	}
	return f
}

func TestFrontmatterTypedGetters(t *testing.T) {
	f := mustParse(t, sampleFM)

	if got := f.ID(); got != "01K3QF8ZN7X2WPBV4YHMC6TDAE" {
		t.Errorf("ID = %q", got)
	}
	title, err := f.Title()
	if err != nil || title != "Перерасчёт значений свойств" {
		t.Errorf("Title = %q, %v", title, err)
	}
	status, err := f.Status()
	if err != nil || status != StatusActive {
		t.Errorf("Status = %q, %v", status, err)
	}
	tags, err := f.Tags()
	if err != nil {
		t.Fatalf("Tags: %v", err)
	}
	if len(tags) != 3 || tags[0] != "работа" || tags[2] != "баг" {
		t.Errorf("Tags = %v", tags)
	}
	pinned, err := f.Pinned()
	if err != nil || pinned {
		t.Errorf("Pinned = %v, %v", pinned, err)
	}
	origin, err := f.Origin()
	if err != nil || origin != OriginAgent {
		t.Errorf("Origin = %q, %v", origin, err)
	}
	created, err := f.Created()
	if err != nil {
		t.Fatalf("Created: %v", err)
	}
	want := time.Date(2026, 8, 18, 13, 12, 3, 0, time.FixedZone("", 3*3600))
	if !created.Equal(want) {
		t.Errorf("Created = %v, ожидалось %v", created, want)
	}
	ctx, err := f.Context()
	if err != nil {
		t.Fatalf("Context: %v", err)
	}
	if ctx == nil || ctx.Repo != "armz-frontend" || ctx.Branch != "feature/recalc" || ctx.Commit != "3f9a1c2" {
		t.Errorf("Context = %+v", ctx)
	}
}

// Отсутствующие поля — не ошибка. SPEC §4.2: нет status — значит none.
func TestFrontmatterMissingFields(t *testing.T) {
	f := mustParse(t, "title: Только заголовок\n")

	if got := f.ID(); got != "" {
		t.Errorf("ID = %q, ожидалась пустая строка", got)
	}
	if status, err := f.Status(); err != nil || status != StatusNone {
		t.Errorf("Status = %q, %v — отсутствие status должно давать none", status, err)
	}
	if tags, err := f.Tags(); err != nil || len(tags) != 0 {
		t.Errorf("Tags = %v, %v", tags, err)
	}
	if pinned, err := f.Pinned(); err != nil || pinned {
		t.Errorf("Pinned = %v, %v", pinned, err)
	}
	if origin, err := f.Origin(); err != nil || origin != OriginUser {
		t.Errorf("Origin = %q, %v — по умолчанию user", origin, err)
	}
	created, err := f.Created()
	if err != nil {
		t.Errorf("Created: %v", err)
	}
	if !created.IsZero() {
		t.Errorf("Created = %v, ожидался нулевой time", created)
	}
	ctx, err := f.Context()
	if err != nil || ctx != nil {
		t.Errorf("Context = %+v, %v — ожидался nil", ctx, err)
	}
}

// Сломанные руками значения должны диагностироваться, а не подставляться молча.
func TestFrontmatterInvalidValues(t *testing.T) {
	t.Run("статус вне перечисления", func(t *testing.T) {
		f := mustParse(t, "status: почтиГотово\n")
		if _, err := f.Status(); !errors.Is(err, ErrInvalidStatus) {
			t.Errorf("ожидалась ErrInvalidStatus, получено %v", err)
		}
	})
	t.Run("origin вне перечисления", func(t *testing.T) {
		f := mustParse(t, "origin: робот\n")
		if _, err := f.Origin(); !errors.Is(err, ErrInvalidOrigin) {
			t.Errorf("ожидалась ErrInvalidOrigin, получено %v", err)
		}
	})
	t.Run("created не по RFC 3339", func(t *testing.T) {
		f := mustParse(t, "created: вчера\n")
		if _, err := f.Created(); err == nil {
			t.Error("ожидалась ошибка разбора времени")
		}
	})
	t.Run("title не строка", func(t *testing.T) {
		f := mustParse(t, "title:\n  a: 1\n")
		if _, err := f.Title(); err == nil {
			t.Error("ожидалась ошибка: title должен быть строкой")
		}
	})
}

// Сердце пакета: правка одного поля не должна задевать ничего вокруг.
func TestFrontmatterSetPreservesEverything(t *testing.T) {
	src := `id: 01K3   # ULID, неизменяемый
title: Старый заголовок
# комментарий отдельной строкой
чужое_поле: значение
tags: [работа, баг]
pinned: false
вложенное_чужое:
  a: 1
  b: [x, y]
`
	f := mustParse(t, src)
	if err := f.SetTitle("Новый заголовок"); err != nil {
		t.Fatalf("SetTitle: %v", err)
	}

	want := `id: 01K3 # ULID, неизменяемый
title: Новый заголовок
# комментарий отдельной строкой
чужое_поле: значение
tags: [работа, баг]
pinned: false
вложенное_чужое:
  a: 1
  b: [x, y]
`
	if got := string(f.Bytes()); got != want {
		t.Errorf("--- ожидалось ---\n%s\n--- получено ---\n%s", want, got)
	}
}

// Порядок ключей не должен меняться от правок, иначе git-диффы превращаются в кашу.
func TestFrontmatterKeyOrderStable(t *testing.T) {
	f := mustParse(t, sampleFM)
	before := f.Keys()

	if err := f.SetStatus(StatusCompleted); err != nil {
		t.Fatalf("SetStatus: %v", err)
	}
	if err := f.SetPinned(true); err != nil {
		t.Fatalf("SetPinned: %v", err)
	}

	after := f.Keys()
	if len(before) != len(after) {
		t.Fatalf("число ключей изменилось: %v → %v", before, after)
	}
	for i := range before {
		if before[i] != after[i] {
			t.Errorf("порядок ключей поехал на позиции %d: %v → %v", i, before, after)
		}
	}
}

// Новый ключ дописывается в конец, а не втискивается в середину.
func TestFrontmatterSetNewKeyAppends(t *testing.T) {
	f := mustParse(t, "id: 01K3\ntitle: Заметка\n")
	if err := f.SetUpdated(time.Date(2026, 8, 19, 10, 0, 0, 0, time.FixedZone("", 3*3600))); err != nil {
		t.Fatalf("SetUpdated: %v", err)
	}
	keys := f.Keys()
	if len(keys) != 3 || keys[2] != "updated" {
		t.Fatalf("ключи = %v, ожидался updated в конце", keys)
	}
	got, err := f.Updated()
	if err != nil {
		t.Fatalf("Updated: %v", err)
	}
	if got.Format(time.RFC3339) != "2026-08-19T10:00:00+03:00" {
		t.Errorf("Updated = %s", got.Format(time.RFC3339))
	}
}

// tags: [a, b] не должны превращаться в блочный список при правке — это
// перепишет строку в git-диффе без причины.
func TestFrontmatterTagsKeepFlowStyle(t *testing.T) {
	f := mustParse(t, "tags: [работа, баг]\n")
	if err := f.SetTags([]string{"работа", "новый"}); err != nil {
		t.Fatalf("SetTags: %v", err)
	}
	want := "tags: [работа, новый]\n"
	if got := string(f.Bytes()); got != want {
		t.Errorf("ожидалось %q, получено %q", want, got)
	}
}

func TestFrontmatterTagsKeepBlockStyle(t *testing.T) {
	src := "tags:\n- работа\n- баг\n"
	f := mustParse(t, src)
	if err := f.SetTags([]string{"работа", "новый"}); err != nil {
		t.Fatalf("SetTags: %v", err)
	}
	want := "tags:\n- работа\n- новый\n"
	if got := string(f.Bytes()); got != want {
		t.Errorf("ожидалось %q, получено %q", want, got)
	}
}

func TestFrontmatterRemove(t *testing.T) {
	f := mustParse(t, "id: 01K3\ntrashedAt: 2026-08-19T10:00:00+03:00\ntitle: A\n")
	if !f.Remove("trashedAt") {
		t.Fatal("Remove вернул false для существующего ключа")
	}
	if f.Remove("trashedAt") {
		t.Error("Remove вернул true для уже удалённого ключа")
	}
	want := "id: 01K3\ntitle: A\n"
	if got := string(f.Bytes()); got != want {
		t.Errorf("ожидалось %q, получено %q", want, got)
	}
}

func TestFrontmatterGenericGetSet(t *testing.T) {
	f := mustParse(t, "чужое: 42\n")

	var n int
	ok, err := f.Get("чужое", &n)
	if err != nil || !ok || n != 42 {
		t.Fatalf("Get: %v %v %d", ok, err, n)
	}

	ok, err = f.Get("нет_такого", &n)
	if err != nil || ok {
		t.Errorf("Get для отсутствующего ключа: ok=%v err=%v — ожидалось false, nil", ok, err)
	}

	if err := f.Set("чужое", 43); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if got := string(f.Bytes()); got != "чужое: 43\n" {
		t.Errorf("получено %q", got)
	}
}

// Значение с двоеточием обязано быть заэкранировано, иначе файл перестанет
// разбираться при следующем чтении.
func TestFrontmatterEscapesDangerousValues(t *testing.T) {
	for _, title := range []string{
		"Заголовок: с двоеточием",
		"Заголовок с # решёткой",
		"- заголовок с дефиса",
		"[скобки]",
		"кавычки \"внутри\"",
		"",
	} {
		t.Run(title, func(t *testing.T) {
			f := mustParse(t, "id: 01K3\n")
			if err := f.SetTitle(title); err != nil {
				t.Fatalf("SetTitle: %v", err)
			}
			reparsed, err := ParseFrontmatter(f.Bytes())
			if err != nil {
				t.Fatalf("перечитывание: %v\nсодержимое:\n%s", err, f.Bytes())
			}
			got, err := reparsed.Title()
			if err != nil {
				t.Fatalf("Title: %v", err)
			}
			if got != title {
				t.Errorf("title после round-trip = %q, ожидалось %q", got, title)
			}
		})
	}
}

func TestFrontmatterEmpty(t *testing.T) {
	f := mustParse(t, "")
	if len(f.Keys()) != 0 {
		t.Errorf("Keys = %v для пустого frontmatter", f.Keys())
	}
	if got := string(f.Bytes()); got != "" {
		t.Errorf("Bytes = %q, ожидалась пустота", got)
	}
	if err := f.SetTitle("A"); err != nil {
		t.Fatalf("SetTitle на пустом: %v", err)
	}
	if got := string(f.Bytes()); got != "title: A\n" {
		t.Errorf("получено %q", got)
	}
}

// Неизменённый заголовок отдаётся ровно теми байтами, что пришли: рендер из AST
// нормализует пробелы перед хвостовыми комментариями и переписал бы строку в
// git-диффе на пустом месте.
func TestFrontmatterBytesUnchangedIsVerbatim(t *testing.T) {
	cases := []string{
		"id: 01K3   # три пробела перед комментарием\n",
		"title:    Заголовок с отступом\n",
		"tags:  [работа,  баг]\n",
		"a: 1\n\n\nb: 2\n",
		"# только комментарий\nkey: value\n",
		sampleFM,
	}
	for _, src := range cases {
		f := mustParse(t, src)
		if got := string(f.Bytes()); got != src {
			t.Errorf("неизменённый заголовок переписан\n--- было ---\n%s\n--- стало ---\n%s", src, got)
		}
	}
}

// Правка одного ключа не должна перерисовывать соседей: у остальных строк
// оформление обязано остаться прежним.
func TestFrontmatterSetTouchesOnlyTargetLine(t *testing.T) {
	src := "id: 01K3\ntitle: Старый\ntags:  [работа,  баг]\npinned: false\n"
	f := mustParse(t, src)
	if err := f.SetPinned(true); err != nil {
		t.Fatalf("SetPinned: %v", err)
	}
	got := string(f.Bytes())
	for _, keep := range []string{"id: 01K3", "title: Старый"} {
		if !strings.Contains(got, keep) {
			t.Errorf("строка %q не пережила правку соседнего ключа:\n%s", keep, got)
		}
	}
	if !strings.Contains(got, "pinned: true") {
		t.Errorf("pinned не изменился:\n%s", got)
	}
}

// Остальные поля схемы SPEC §4.2 и §4.3: записали — прочитали то же самое.
func TestFrontmatterRemainingFieldsRoundTrip(t *testing.T) {
	f := mustParse(t, "id: 01K3\n")
	created := time.Date(2026, 8, 18, 13, 12, 3, 0, time.FixedZone("", 3*3600))

	if err := f.SetCreated(created); err != nil {
		t.Fatalf("SetCreated: %v", err)
	}
	if err := f.SetOrigin(OriginAgent); err != nil {
		t.Fatalf("SetOrigin: %v", err)
	}
	if err := f.SetContext(Context{Repo: "tasker", Branch: "feature/x", Commit: "3f9a1c2", File: "internal/vault/frontmatter.go"}); err != nil {
		t.Fatalf("SetContext: %v", err)
	}
	if err := f.Set(fieldTrashedFrom, "Работа/Баги/note.md"); err != nil {
		t.Fatalf("Set trashedFrom: %v", err)
	}
	if err := f.setTimeField(fieldTrashedAt, created); err != nil {
		t.Fatalf("set trashedAt: %v", err)
	}

	reparsed, err := ParseFrontmatter(f.Bytes())
	if err != nil {
		t.Fatalf("перечитывание: %v\n%s", err, f.Bytes())
	}

	got, err := reparsed.Created()
	if err != nil || !got.Equal(created) {
		t.Errorf("Created = %v, %v", got, err)
	}
	if origin, err := reparsed.Origin(); err != nil || origin != OriginAgent {
		t.Errorf("Origin = %q, %v", origin, err)
	}
	ctx, err := reparsed.Context()
	if err != nil || ctx == nil || ctx.Repo != "tasker" || ctx.File != "internal/vault/frontmatter.go" {
		t.Errorf("Context = %+v, %v", ctx, err)
	}
	if from, err := reparsed.TrashedFrom(); err != nil || from != "Работа/Баги/note.md" {
		t.Errorf("TrashedFrom = %q, %v", from, err)
	}
	if at, err := reparsed.TrashedAt(); err != nil || !at.Equal(created) {
		t.Errorf("TrashedAt = %v, %v", at, err)
	}
}

// Сеттеры перечислений не должны пускать в файл значение, которое потом не
// прочитается: проверка на входе, а не на выходе.
func TestFrontmatterSettersRejectInvalidEnums(t *testing.T) {
	f := mustParse(t, "id: 01K3\n")

	if err := f.SetStatus(Status("почтиГотово")); !errors.Is(err, ErrInvalidStatus) {
		t.Errorf("SetStatus: ожидалась ErrInvalidStatus, получено %v", err)
	}
	if err := f.SetOrigin(Origin("робот")); !errors.Is(err, ErrInvalidOrigin) {
		t.Errorf("SetOrigin: ожидалась ErrInvalidOrigin, получено %v", err)
	}
	// Отвергнутое значение не должно осесть в заголовке.
	if got := string(f.Bytes()); got != "id: 01K3\n" {
		t.Errorf("заголовок изменился после отклонённой правки: %q", got)
	}
}

// Пустой frontmatter не должен падать на удалении и чтении.
func TestFrontmatterOperationsOnEmpty(t *testing.T) {
	f := mustParse(t, "")
	if f.Remove("что-угодно") {
		t.Error("Remove на пустом заголовке вернул true")
	}
	if ok, err := f.Get("что-угодно", new(string)); ok || err != nil {
		t.Errorf("Get на пустом: ok=%v err=%v", ok, err)
	}
	if id := f.ID(); id != "" {
		t.Errorf("ID на пустом = %q", id)
	}
}

// Заголовок, который разобрался как YAML, но отображением не является.
func TestParseFrontmatterNotAMapping(t *testing.T) {
	for _, src := range []string{"- список\n- вместо\n- карты\n", "просто строка\n", "42\n"} {
		if _, err := ParseFrontmatter([]byte(src)); !errors.Is(err, ErrInvalidFrontmatter) {
			t.Errorf("ParseFrontmatter(%q): ожидалась ErrInvalidFrontmatter, получено %v", src, err)
		}
	}
}

// Новая заметка должна выглядеть ровно так, как показано в SPEC §4.2: время без
// кавычек, теги в строку. Иначе каждая заметка расходится со спекой в двух
// строках, а git-диффы шумят.
func TestFrontmatterNewNoteMatchesSpecFormatting(t *testing.T) {
	f := mustParse(t, "id: 01K3QF8ZN7X2WPBV4YHMC6TDAE\n")
	created := time.Date(2026, 8, 19, 13, 12, 3, 0, time.FixedZone("", 3*3600))

	for _, step := range []struct {
		name string
		fn   func() error
	}{
		{"title", func() error { return f.SetTitle("Счётчик перерасчёта не обновляется") }},
		{"created", func() error { return f.SetCreated(created) }},
		{"status", func() error { return f.SetStatus(StatusActive) }},
		{"tags", func() error { return f.SetTags([]string{"работа", "баг"}) }},
		{"pinned", func() error { return f.SetPinned(false) }},
		{"origin", func() error { return f.SetOrigin(OriginAgent) }},
	} {
		if err := step.fn(); err != nil {
			t.Fatalf("%s: %v", step.name, err)
		}
	}

	want := `id: 01K3QF8ZN7X2WPBV4YHMC6TDAE
title: Счётчик перерасчёта не обновляется
created: 2026-08-19T13:12:03+03:00
status: active
tags: [работа, баг]
pinned: false
origin: agent
`
	if got := string(f.Bytes()); got != want {
		t.Errorf("--- ожидалось ---\n%s\n--- получено ---\n%s", want, got)
	}

	// И всё это обязано читаться обратно.
	back, err := ParseFrontmatter(f.Bytes())
	if err != nil {
		t.Fatalf("перечитывание: %v", err)
	}
	if got, err := back.Created(); err != nil || !got.Equal(created) {
		t.Errorf("Created после round-trip = %v, %v", got, err)
	}
	if tags, err := back.Tags(); err != nil || len(tags) != 2 || tags[0] != "работа" {
		t.Errorf("Tags после round-trip = %v, %v", tags, err)
	}
}

// Небезопасное значение обязано уйти обычным путём и получить кавычки, даже
// если его отдали в setScalarRaw.
func TestFrontmatterSetScalarRawFallsBackToEscaping(t *testing.T) {
	cases := []string{"значение: с двоеточием", "# решётка", "- дефис", "с пробелом", ""}
	for _, v := range cases {
		t.Run(v, func(t *testing.T) {
			f := mustParse(t, "id: 01K3\n")
			if err := f.setScalarRaw("поле", v); err != nil {
				t.Fatalf("setScalarRaw: %v", err)
			}
			back, err := ParseFrontmatter(f.Bytes())
			if err != nil {
				t.Fatalf("перечитывание: %v\n%s", err, f.Bytes())
			}
			var got string
			if _, err := back.Get("поле", &got); err != nil {
				t.Fatalf("Get: %v", err)
			}
			if got != v {
				t.Errorf("значение = %q, ожидалось %q", got, v)
			}
		})
	}
}
