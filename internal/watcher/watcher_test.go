package watcher

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
)

// Окна в тестах короткие: иначе каждый прогон стоит секунды, а проверяем мы
// логику, а не конкретные числа.
func testOptions() Options {
	return Options{
		Debounce: 40 * time.Millisecond,
		Sweep:    time.Hour, // сверку включаем только там, где она и проверяется
		OwnWrite: 500 * time.Millisecond,
	}
}

func start(t *testing.T, root string, opts Options) *Watcher {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	w, err := Start(ctx, root, opts)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	// Дать дереву обойтись и watch'ам встать до первой правки.
	time.Sleep(60 * time.Millisecond)
	return w
}

// waitBatch ждёт пакет, в котором есть хоть что-то. Пустые пакеты не приходят,
// но дребезг файловой системы может размазать события по нескольким.
func waitBatch(t *testing.T, w *Watcher, timeout time.Duration) Batch {
	t.Helper()
	select {
	case b, ok := <-w.Events():
		if !ok {
			t.Fatal("канал событий закрыт")
		}
		return b
	case <-time.After(timeout):
		t.Fatal("пакет не пришёл за " + timeout.String())
		return Batch{}
	}
}

// collect собирает всё, что пришло за отведённое время.
func collect(t *testing.T, w *Watcher, d time.Duration) []Batch {
	t.Helper()
	var out []Batch
	deadline := time.After(d)
	for {
		select {
		case b, ok := <-w.Events():
			if !ok {
				return out
			}
			out = append(out, b)
		case <-deadline:
			return out
		}
	}
}

func hasPath(b Batch, want string) bool {
	for _, p := range b.Paths {
		if filepath.Base(p) == want {
			return true
		}
	}
	return false
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestWatcherNoticesCreate(t *testing.T) {
	root := t.TempDir()
	w := start(t, root, testOptions())

	write(t, filepath.Join(root, "новая.md"), "тело\n")

	b := waitBatch(t, w, 2*time.Second)
	if !hasPath(b, "новая.md") {
		t.Errorf("пакет = %+v, ожидалась новая.md", b)
	}
}

// Правка на месте — та, что не создаёт временного файла. Именно её пропускает
// наблюдение за одними лишь папками.
func TestWatcherNoticesInPlaceEdit(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "note.md")
	write(t, path, "старое\n")
	w := start(t, root, testOptions())

	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString("дописали\n")
	f.Sync()
	f.Close()

	b := waitBatch(t, w, 2*time.Second)
	if !hasPath(b, "note.md") {
		t.Errorf("пакет = %+v, ожидалась note.md", b)
	}
}

func TestWatcherNoticesRemove(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "note.md")
	write(t, path, "тело\n")
	w := start(t, root, testOptions())

	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}

	b := waitBatch(t, w, 2*time.Second)
	if !hasPath(b, "note.md") {
		t.Errorf("пакет = %+v, ожидалась note.md", b)
	}
}

// Атомарная запись — это шторм из CREATE, REMOVE, CREATE и RENAME. Наружу
// должен выйти один путь, и временного файла среди путей быть не должно.
func TestWatcherCollapsesAtomicWrite(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "note.md")
	write(t, path, "старое\n")
	w := start(t, root, testOptions())

	tmp := filepath.Join(root, ".note.md.tmp123")
	write(t, tmp, "новое\n")
	if err := os.Rename(tmp, path); err != nil {
		t.Fatal(err)
	}

	b := waitBatch(t, w, 2*time.Second)
	if len(b.Paths) != 1 || !hasPath(b, "note.md") {
		t.Errorf("пакет = %+v, ожидался ровно один путь note.md", b)
	}
	for _, p := range b.Paths {
		if strings.Contains(filepath.Base(p), ".tmp") {
			t.Errorf("временный файл просочился: %s", p)
		}
	}
}

func TestWatcherIgnoresNoise(t *testing.T) {
	root := t.TempDir()
	w := start(t, root, testOptions())

	write(t, filepath.Join(root, ".DS_Store"), "мусор")
	write(t, filepath.Join(root, "заметки.txt"), "не markdown")
	write(t, filepath.Join(root, ".скрытая.md"), "скрытая")

	batches := collect(t, w, 400*time.Millisecond)
	for _, b := range batches {
		if len(b.Paths) > 0 {
			t.Errorf("шум прошёл фильтр: %+v", b)
		}
	}
}

// Новая папка: на неё надо довесить watch, а то, что уже успело в ней
// появиться, отдать пакетом — событий об этом не будет.
func TestWatcherWatchesNewDirectories(t *testing.T) {
	root := t.TempDir()
	w := start(t, root, testOptions())

	sub := filepath.Join(root, "Работа", "Баги")
	write(t, filepath.Join(sub, "внутри.md"), "тело\n")

	var seen bool
	for _, b := range collect(t, w, 1500*time.Millisecond) {
		if hasPath(b, "внутри.md") {
			seen = true
		}
	}
	if !seen {
		t.Error("файл во вновь созданной папке не замечен")
	}

	// И следующая правка в этой папке тоже должна доходить.
	write(t, filepath.Join(sub, "вторая.md"), "тело\n")
	var second bool
	for _, b := range collect(t, w, 1500*time.Millisecond) {
		if hasPath(b, "вторая.md") {
			second = true
		}
	}
	if !second {
		t.Error("watch на новую папку не повешен")
	}
}

func TestWatcherSkipsHiddenDirsButNotTrash(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, ".git", "note.md"), "тело\n")
	write(t, filepath.Join(root, ".trash", "старое.md"), "тело\n")
	w := start(t, root, testOptions())

	write(t, filepath.Join(root, ".git", "вторая.md"), "тело\n")
	write(t, filepath.Join(root, ".trash", "удалённая.md"), "тело\n")

	var trash, git bool
	for _, b := range collect(t, w, 800*time.Millisecond) {
		if hasPath(b, "удалённая.md") {
			trash = true
		}
		if hasPath(b, "вторая.md") {
			git = true
		}
	}
	if !trash {
		t.Error("корзина не отслеживается, а должна (SPEC §4.1)")
	}
	if git {
		t.Error(".git отслеживается, а не должен")
	}
}

// Реестр своих записей: событие по файлу, который записали мы сами, наружу не
// выходит, иначе редактор будет сам себя перечитывать по кругу (SPEC §5.3).
func TestWatcherIgnoresOwnWrites(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "note.md")
	write(t, path, "старое\n")
	w := start(t, root, testOptions())

	write(t, path, "записали сами\n")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	w.Ignore(path, info.ModTime())

	for _, b := range collect(t, w, 500*time.Millisecond) {
		if hasPath(b, "note.md") {
			t.Errorf("своя запись просочилась наружу: %+v", b)
		}
	}
}

// А вот если после нашей записи файл тронул кто-то ещё, mtime не совпадёт — и
// такое событие обязано дойти.
func TestWatcherDeliversForeignWriteAfterOwn(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "note.md")
	write(t, path, "старое\n")
	w := start(t, root, testOptions())

	write(t, path, "записали сами\n")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	w.Ignore(path, info.ModTime())

	// Кто-то другой правит тот же файл в том же окне.
	time.Sleep(10 * time.Millisecond)
	write(t, path, "а это уже не мы, и содержимое другой длины\n")

	var seen bool
	for _, b := range collect(t, w, 1500*time.Millisecond) {
		if hasPath(b, "note.md") {
			seen = true
		}
	}
	if !seen {
		t.Error("чужая правка после своей потерялась — буфер редактора разойдётся с диском")
	}
}

// Дебаунс: очередь быстрых правок должна свернуться в один пакет.
func TestWatcherDebounces(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "note.md")
	write(t, path, "0\n")
	w := start(t, root, testOptions())

	for i := 0; i < 8; i++ {
		write(t, path, strings.Repeat("x", i+1)+"\n")
		time.Sleep(3 * time.Millisecond)
	}

	batches := collect(t, w, 600*time.Millisecond)
	withPath := 0
	for _, b := range batches {
		if len(b.Paths) > 0 {
			withPath++
		}
	}
	if withPath == 0 {
		t.Fatal("не пришло ни одного пакета")
	}
	if withPath > 2 {
		t.Errorf("пакетов с путями %d — дебаунс не сработал", withPath)
	}
}

// Периодическая сверка: watcher — оптимизация задержки, а не источник правды,
// и обязан регулярно просить пересчитать всё (SPEC §5.3).
func TestWatcherPeriodicSweep(t *testing.T) {
	root := t.TempDir()
	opts := testOptions()
	opts.Sweep = 80 * time.Millisecond
	w := start(t, root, opts)

	var full bool
	for _, b := range collect(t, w, 600*time.Millisecond) {
		if b.Full {
			full = true
		}
	}
	if !full {
		t.Error("периодическая сверка не пришла")
	}
}

func TestWatcherStopsOnContextCancel(t *testing.T) {
	root := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	w, err := Start(ctx, root, testOptions())
	if err != nil {
		t.Fatal(err)
	}

	cancel()
	select {
	case _, ok := <-w.Events():
		if ok {
			// Мог прийти последний пакет; ждём закрытия дальше.
			select {
			case _, ok := <-w.Events():
				if ok {
					t.Error("канал не закрылся после отмены")
				}
			case <-time.After(time.Second):
				t.Error("канал не закрылся после отмены")
			}
		}
	case <-time.After(time.Second):
		t.Error("канал не закрылся после отмены")
	}
}

func TestStartRejectsMissingRoot(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if _, err := Start(ctx, filepath.Join(t.TempDir(), "нет"), testOptions()); err == nil {
		t.Error("ожидалась ошибка для несуществующей папки")
	}
}

// Окно дебаунса фиксированное, а не перезапускаемое на каждом событии. Разница
// принципиальная: при перезапуске непрерывная запись — автосохранение редактора
// раз в 200 мс при окне 300 мс — не отдала бы наружу вообще ничего.
func TestWatcherDebounceDoesNotStarve(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "note.md")
	write(t, path, "0\n")
	w := start(t, root, testOptions())

	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; ; i++ {
			select {
			case <-stop:
				return
			default:
			}
			os.WriteFile(path, []byte(strings.Repeat("x", i%50+1)+"\n"), 0o644)
			time.Sleep(8 * time.Millisecond)
		}
	}()
	defer func() { close(stop); <-done }()

	// Пакет обязан прийти, пока запись ещё идёт.
	select {
	case b := <-w.Events():
		if !hasPath(b, "note.md") {
			t.Errorf("пакет = %+v", b)
		}
	case <-time.After(400 * time.Millisecond):
		t.Fatal("за 400 мс непрерывной записи не пришло ни одного пакета — дебаунс голодает")
	}
}

// Окно своих записей истекает: если приложение записало файл и забыло про
// него, а через минуту тот же файл правит человек снаружи, событие обязано
// дойти.
func TestWatcherOwnWriteWindowExpires(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "note.md")
	write(t, path, "старое\n")

	opts := testOptions()
	opts.OwnWrite = 60 * time.Millisecond
	w := start(t, root, opts)

	write(t, path, "наша запись\n")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	w.Ignore(path, info.ModTime())

	// Ждём, пока окно закроется, и трогаем файл, НЕ меняя mtime: иначе событие
	// прошло бы и по несовпадению времени, и про истечение окна тест бы ничего
	// не сказал. Chtimes с тем же временем меняет только ctime.
	time.Sleep(150 * time.Millisecond)
	if err := os.Chtimes(path, info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}

	var seen bool
	for _, b := range collect(t, w, 1500*time.Millisecond) {
		if hasPath(b, "note.md") {
			seen = true
		}
	}
	if !seen {
		t.Error("после истечения окна событие всё ещё гасится")
	}
}

// Появление или исчезновение каталога меняет пути сразу у многих заметок.
// Событий об этом не будет — единственный честный ответ это попросить сверку
// целиком.
func TestWatcherAsksFullOnTreeChange(t *testing.T) {
	root := t.TempDir()
	w := start(t, root, testOptions())

	sub := filepath.Join(root, "Работа")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}

	var full bool
	for _, b := range collect(t, w, 800*time.Millisecond) {
		if b.Full {
			full = true
		}
	}
	if !full {
		t.Error("создание каталога не попросило сверку")
	}

	if err := os.RemoveAll(sub); err != nil {
		t.Fatal(err)
	}
	full = false
	for _, b := range collect(t, w, 800*time.Millisecond) {
		if b.Full {
			full = true
		}
	}
	if !full {
		t.Error("удаление каталога не попросило сверку")
	}
}

// Ошибка наблюдения означает, что события могли потеряться. Единственный
// честный ответ — попросить сверку целиком и сказать об этом вызывающему.
func TestWatcherFullOnFsnotifyError(t *testing.T) {
	root := t.TempDir()
	opts := testOptions()
	reported := make(chan error, 1)
	opts.OnError = func(err error) {
		select {
		case reported <- err:
		default:
		}
	}

	// Свой канал ошибок вместо fsnotify'шного: писать в чужой канал, который
	// параллельно закрывает его владелец, — это гонка.
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		t.Fatal(err)
	}
	errs := make(chan error, 1)
	w := &Watcher{
		root:   root,
		opts:   opts.withDefaults(),
		fsw:    fsw,
		out:    make(chan Batch, 1),
		events: fsw.Events,
		errs:   errs,
		own:    make(map[string]ownWrite),
		dirs:   make(map[string]struct{}),
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go w.loop(ctx)

	boom := errors.New("наблюдение сорвалось")
	errs <- boom

	select {
	case got := <-reported:
		if !errors.Is(got, boom) {
			t.Errorf("вызывающему сообщили %v", got)
		}
	case <-time.After(time.Second):
		t.Error("ошибка не доехала до OnError — проглочена молча")
	}

	var full bool
	for _, b := range collect(t, w, 500*time.Millisecond) {
		if b.Full {
			full = true
		}
	}
	if !full {
		t.Error("после ошибки не попросили сверку — потерянные события так и останутся потерянными")
	}
}

// classify — единственное место, где решается судьба события. Путь снаружи
// корня прийти не должен, но если придёт, отдавать его наружу нельзя.
func TestClassifyRejectsPathsOutsideRoot(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "чужая.md")
	write(t, outside, "тело\n")
	w := start(t, root, testOptions())

	paths, tree := w.classify(fsnotify.Event{Name: outside, Op: fsnotify.Write})
	if len(paths) != 0 || tree {
		t.Errorf("classify отдал %v tree=%v для пути вне корня", paths, tree)
	}

	inside := filepath.Join(root, "своя.md")
	write(t, inside, "тело\n")
	paths, _ = w.classify(fsnotify.Event{Name: inside, Op: fsnotify.Write})
	if len(paths) != 1 {
		t.Errorf("classify отдал %v для пути внутри корня", paths)
	}
}

// Нечитаемый каталог не должен ронять watcher.
//
// На большее рассчитывать нельзя: замерено 2026-08-20, что любой нечитаемый
// элемент внутри наблюдаемого каталога — хоть файл, хоть папка — заставляет
// fsnotify молча перестать слать события по всему этому каталогу, причём Add
// возвращает nil. Единственное, что спасает, — периодическая сверка, и этот
// тест проверяет, что она продолжает приходить.
func TestWatcherSurvivesUnwatchableDir(t *testing.T) {
	root := t.TempDir()
	blocked := filepath.Join(root, "закрытая")
	if err := os.Mkdir(blocked, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(blocked, 0o755) })

	opts := testOptions()
	opts.Sweep = 80 * time.Millisecond
	reported := make(chan error, 4)
	opts.OnError = func(err error) {
		select {
		case reported <- err:
		default:
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	w, err := Start(ctx, root, opts)
	if err != nil {
		t.Fatalf("Start упал из-за одного нечитаемого каталога: %v", err)
	}

	select {
	case err := <-reported:
		if !strings.Contains(err.Error(), "закрытая") {
			t.Errorf("сообщили не о том каталоге: %v", err)
		}
	case <-time.After(time.Second):
		t.Error("о недоступном каталоге не сообщили — проблема осталась немой")
	}

	var full bool
	for _, b := range collect(t, w, 600*time.Millisecond) {
		if b.Full {
			full = true
		}
	}
	if !full {
		t.Error("сверка не приходит — а она здесь единственное, что работает")
	}
}
