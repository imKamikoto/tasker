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

// settingsFile — где живут настройки интерфейса (SPEC §4.1).
const settingsFile = "config.json"

// maxSettings — потолок размера. Настройки интерфейса — это несколько
// чисел и список развёрнутых ноутбуков; мегабайт здесь означает, что
// что-то пошло не так, и записывать это в vault не надо.
const maxSettings = 256 * 1024

// ErrSettingsTooBig — настройки не влезают в разумный размер.
var ErrSettingsTooBig = errors.New("settings too big")

// Settings — сервис Wails: хранение настроек интерфейса.
//
// Содержимое для Go непрозрачно и хранится как есть. Это не отступление от
// «вся логика в Go»: ширины колонок, порядок сортировки и развёрнутые ветки
// дерева — состояние интерфейса, которого в Go нет и быть не должно. Типизируй
// мы его здесь, каждая новая настройка требовала бы правки Go, перегенерации
// биндингов и пересборки — ради числа, которое Go никогда не прочитает.
//
// Агенту это недоступно: писать в config.json ему не дано (docs/MCP.md §4), и
// сервис живёт только в приложении.
type Settings struct {
	path string
	mu   sync.Mutex
}

// NewSettings создаёт сервис поверх служебного каталога vault.
func NewSettings(configDir string) *Settings {
	return &Settings{path: filepath.Join(configDir, settingsFile)}
}

// Load читает настройки. Пустая строка означает, что их ещё нет.
func (s *Settings) Load() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	raw, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read settings: %w", err)
	}
	if !json.Valid(raw) {
		// Испорченный файл — не повод не запуститься: интерфейс просто начнёт
		// с умолчаний и перезапишет его при первом же изменении.
		return "", nil
	}
	return string(raw), nil
}

// Save записывает настройки.
func (s *Settings) Save(raw string) error {
	if len(raw) > maxSettings {
		return fmt.Errorf("save settings: %w", ErrSettingsTooBig)
	}
	if !json.Valid([]byte(raw)) {
		return errors.New("save settings: not valid JSON")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	return vault.WriteFileAtomic(s.path, []byte(raw), 0o644)
}
