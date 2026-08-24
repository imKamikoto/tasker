// Command tasker — само приложение: окно macOS с WKWebView внутри и сервисы
// Wails поверх internal/notes. См. docs/SPEC.md §6.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync/atomic"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"tasker"
	"tasker/internal/app"
	"tasker/internal/gitstore"
	"tasker/internal/notes"
	"tasker/internal/vault"
	"tasker/internal/watcher"
)

// defaultVault — папка с заметками по умолчанию (SPEC §4.1).
const defaultVault = "Notes"

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "tasker:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("tasker", flag.ContinueOnError)
	root := fs.String("vault", "", "путь к папке с заметками (по умолчанию ~/"+defaultVault+")")
	if err := fs.Parse(args); err != nil {
		return err
	}

	// Раскладка клавиш и список хранилищ живут в домашней папке: они
	// принадлежат человеку, а не набору заметок (SPEC §8.11).
	keymap, err := app.NewKeymap("")
	if err != nil {
		return err
	}
	// Замыкание, а не готовый диалог: приложения, у которого его спрашивать,
	// ещё нет, а путь к хранилищу нужен прямо сейчас. К моменту вызова
	// instance уже заполнен — Choose зовут из вебвью, то есть после запуска.
	var (
		instance *application.App
		window   *application.WebviewWindow
	)
	vaults, err := app.NewVaults("", func() (string, error) {
		if instance == nil {
			return "", errors.New("приложение ещё не запущено")
		}
		return instance.Dialog.OpenFile().
			CanChooseFiles(false).
			CanChooseDirectories(true).
			CanCreateDirectories(true).
			SetTitle("Папка с заметками").
			PromptForSingleSelection()
	}, func() {
		// Не Quit, а закрытие окна: на нём висит хук, который просит интерфейс
		// дописать буфер и уводит последние правки в историю. Приложение
		// завершится следом само — ApplicationShouldTerminateAfterLastWindowClosed.
		if window != nil {
			window.Close()
		}
	})
	if err != nil {
		return err
	}

	path, err := vaultPath(*root, vaults.Current())
	if err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Репозиторий и пачку коммитов поднимает сам сервис — и только если
	// история в этом хранилище включена. Выключенная означает обычную папку с
	// файлами, в которой .git не появляется вовсе.
	service, err := notes.Open(ctx, path, notes.Options{
		Origin:  vault.OriginUser,
		OnError: logError,
	})
	if err != nil {
		return err
	}
	defer service.Close()

	// Первая сверка до открытия окна: список заметок должен быть готов к
	// моменту, когда пользователь его увидит.
	if _, err := service.Sync(ctx); err != nil {
		return err
	}

	files, err := watcher.Start(ctx, service.Vault().Root(), watcher.Options{OnError: logError})
	if err != nil {
		return err
	}
	// Событие по файлу, который записали мы сами, наружу не выходит — иначе
	// редактор примется перечитывать собственный буфер (SPEC §5.3).
	service.Vault().OnWrite(files.Ignore)

	instance, window = newApplication(service, keymap, vaults)

	go app.NewWatch(service, emitter(instance), logError).Run(ctx, files.Events())
	registerClosing(instance, window, service)

	return instance.Run()
}

// registerClosing перехватывает закрытие окна, чтобы интерфейс успел дописать
// буфер, а история — забрать последние правки (SPEC §6).
func registerClosing(instance *application.App, window *application.WebviewWindow, service *notes.Service) {
	closing := app.NewClosing(func() { instance.Event.Emit(app.EventBeforeClose, nil) }, 0)
	// Сервис регистрируется отдельно: Ready вызывается из вебвью и иначе туда
	// не попадёт.
	instance.RegisterService(application.NewService(closing))

	var finishing atomic.Bool
	window.RegisterHook(events.Common.WindowClosing, func(event *application.WindowEvent) {
		if finishing.Load() {
			// Второй заход: буфер уже сброшен, закрываемся по-настоящему.
			return
		}
		event.Cancel()
		finishing.Store(true)

		// В отдельной горутине: хук выполняется в главном потоке, и ждать
		// ответа прямо здесь значит остановить тот самый вебвью, от которого
		// ответа и ждём.
		go func() {
			if !closing.Wait() {
				logError(errors.New("интерфейс не ответил до закрытия, несохранённое могло пропасть"))
			}
			// При включённой пачке здесь лежит всё, что накопилось с последнего
			// окна; без неё — то, что записалось мимо обычного пути. С
			// выключенной историей коммитить нечего и некуда.
			if err := service.FlushCommits(context.Background()); err != nil {
				logError(err)
			}
			if git := service.Git(); git != nil {
				if _, err := git.Commit(context.Background(), gitstore.NotesMessage(nil)); err != nil {
					logError(err)
				}
			}
			window.Close()
		}()
	})
}

// emitter отдаёт функцию рассылки событий в окно.
func emitter(instance *application.App) func(name string, data any) {
	return func(name string, data any) {
		instance.Event.Emit(name, data)
	}
}

// logError пишет в stderr.
//
// При запуске из Finder stderr уходит в никуда — это известное ограничение
// десктопного приложения (docs/DESKTOP.md §7). Пока сообщений мало, отдельного
// журнала не заводим.
func logError(err error) {
	log.Printf("tasker: %v", err)
}

func newApplication(
	service *notes.Service,
	keymap *app.Keymap,
	vaults *app.Vaults,
) (*application.App, *application.WebviewWindow) {
	instance := application.New(application.Options{
		Name:        "Tasker",
		Description: "Заметки и задачи",
		Services: []application.Service{
			application.NewService(app.NewNotes(service)),
			application.NewService(app.NewSettings(filepath.Join(service.Vault().Root(), ".tasker"))),
			application.NewService(keymap),
			application.NewService(vaults),
			application.NewService(app.NewInfo(service, keymap, vaults)),
			application.NewService(app.NewGit(service)),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(tasker.Assets),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})

	// Своё меню вместо стандартного.
	//
	// Стандартное молча забирает себе ⌘+, ⌘- и ⌘0 под масштаб вебвью, а
	// ускорители меню в macOS перехватывают клавишу до вебвью — обработчик
	// в интерфейсе не вызывается вовсе, и переназначить их через keymap.json
	// невозможно. Заодно уходит ⌘R: перезагрузка страницы в редакторе
	// заметок выбрасывает несохранённый буфер.
	instance.Menu.SetApplicationMenu(taskerMenu())

	window := instance.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:  "Tasker",
		Width:  1200,
		Height: 800,
		Mac: application.MacWindow{
			// Перетаскивание задаётся разметкой (--wails-draggable), а не
			// высотой невидимого заголовка. Тот вариант тянет окно на любой
			// щелчок в верхней полосе, без исключений, — и кнопку туда уже
			// не поставить. С разметкой полоса тянется, а то, что на ней
			// лежит, остаётся нажимаемым.
			InvisibleTitleBarHeight: 0,
			TitleBar:                application.MacTitleBarHiddenInset,
			// Окно всегда полупрозрачное, а плотность панелей решает CSS.
			// Переключателя на лету у Wails нет, поэтому иначе смена
			// прозрачности требовала бы перезапуска. При нулевой настройке
			// панели красятся непрозрачно и разницы не видно.
			Backdrop: application.MacBackdropTranslucent,
		},
		BackgroundColour: application.NewRGBA(0, 0, 0, 0),
		URL:              "/",
	})
	return instance, window
}

// taskerMenu собирает меню из ролей, оставляя клавиши приложению.
//
// Всё, что здесь есть, — системное: буфер обмена, окно, справка. Команды
// самого приложения живут в keymap.json и в экране шоткатов, и дублировать их
// пунктами меню значит завести второе место, где они могут разойтись.
func taskerMenu() *application.Menu {
	menu := application.NewMenu()
	menu.AddRole(application.AppMenu)
	menu.AddRole(application.FileMenu)
	menu.AddRole(application.EditMenu)
	menu.AddRole(application.WindowMenu)
	menu.AddRole(application.HelpMenu)
	return menu
}

// vaultPath разворачивает путь к vault.
//
// Порядок: флаг сильнее всего — им запускают на чужой папке, не трогая
// выбранную; дальше записанное в vaults.json; в последнюю очередь ~/Notes.
func vaultPath(flagValue, stored string) (string, error) {
	if flagValue != "" {
		return flagValue, nil
	}
	// Записанную папку могли переименовать или отключить внешний диск. Тогда
	// молча уходим к умолчанию: приложение, которое не открывается, хуже
	// приложения, открывшего не ту папку.
	if stored != "" {
		if info, err := os.Stat(stored); err == nil && info.IsDir() {
			return stored, nil
		}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("определить домашнюю папку: %w", err)
	}
	path := filepath.Join(home, defaultVault)
	if err := os.MkdirAll(path, 0o755); err != nil {
		return "", fmt.Errorf("создать %s: %w", path, err)
	}
	return path, nil
}
