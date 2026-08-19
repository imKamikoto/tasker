package vault

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Ошибки, которые вызывающий обязан различать.
var (
	// ErrNoteNotFound — заметки по такому пути нет.
	ErrNoteNotFound = errors.New("note not found")
	// ErrOutsideVault — путь ведёт за пределы vault.
	ErrOutsideVault = errors.New("path outside vault")
	// ErrEmptyTitle — заголовок пустой. Он единственный источник имени файла и
	// заголовка заметки, подставлять вместо него что-то своё нельзя.
	ErrEmptyTitle = errors.New("empty title")
	// ErrHiddenPath — путь ведёт в скрытый каталог. Такие vault игнорирует
	// (SPEC §4.1), и заметка в них просто не появится в приложении.
	ErrHiddenPath = errors.New("hidden path")
)

// notePerm — права на файлы заметок.
const notePerm = 0o644

// Vault — папка с заметками.
type Vault struct {
	root string

	// now вынесено в поле, чтобы тесты могли зафиксировать время: updated
	// проставляется здесь, и проверить его иначе нечем.
	now func() time.Time
}

// Note — заметка вместе с её местом в vault.
type Note struct {
	// Doc — содержимое файла: заголовок и тело.
	Doc *Document
	// Path — абсолютный путь к файлу.
	Path string
	// Notebook — путь ноутбука относительно корня vault, пустой для корня.
	Notebook string
	// ModTime и Size нужны индексу для инкрементального скана (SPEC §5.2).
	ModTime time.Time
	Size    int64
}

// NewNote — параметры создания заметки.
type NewNote struct {
	Title    string
	Body     string
	Notebook string
	Tags     []string
	Status   Status
	Pinned   bool
	Origin   Origin
	Context  *Context
}

// Open открывает vault по пути к каталогу.
//
// Корень сразу разрешается через EvalSymlinks: все проверки вложенности
// сравнивают реальные пути, и если корень останется неразрешённым, они начнут
// врать на первой же символической ссылке в родительских каталогах — на macOS
// это /var, который сам ссылка на /private/var.
func Open(root string) (*Vault, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("open vault %s: %w", root, err)
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return nil, fmt.Errorf("open vault %s: %w", root, err)
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return nil, fmt.Errorf("open vault %s: %w", root, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("open vault %s: not a directory", root)
	}
	return &Vault{root: resolved, now: time.Now}, nil
}

// Root возвращает абсолютный разрешённый путь к корню vault.
func (v *Vault) Root() string { return v.root }

// Load читает заметку. Путь может быть относительным к корню vault или
// абсолютным; в обоих случаях он проверяется на выход за пределы.
func (v *Vault) Load(path string) (*Note, error) {
	abs, err := v.resolveExisting(path)
	if err != nil {
		return nil, err
	}

	f, err := os.Open(abs)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("load note %s: %w", path, ErrNoteNotFound)
		}
		return nil, fmt.Errorf("load note %s: %w", path, err)
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("stat note %s: %w", path, err)
	}
	src := make([]byte, info.Size())
	if _, err := io.ReadFull(f, src); err != nil {
		return nil, fmt.Errorf("read note %s: %w", path, err)
	}

	doc, err := ParseDocument(src)
	if err != nil {
		return nil, fmt.Errorf("load note %s: %w", path, err)
	}

	return &Note{
		Doc:      doc,
		Path:     abs,
		Notebook: v.notebookOf(abs),
		ModTime:  info.ModTime(),
		Size:     info.Size(),
	}, nil
}

// Save записывает заметку на диск, если она менялась, и проставляет updated.
//
// Неизменённая заметка не перезаписывается: лишняя запись — это событие
// watcher'а, коммит в git и сдвинутый mtime на пустом месте.
func (v *Vault) Save(n *Note) error {
	if !n.Doc.Modified() {
		return nil
	}
	if err := n.Doc.Meta.SetUpdated(v.now()); err != nil {
		return fmt.Errorf("save note %s: %w", n.Path, err)
	}

	data := n.Doc.Bytes()
	if err := writeFileAtomic(n.Path, data, notePerm); err != nil {
		return fmt.Errorf("save note %s: %w", n.Path, err)
	}
	n.Doc.markClean()

	info, err := os.Stat(n.Path)
	if err != nil {
		return fmt.Errorf("stat note %s: %w", n.Path, err)
	}
	n.ModTime = info.ModTime()
	n.Size = info.Size()
	return nil
}

// Create создаёт заметку: подбирает свободное имя файла, собирает frontmatter в
// порядке из SPEC §4.2 и пишет файл. Ноутбук создаётся, если его нет.
func (v *Vault) Create(n NewNote) (*Note, error) {
	title := strings.TrimSpace(n.Title)
	if title == "" {
		return nil, fmt.Errorf("create note: %w", ErrEmptyTitle)
	}

	dir, err := v.ensureNotebook(n.Notebook)
	if err != nil {
		return nil, err
	}

	id := NewID()
	slug := Slug(title)
	if slug == "" {
		// Заголовок из одних эмодзи или иероглифов. Имя из id читается хуже, но
		// оно всегда есть, всегда уникально и по нему заметка находится.
		slug = strings.ToLower(id)
	}

	doc, err := buildDocument(id, title, n, v.now())
	if err != nil {
		return nil, fmt.Errorf("create note %q: %w", title, err)
	}

	path, err := createUnique(dir, slug, doc.Bytes(), notePerm)
	if err != nil {
		return nil, fmt.Errorf("create note %q: %w", title, err)
	}

	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("stat note %s: %w", path, err)
	}

	return &Note{
		Doc:      doc,
		Path:     path,
		Notebook: v.notebookOf(path),
		ModTime:  info.ModTime(),
		Size:     info.Size(),
	}, nil
}

// buildDocument собирает заметку с нуля. Порядок вызовов и есть порядок ключей
// в файле: Set дописывает новый ключ в конец, а фиксированный порядок нужен
// ради чистых git-диффов (SPEC §4.2).
func buildDocument(id, title string, n NewNote, now time.Time) (*Document, error) {
	status := n.Status
	if status == "" {
		status = StatusNone
	}
	origin := n.Origin
	if origin == "" {
		origin = OriginUser
	}
	tags := n.Tags
	if tags == nil {
		tags = []string{}
	}

	doc, err := ParseDocument(nil)
	if err != nil {
		return nil, err
	}
	f := doc.Meta

	if err := f.setScalarRaw(fieldID, id); err != nil {
		return nil, err
	}
	if err := f.SetTitle(title); err != nil {
		return nil, err
	}
	if err := f.SetCreated(now); err != nil {
		return nil, err
	}
	if err := f.SetUpdated(now); err != nil {
		return nil, err
	}
	if err := f.SetStatus(status); err != nil {
		return nil, err
	}
	if err := f.SetTags(tags); err != nil {
		return nil, err
	}
	if err := f.SetPinned(n.Pinned); err != nil {
		return nil, err
	}
	if err := f.SetOrigin(origin); err != nil {
		return nil, err
	}
	if n.Context != nil {
		if err := f.SetContext(*n.Context); err != nil {
			return nil, err
		}
	}

	doc.Body = n.Body
	return doc, nil
}

// ensureNotebook проверяет путь ноутбука и создаёт каталог.
//
// Порядок важен: сначала лексическая проверка, потом создание, потом проверка
// реального пути. Пока каталога нет, EvalSymlinks по нему не отработает, а
// проверять только лексически недостаточно — промежуточный компонент может
// оказаться ссылкой наружу.
func (v *Vault) ensureNotebook(notebook string) (string, error) {
	clean, err := v.cleanRelative(notebook)
	if err != nil {
		return "", err
	}

	dir := filepath.Join(v.root, clean)

	// Проверка до создания: MkdirAll сквозь символическую ссылку наружу создал
	// бы каталог за пределами vault, и отменять это было бы уже поздно.
	if err := v.checkAncestor(dir, notebook); err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create notebook %q: %w", notebook, err)
	}

	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return "", fmt.Errorf("create notebook %q: %w", notebook, err)
	}
	if !v.contains(resolved) {
		return "", fmt.Errorf("create notebook %q: %w", notebook, ErrOutsideVault)
	}
	return resolved, nil
}

// checkAncestor поднимается до первого существующего каталога на пути и
// проверяет, что он лежит внутри vault. Несуществующие звенья пропускаются:
// их ещё предстоит создать, а вот существующее может оказаться ссылкой наружу.
func (v *Vault) checkAncestor(dir, notebook string) error {
	for p := dir; ; {
		resolved, err := filepath.EvalSymlinks(p)
		if err == nil {
			if !v.contains(resolved) {
				return fmt.Errorf("create notebook %q: %w", notebook, ErrOutsideVault)
			}
			return nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("create notebook %q: %w", notebook, err)
		}
		parent := filepath.Dir(p)
		if parent == p {
			return fmt.Errorf("create notebook %q: %w", notebook, ErrOutsideVault)
		}
		p = parent
	}
}

// cleanRelative приводит путь к относительному внутри vault и отвергает всё,
// что выходит наружу или ведёт в скрытый каталог.
func (v *Vault) cleanRelative(path string) (string, error) {
	if filepath.IsAbs(path) {
		rel, err := filepath.Rel(v.root, filepath.Clean(path))
		if err != nil || escapes(rel) {
			return "", fmt.Errorf("resolve %q: %w", path, ErrOutsideVault)
		}
		path = rel
	}

	clean := filepath.Clean(filepath.FromSlash(path))
	if clean == "." {
		return "", nil
	}
	if escapes(clean) {
		return "", fmt.Errorf("resolve %q: %w", path, ErrOutsideVault)
	}
	for _, part := range strings.Split(clean, string(filepath.Separator)) {
		if strings.HasPrefix(part, ".") {
			return "", fmt.Errorf("resolve %q: %w", path, ErrHiddenPath)
		}
	}
	return clean, nil
}

// resolveExisting превращает путь в абсолютный разрешённый путь внутри vault.
func (v *Vault) resolveExisting(path string) (string, error) {
	abs := path
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(v.root, filepath.FromSlash(path))
	}

	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			// Путь может не существовать, но при этом вести наружу — сначала
			// отвечаем про выход за пределы, иначе ответ подсказывает, чего
			// снаружи нет.
			if !v.contains(filepath.Clean(abs)) {
				return "", fmt.Errorf("resolve %q: %w", path, ErrOutsideVault)
			}
			return "", fmt.Errorf("resolve %q: %w", path, ErrNoteNotFound)
		}
		return "", fmt.Errorf("resolve %q: %w", path, err)
	}
	if !v.contains(resolved) {
		return "", fmt.Errorf("resolve %q: %w", path, ErrOutsideVault)
	}
	return resolved, nil
}

// contains проверяет вложенность по разобранному относительному пути, а не по
// префиксу строки: у strings.HasPrefix соседний каталог /vault-evil выглядит
// вложенным в /vault.
func (v *Vault) contains(abs string) bool {
	rel, err := filepath.Rel(v.root, abs)
	if err != nil {
		return false
	}
	return !escapes(rel)
}

func escapes(rel string) bool {
	return rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// notebookOf возвращает путь ноутбука относительно корня в виде со слэшами:
// он попадает в индекс и в MCP, где разделитель всегда "/".
func (v *Vault) notebookOf(abs string) string {
	rel, err := filepath.Rel(v.root, filepath.Dir(abs))
	if err != nil || rel == "." {
		return ""
	}
	return filepath.ToSlash(rel)
}
