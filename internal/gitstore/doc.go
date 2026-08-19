// Package gitstore ведёт историю vault: инициализация репозитория, автокоммит с
// дебаунсом, log --follow по файлу, diff двух ревизий.
//
// Это единственная страховка от потери данных, поэтому здесь никогда не
// вызываются reset --hard, clean, checkout -- . и force-push. См. SPEC §4.5.
//
// Пакет не импортирует Wails — см. CLAUDE.md, инвариант 4.
package gitstore
