package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"tasker/internal/vault"
)

// keymapDir и keymapFile — где живёт раскладка клавиш (SPEC §8.11).
//
// В домашней папке, а не в vault: клавиши принадлежат человеку, а не набору
// заметок, и переносить их между хранилищами руками никто не должен.
const (
	keymapDir  = ".tasker"
	keymapFile = "keymap.json"
)

// maxKeymap — потолок размера файла. Раскладка это несколько десятков строк.
const maxKeymap = 128 * 1024

// ErrKeymapTooBig — файл раскладки не влезает в разумный размер.
var ErrKeymapTooBig = errors.New("keymap too big")

// Контексты, в которых действуют сочетания (SPEC §8.11).
//
// Контекст решает спор за одну и ту же клавишу: j в списке двигает выделение,
// а в тексте принадлежит виму, и разводит их именно контекст, а не порядок
// проверок в обработчике.
const (
	ContextGlobal  = "global"
	ContextSidebar = "sidebar"
	ContextList    = "note-list"
	ContextEditor  = "editor"
)

// defaultKeymap — раскладка по умолчанию.
//
// Живёт в Go, а не в интерфейсе: это единственный источник правды, из которого
// файл создаётся при первом запуске и с которым сливается при чтении. Иначе
// новая команда появлялась бы только у тех, кто удалил свой keymap.json.
func defaultKeymap() map[string]map[string]string {
	return map[string]map[string]string{
		ContextGlobal: {
			"cmd+n": "note.create",
			// Запятая с cmd — общесистемное «настройки», человек попробует её
			// первой, ещё не зная, есть она здесь или нет.
			"cmd+,": "note.settings",
			// Шаблон применяют к уже заведённой заметке (SPEC §8.10), поэтому
			// сочетание глобальное: жать его будут прямо из текста.
			"cmd+t":      "note.template",
			"cmd+ctrl+1": "note.status.none",
			"cmd+ctrl+2": "note.status.active",
			"cmd+ctrl+3": "note.status.onhold",
			"cmd+ctrl+4": "note.status.completed",
			"cmd+ctrl+5": "note.status.dropped",
			// Фокус ходит по трём колонкам. H и L, а не J и K: колонки стоят
			// рядом, и влево-вправо здесь то же движение, что и у вима.
			// С шифтом, потому что голые Ctrl+H и Ctrl+L в CodeMirror заняты
			// (backspace и «выделить строку») и в режиме вставки нужны тексту.
			// Ctrl+Shift+* не занят ни вимом, ни редактором, поэтому работает
			// откуда угодно, без предварительного Esc.
			//
			// Дублёра на стрелках у этой пары нет намеренно, в отличие от
			// движений внутри колонки. Переключение колонок вслепую — и есть
			// вимовская навигация: выключил движения — выключил её целиком,
			// вместе с индикатором в титульной строке. Фокус при этом всё
			// равно переезжает: Enter уводит в текст, Esc возвращает в список,
			// щелчок отдаёт колонке — но там он переезжает на глазах.
			"ctrl+shift+h": "focus.prev",
			"ctrl+shift+l": "focus.next",
			// Сайдбар прячется и возвращается — как во всех трёхколоночных
			// приложениях macOS.
			"cmd+/": "view.sidebar",
			// Масштаб интерфейса. Обе формы плюса: на маке Cmd+= и Cmd++ —
			// одна и та же клавиша, и человек не должен об этом думать.
			"cmd+=": "view.zoom.in",
			"cmd++": "view.zoom.in",
			"cmd+-": "view.zoom.out",
			"cmd+0": "view.zoom.reset",
		},

		ContextSidebar: {
			"j":     "sidebar.down",
			"k":     "sidebar.up",
			"down":  "sidebar.down",
			"up":    "sidebar.up",
			"enter": "sidebar.open",
			// Свернуть и развернуть ветку: в дереве это движение поперёк,
			// поэтому влево-вправо, как в файловых менеджерах и в NERDTree.
			"left":  "sidebar.collapse",
			"right": "sidebar.expand",
			"h":     "sidebar.collapse",
			"l":     "sidebar.expand",
		},
		ContextList: {
			"j":     "list.down",
			"k":     "list.up",
			"down":  "list.down",
			"up":    "list.up",
			"enter": "list.open",
			"p":     "note.pin",
			"m":     "note.move",
			"cmd+d": "note.duplicate",
			// В тексте это удаление до начала строки, поэтому только в списке.
			"cmd+backspace": "note.trash",
		},
		// Пусто намеренно: в тексте всё принадлежит редактору и виму, кроме
		// глобальных сочетаний. Пользователь может добавить сюда своё.
		ContextEditor: {},
	}
}

// Keymap — сервис Wails: раскладка клавиш.
type Keymap struct {
	path string
	mu   sync.Mutex
}

// NewKeymap создаёт сервис. Пустой home означает домашнюю папку пользователя.
func NewKeymap(home string) (*Keymap, error) {
	if home == "" {
		resolved, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("keymap: %w", err)
		}
		home = resolved
	}
	dir := filepath.Join(home, keymapDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("keymap: %w", err)
	}
	return &Keymap{path: filepath.Join(dir, keymapFile)}, nil
}

// Path возвращает путь к файлу — интерфейсу есть что показать человеку.
func (k *Keymap) Path() string { return k.path }

// Load отдаёт раскладку: умолчания, поверх которых легло содержимое файла.
//
// Слияние, а не замена: файл может описывать одно сочетание, и остальные
// должны продолжать работать. Пустая строка вместо команды снимает привязку —
// иначе отказаться от умолчания было бы нечем.
func (k *Keymap) Load() (map[string]map[string]string, error) {
	k.mu.Lock()
	defer k.mu.Unlock()

	merged := defaultKeymap()

	raw, err := os.ReadFile(k.path)
	if errors.Is(err, os.ErrNotExist) {
		// Файла нет — создаём с умолчаниями, чтобы человеку было что править.
		return merged, k.writeLocked(merged)
	}
	if err != nil {
		return merged, fmt.Errorf("read keymap: %w", err)
	}

	var stored map[string]map[string]string
	if err := json.Unmarshal(raw, &stored); err != nil {
		// Испорченный файл не должен оставлять приложение без клавиш.
		return merged, nil
	}

	for context, bindings := range stored {
		if merged[context] == nil {
			merged[context] = map[string]string{}
		}
		for combination, command := range bindings {
			if command == "" {
				delete(merged[context], combination)
				continue
			}
			merged[context][combination] = command
		}
	}
	return merged, nil
}

// Save записывает раскладку целиком.
func (k *Keymap) Save(raw string) error {
	if len(raw) > maxKeymap {
		return fmt.Errorf("save keymap: %w", ErrKeymapTooBig)
	}
	var parsed map[string]map[string]string
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return fmt.Errorf("save keymap: %w", err)
	}

	k.mu.Lock()
	defer k.mu.Unlock()
	return k.writeLocked(parsed)
}

// Reset возвращает раскладку к умолчаниям.
func (k *Keymap) Reset() error {
	k.mu.Lock()
	defer k.mu.Unlock()
	return k.writeLocked(defaultKeymap())
}

func (k *Keymap) writeLocked(bindings map[string]map[string]string) error {
	raw, err := json.MarshalIndent(bindings, "", "  ")
	if err != nil {
		return fmt.Errorf("save keymap: %w", err)
	}
	return vault.WriteFileAtomic(k.path, append(raw, '\n'), 0o644)
}
