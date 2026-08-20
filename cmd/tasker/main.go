// Command tasker — само приложение: окно macOS с WKWebView внутри и сервисы
// Wails поверх internal/notes. См. docs/SPEC.md §6.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/wailsapp/wails/v3/pkg/application"

	"tasker"
	"tasker/internal/app"
	"tasker/internal/notes"
	"tasker/internal/vault"
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

	path, err := vaultPath(*root)
	if err != nil {
		return err
	}

	ctx := context.Background()
	service, err := notes.Open(ctx, path, notes.Options{Origin: vault.OriginUser})
	if err != nil {
		return err
	}
	defer service.Close()

	// Первая сверка до открытия окна: список заметок должен быть готов к
	// моменту, когда пользователь его увидит.
	if _, err := service.Sync(ctx); err != nil {
		return err
	}

	return newApplication(service).Run()
}

func newApplication(service *notes.Service) *application.App {
	instance := application.New(application.Options{
		Name:        "Tasker",
		Description: "Заметки и задачи",
		Services: []application.Service{
			application.NewService(app.NewNotes(service)),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(tasker.Assets),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})

	instance.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:  "Tasker",
		Width:  1200,
		Height: 800,
		Mac: application.MacWindow{
			InvisibleTitleBarHeight: 42,
			TitleBar:                application.MacTitleBarHiddenInset,
		},
		BackgroundColour: application.NewRGB(26, 23, 20),
		URL:              "/",
	})
	return instance
}

// vaultPath разворачивает путь к vault: флаг, иначе ~/Notes.
func vaultPath(flagValue string) (string, error) {
	if flagValue != "" {
		return flagValue, nil
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
