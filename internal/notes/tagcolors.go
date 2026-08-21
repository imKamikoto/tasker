package notes

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"tasker/internal/vault"
)

// tagColorsFile — где живут выбранные вручную цвета тегов.
//
// Отдельным файлом, а не колонкой в индексе: индекс объявлен производным и
// сносится при смене схемы без потери данных (SPEC §5.2), а выбранный цвет —
// пользовательские данные. Лежит рядом с notebooks.json из SPEC §4.1.
const tagColorsFile = "tags.json"

// TagPalette — сколько цветов в палитре (SPEC §8.2).
const TagPalette = 14

// AutoColor означает «цвет не выбран»: интерфейс выведет его из имени тега.
const AutoColor = -1

// ErrBadColor — цвет вне палитры.
var ErrBadColor = errors.New("color outside palette")

// tagColors — содержимое файла. Объектом, а не голой картой: формат ещё
// вырастет, и версия рядом с данными дешевле, чем миграция без неё.
type tagColors struct {
	Colors map[string]int `json:"colors"`
}

// colorStore читает и пишет цвета тегов.
type colorStore struct {
	path string
	mu   sync.Mutex
}

func newColorStore(configDir string) *colorStore {
	return &colorStore{path: filepath.Join(configDir, tagColorsFile)}
}

// load читает файл. Отсутствие и порча — не ошибка: цвета вторичны, и терять
// из-за них запуск приложения незачем.
func (c *colorStore) load() map[string]int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.loadLocked()
}

func (c *colorStore) loadLocked() map[string]int {
	raw, err := os.ReadFile(c.path)
	if err != nil {
		return map[string]int{}
	}
	var stored tagColors
	if err := json.Unmarshal(raw, &stored); err != nil || stored.Colors == nil {
		return map[string]int{}
	}
	// Чужие значения отбрасываем поимённо: один испорченный цвет не должен
	// стирать остальные.
	clean := make(map[string]int, len(stored.Colors))
	for name, color := range stored.Colors {
		if name != "" && color >= 0 && color < TagPalette {
			clean[name] = color
		}
	}
	return clean
}

// set проставляет или снимает цвет тега.
func (c *colorStore) set(name string, color int) error {
	if color != AutoColor && (color < 0 || color >= TagPalette) {
		return fmt.Errorf("set color %d: %w", color, ErrBadColor)
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	colors := c.loadLocked()
	if color == AutoColor {
		delete(colors, name)
	} else {
		colors[name] = color
	}

	raw, err := json.MarshalIndent(tagColors{Colors: colors}, "", "  ")
	if err != nil {
		return fmt.Errorf("save tag colors: %w", err)
	}
	return vault.WriteFileAtomic(c.path, append(raw, '\n'), 0o644)
}
