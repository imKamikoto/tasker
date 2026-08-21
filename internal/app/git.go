package app

import (
	"context"
	"fmt"
	"time"

	"tasker/internal/notes"
)

// maxCommitWindow — потолок окна автокоммита.
//
// Полчаса: дальше обещание «больше окна работы потерять нельзя» перестаёт
// быть обещанием, а история превращается в один коммит за сессию.
const maxCommitWindow = 30 * 60

// GitSettings — то, что настройки показывают про историю.
type GitSettings struct {
	// WindowSeconds — окно автокоммита. Ноль означает коммит на каждое
	// сохранение.
	WindowSeconds int
}

// Git — сервис Wails: как часто правки уезжают в историю.
//
// Настройка живёт в интерфейсе (config.json) и приезжает сюда вызовом, а не
// читается Go напрямую: config.json для Go непрозрачен, и разбирать его в двух
// местах значит однажды разойтись.
type Git struct {
	service *notes.Service
}

// NewGit создаёт сервис.
func NewGit(service *notes.Service) *Git {
	return &Git{service: service}
}

// Settings возвращает текущее окно.
func (g *Git) Settings() GitSettings {
	return GitSettings{WindowSeconds: int(g.service.CommitWindow() / time.Second)}
}

// Configure задаёт окно автокоммита в секундах. Ноль — коммитить сразу.
//
// Переключение вниз, на немедленный коммит, сначала сбрасывает накопленное:
// иначе пачка, собранная при старой настройке, повисла бы до закрытия окна.
func (g *Git) Configure(ctx context.Context, seconds int) (GitSettings, error) {
	if seconds < 0 || seconds > maxCommitWindow {
		return GitSettings{}, fmt.Errorf("commit window %d: вне диапазона 0..%d", seconds, maxCommitWindow)
	}
	if seconds == 0 {
		if err := g.service.FlushCommits(ctx); err != nil {
			return GitSettings{}, err
		}
	}
	g.service.SetCommitWindow(time.Duration(seconds) * time.Second)
	return g.Settings(), nil
}

// CommitNow фиксирует накопленное немедленно.
func (g *Git) CommitNow(ctx context.Context) error {
	return g.service.FlushCommits(ctx)
}
