// Command tasker-scan — отладочный CLI поверх ядра: строит индекс по папке и
// отвечает на запросы, не поднимая ни окна, ни вебвью.
//
// Он существует не ради удобства, а как доказательство того, что граница
// проведена правильно: если поиск работает отсюда, значит логика действительно
// в Go. В день, когда tasker-scan перестанет собираться без фронтенда, граница
// поехала.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"tasker/internal/index"
	"tasker/internal/vault"
)

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "tasker-scan:", err)
		os.Exit(1)
	}
}

func run(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("tasker-scan", flag.ContinueOnError)
	fs.SetOutput(stderr)
	limit := fs.Int("limit", 20, "сколько заметок показать")
	trashed := fs.Bool("trashed", false, "искать и в корзине")
	noScan := fs.Bool("no-scan", false, "не сканировать, искать по тому, что уже в индексе")
	fs.Usage = func() {
		fmt.Fprintf(stderr, "usage: tasker-scan [флаги] <путь-к-vault> [запрос]\n\n")
		fmt.Fprintf(stderr, "Строит индекс по папке с заметками и отвечает на запросы\n")
		fmt.Fprintf(stderr, "на языке из docs/SPEC.md §8.5. Примеры запросов:\n")
		fmt.Fprintf(stderr, "  счётчик tag:баг -status:completed\n")
		fmt.Fprintf(stderr, "  book:Работа is:pinned\n")
		fmt.Fprintf(stderr, "  \"точная фраза\" has:task\n\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() < 1 {
		fs.Usage()
		return fmt.Errorf("не указан путь к vault")
	}

	root := fs.Arg(0)
	query := strings.Join(fs.Args()[1:], " ")

	// Запрос разбираем до открытия чего-либо: опечатку в нём надо показать
	// сразу, а не после минуты индексации.
	q, err := index.ParseQuery(query)
	if err != nil {
		return err
	}

	ctx := context.Background()

	v, err := vault.Open(root)
	if err != nil {
		return err
	}
	ix, err := openIndex(ctx, v)
	if err != nil {
		return err
	}
	defer ix.Close()

	if !*noScan {
		if err := doScan(ctx, ix, v, stdout); err != nil {
			return err
		}
	}
	if query == "" {
		return nil
	}
	return doSearch(ctx, ix, q, index.SearchOptions{Limit: *limit, IncludeTrashed: *trashed}, stdout)
}

// openIndex открывает индекс в служебном каталоге vault (SPEC §4.1).
func openIndex(ctx context.Context, v *vault.Vault) (*index.Index, error) {
	dir := filepath.Join(v.Root(), ".tasker")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create %s: %w", dir, err)
	}
	return index.Open(ctx, filepath.Join(dir, "index.sqlite"))
}

func doScan(ctx context.Context, ix *index.Index, v *vault.Vault, out io.Writer) error {
	start := time.Now()
	res, err := ix.Scan(ctx, v)
	if err != nil {
		return err
	}

	fmt.Fprintf(out, "индекс: +%d ~%d -%d, без изменений %d · %s\n",
		res.Added, res.Updated, res.Removed, res.Unchanged, took(start))

	if res.Backfilled > 0 {
		fmt.Fprintf(out, "дописан frontmatter в %d %s на диске\n",
			res.Backfilled, plural(res.Backfilled, "файл", "файла", "файлов"))
	}
	if len(res.Failed) > 0 {
		fmt.Fprintf(out, "не прочитано %d:\n", len(res.Failed))
		for _, f := range res.Failed {
			fmt.Fprintf(out, "  %s: %v\n", f.Path, f.Err)
		}
	}
	return nil
}

func doSearch(ctx context.Context, ix *index.Index, q index.Query, opts index.SearchOptions, out io.Writer) error {
	start := time.Now()
	found, err := ix.Search(ctx, q, opts)
	if err != nil {
		return err
	}

	fmt.Fprintf(out, "\nнайдено %d · %s\n", len(found), took(start))
	for _, r := range found {
		fmt.Fprintf(out, "\n%s %s\n", pinMark(r.Pinned), r.Title)
		fmt.Fprintf(out, "  %s\n", strings.Join(details(r), " · "))
		if r.Excerpt != "" {
			fmt.Fprintf(out, "  %s\n", cut(r.Excerpt, 100))
		}
	}
	return nil
}

// details собирает вторую строку карточки, пропуская всё, чего у заметки нет.
func details(r index.Record) []string {
	parts := []string{r.Path}
	if r.Status != string(vault.StatusNone) {
		parts = append(parts, r.Status)
	}
	if len(r.Tags) > 0 {
		parts = append(parts, "#"+strings.Join(r.Tags, " #"))
	}
	if r.NumTasks > 0 {
		parts = append(parts, fmt.Sprintf("%d/%d", r.NumDone, r.NumTasks))
	}
	if r.Trashed {
		parts = append(parts, "в корзине")
	}
	return append(parts, r.Updated.Format("02.01.2006 15:04"))
}

func pinMark(pinned bool) string {
	if pinned {
		return "★"
	}
	return "·"
}

func cut(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return strings.TrimSpace(string(runes[:max])) + "…"
}

func took(start time.Time) string {
	d := time.Since(start)
	if d < time.Millisecond {
		return d.Round(10 * time.Microsecond).String()
	}
	return d.Round(time.Millisecond).String()
}

// plural — русские окончания для счётчиков: «1 файл», «2 файла», «5 файлов».
func plural(n int, one, few, many string) string {
	mod100 := n % 100
	if mod100 >= 11 && mod100 <= 14 {
		return many
	}
	switch n % 10 {
	case 1:
		return one
	case 2, 3, 4:
		return few
	default:
		return many
	}
}
