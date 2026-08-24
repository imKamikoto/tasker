package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"tasker/internal/vault"
)

// vaultsFile — где записано, какую папку открывать.
//
// В домашней папке, а не в самом хранилище: положить путь к хранилищу внутрь
// хранилища — замкнутый круг, прочитать его будет неоткуда.
const vaultsFile = "vaults.json"

// maxRecent — сколько недавних папок помним. Список для того, чтобы вернуться
// в предыдущую пару, а не чтобы вести историю.
const maxRecent = 8

// Ошибки смены хранилища.
var (
	// ErrNotDirectory — по указанному пути не папка.
	ErrNotDirectory = errors.New("vault path is not a directory")
	// ErrNoVaultChosen — диалог закрыли, ничего не выбрав.
	ErrNoVaultChosen = errors.New("no vault chosen")
)

// vaultsState — содержимое vaults.json.
type vaultsState struct {
	Current string   `json:"current"`
	Recent  []string `json:"recent"`
}

// Vaults — сервис Wails: какая папка открыта и как сменить её на другую.
//
// Смена требует перезапуска, и это осознанно. Переоткрыть хранилище на лету
// значит погасить и поднять заново индекс, watcher, git и открытый буфер
// редактора — четыре подсистемы, каждая со своим временем жизни. Ради
// действия, которое делают раз в полгода, это не окупается.
type Vaults struct {
	path string // путь к vaults.json
	// pick открывает системный диалог. Отдельным полем, чтобы тесты не
	// поднимали окно: в них подставляется функция без Wails. Задаётся при
	// создании и больше не меняется — приложение, у которого диалог
	// спрашивают, к моменту вызова уже есть, потому что вызывают из вебвью.
	pick func() (string, error)
	// launch поднимает новый экземпляр приложения, shutdown гасит текущий.
	// Полями по той же причине, что и pick: в тестах здесь функции, которые
	// не трогают ни процессы, ни окно. shutdown приходит снаружи — гасить надо
	// через хук закрытия окна, а он живёт в cmd/tasker вместе с Wails.
	launch   func() error
	shutdown func()
	mu       sync.Mutex
}

// NewVaults создаёт сервис. Пустой home означает домашнюю папку пользователя.
//
// shutdown должен гасить приложение тем же путём, что и закрытие окна: иначе
// перезапуск станет способом потерять несохранённый буфер.
func NewVaults(home string, pick func() (string, error), shutdown func()) (*Vaults, error) {
	if home == "" {
		resolved, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("vaults: %w", err)
		}
		home = resolved
	}
	dir := filepath.Join(home, keymapDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("vaults: %w", err)
	}
	v := &Vaults{path: filepath.Join(dir, vaultsFile), pick: pick, shutdown: shutdown}
	v.launch = v.launchInstance
	return v, nil
}

// Path возвращает путь к vaults.json — интерфейсу есть что показать.
func (v *Vaults) Path() string { return v.path }

// Current отдаёт записанный путь. Пустая строка — записи ещё нет.
//
// Читает это и cmd/tasker при запуске, поэтому метод не должен ничего
// требовать от Wails.
func (v *Vaults) Current() string {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.load().Current
}

// Recent отдаёт недавние папки без текущей.
func (v *Vaults) Recent() []string {
	v.mu.Lock()
	defer v.mu.Unlock()

	state := v.load()
	out := make([]string, 0, len(state.Recent))
	for _, path := range state.Recent {
		if path != state.Current {
			out = append(out, path)
		}
	}
	return out
}

// Choose открывает системный диалог выбора папки и запоминает выбранное.
//
// Возвращает выбранный путь; ErrNoVaultChosen, если диалог закрыли. Само
// приложение при этом продолжает работать со старой папкой — переезд
// случится при следующем запуске.
func (v *Vaults) Choose() (string, error) {
	pick := v.pick
	if pick == nil {
		return "", errors.New("choose vault: no picker")
	}
	picked, err := pick()
	if err != nil {
		return "", fmt.Errorf("choose vault: %w", err)
	}
	if strings.TrimSpace(picked) == "" {
		return "", ErrNoVaultChosen
	}
	if err := v.Switch(picked); err != nil {
		return "", err
	}
	return picked, nil
}

// Switch записывает новую текущую папку.
//
// Проверяет, что это папка, до записи: узнать о том, что путь негодный, при
// следующем запуске — значит получить приложение, которое не открывается.
func (v *Vaults) Switch(path string) error {
	clean := filepath.Clean(strings.TrimSpace(path))
	if clean == "" || clean == "." {
		return fmt.Errorf("switch vault: %w", ErrNoVaultChosen)
	}

	info, err := os.Stat(clean)
	if err != nil {
		return fmt.Errorf("switch vault %s: %w", clean, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("switch vault %s: %w", clean, ErrNotDirectory)
	}

	v.mu.Lock()
	defer v.mu.Unlock()

	state := v.load()
	// Прежняя текущая уезжает в начало недавних: чаще всего возвращаются
	// именно в неё, а не в что-то из глубины списка.
	state.Recent = promote(state.Recent, state.Current, clean)
	state.Current = clean
	return v.writeLocked(state)
}

// Forget убирает папку из недавних. Текущую забыть нельзя.
func (v *Vaults) Forget(path string) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	state := v.load()
	if path == state.Current {
		return nil
	}
	state.Recent = slices.DeleteFunc(state.Recent, func(item string) bool { return item == path })
	return v.writeLocked(state)
}

// Reveal показывает папку в Finder.
func (v *Vaults) Reveal(path string) error {
	if path == "" {
		path = v.Current()
	}
	if path == "" {
		return ErrNoVaultChosen
	}
	if err := exec.Command("open", path).Run(); err != nil {
		return fmt.Errorf("reveal %s: %w", path, err)
	}
	return nil
}

// Restart перезапускает приложение, чтобы смена папки вступила в силу.
//
// Запускается новый экземпляр, и только потом гасится текущий: если запуск не
// удался, человек останется в работающем приложении, а не перед пустым
// экраном. Гашение обязательно, и обязательно через shutdown: без него на
// экране оказывались два приложения разом — новое с новой папкой и старое со
// старой, — а прямой выход мимо хука закрытия потерял бы буфер редактора.
func (v *Vaults) Restart() error {
	if err := v.launch(); err != nil {
		return err
	}
	if v.shutdown == nil {
		return errors.New("restart: no shutdown")
	}
	v.shutdown()
	return nil
}

// launchInstance поднимает второй экземпляр приложения.
func (v *Vaults) launchInstance() error {
	binary, err := os.Executable()
	if err != nil {
		return fmt.Errorf("restart: %w", err)
	}

	// Внутри бандла перезапускать надо сам бандл, а не исполняемый файл:
	// запущенный напрямую, он не получит ни иконки, ни Info.plist.
	if bundle, ok := appBundle(binary); ok {
		// Run, а не Start: open завершается сразу после того, как отдал
		// приложение системе, и его код возврата — единственный способ узнать,
		// что новый экземпляр не поднялся. Со Start мы бы погасили текущее
		// приложение, не заметив этого.
		if err := exec.Command("open", "-n", bundle).Run(); err != nil {
			return fmt.Errorf("restart %s: %w", bundle, err)
		}
		return nil
	}
	// Вне бандла запущен сам бинарник, и ждать его нельзя: это и есть
	// приложение, а не утилита запуска.
	if err := exec.Command(binary).Start(); err != nil {
		return fmt.Errorf("restart %s: %w", binary, err)
	}
	return nil
}

// appBundle достаёт путь к .app из пути к исполняемому файлу внутри него.
func appBundle(binary string) (string, bool) {
	const inside = "/Contents/MacOS/"
	cut := strings.LastIndex(binary, inside)
	if cut < 0 {
		return "", false
	}
	bundle := binary[:cut]
	if !strings.HasSuffix(bundle, ".app") {
		return "", false
	}
	return bundle, true
}

// promote ставит prev в начало списка недавних и убирает оттуда next.
//
// Отдельной функцией, потому что здесь три правила разом — не дублировать,
// не хранить будущую текущую, не расти без предела, — и проверять их надо
// таблицей.
func promote(recent []string, prev, next string) []string {
	out := make([]string, 0, len(recent)+1)
	if prev != "" && prev != next {
		out = append(out, prev)
	}
	for _, path := range recent {
		if path == prev || path == next {
			continue
		}
		out = append(out, path)
	}
	if len(out) > maxRecent {
		out = out[:maxRecent]
	}
	return out
}

// load читает состояние. Испорченный файл — не повод не запуститься: он
// вернётся к умолчаниям и перезапишется при первой же смене папки.
func (v *Vaults) load() vaultsState {
	raw, err := os.ReadFile(v.path)
	if err != nil {
		return vaultsState{}
	}
	var state vaultsState
	if err := json.Unmarshal(raw, &state); err != nil {
		return vaultsState{}
	}
	return state
}

func (v *Vaults) writeLocked(state vaultsState) error {
	raw, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("save vaults: %w", err)
	}
	return vault.WriteFileAtomic(v.path, append(raw, '\n'), 0o644)
}
