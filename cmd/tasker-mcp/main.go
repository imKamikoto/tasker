// Command tasker-mcp — MCP-сервер поверх того же ядра, поднимаемый Claude Code
// по stdio. Он не общается с приложением: работает с vault напрямую и потому
// пишет заметки даже когда приложение закрыто. См. docs/MCP.md.
package main

import (
	"fmt"
	"os"
)

func main() {
	// Фаза 2. См. docs/ROADMAP.md.
	fmt.Fprintln(os.Stderr, "tasker-mcp: не реализовано, фаза 2")
	os.Exit(1)
}
