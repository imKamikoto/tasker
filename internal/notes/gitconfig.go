package notes

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"tasker/internal/vault"
)

// gitConfigFile — где записано, ведётся ли в этом хранилище история.
//
// В самом хранилище, а не в домашней папке: история — свойство папки с
// заметками, а не человека. Перенёс папку на другую машину — вместе с ней
// переехало и решение.
//
// Отдельным файлом, а не в config.json: тот для Go непрозрачен и принадлежит
// интерфейсу, а этот флаг читают два разных процесса — приложение и
// tasker-mcp. Положить его в непрозрачный blob значит завести второе место,
// где его разбирают, и однажды разойтись.
const gitConfigFile = "git.json"

// gitConfigState — содержимое файла. Объектом ради того же, ради чего им
// сделаны цвета тегов: формат ещё вырастет.
type gitConfigState struct {
	Enabled bool `json:"enabled"`
}

// gitConfig читает и пишет решение о том, вести ли историю.
type gitConfig struct {
	path string
	// root нужен, чтобы вывести умолчание из наличия репозитория.
	root string
	mu   sync.Mutex
}

func newGitConfig(root, configDir string) *gitConfig {
	return &gitConfig{path: filepath.Join(configDir, gitConfigFile), root: root}
}

// enabled отвечает, вести ли историю.
//
// Умолчание выводится из того, есть ли уже репозиторий, а не берётся
// константой. Новая папка — это просто папка с файлами, и заводить в ней .git
// без спросу приложение не должно. Но папка, в которой репозиторий уже есть,
// историю уже ведёт, и молча перестать — значит на глазах у человека
// прекратить делать то, что он видел вчера.
func (g *gitConfig) enabled() bool {
	g.mu.Lock()
	defer g.mu.Unlock()

	raw, err := os.ReadFile(g.path)
	if err != nil {
		return g.repoExists()
	}
	var state gitConfigState
	if err := json.Unmarshal(raw, &state); err != nil {
		// Испорченный файл — не повод не запуститься и не повод завести
		// репозиторий молча: возвращаемся к тому же умолчанию.
		return g.repoExists()
	}
	return state.Enabled
}

// set записывает решение.
func (g *gitConfig) set(enabled bool) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	raw, err := json.MarshalIndent(gitConfigState{Enabled: enabled}, "", "  ")
	if err != nil {
		return fmt.Errorf("save git config: %w", err)
	}
	return vault.WriteFileAtomic(g.path, append(raw, '\n'), 0o644)
}

// repoExists проверяет, заведён ли уже репозиторий.
func (g *gitConfig) repoExists() bool {
	_, err := os.Stat(filepath.Join(g.root, ".git"))
	return err == nil
}
