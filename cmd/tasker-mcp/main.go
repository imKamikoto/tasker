// Command tasker-mcp — MCP-сервер поверх того же ядра, поднимаемый Claude Code
// по stdio. Он не общается с приложением: работает с vault напрямую и потому
// пишет заметки даже когда приложение закрыто. См. docs/MCP.md.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"tasker/internal/notes"
	"tasker/internal/vault"
)

// version попадает в рукопожатие MCP и видна клиенту.
const version = "0.1.0"

func main() {
	if err := run(context.Background(), os.Args[1:], os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "tasker-mcp:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, stderr io.Writer) error {
	fs := flag.NewFlagSet("tasker-mcp", flag.ContinueOnError)
	fs.SetOutput(stderr)
	root := fs.String("vault", "", "путь к папке с заметками (обязательно)")
	fs.Usage = func() {
		fmt.Fprintf(stderr, "usage: tasker-mcp --vault <путь>\n\n")
		fmt.Fprintf(stderr, "MCP-сервер для Claude Code. Подключается по stdio:\n")
		fmt.Fprintf(stderr, "  claude mcp add tasker --scope user -- tasker-mcp --vault ~/Notes\n\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *root == "" {
		fs.Usage()
		return fmt.Errorf("не указан --vault")
	}

	svc, err := newService(ctx, *root)
	if err != nil {
		return err
	}
	defer svc.Close()

	return newServer(svc).Run(ctx, &mcp.StdioTransport{})
}

// newService открывает vault от имени агента: всё, что он создаёт, помечается
// origin: agent, а коммиты получают префикс agent (SPEC §4.2, §4.5).
func newService(ctx context.Context, root string) (*notes.Service, error) {
	return notes.Open(ctx, root, notes.Options{Origin: vault.OriginAgent})
}

func newServer(svc *notes.Service) *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "tasker",
		Title:   "Tasker — заметки и задачи",
		Version: version,
	}, nil)
	register(server, svc)
	return server
}
