package gitstore

import (
	"context"
	"sync"
	"time"
)

// DefaultDelay — окно автокоммита из SPEC §4.5.
const DefaultDelay = 90 * time.Second

// Autocommit собирает правки и коммитит их пачкой.
//
// Окно отсчитывается от первой накопленной правки, а не от последней. Спека
// говорит «дебаунс 90 секунд после последнего изменения», но при непрерывной
// работе такое окно не закрывается никогда, и обещание «больше 90 секунд работы
// потерять физически невозможно» (SPEC §10) перестало бы выполняться. Разница
// заметна только когда правки прекратились посередине окна — и там отсчёт от
// первой правки коммитит раньше, то есть безопаснее.
type Autocommit struct {
	store   *Store
	delay   time.Duration
	onError func(error)

	// wake будит цикл, не блокируя того, кто правит заметку.
	wake chan struct{}

	mu      sync.Mutex
	delayMu sync.RWMutex
	titles  []string
	seen    map[string]struct{}
}

// SetDelay меняет окно на лету. Уже запущенный отсчёт не трогает: сдвигать
// его при каждой правке настроек значит откладывать коммит бесконечно —
// ровно та ошибка, из-за которой окно считается от первой правки, а не от
// последней.
func (a *Autocommit) SetDelay(d time.Duration) {
	if d <= 0 {
		d = DefaultDelay
	}
	a.delayMu.Lock()
	a.delay = d
	a.delayMu.Unlock()
}

// Delay возвращает текущее окно.
func (a *Autocommit) Delay() time.Duration {
	a.delayMu.RLock()
	defer a.delayMu.RUnlock()
	return a.delay
}

// NewAutocommit создаёт автокоммит. Нулевая задержка заменяется DefaultDelay,
// нулевой обработчик ошибок — пустым.
func NewAutocommit(store *Store, delay time.Duration, onError func(error)) *Autocommit {
	if delay <= 0 {
		delay = DefaultDelay
	}
	if onError == nil {
		onError = func(error) {}
	}
	return &Autocommit{
		store:   store,
		delay:   delay,
		onError: onError,
		wake:    make(chan struct{}, 1),
		seen:    make(map[string]struct{}),
	}
}

// Touch сообщает, что заметка с таким заголовком изменилась. Не блокирует.
func (a *Autocommit) Touch(title string) {
	a.mu.Lock()
	if _, dup := a.seen[title]; !dup {
		a.seen[title] = struct{}{}
		a.titles = append(a.titles, title)
	}
	a.mu.Unlock()

	select {
	case a.wake <- struct{}{}:
	default:
	}
}

// Flush коммитит накопленное немедленно. Нужен перед массовыми операциями и
// при закрытии окна (SPEC §4.5).
//
// Работает и без запущенного Run: коммит делается прямо здесь, а проснувшийся
// позже таймер обнаружит, что коммитить нечего, и промолчит.
func (a *Autocommit) Flush(ctx context.Context) error {
	return a.commit(ctx)
}

// Run крутит цикл автокоммита до отмены контекста. При выходе фиксирует
// накопленное — иначе последняя правка осталась бы только на диске.
func (a *Autocommit) Run(ctx context.Context) {
	var (
		timer   *time.Timer
		fire    <-chan time.Time
		stopped = func() {
			timer = nil
			fire = nil
		}
	)
	defer func() {
		if timer != nil {
			timer.Stop()
		}
	}()

	for {
		select {
		case <-ctx.Done():
			// Контекст уже отменён, и передавать его в коммит нельзя: он бы
			// сразу же и отменил запись.
			if err := a.commit(context.WithoutCancel(ctx)); err != nil {
				a.onError(err)
			}
			return

		case <-a.wake:
			if timer == nil {
				timer = time.NewTimer(a.Delay())
				fire = timer.C
			}

		case <-fire:
			stopped()
			if err := a.commit(ctx); err != nil {
				a.onError(err)
			}
		}
	}
}

// commit забирает накопленные заголовки и фиксирует изменения.
func (a *Autocommit) commit(ctx context.Context) error {
	a.mu.Lock()
	titles := a.titles
	a.titles = nil
	a.seen = make(map[string]struct{})
	a.mu.Unlock()

	_, err := a.store.Commit(ctx, NotesMessage(titles))
	if err != nil {
		// Заголовки не возвращаем: следующая правка всё равно закоммитит и
		// эти файлы тоже, а копить их до бесконечности при постоянной ошибке
		// значит копить и память.
		return err
	}
	return nil
}
