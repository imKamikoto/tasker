package gitstore

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// defaultIgnore — что не место в истории. Пишется только при создании
// репозитория; чужой .gitignore не трогается.
const defaultIgnore = `# Индекс производный: сносится и строится заново из файлов
.tasker/index.sqlite
.tasker/index.sqlite-shm
.tasker/index.sqlite-wal

.DS_Store
`

// Подпись на случай, когда в git не настроен пользователь. Коммиты локальные и
// никуда не уезжают, так что заглушка никому не попадётся на глаза.
const (
	fallbackName  = "Tasker"
	fallbackEmail = "tasker@localhost"
)

// ErrOutsideVault — путь ведёт за пределы vault. Отдельная ошибка, а не просто
// «git не смог»: путь уходит в аргументы команды, и отличать свою проверку от
// чужого отказа обязательно.
var ErrOutsideVault = errors.New("path outside vault")

// Store — история vault.
//
// Запись идёт через go-git: на инкрементальном коммите он быстрее системного
// git, потому что не платит за запуск процесса (замерено 2026-08-20: 18 мс
// против 118 мс на 2000 заметок). Чтение истории и diff — через системный git:
// у go-git нет аналога --follow, а без него «История заметки» обрывается на
// каждом переименовании.
type Store struct {
	root string
	repo *git.Repository

	// mu сериализует запись: коммитить могут и автокоммит по таймеру, и
	// принудительный сброс при выходе, и агент через MCP.
	mu sync.Mutex
}

// Revision — одна запись истории файла.
type Revision struct {
	Hash    string
	Author  string
	When    time.Time
	Message string
}

// Open открывает репозиторий vault, создавая его при первом запуске.
func Open(root string) (*Store, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("open git store %s: %w", root, err)
	}
	if info, err := os.Stat(abs); err != nil {
		return nil, fmt.Errorf("open git store %s: %w", root, err)
	} else if !info.IsDir() {
		return nil, fmt.Errorf("open git store %s: not a directory", root)
	}

	repo, err := git.PlainOpen(abs)
	if err == nil {
		return &Store{root: abs, repo: repo}, nil
	}
	if err != git.ErrRepositoryNotExists {
		return nil, fmt.Errorf("open git store %s: %w", root, err)
	}

	repo, err = git.PlainInitWithOptions(abs, &git.PlainInitOptions{
		// Имя ветки задаём явно: go-git по умолчанию берёт master, а
		// современный git — main, и расхождение видно в любой команде.
		InitOptions: git.InitOptions{DefaultBranch: plumbing.Main},
	})
	if err != nil {
		return nil, fmt.Errorf("init git store %s: %w", root, err)
	}

	ignore := filepath.Join(abs, ".gitignore")
	if _, err := os.Stat(ignore); os.IsNotExist(err) {
		if err := os.WriteFile(ignore, []byte(defaultIgnore), 0o644); err != nil {
			return nil, fmt.Errorf("init git store %s: %w", root, err)
		}
	}

	return &Store{root: abs, repo: repo}, nil
}

// Root возвращает путь к vault.
func (s *Store) Root() string { return s.root }

// Commit фиксирует всё, что изменилось, и возвращает хеш.
//
// Пустая строка означает, что коммитить было нечего: автокоммит просыпается по
// таймеру и не должен засорять историю пустыми коммитами.
func (s *Store) Commit(ctx context.Context, message string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := ctx.Err(); err != nil {
		return "", err
	}

	wt, err := s.repo.Worktree()
	if err != nil {
		return "", fmt.Errorf("commit: %w", err)
	}
	if err := wt.AddWithOptions(&git.AddOptions{All: true}); err != nil {
		return "", fmt.Errorf("commit: %w", err)
	}

	status, err := wt.Status()
	if err != nil {
		return "", fmt.Errorf("commit: %w", err)
	}
	if status.IsClean() {
		return "", nil
	}

	hash, err := wt.Commit(message, &git.CommitOptions{Author: s.signature()})
	if err != nil {
		return "", fmt.Errorf("commit: %w", err)
	}
	return hash.String(), nil
}

// signature берёт имя и почту из настроек git, а если их там нет — подставляет
// заглушку. Коммит важнее, чем правильная подпись под ним: без него потеряется
// работа пользователя.
func (s *Store) signature() *object.Signature {
	sig := &object.Signature{Name: fallbackName, Email: fallbackEmail, When: time.Now()}
	// LocalScope, а не Global: он включает и системные, и глобальные, и
	// репозиторные настройки — то есть ровно то, что увидел бы сам git.
	cfg, err := s.repo.ConfigScoped(config.LocalScope)
	if err != nil {
		return sig
	}
	if cfg.User.Name != "" {
		sig.Name = cfg.User.Name
	}
	if cfg.User.Email != "" {
		sig.Email = cfg.User.Email
	}
	return sig
}

// History возвращает историю файла, новые ревизии первыми.
//
// Через системный git, а не go-git: --follow там не реализован, а без него
// история обрывается на каждом переименовании — то есть ровно там, где она
// нужнее всего.
func (s *Store) History(ctx context.Context, path string, limit int) ([]Revision, error) {
	rel, err := s.relative(path)
	if err != nil {
		return nil, err
	}

	args := []string{"log", "--follow", "--format=%H%x1f%an%x1f%aI%x1f%s%x1e"}
	if limit > 0 {
		args = append(args, "-n", strconv.Itoa(limit))
	}
	args = append(args, "--", rel)

	out, err := s.git(ctx, args...)
	if err != nil {
		return nil, err
	}

	var revs []Revision
	for _, record := range strings.Split(out, "\x1e") {
		record = strings.TrimLeft(record, "\n")
		if record == "" {
			continue
		}
		fields := strings.Split(record, "\x1f")
		if len(fields) != 4 {
			return nil, fmt.Errorf("history %s: неразборчивая запись %q", path, record)
		}
		when, err := time.Parse(time.RFC3339, fields[2])
		if err != nil {
			return nil, fmt.Errorf("history %s: %w", path, err)
		}
		revs = append(revs, Revision{
			Hash:    fields[0],
			Author:  fields[1],
			When:    when,
			Message: fields[3],
		})
	}
	return revs, nil
}

// Diff показывает, что изменилось в файле между двумя ревизиями.
func (s *Store) Diff(ctx context.Context, path, from, to string) (string, error) {
	rel, err := s.relative(path)
	if err != nil {
		return "", err
	}
	return s.git(ctx, "diff", from, to, "--", rel)
}

// Show возвращает содержимое файла на указанной ревизии.
//
// Это основа «Восстановить эту версию»: старое содержимое читается и пишется
// поверх текущего обычной записью. Никаких reset и checkout — история должна
// сохранять и то, что мы откатили.
func (s *Store) Show(ctx context.Context, path, rev string) ([]byte, error) {
	rel, err := s.relative(path)
	if err != nil {
		return nil, err
	}
	out, err := s.git(ctx, "show", rev+":"+rel)
	if err != nil {
		return nil, err
	}
	return []byte(out), nil
}

// git запускает системный git в корне vault.
func (s *Store) git(ctx context.Context, args ...string) (string, error) {
	// core.quotepath=false — иначе кириллица в путях выезжает восьмеричными
	// последовательностями, и человек видит их в diff вместо имён файлов.
	cmd := exec.CommandContext(ctx, "git", append([]string{"-c", "core.quotepath=false"}, args...)...)
	cmd.Dir = s.root

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}

// relative приводит путь к относительному внутри vault и отвергает всё, что
// ведёт наружу: путь уходит в аргументы git, и выпускать его за пределы vault
// нельзя.
func (s *Store) relative(path string) (string, error) {
	p := path
	if !filepath.IsAbs(p) {
		p = filepath.Join(s.root, filepath.FromSlash(p))
	}
	rel, err := filepath.Rel(s.root, filepath.Clean(p))
	if err != nil {
		return "", fmt.Errorf("path %q: %w", path, ErrOutsideVault)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path %q: %w", path, ErrOutsideVault)
	}
	return filepath.ToSlash(rel), nil
}

// NotesMessage — сообщение коммита для правок пользователя (SPEC §4.5).
func NotesMessage(titles []string) string {
	switch len(titles) {
	case 0:
		return "notes: изменения"
	case 1:
		return "notes: " + titles[0]
	default:
		return fmt.Sprintf("notes: %d изменено", len(titles))
	}
}

// AgentMessage — сообщение коммита для операции агента (SPEC §4.5).
func AgentMessage(action, title string) string {
	return fmt.Sprintf("agent: %s %q", action, title)
}
