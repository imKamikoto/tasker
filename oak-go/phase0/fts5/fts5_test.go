package fts5

import (
	"database/sql"
	"fmt"
	"math/rand"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func open(t *testing.T, path string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	for _, p := range []string{
		"PRAGMA journal_mode = WAL",
		"PRAGMA busy_timeout = 5000",
		"PRAGMA foreign_keys = ON",
	} {
		if _, err := db.Exec(p); err != nil {
			t.Fatalf("%s: %v", p, err)
		}
	}
	return db
}

// 1. Какая вообще версия SQLite внутри modernc и скомпилирован ли FTS5.
func TestVersionAndFTS5Available(t *testing.T) {
	db := open(t, ":memory:")

	var ver string
	if err := db.QueryRow("SELECT sqlite_version()").Scan(&ver); err != nil {
		t.Fatalf("version: %v", err)
	}
	t.Logf("SQLite version: %s", ver)

	rows, err := db.Query("PRAGMA compile_options")
	if err != nil {
		t.Fatalf("compile_options: %v", err)
	}
	defer rows.Close()
	var opts []string
	for rows.Next() {
		var o string
		if err := rows.Scan(&o); err != nil {
			t.Fatal(err)
		}
		opts = append(opts, o)
	}
	t.Logf("compile_options: %v", opts)

	// Главное: создаётся ли fts5 без build-тега.
	if _, err := db.Exec(`CREATE VIRTUAL TABLE t USING fts5(x, tokenize='trigram')`); err != nil {
		t.Fatalf("CREATE VIRTUAL TABLE ... fts5 trigram: %v", err)
	}
}

// 2. Главный вопрос: находится ли подстрока из середины слова. Русский и код.
func TestTrigramSubstringMatch(t *testing.T) {
	db := open(t, ":memory:")
	if _, err := db.Exec(`CREATE VIRTUAL TABLE t USING fts5(x, tokenize='trigram')`); err != nil {
		t.Fatalf("create: %v", err)
	}
	docs := []string{
		"Счётчик перерасчёта не обновляется после ручной правки значения",
		"func (s *Service) RecalculateHeaderValues(ctx context.Context) error {",
		"Работа/Баги — armz-frontend",
		"HTTP-запрос возвращает 500 при пустом теле",
	}
	for i, d := range docs {
		if _, err := db.Exec(`INSERT INTO t(rowid, x) VALUES(?, ?)`, i+1, d); err != nil {
			t.Fatalf("insert: %v", err)
		}
	}

	cases := []struct {
		name  string
		query string
		want  int // ожидаемый rowid, 0 = ничего не должно найтись
	}{
		{"русская подстрока из середины слова", `"ерерасч"`, 1},
		{"русский с ё", `"счёт"`, 1},
		{"подстрока внутри CamelCase", `"culateHeader"`, 2},
		{"подстрока внутри идентификатора", `"ctx cont"`, 2},
		{"дефис внутри слова", `"mz-front"`, 3},
		{"цифры", `"500 при"`, 4},
		{"два символа — не должно найтись", `"сч"`, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var ids []int
			rows, err := db.Query(`SELECT rowid FROM t WHERE t MATCH ? ORDER BY rowid`, c.query)
			if err != nil {
				if c.want == 0 {
					t.Logf("query %s → ошибка (ожидаемо для <3 символов): %v", c.query, err)
					return
				}
				t.Fatalf("query %s: %v", c.query, err)
			}
			for rows.Next() {
				var id int
				rows.Scan(&id)
				ids = append(ids, id)
			}
			rows.Close()
			if c.want == 0 {
				if len(ids) != 0 {
					t.Errorf("query %s: ожидалось пусто, получено %v", c.query, ids)
				}
				return
			}
			found := false
			for _, id := range ids {
				if id == c.want {
					found = true
				}
			}
			if !found {
				t.Errorf("query %s: ожидался rowid %d, получено %v", c.query, c.want, ids)
			}
		})
	}
}

// 3. Регистронезависимость для кириллицы. Для пользователя с русским основным
// это не мелочь: если фолдинг только ASCII, поиск "счётчик" не найдёт "Счётчик".
func TestTrigramCaseFoldingCyrillic(t *testing.T) {
	db := open(t, ":memory:")
	if _, err := db.Exec(`CREATE VIRTUAL TABLE t USING fts5(x, tokenize='trigram')`); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO t(rowid, x) VALUES(1, ?)`, "Счётчик Перерасчёта СЛОМАЛСЯ"); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{`"счётчик"`, `"СЧЁТЧИК"`, `"ерерасчёт"`, `"сломался"`, `"Сломался"`} {
		var n int
		if err := db.QueryRow(`SELECT count(*) FROM t WHERE t MATCH ?`, q).Scan(&n); err != nil {
			t.Fatalf("query %s: %v", q, err)
		}
		if n != 1 {
			t.Errorf("query %s: найдено %d, ожидалось 1 — регистронезависимости для кириллицы НЕТ", q, n)
		} else {
			t.Logf("query %s: ok", q)
		}
	}
}

// 4. Contentless-таблица (content='' из SPEC §5.1): можно ли вообще удалять строки.
// Без contentless_delete=1 удаление требует знать старое содержимое — а файла
// на диске уже нет. Это влияет на схему индекса, поэтому проверяем сейчас.
func TestContentlessDelete(t *testing.T) {
	db := open(t, ":memory:")

	t.Run("contentless_delete=1 поддерживается", func(t *testing.T) {
		_, err := db.Exec(`CREATE VIRTUAL TABLE cd USING fts5(title, body, tokenize='trigram', content='', contentless_delete=1)`)
		if err != nil {
			t.Fatalf("НЕ поддерживается: %v", err)
		}
		if _, err := db.Exec(`INSERT INTO cd(rowid, title, body) VALUES(1, ?, ?)`, "Заголовок", "Тело заметки про перерасчёт"); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`DELETE FROM cd WHERE rowid = 1`); err != nil {
			t.Fatalf("простой DELETE не работает: %v", err)
		}
		var n int
		db.QueryRow(`SELECT count(*) FROM cd WHERE cd MATCH ?`, `"перерасч"`).Scan(&n)
		if n != 0 {
			t.Errorf("после DELETE найдено %d", n)
		}
		// И UPDATE — то, что происходит при каждом сохранении заметки.
		if _, err := db.Exec(`INSERT INTO cd(rowid, title, body) VALUES(2, ?, ?)`, "Второй", "старый текст"); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`INSERT INTO cd(rowid, title, body) VALUES(2, ?, ?) ON CONFLICT(rowid) DO NOTHING`, "x", "y"); err != nil {
			t.Logf("ON CONFLICT на contentless: %v (ожидаемо, апдейт делается delete+insert)", err)
		}
		if _, err := db.Exec(`DELETE FROM cd WHERE rowid = 2`); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`INSERT INTO cd(rowid, title, body) VALUES(2, ?, ?)`, "Второй", "новый текст"); err != nil {
			t.Fatal(err)
		}
		var m int
		db.QueryRow(`SELECT count(*) FROM cd WHERE cd MATCH ?`, `"новый"`).Scan(&m)
		if m != 1 {
			t.Errorf("после delete+insert найдено %d, ожидалось 1", m)
		}
	})

	t.Run("поиск по конкретной колонке", func(t *testing.T) {
		if _, err := db.Exec(`INSERT INTO cd(rowid, title, body) VALUES(10, ?, ?)`, "Перерасчёт значений", "тело без ключевого слова"); err != nil {
			t.Fatal(err)
		}
		var n int
		if err := db.QueryRow(`SELECT count(*) FROM cd WHERE cd MATCH ?`, `title:"ерерасч"`).Scan(&n); err != nil {
			t.Fatalf("title: %v", err)
		}
		if n != 1 {
			t.Errorf("title:-поиск нашёл %d, ожидалось 1", n)
		}
		if err := db.QueryRow(`SELECT count(*) FROM cd WHERE cd MATCH ?`, `body:"ерерасч"`).Scan(&n); err != nil {
			t.Fatalf("body: %v", err)
		}
		if n != 0 {
			t.Errorf("body:-поиск нашёл %d, ожидалось 0", n)
		}
	})
}

// 5. Масштаб из SPEC: 10 000 заметок. Сколько строится индекс, сколько весит,
// сколько занимает поиск.
func TestScale10kNotes(t *testing.T) {
	if testing.Short() {
		t.Skip("short")
	}
	dir := t.TempDir()
	db := open(t, dir+"/index.sqlite")
	if _, err := db.Exec(`CREATE VIRTUAL TABLE notes_fts USING fts5(title, body, tokenize='trigram', content='', contentless_delete=1)`); err != nil {
		t.Fatalf("create: %v", err)
	}

	words := []string{
		"перерасчёт", "значение", "счётчик", "заголовок", "запрос", "ошибка", "рефакторинг",
		"сервис", "клиент", "миграция", "индекс", "поиск", "заметка", "задача", "проверка",
		"handler", "context", "request", "parser", "buffer", "commit", "branch", "config",
	}
	rnd := rand.New(rand.NewSource(42))
	para := func(n int) string {
		s := ""
		for i := 0; i < n; i++ {
			s += words[rnd.Intn(len(words))] + " "
		}
		return s
	}

	start := time.Now()
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	stmt, err := tx.Prepare(`INSERT INTO notes_fts(rowid, title, body) VALUES(?, ?, ?)`)
	if err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 10000; i++ {
		// ~2 КБ тела — реалистичная рабочая заметка
		if _, err := stmt.Exec(i, para(6), para(300)); err != nil {
			t.Fatal(err)
		}
	}
	stmt.Close()
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	build := time.Since(start)

	// Одна редкая иголка, чтобы искать именно её
	if _, err := db.Exec(`INSERT INTO notes_fts(rowid, title, body) VALUES(99999, ?, ?)`,
		"Счётчик перерасчёта не обновляется", "уникальная строка йцукенгшщз внутри тела"); err != nil {
		t.Fatal(err)
	}

	var pageCount, pageSize int64
	db.QueryRow("PRAGMA page_count").Scan(&pageCount)
	db.QueryRow("PRAGMA page_size").Scan(&pageSize)

	queries := []string{`"йцукенгш"`, `"ерерасч"`, `"context"`, `"счётчик" AND "запрос"`}
	timings := map[string]string{}
	for _, q := range queries {
		s := time.Now()
		var n int
		if err := db.QueryRow(`SELECT count(*) FROM notes_fts WHERE notes_fts MATCH ?`, q).Scan(&n); err != nil {
			t.Fatalf("query %s: %v", q, err)
		}
		timings[q] = fmt.Sprintf("%v (%d совпадений)", time.Since(s).Round(time.Microsecond), n)
	}

	t.Logf("построение индекса 10k заметок по ~2КБ: %v", build.Round(time.Millisecond))
	t.Logf("размер файла индекса: %.1f МБ", float64(pageCount*pageSize)/1024/1024)
	for _, q := range queries {
		t.Logf("поиск %-28s → %s", q, timings[q])
	}
}
