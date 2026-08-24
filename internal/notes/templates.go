package notes

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"

	"tasker/internal/index"
	"tasker/internal/vault"
)

// templatesDir — где лежат шаблоны (SPEC §8.10).
//
// Обычная папка внутри хранилища, и лежат в ней обычные заметки: файлы —
// источник правды, и прятать часть из них от индекса значит заводить второе
// правило о том, что считается заметкой.
const templatesDir = "templates"

// Template — шаблон, каким его показывают в пикере.
type Template struct {
	// Name — имя файла без расширения: по нему шаблон ищут в пикере.
	Name string
	// Path — путь от корня хранилища.
	Path string
	// Title — заголовок будущей заметки из блока _template, если он задан.
	Title string
	// Preview — начало тела, чтобы было видно, что именно вставится.
	Preview string
}

// Templates перечисляет шаблоны из папки templates/.
func (s *Service) Templates(ctx context.Context) ([]Template, error) {
	dir := filepath.Join(s.vault.Root(), templatesDir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			// Папки нет — шаблонов нет. Заводить её самим незачем: пустая
			// папка в хранилище выглядит как забытый мусор.
			return nil, nil
		}
		return nil, fmt.Errorf("list templates: %w", err)
	}

	var out []Template
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		rel := path.Join(templatesDir, entry.Name())
		n, err := s.vault.Load(rel)
		if err != nil {
			// Битый шаблон не должен прятать остальные.
			continue
		}
		settings := templateSettings(n.Doc.Meta)
		out = append(out, Template{
			Name:    strings.TrimSuffix(entry.Name(), ".md"),
			Path:    rel,
			Title:   settings.Title,
			Preview: preview(n.Doc.Body),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// preview обрезает тело до одной строки для списка.
func preview(body string) string {
	line := strings.TrimSpace(strings.SplitN(strings.TrimSpace(body), "\n", 2)[0])
	if len([]rune(line)) > 80 {
		return string([]rune(line)[:80]) + "…"
	}
	return line
}

// templateFields — что задаёт блок _template во frontmatter (SPEC §8.10).
type templateFields struct {
	Title    string   `yaml:"title"`
	Notebook string   `yaml:"notebook"`
	Tags     []string `yaml:"tags"`
	Status   string   `yaml:"status"`
}

// templateSettings достаёт блок _template.
//
// Ошибка разбора проглатывается намеренно: испорченный блок означает шаблон
// без настроек, а не шаблон, который нельзя показать. Файл при этом читается
// целиком и остальные его ключи не трогаются — frontmatter правится по месту.
func templateSettings(meta *vault.Frontmatter) templateFields {
	var out templateFields
	_, _ = meta.Get("_template", &out)
	return out
}

var (
	rePlaceholder = regexp.MustCompile(`\{\{(date|time|uuid|cursor)(?::([^}]*))?\}\}`)
)

// CursorMark — чем помечена позиция каретки после раскрытия шаблона.
const cursorNone = -1

// expandPlaceholders раскрывает плейсхолдеры и находит место для каретки.
//
// Возвращает готовый текст и позицию каретки в нём (в байтах) либо -1, если
// `{{cursor}}` в шаблоне нет.
//
// Формат даты — как в Go (`2006-01-02`), а не strftime: это записано в спеке и
// сделано нарочно, чтобы не заводить второй язык форматов ради одного места.
func expandPlaceholders(body string, now time.Time, id func() string) (string, int) {
	cursor := cursorNone
	var b strings.Builder
	last := 0

	for _, m := range rePlaceholder.FindAllStringSubmatchIndex(body, -1) {
		b.WriteString(body[last:m[0]])
		last = m[1]

		kind := body[m[2]:m[3]]
		arg := ""
		if m[4] >= 0 {
			arg = body[m[4]:m[5]]
		}

		switch kind {
		case "date":
			layout := arg
			if layout == "" {
				layout = "2006-01-02"
			}
			b.WriteString(now.Format(layout))
		case "time":
			layout := arg
			if layout == "" {
				layout = "15:04"
			}
			b.WriteString(now.Format(layout))
		case "uuid":
			b.WriteString(id())
		case "cursor":
			// Первый выигрывает: кареток в редакторе одна, и выбирать из двух
			// одинаковых меток нечем.
			if cursor == cursorNone {
				cursor = b.Len()
			}
		}
	}
	b.WriteString(body[last:])
	return b.String(), cursor
}

// TemplateResult — заметка после применения шаблона и место для каретки.
type TemplateResult struct {
	Record index.Record
	// Cursor — смещение каретки в теле; -1, если шаблон её не задавал.
	Cursor int
}

// ApplyTemplate накладывает шаблон на существующую заметку.
//
// На существующую, а не «создать из шаблона»: заметка уже заведена (⌘N), и
// шаблон применяют к ней. Так не появляется второго пути создания со своими
// правилами имени файла, id и коллизий.
//
// Уже проставленное человеком не затирается (SPEC §8.10): статус остаётся,
// если он не none, теги дополняются, а не заменяются, заголовок берётся из
// шаблона только когда он там задан. Тело заменяется целиком — накладывать
// шаблон поверх написанного текста бессмысленно, и применяют его к пустой.
func (s *Service) ApplyTemplate(ctx context.Context, id, templatePath string) (TemplateResult, error) {
	release, err := s.lock.acquire(ctx)
	if err != nil {
		return TemplateResult{}, err
	}
	defer release()

	rec, err := s.index.Get(ctx, id)
	if err != nil {
		return TemplateResult{}, err
	}
	n, err := s.loadByPath(rec.Path)
	if err != nil {
		return TemplateResult{}, err
	}

	tpl, err := s.vault.Load(templatePath)
	if err != nil {
		return TemplateResult{}, fmt.Errorf("apply template %s: %w", templatePath, err)
	}
	fields := templateSettings(tpl.Doc.Meta)
	body, cursor := expandPlaceholders(tpl.Doc.Body, time.Now(), vault.NewID)

	n.Doc.Body = body

	if fields.Title != "" {
		if err := n.Doc.Meta.SetTitle(fields.Title); err != nil {
			return TemplateResult{}, err
		}
	}
	if fields.Status != "" {
		current, err := n.Doc.Meta.Status()
		if err != nil {
			return TemplateResult{}, err
		}
		// Статус, выбранный руками, сильнее шаблонного: шаблон описывает, с
		// чего начать, а не чем переписать уже решённое.
		if current == vault.StatusNone {
			status, err := vault.ParseStatus(fields.Status)
			if err != nil {
				return TemplateResult{}, fmt.Errorf("apply template %s: %w", templatePath, err)
			}
			if err := n.Doc.Meta.SetStatus(status); err != nil {
				return TemplateResult{}, err
			}
		}
	}
	if len(fields.Tags) > 0 {
		existing, err := n.Doc.Meta.Tags()
		if err != nil {
			return TemplateResult{}, err
		}
		merged := append([]string{}, existing...)
		for _, tag := range fields.Tags {
			if !slices.ContainsFunc(merged, func(item string) bool { return strings.EqualFold(item, tag) }) {
				merged = append(merged, tag)
			}
		}
		if err := n.Doc.Meta.SetTags(merged); err != nil {
			return TemplateResult{}, err
		}
	}

	if err := s.vault.Save(n); err != nil {
		return TemplateResult{}, err
	}
	if fields.Notebook != "" {
		if err := s.vault.Move(n, fields.Notebook); err != nil {
			return TemplateResult{}, err
		}
	}
	if fields.Title != "" {
		if err := s.vault.Rename(n); err != nil {
			return TemplateResult{}, err
		}
	}

	updated, err := s.reindex(ctx, n)
	if err != nil {
		return TemplateResult{}, err
	}
	if err := s.commit(ctx, "update", updated.Title); err != nil {
		return TemplateResult{}, err
	}
	return TemplateResult{Record: updated, Cursor: cursor}, nil
}
