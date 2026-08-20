package gitstore

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func testStore(t *testing.T) (*Store, string) {
	t.Helper()
	root := t.TempDir()
	s, err := Open(root)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	return s, root
}

func write(t *testing.T, root, rel, content string) string {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// gitOut запускает системный git — им проверяем результат, чтобы тест не
// доказывал сам себя тем же кодом, которым пользуется реализация.
func gitOut(t *testing.T, root string, args ...string) string {
	t.Helper()
	c := exec.Command("git", append([]string{"-c", "core.quotepath=false"}, args...)...)
	c.Dir = root
	out, err := c.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

func TestOpenInitialisesRepo(t *testing.T) {
	s, root := testStore(t)

	if _, err := os.Stat(filepath.Join(root, ".git")); err != nil {
		t.Fatalf("репозиторий не создан: %v", err)
	}
	if s.Root() != root {
		t.Errorf("Root() = %q", s.Root())
	}

	// Ветка должна называться так же, как её назвал бы современный git.
	if branch := strings.TrimSpace(gitOut(t, root, "branch", "--show-current")); branch != "main" {
		t.Errorf("ветка %q, ожидалась main", branch)
	}

	ignore, err := os.ReadFile(filepath.Join(root, ".gitignore"))
	if err != nil {
		t.Fatalf(".gitignore не создан: %v", err)
	}
	if !strings.Contains(string(ignore), ".tasker/index.sqlite") {
		t.Errorf(".gitignore не закрывает индекс:\n%s", ignore)
	}
}

// Повторное открытие ничего не пересоздаёт.
func TestOpenIsIdempotent(t *testing.T) {
	root := t.TempDir()
	if _, err := Open(root); err != nil {
		t.Fatal(err)
	}
	write(t, root, "заметка.md", "тело\n")
	s, err := Open(root)
	if err != nil {
		t.Fatalf("повторное Open: %v", err)
	}
	if _, err := s.Commit(context.Background(), "notes: заметка"); err != nil {
		t.Fatalf("Commit после повторного Open: %v", err)
	}
}

// Папка с уже написанным .gitignore, но без репозитория: репозиторий заводим,
// чужой файл не трогаем. Именно этот порядок и бывает у человека, который
// вёл заметки в папке до нас.
func TestOpenKeepsExistingIgnore(t *testing.T) {
	root := t.TempDir()
	write(t, root, ".gitignore", "мой собственный игнор\n")

	if _, err := Open(root); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(root, ".gitignore"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "мой собственный игнор\n" {
		t.Errorf("чужой .gitignore переписан:\n%s", got)
	}
}

func TestCommit(t *testing.T) {
	s, root := testStore(t)
	ctx := context.Background()

	write(t, root, "Работа/заметка.md", "тело\n")
	hash, err := s.Commit(ctx, "notes: заметка")
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if hash == "" {
		t.Fatal("Commit не вернул хеш")
	}

	log := gitOut(t, root, "log", "--oneline")
	if !strings.Contains(log, "notes: заметка") {
		t.Errorf("коммита нет в логе:\n%s", log)
	}
	files := gitOut(t, root, "show", "--name-only", "--format=", "HEAD")
	if !strings.Contains(files, "Работа/заметка.md") {
		t.Errorf("файла нет в коммите:\n%s", files)
	}
}

// Пустой коммит не создаётся: автокоммит просыпается по таймеру и не должен
// засорять историю, когда ничего не менялось.
func TestCommitNothingToDo(t *testing.T) {
	s, root := testStore(t)
	ctx := context.Background()

	write(t, root, "заметка.md", "тело\n")
	if _, err := s.Commit(ctx, "notes: первая"); err != nil {
		t.Fatal(err)
	}
	before := gitOut(t, root, "rev-list", "--count", "HEAD")

	hash, err := s.Commit(ctx, "notes: пусто")
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if hash != "" {
		t.Errorf("создан пустой коммит %s", hash)
	}
	if after := gitOut(t, root, "rev-list", "--count", "HEAD"); after != before {
		t.Errorf("число коммитов изменилось: %s → %s", before, after)
	}
}

// Индекс производный, его место не в истории (SPEC §4.1).
func TestCommitSkipsIndexFiles(t *testing.T) {
	s, root := testStore(t)
	ctx := context.Background()

	write(t, root, "заметка.md", "тело\n")
	write(t, root, ".tasker/index.sqlite", "двоичный мусор")
	write(t, root, ".tasker/index.sqlite-wal", "ещё мусор")
	write(t, root, ".DS_Store", "мусор macOS")

	if _, err := s.Commit(ctx, "notes: заметка"); err != nil {
		t.Fatal(err)
	}
	files := gitOut(t, root, "ls-files")
	for _, unwanted := range []string{"index.sqlite", ".DS_Store"} {
		if strings.Contains(files, unwanted) {
			t.Errorf("%s попал в историю:\n%s", unwanted, files)
		}
	}
	if !strings.Contains(files, "заметка.md") {
		t.Errorf("заметка не закоммичена:\n%s", files)
	}
}

func TestCommitRecordsDeletions(t *testing.T) {
	s, root := testStore(t)
	ctx := context.Background()

	path := write(t, root, "заметка.md", "тело\n")
	if _, err := s.Commit(ctx, "notes: заметка"); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Commit(ctx, "notes: удалили"); err != nil {
		t.Fatalf("Commit после удаления: %v", err)
	}

	if files := gitOut(t, root, "ls-files"); strings.Contains(files, "заметка.md") {
		t.Errorf("удалённый файл остался в индексе git:\n%s", files)
	}
}

func TestHistory(t *testing.T) {
	s, root := testStore(t)
	ctx := context.Background()

	write(t, root, "заметка.md", "первая версия\n")
	write(t, root, "другая.md", "не при чём\n")
	if _, err := s.Commit(ctx, "notes: первая"); err != nil {
		t.Fatal(err)
	}
	write(t, root, "заметка.md", "вторая версия\n")
	if _, err := s.Commit(ctx, "notes: вторая"); err != nil {
		t.Fatal(err)
	}
	write(t, root, "другая.md", "правка чужого файла\n")
	if _, err := s.Commit(ctx, "notes: третья"); err != nil {
		t.Fatal(err)
	}

	revs, err := s.History(ctx, "заметка.md", 10)
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	if len(revs) != 2 {
		t.Fatalf("ревизий %d, ожидалось 2: %+v", len(revs), revs)
	}
	if revs[0].Message != "notes: вторая" {
		t.Errorf("первой должна идти самая свежая, а идёт %q", revs[0].Message)
	}
	if revs[0].Hash == "" || revs[0].When.IsZero() || revs[0].Author == "" {
		t.Errorf("ревизия заполнена не полностью: %+v", revs[0])
	}
}

// --follow: история должна пережить переименование файла, иначе «История
// заметки» обрывается на каждом переименовании (SPEC §4.5).
func TestHistoryFollowsRenames(t *testing.T) {
	s, root := testStore(t)
	ctx := context.Background()

	write(t, root, "старое-имя.md", "первая версия\n")
	if _, err := s.Commit(ctx, "notes: создали"); err != nil {
		t.Fatal(err)
	}
	gitOut(t, root, "mv", "старое-имя.md", "новое-имя.md")
	if _, err := s.Commit(ctx, "notes: переименовали"); err != nil {
		t.Fatal(err)
	}
	write(t, root, "новое-имя.md", "вторая версия\n")
	if _, err := s.Commit(ctx, "notes: правка"); err != nil {
		t.Fatal(err)
	}

	revs, err := s.History(ctx, "новое-имя.md", 10)
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	if len(revs) != 3 {
		t.Fatalf("ревизий %d, ожидалось 3 — история оборвалась на переименовании: %+v", len(revs), revs)
	}
}

func TestHistoryLimit(t *testing.T) {
	s, root := testStore(t)
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		write(t, root, "заметка.md", strings.Repeat("x", i+1)+"\n")
		if _, err := s.Commit(ctx, "notes: правка"); err != nil {
			t.Fatal(err)
		}
	}
	revs, err := s.History(ctx, "заметка.md", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(revs) != 2 {
		t.Errorf("ревизий %d, ожидалось 2", len(revs))
	}
}

func TestHistoryUnknownFile(t *testing.T) {
	s, root := testStore(t)
	ctx := context.Background()
	write(t, root, "заметка.md", "тело\n")
	if _, err := s.Commit(ctx, "notes: заметка"); err != nil {
		t.Fatal(err)
	}

	revs, err := s.History(ctx, "нет-такой.md", 10)
	if err != nil {
		t.Errorf("History для неизвестного файла вернул ошибку: %v", err)
	}
	if len(revs) != 0 {
		t.Errorf("ревизий %d, ожидалось 0", len(revs))
	}
}

func TestDiffAndShow(t *testing.T) {
	s, root := testStore(t)
	ctx := context.Background()

	write(t, root, "заметка.md", "первая строка\n")
	first, err := s.Commit(ctx, "notes: первая")
	if err != nil {
		t.Fatal(err)
	}
	write(t, root, "заметка.md", "вторая строка\n")
	second, err := s.Commit(ctx, "notes: вторая")
	if err != nil {
		t.Fatal(err)
	}

	diff, err := s.Diff(ctx, "заметка.md", first, second)
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}
	if !strings.Contains(diff, "-первая строка") || !strings.Contains(diff, "+вторая строка") {
		t.Errorf("diff не показывает правку:\n%s", diff)
	}

	old, err := s.Show(ctx, "заметка.md", first)
	if err != nil {
		t.Fatalf("Show: %v", err)
	}
	if string(old) != "первая строка\n" {
		t.Errorf("Show вернул %q", old)
	}
}

// Пути наружу vault не принимаются ни одной операцией.
func TestRejectsPathsOutsideVault(t *testing.T) {
	s, root := testStore(t)
	ctx := context.Background()
	write(t, root, "заметка.md", "тело\n")
	rev, err := s.Commit(ctx, "notes: заметка")
	if err != nil {
		t.Fatal(err)
	}

	// Проверяем именно нашу ошибку: «любая ошибка» здесь ничего не значит,
	// потому что git и сам откажется работать с путём вне репозитория, и тест
	// проходил бы даже без проверки.
	for _, bad := range []string{"../снаружи.md", "/etc/passwd", "Работа/../../снаружи.md"} {
		if _, err := s.History(ctx, bad, 10); !errors.Is(err, ErrOutsideVault) {
			t.Errorf("History(%q) вернул %v, ожидалась ErrOutsideVault", bad, err)
		}
		if _, err := s.Show(ctx, bad, rev); !errors.Is(err, ErrOutsideVault) {
			t.Errorf("Show(%q) вернул %v, ожидалась ErrOutsideVault", bad, err)
		}
		if _, err := s.Diff(ctx, bad, rev, rev); !errors.Is(err, ErrOutsideVault) {
			t.Errorf("Diff(%q) вернул %v, ожидалась ErrOutsideVault", bad, err)
		}
	}
}

func TestNotesMessage(t *testing.T) {
	cases := []struct {
		titles []string
		want   string
	}{
		{nil, "notes: изменения"},
		{[]string{"Счётчик"}, "notes: Счётчик"},
		{[]string{"Счётчик", "План"}, "notes: 2 изменено"},
		{[]string{"а", "б", "в"}, "notes: 3 изменено"},
	}
	for _, c := range cases {
		if got := NotesMessage(c.titles); got != c.want {
			t.Errorf("NotesMessage(%v) = %q, ожидалось %q", c.titles, got, c.want)
		}
	}
}

func TestAgentMessage(t *testing.T) {
	if got := AgentMessage("create", "Счётчик не обновляется"); got != `agent: create "Счётчик не обновляется"` {
		t.Errorf("AgentMessage = %q", got)
	}
}

// Страховка от того, чего делать нельзя ни при каких условиях (CLAUDE.md).
// Восстановление версии — это запись поверх, а не reset.
func TestNoDestructiveGitCommands(t *testing.T) {
	forbidden := []string{"reset", "--hard", "clean", "checkout", "push", "--force"}
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, word := range forbidden {
			if strings.Contains(string(src), `"`+word+`"`) {
				t.Errorf("%s содержит запрещённый аргумент git %q", f, word)
			}
		}
	}
}

func TestOpenRejectsMissingRoot(t *testing.T) {
	if _, err := Open(filepath.Join(t.TempDir(), "нет")); err == nil {
		t.Error("ожидалась ошибка для несуществующей папки")
	}
}

var _ = time.Now

// Подпись берётся из настроек git, а не выдумывается. Проверить конкретное имя
// нельзя — оно зависит от машины, — но пустой подписи быть не должно, и если
// настройки есть, коммит обязан их использовать.
func TestCommitSignature(t *testing.T) {
	s, root := testStore(t)
	ctx := context.Background()

	gitOut(t, root, "config", "user.name", "Проверочный Автор")
	gitOut(t, root, "config", "user.email", "author@example.test")

	write(t, root, "заметка.md", "тело\n")
	if _, err := s.Commit(ctx, "notes: заметка"); err != nil {
		t.Fatal(err)
	}

	got := strings.TrimSpace(gitOut(t, root, "log", "-1", "--format=%an <%ae>"))
	if got != "Проверочный Автор <author@example.test>" {
		t.Errorf("подпись = %q — настройки репозитория не подхвачены", got)
	}
}
