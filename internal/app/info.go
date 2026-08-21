package app

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"

	"tasker/internal/notes"
)

// Build — что за сборка запущена. Показывается в разделе «О программе».
type Build struct {
	// Revision и Time берутся из vcs-меток, которые Go кладёт в бинарник сам.
	// Своей переменной версии у проекта нет: приложение собирается из
	// исходников на этой же машине, и коммит опознаёт сборку точнее номера.
	Revision string
	Time     string
	Modified bool
	Go       string
	Wails    string
	// Binary — путь к запущенному исполняемому файлу.
	Binary string
}

// Paths — где что лежит. Из настроек эти пути открываются в Finder.
type Paths struct {
	Vault  string
	Config string
	Keymap string
	Vaults string
	Index  string
	MCP    string
}

// Info — сервис Wails: сведения о хранилище и о самой программе.
//
// Только чтение и одна операция пересборки индекса. Ничего из этого не меняет
// заметки, поэтому сервис отделён от Notes: там операции над данными человека,
// здесь справка.
type Info struct {
	service *notes.Service
	keymap  *Keymap
	vaults  *Vaults
}

// NewInfo создаёт сервис.
func NewInfo(service *notes.Service, keymap *Keymap, vaults *Vaults) *Info {
	return &Info{service: service, keymap: keymap, vaults: vaults}
}

// Stats отдаёт сводку о хранилище: сколько чего и сколько весит индекс.
func (i *Info) Stats(ctx context.Context) (notes.Stats, error) {
	return i.service.Stats(ctx)
}

// Rebuild сносит индекс и собирает заново. Возвращает сводку после пересборки,
// чтобы интерфейсу не пришлось спрашивать её вторым вызовом.
func (i *Info) Rebuild(ctx context.Context) (notes.Stats, error) {
	if _, err := i.service.Rebuild(ctx); err != nil {
		return notes.Stats{}, err
	}
	return i.service.Stats(ctx)
}

// Build отдаёт сведения о сборке.
func (i *Info) Build() Build {
	out := Build{Go: runtime.Version()}
	if binary, err := os.Executable(); err == nil {
		out.Binary = binary
	}

	info, ok := debug.ReadBuildInfo()
	if !ok {
		return out
	}
	for _, setting := range info.Settings {
		switch setting.Key {
		case "vcs.revision":
			out.Revision = setting.Value
		case "vcs.time":
			out.Time = setting.Value
		case "vcs.modified":
			out.Modified = setting.Value == "true"
		}
	}
	for _, dep := range info.Deps {
		if strings.HasPrefix(dep.Path, "github.com/wailsapp/wails/") {
			out.Wails = dep.Version
		}
	}
	return out
}

// Paths отдаёт пути ко всему, что приложение держит на диске.
func (i *Info) Paths() Paths {
	root := i.service.Vault().Root()
	return Paths{
		Vault:  root,
		Config: filepath.Join(root, ".tasker", settingsFile),
		Keymap: i.keymap.Path(),
		Vaults: i.vaults.Path(),
		Index:  filepath.Join(root, ".tasker", "index.sqlite"),
		MCP:    mcpBinary(),
	}
}

// Reveal показывает файл или папку в Finder.
//
// Через `open -R`, а не `open`: у файла второе открыло бы его в редакторе по
// умолчанию, а спрашивают тут именно «где он лежит».
func (i *Info) Reveal(path string) error {
	if path == "" {
		return ErrNoVaultChosen
	}
	return exec.Command("open", "-R", path).Run()
}

// OnBattery — работает ли машина от батареи.
//
// Через `pmset`, потому что чистого способа спросить об этом из Go без cgo
// нет, а cgo-зависимость ради одной строки ломает сборку universal binary
// (CLAUDE.md, раздел про зависимости). Ошибка означает «не от батареи»: это
// безопасная сторона — прозрачность просто не отключится.
func (i *Info) OnBattery() bool {
	out, err := exec.Command("pmset", "-g", "batt").Output()
	if err != nil {
		return false
	}
	return strings.Contains(string(out), "Battery Power")
}

// mcpBinary ищет tasker-mcp там, где он может лежать.
//
// Порядок не случаен: рядом со сборкой его находят сразу после `task package`,
// а `~/bin` — место из docs/MCP.md, куда он переезжает при установке. Внутрь
// бандла его не кладут: приложение и агент — разные процессы, и обновлять их
// по отдельности удобнее, чем распаковывать .app.
//
// Пустая строка означает «не нашли» — интерфейс тогда покажет, как собрать.
func mcpBinary() string {
	var candidates []string

	if binary, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(binary), "tasker-mcp"))
		if bundle, ok := appBundle(binary); ok {
			candidates = append(candidates, filepath.Join(filepath.Dir(bundle), "tasker-mcp"))
		}
	}
	if home, err := os.UserHomeDir(); err == nil {
		candidates = append(candidates, filepath.Join(home, "bin", "tasker-mcp"))
	}
	// PATH последним: там он оказывается, только если человек положил его туда
	// сам, и такой выбор перебивать своими догадками не стоит.
	if found, err := exec.LookPath("tasker-mcp"); err == nil {
		candidates = append(candidates, found)
	}

	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate
		}
	}
	return ""
}
