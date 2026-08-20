package watcher

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// Значения по умолчанию.
//
// Дебаунс обязателен: атомарная запись выглядит как шторм из CREATE, REMOVE,
// CREATE и RENAME, а macOS вдобавок постоянно шуршит .DS_Store (SPEC §5.3).
//
// Окно «своих» записей — 2 секунды: столько может пройти между тем, как файл
// лёг на диск, и тем, как событие о нём доедет до нас.
const (
	defaultDebounce = 300 * time.Millisecond
	defaultSweep    = 5 * time.Minute
	defaultOwnWrite = 2 * time.Second
)

// trashDir — единственный скрытый каталог, за которым следим (SPEC §4.1, §4.3).
const trashDir = ".trash"

// Batch — накопленные за окно дебаунса изменения.
type Batch struct {
	// Paths — абсолютные пути заметок, которые надо перечитать. Существует ли
	// файл сейчас, решает потребитель: между событием и обработкой он мог и
	// появиться, и исчезнуть.
	Paths []string

	// Full — пора свериться целиком. Ставится по таймеру периодической сверки,
	// при перестройке дерева каталогов и когда fsnotify сообщил об ошибке, то
	// есть мог потерять события.
	Full bool
}

// Options — настройки наблюдения. Нулевые значения заменяются умолчаниями.
type Options struct {
	Debounce time.Duration
	Sweep    time.Duration
	OwnWrite time.Duration

	// OnError вызывается на ошибках наблюдения. Их нельзя проглатывать: каждая
	// означает, что события могли потеряться. Потеря при этом безопасна —
	// вместе с вызовом всегда уходит пакет с Full, — но знать о ней надо.
	OnError func(error)
}

func (o Options) withDefaults() Options {
	if o.Debounce <= 0 {
		o.Debounce = defaultDebounce
	}
	if o.Sweep <= 0 {
		o.Sweep = defaultSweep
	}
	if o.OwnWrite <= 0 {
		o.OwnWrite = defaultOwnWrite
	}
	if o.OnError == nil {
		o.OnError = func(error) {}
	}
	return o
}

// ownWrite — запись, сделанная нами самими.
type ownWrite struct {
	modTime  time.Time
	deadline time.Time
}

// Watcher следит за изменениями в vault.
//
// Он — оптимизация задержки, а не источник правды: периодическая сверка ловит
// всё, что он проспал (SPEC §5.3). Поэтому терять события здесь не страшно, а
// вот врать про них — страшно.
type Watcher struct {
	root string
	opts Options
	fsw  *fsnotify.Watcher
	out  chan Batch

	// Источники событий держим полями, а не читаем из fsw напрямую: так тест
	// подставляет свой канал вместо того, чтобы писать в чужой и ловить гонку
	// с горутиной fsnotify.
	events <-chan fsnotify.Event
	errs   <-chan error

	mu   sync.Mutex
	own  map[string]ownWrite
	dirs map[string]struct{}
}

// Start запускает наблюдение. Останавливается отменой ctx: тогда канал Events
// закрывается, а все ресурсы освобождаются.
func Start(ctx context.Context, root string, opts Options) (*Watcher, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("watch %s: %w", root, err)
	}
	if info, err := os.Stat(abs); err != nil {
		return nil, fmt.Errorf("watch %s: %w", root, err)
	} else if !info.IsDir() {
		return nil, fmt.Errorf("watch %s: not a directory", root)
	}

	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("watch %s: %w", root, err)
	}

	w := &Watcher{
		root:   abs,
		opts:   opts.withDefaults(),
		fsw:    fsw,
		out:    make(chan Batch, 1),
		events: fsw.Events,
		errs:   fsw.Errors,
		own:    make(map[string]ownWrite),
		dirs:   make(map[string]struct{}),
	}

	if _, err := w.addTree(abs); err != nil {
		fsw.Close()
		return nil, fmt.Errorf("watch %s: %w", root, err)
	}

	go w.loop(ctx)
	return w, nil
}

// Events отдаёт пакеты изменений. Канал закрывается при отмене контекста.
func (w *Watcher) Events() <-chan Batch { return w.out }

// Ignore помечает запись как свою: события по этому пути не выйдут наружу,
// пока mtime файла совпадает с переданным и не истекло окно.
//
// Сверяется именно mtime, а не один только путь: если в том же окне файл
// тронет кто-то ещё, mtime разойдётся, и такое событие обязано дойти — иначе
// открытый в редакторе буфер молча разъедется с диском.
func (w *Watcher) Ignore(path string, modTime time.Time) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.own[abs] = ownWrite{modTime: modTime, deadline: time.Now().Add(w.opts.OwnWrite)}
}

// isOwn проверяет и подчищает реестр. Вызывается в момент отправки пакета, а не
// приёма события: Ignore может прийти уже после того, как событие получено.
func (w *Watcher) isOwn(path string) bool {
	w.mu.Lock()
	rec, ok := w.own[path]
	if ok && time.Now().After(rec.deadline) {
		delete(w.own, path)
		ok = false
	}
	w.mu.Unlock()
	if !ok {
		return false
	}

	info, err := os.Stat(path)
	if err != nil {
		// Файла нет — значит его удалили, и это уже не наша запись.
		return false
	}
	return info.ModTime().Equal(rec.modTime)
}

func (w *Watcher) loop(ctx context.Context) {
	defer close(w.out)
	defer w.fsw.Close()

	sweep := time.NewTicker(w.opts.Sweep)
	defer sweep.Stop()

	var (
		pending  = make(map[string]struct{})
		full     bool
		flushAt  <-chan time.Time
		debounce *time.Timer
	)
	arm := func() {
		if debounce == nil {
			debounce = time.NewTimer(w.opts.Debounce)
			flushAt = debounce.C
		}
	}
	disarm := func() {
		debounce = nil
		flushAt = nil
	}

	for {
		select {
		case <-ctx.Done():
			return

		case ev, ok := <-w.events:
			if !ok {
				return
			}
			paths, tree := w.classify(ev)
			for _, p := range paths {
				pending[p] = struct{}{}
			}
			if tree {
				full = true
			}
			if len(paths) > 0 || tree {
				arm()
			}

		case err, ok := <-w.errs:
			if !ok {
				return
			}
			// Ошибка наблюдения означает, что события могли потеряться.
			// Единственный честный ответ — попросить сверку целиком.
			w.opts.OnError(err)
			full = true
			arm()

		case <-sweep.C:
			full = true
			arm()

		case <-flushAt:
			disarm()
			w.emit(ctx, pending, full)
			pending = make(map[string]struct{})
			full = false
		}
	}
}

// emit отправляет накопленное, отфильтровав собственные записи.
func (w *Watcher) emit(ctx context.Context, pending map[string]struct{}, full bool) {
	batch := Batch{Full: full}
	for p := range pending {
		if w.isOwn(p) {
			continue
		}
		batch.Paths = append(batch.Paths, p)
	}
	if len(batch.Paths) == 0 && !batch.Full {
		return
	}

	select {
	case w.out <- batch:
	case <-ctx.Done():
	}
}

// classify решает, что делать с событием: какие пути отдать наружу и надо ли
// просить полную сверку.
func (w *Watcher) classify(ev fsnotify.Event) (paths []string, tree bool) {
	if !w.inside(ev.Name) {
		return nil, false
	}

	// Каталог создали: событий о том, что уже лежит внутри, не будет — их надо
	// собрать самим, а на каталог довесить наблюдение.
	if info, err := os.Stat(ev.Name); err == nil && info.IsDir() {
		if skipDir(filepath.Base(ev.Name)) {
			return nil, false
		}
		found, err := w.addTree(ev.Name)
		if err != nil {
			w.opts.OnError(err)
		}
		return found, true
	}

	// Пути больше нет. Если это был каталог, дерево перестроилось.
	if _, watched := w.watching(ev.Name); watched {
		w.forget(ev.Name)
		return nil, true
	}

	if !isNote(ev.Name) {
		return nil, false
	}
	return []string{ev.Name}, false
}

// addTree вешает наблюдение на каталог и все вложенные, возвращая найденные по
// дороге заметки.
//
// Каталог, за которым не удалось проследить, пропускается, а не роняет весь
// обход. Причины бывают разные — нет прав, кончились дескрипторы, каталог
// исчез, пока мы до него шли, — и ни одна из них не стоит того, чтобы остаться
// вообще без наблюдения: периодическая сверка всё равно всё найдёт, просто
// медленнее. Вызывающий узнаёт о каждом таком случае через OnError.
func (w *Watcher) addTree(dir string) ([]string, error) {
	var found []string
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return nil
			}
			if p == dir {
				return err
			}
			w.opts.OnError(fmt.Errorf("watch %s: %w", p, err))
			return fs.SkipDir
		}
		if d.IsDir() {
			if p != dir && skipDir(d.Name()) {
				return fs.SkipDir
			}
			if err := w.fsw.Add(p); err != nil {
				if p == dir {
					return fmt.Errorf("watch %s: %w", p, err)
				}
				w.opts.OnError(fmt.Errorf("watch %s: %w", p, err))
				return fs.SkipDir
			}
			w.remember(p)
			return nil
		}
		if isNote(p) {
			found = append(found, p)
		}
		return nil
	})
	return found, err
}

func (w *Watcher) remember(dir string) {
	w.mu.Lock()
	w.dirs[dir] = struct{}{}
	w.mu.Unlock()
}

func (w *Watcher) forget(dir string) {
	w.mu.Lock()
	delete(w.dirs, dir)
	w.mu.Unlock()
}

func (w *Watcher) watching(dir string) (struct{}, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	v, ok := w.dirs[dir]
	return v, ok
}

// inside отсекает пути за пределами корня: наблюдение вешается только внутри,
// но событие может прийти с чем угодно.
func (w *Watcher) inside(path string) bool {
	rel, err := filepath.Rel(w.root, path)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// skipDir — скрытые каталоги игнорируются, кроме корзины (SPEC §4.1).
func skipDir(name string) bool {
	return strings.HasPrefix(name, ".") && name != trashDir
}

// isNote — заметка, а не скрытый файл, не .DS_Store и не наш временный файл
// (он тоже начинается с точки).
func isNote(path string) bool {
	name := filepath.Base(path)
	if strings.HasPrefix(name, ".") {
		return false
	}
	return strings.EqualFold(filepath.Ext(name), ".md")
}
