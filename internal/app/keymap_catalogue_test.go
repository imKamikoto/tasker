package app

import (
	"os"
	"regexp"
	"testing"
)

// catalogue — путь к списку команд, который показывает экран настроек.
const catalogue = "../../frontend/src/commands.ts"

var catalogueEntry = regexp.MustCompile(`id:\s*"([^"]+)"`)

// Экран настроек перечисляет команды своим списком: keymap.json описывает, что
// на что назначено, и снятая привязка стёрла бы команду оттуда вместе с
// возможностью назначить её заново. Цена такого решения — два списка, которые
// обязаны совпадать. Этот тест и есть их сверка.
func TestEveryDefaultCommandIsInTheCatalogue(t *testing.T) {
	raw, err := os.ReadFile(catalogue)
	if err != nil {
		t.Skipf("каталога команд нет: %v", err)
	}

	known := make(map[string]bool)
	for _, found := range catalogueEntry.FindAllStringSubmatch(string(raw), -1) {
		known[found[1]] = true
	}
	if len(known) == 0 {
		t.Fatalf("в %s не нашлось ни одной команды — сломался разбор", catalogue)
	}

	for context, bindings := range defaultKeymap() {
		for combination, command := range bindings {
			if !known[command] {
				t.Errorf("команда %q (%s, %s) есть в умолчаниях, но её нет в %s",
					command, context, combination, catalogue)
			}
		}
	}
}
