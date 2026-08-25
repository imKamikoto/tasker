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

// Движение внутри колонки не должно пропадать вместе с вимом.
//
// j и k ходят по списку, h и l сворачивают ветку — это навигация по
// содержимому, и отказ от вима не должен её отнимать. У каждой такой клавиши
// есть невимовый дублёр — стрелка. Обещание живёт в двух местах (список
// движений в интерфейсе, умолчания здесь), и без сверки они разойдутся молча.
//
// Смена колонок сюда не входит и проверяется отдельно ниже: она сама и есть
// вимовская навигация, а не навигация, которой вим лишь помогает.
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
		// Глобальный контекст — это смена колонок, у неё правило своё.
		if context == ContextGlobal {
			continue
		}
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

// Смена колонок — вимовская навигация целиком, а не навигация с вимовским
// ускорителем.
//
// Дублёра на стрелках у неё нет намеренно: индикатор фокуса в титульной строке
// существует ради слепого переключения по ⌃⇧H и ⌃⇧L и вместе с движениями
// пропадает. Вернёте стрелки — фокус начнёт переезжать вслепую и без
// индикатора; тогда надо возвращать и его, а не только привязку. Этот тест —
// напоминание об этой связке, а не запрет.
func TestColumnSwitchingIsVimOnly(t *testing.T) {
	bindings := defaultKeymap()[ContextGlobal]

	for command, vimKey := range map[string]string{
		"focus.prev": "ctrl+shift+h",
		"focus.next": "ctrl+shift+l",
	} {
		if bindings[vimKey] != command {
			t.Errorf("%s не привязан к %s", command, vimKey)
		}
		for key, bound := range bindings {
			if bound == command && key != vimKey {
				t.Errorf("у %s появился дублёр %q — вместе с ним надо решить, "+
					"что делать с индикатором фокуса: он пропадает вместе с движениями",
					command, key)
			}
		}
	}
}
