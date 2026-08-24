package app

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// motionsFile — где интерфейс перечисляет вимовские движения.
const motionsFile = "../../frontend/src/keys.ts"

var (
	motionsBlock = regexp.MustCompile(`(?s)vimMotionKeys[^{]*\{(.*?)\n\};`)
	motionsLine  = regexp.MustCompile(`"?([\w-]+)"?:\s*\[([^\]]*)\]`)
	motionsKey   = regexp.MustCompile(`"([^"]+)"`)
)

// Выключенные движения вима не должны ничего отнимать.
//
// Интерфейс снимает эти привязки, когда человек отказался от вима, и держится
// на обещании: у каждой такой клавиши есть невимовый дублёр — стрелка. Обещание
// живёт в двух местах (список движений в интерфейсе, умолчания здесь), и без
// сверки они разойдутся молча. Именно так и вышло со сменой фокуса: буквы
// Ctrl+Shift+H и L были, стрелок к ним не было, и снятие букв оставило бы
// колонки без клавиатуры вовсе.
func TestEveryVimMotionHasANonVimTwin(t *testing.T) {
	raw, err := os.ReadFile(motionsFile)
	if err != nil {
		t.Skipf("списка движений нет: %v", err)
	}

	block := motionsBlock.FindStringSubmatch(string(raw))
	if block == nil {
		t.Fatalf("в %s не нашёлся vimMotionKeys — сломался разбор", motionsFile)
	}

	motions := make(map[string]map[string]bool)
	for _, line := range motionsLine.FindAllStringSubmatch(block[1], -1) {
		keys := make(map[string]bool)
		for _, key := range motionsKey.FindAllStringSubmatch(line[2], -1) {
			keys[key[1]] = true
		}
		motions[line[1]] = keys
	}
	if len(motions) == 0 {
		t.Fatalf("в %s не разобралось ни одного контекста", motionsFile)
	}

	defaults := defaultKeymap()
	for context, keys := range motions {
		bindings, ok := defaults[context]
		if !ok {
			t.Errorf("контекст %q есть в %s, но не в умолчаниях", context, motionsFile)
			continue
		}
		for key := range keys {
			command, bound := bindings[key]
			if !bound {
				// Движение может быть перечислено на вырост — это не ошибка,
				// проверять тогда нечего.
				continue
			}
			if !hasTwin(bindings, keys, command, key) {
				t.Errorf("%s: команда %q висит только на вимовской клавише %q — "+
					"со снятыми движениями она станет недоступна",
					context, command, key)
			}
		}
	}
}

// hasTwin ищет ту же команду на клавише, которая движением вима не считается.
func hasTwin(bindings map[string]string, motions map[string]bool, command, self string) bool {
	for key, bound := range bindings {
		if key == self || bound != command || motions[key] {
			continue
		}
		return true
	}
	return false
}

// Разбор списка движений должен видеть то, что там действительно написано.
// Без этой проверки сломанный регексп дал бы пустую карту и зелёный тест выше.
func TestVimMotionsParse(t *testing.T) {
	raw, err := os.ReadFile(motionsFile)
	if err != nil {
		t.Skipf("списка движений нет: %v", err)
	}
	block := motionsBlock.FindStringSubmatch(string(raw))
	if block == nil {
		t.Fatalf("vimMotionKeys не нашёлся")
	}
	found := motionsLine.FindAllStringSubmatch(block[1], -1)
	if len(found) < 2 {
		t.Fatalf("разобрано контекстов: %d", len(found))
	}
	for _, line := range found {
		if !strings.Contains(line[2], `"`) {
			t.Errorf("контекст %q разобрался без клавиш: %q", line[1], line[2])
		}
	}
}
