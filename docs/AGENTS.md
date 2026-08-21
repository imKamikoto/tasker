# Tasker для агента

Справочник по тому, что в проекте есть и как оно устроено. Читать перед тем,
как что-то менять; `CLAUDE.md` рядом говорит **как** работать, этот файл —
**с чем**.

Порядок чтения для первого раза: `CLAUDE.md` → этот файл → раздел `docs/SPEC.md`
по своей задаче. `docs/DESKTOP.md` — если не работал с десктопом раньше.

---

## 1. Что это

Локальный markdown-редактор заметок-как-задач под macOS. Три особенности, ради
которых он существует (SPEC §1):

1. Редактор, в котором приятно печатать: CodeMirror 6 с вимом.
2. Заметка = задача: статус, закрепление, теги.
3. **Агент — полноправный клиент** через MCP, а не «AI-фича внутри».

Стек: Go 1.25+ · Wails v3.0.0-beta.9 · modernc.org/sqlite · React 19 + CM6.

Готовы фазы 0–4 из `docs/ROADMAP.md`. Приложение работает: список, редактор с
вимом, поиск, статусы, корзина, теги, массовые операции. Не сделаны фазы 5–8:
превью markdown, палитра команд, ссылки между заметками, картинки, шаблоны.

---

## 2. Три бинарника

| Что | Зачем | Нужен ли Wails |
| --- | --- | --- |
| `cmd/tasker` | приложение с окном | да, cgo обязателен |
| `cmd/tasker-mcp` | MCP-сервер для Claude Code | нет, статический бинарник |
| `cmd/tasker-scan` | отладочный CLI поверх ядра | нет |

`tasker-scan` не игрушка: если поиск работает из него, значит логика
действительно в Go. В день, когда он перестанет собираться без фронтенда,
граница поехала.

---

## 3. Слои

```
cmd/tasker ──┐                    cmd/tasker-mcp ──┐
             ▼                                     ▼
      internal/app  (Wails: сервисы, события)      │
             │        ← только здесь Wails         │
             ▼                                     ▼
                    internal/notes
        операции уровня заметки: файл + индекс + коммит вместе
                          │
        ┌─────────────────┼──────────────────┐
        ▼                 ▼                  ▼
 internal/vault    internal/index    internal/gitstore
   файлы, .md       SQLite, FTS5        история
                          ▲
                   internal/watcher
                    fsnotify, дебаунс
```

**`internal/notes` — главный слой.** «Создать заметку» это файл, строка индекса
и коммит **вместе**, и та же связка нужна и приложению, и MCP. В `internal/app`
её положить нельзя: там Wails, а MCP обязан оставаться бинарником без него.

Ни один пакет ниже `internal/app` не импортирует Wails. Это проверяемо: они
собираются и тестируются обычным `go test`.

---

## 4. Инварианты

Нарушать нельзя. Полный список в `CLAUDE.md`, здесь то, что чаще всего задевают.

**Файлы — источник правды, индекс производный.** Индекс сносится и строится
заново без потери данных. Отсюда следствие, о которое уже спотыкались: всё,
что пользователь выбрал руками, в индексе жить не может. Цвета тегов поэтому
лежат в `.tasker/tags.json`, а не в колонке `tags.color`.

**Запись `.md` только атомарная**: временный файл рядом → `fsync` → `rename` →
`fsync` каталога. `vault.WriteFileAtomic` — единственный способ.

**Неизвестные поля frontmatter сохраняются.** `internal/vault/frontmatter.go`
работает поверх AST goccy, а не структур: декодирование в структуру и обратно
молча убивает чужие поля, порядок ключей и комментарии. Неизменённый заголовок
отдаётся байт в байт.

**Никогда не звать** `git reset --hard`, `git clean`, `git checkout -- .`,
force-push. На это есть тест, который сканирует исходники `internal/gitstore`.

**Вся логика в Go.** Фронтенд не парсит frontmatter, не строит запросы, не
ходит в файловую систему. Понадобилось «по-быстрому распарсить в JS» — это
ошибка проектирования, метод добавляется в Go.

---

## 5. Справочник

### internal/vault — файлы

Работает с одной заметкой. Ничего не знает про индекс и git.

```go
Open(root string) (*Vault, error)          // EvalSymlinks сразу: пути сравниваются реальные

(*Vault) Load(path string) (*Note, error)
(*Vault) Create(n NewNote) (*Note, error)
(*Vault) Save(n *Note) error               // не пишет, если ничего не менялось
(*Vault) Move(n *Note, notebook string) error
(*Vault) Trash(n *Note) error              // + trashedFrom, trashedAt
(*Vault) Restore(n *Note) error
(*Vault) Delete(n *Note) error             // только из корзины
(*Vault) Backfill(n *Note) (bool, error)   // дописать id/title/даты файлу извне
(*Vault) EnsureNotebook(notebook string) (string, error)
(*Vault) RemoveEmptyNotebook(notebook string) error
(*Vault) OnWrite(fn func(path string, modTime time.Time))  // для watcher

NewID() string          // ULID
ValidID(s string) bool
Slug(title string) string
WriteFileAtomic(path string, data []byte, perm os.FileMode) error
```

`Note` = `Doc *Document` + `Path` + `Notebook` + `ModTime` + `Size`.
`Document` = `Meta *Frontmatter` + `Body string`.

Ошибки: `ErrNoteNotFound`, `ErrOutsideVault`, `ErrHiddenPath`, `ErrEmptyTitle`,
`ErrAlreadyTrashed`, `ErrNotTrashed`, `ErrInvalidStatus`, `ErrInvalidOrigin`,
`ErrInvalidFrontmatter`, `ErrNameCollision`.

### internal/index — SQLite и поиск

```go
Open(ctx, path string) (*Index, error)
(*Index) Put(ctx, r Record) error          // вставка или обновление по id
(*Index) Get(ctx, id string) (Record, error)
(*Index) GetByPath(ctx, path string) (Record, error)
(*Index) Delete(ctx, path string) error
(*Index) Search(ctx, q Query, opts SearchOptions) ([]Record, error)
(*Index) Scan(ctx, v *vault.Vault) (ScanResult, error)   // полный и инкрементальный
(*Index) States(ctx) (map[string]FileState, error)
(*Index) Notebooks(ctx) ([]Notebook, error)              // только с заметками
(*Index) Tags(ctx) ([]Tag, error)
(*Index) Backlinks(ctx, id string) ([]Record, error)
(*Index) ApplyTagColors(ctx, colors map[string]int) error

ParseQuery(input string) (Query, error)    // язык из SPEC §8.5
Excerpt(body string) string
CountTasks(body string) (total, done int)
ExtractLinks(body string) []string
```

`SearchOptions`: `Limit`, `Trash` (`TrashHidden` / `TrashIncluded` / `TrashOnly`),
`HideCompleted`, `Sort` (`SortUpdated` / `SortCreated` / `SortTitle` + `Reversed`).

Язык запросов: `слово`, `"фраза"`, `book:`, `tag:`, `status:`, `title:`, `body:`,
`is:pinned`, `has:task`, отрицание минусом, пробел = И. **ИЛИ в языке нет** —
поэтому «Активные» (active ИЛИ onHold) сделаны отдельным методом, а скрытие
завершённых — флагом `HideCompleted`, а не запросом.

### internal/notes — операции уровня заметки

Всё под межпроцессной блокировкой `flock` на `.tasker/vault.lock`.

```go
Open(ctx, root string, opts Options) (*Service, error)   // Options.Origin: user | agent

// одна заметка
Create · Get · Update · SetStatus · SetPinned · SetTags · Duplicate
Trash · Restore · Delete

// пачка: одна блокировка, один коммит
TrashMany · MoveMany · SetStatusMany · SetPinnedMany

// чтение
Search · Tasks · Notebooks · Tags · TagColors · Sync

// ноутбуки и теги
CreateNotebook · RenameNotebook · DeleteNotebook · RenameTag · SetTagColor
```

`UpdateParams` — **указатели**: `*string`, `*bool`. Иначе не отличить «не
передано» от «передан ноль», и любое обновление сбрасывало бы всё, о чём не
упомянули. `Body` взаимоисключающ с `Append`/`Prepend`.

### internal/app — Wails

Единственное место, знающее про Wails. Сервисы тонкие: разобрать аргументы и
позвать слой ниже.

- `Notes` — 25 методов, биндятся в TypeScript (см. полный список ниже).
- `Watch` — цикл watcher → сверка индекса → события в окно.
- `Closing` — перехват закрытия окна: попросить интерфейс дописать буфер.
- `Settings` — `.tasker/config.json`, непрозрачный для Go blob.
- `Keymap` — `~/.tasker/keymap.json`.

События (SPEC §6):

| Событие | Когда |
| --- | --- |
| `tasker:notes-changed` | список надо перечитать, без нагрузки |
| `tasker:note-changed` | `{id, path}` конкретной заметки |
| `tasker:before-close` | окно закрывается, дописать буфер и ответить `Closing.Ready()` |

### internal/watcher и internal/gitstore

`watcher.Start(ctx, root, Options) (*Watcher, error)` → канал `Batch{Paths, Full}`.
`Ignore(path, modTime)` гасит собственные записи.

`gitstore.Open(root)` → `Commit`, `History` (через системный git, `--follow`),
`Diff`, `Show`. `Autocommit` с окном 90 секунд существует, но приложение пока
коммитит на каждое сохранение и его не использует.

---

## 6. Границы

### Wails-биндинги

Генерируются из Go: `wails3 generate bindings -ts -i -clean=true -d frontend/bindings ./cmd/tasker`.
**Явный `./cmd/tasker` обязателен** — без него генератор смотрит в корень и
бодро сообщает «0 Services», не падая.

`frontend/bindings/` руками не редактируется никогда. Doc-комментарии Go
доезжают в TypeScript, `context.Context` из сигнатуры убирается.

Фронтенд зовёт биндинги только через `frontend/src/api.ts` — одно место, где
видна вся граница.

### MCP

`cmd/tasker-mcp`, девять инструментов: `search_notes`, `get_note`, `create_note`,
`update_note`, `set_status`, `list_tasks`, `list_notebooks`, `list_tags`,
`trash_note`. Схемы выводит SDK из структур — руками не писать (MCP.md §3).

Индекс сверяется **перед каждым вызовом**: приложение может быть закрыто, а
заметки правиться руками. Скан инкрементальный, ~30 мс на 10 000 заметок.

Агенту **не дано**: удалять насовсем, массово переименовывать, писать в
`config.json`, трогать git напрямую, читать вне vault.

---

## 7. Что лежит на диске

```
~/Notes/
├── .git/
├── .tasker/
│   ├── index.sqlite     производное, в .gitignore
│   ├── tags.json        выбранные цвета тегов
│   ├── config.json      состояние интерфейса
│   └── vault.lock       межпроцессная блокировка
├── .trash/              корзина: trashedFrom, trashedAt
├── attachments/
├── Работа/Баги/schetchik.md
└── Личное/pokupki.md

~/.tasker/keymap.json    раскладка клавиш, общая для всех vault
```

Ноутбук = папка, **включая пустую**: список ноутбуков строится обходом
каталогов, а не из путей заметок.

Frontmatter — SPEC §4.2. Порядок ключей у новой заметки фиксированный ради
чистых git-диффов.

---

## 8. Настройки

Три файла, у каждого своя причина лежать именно там.

| Файл | Что | Почему здесь |
| --- | --- | --- |
| `<vault>/.tasker/config.json` | ширины колонок, сортировка, свёрнутые ветки | состояние интерфейса, привязано к vault |
| `<vault>/.tasker/tags.json` | цвета тегов | пользовательские данные, индекс их не переживёт |
| `~/.tasker/keymap.json` | раскладка клавиш | принадлежит человеку, а не набору заметок |

### keymap.json

Контексты решают спор за клавишу: `j` в списке двигает выделение, а в тексте
принадлежит виму. Поиск идёт от частного к общему — `note-list` (или `editor`),
затем `global`. Контекст `editor` пуст намеренно.

```json
{
  "global":    { "cmd+n": "note.create", "cmd+ctrl+4": "note.status.completed" },
  "note-list": { "j": "list.down", "m": "note.move", "cmd+backspace": "note.trash" },
  "editor":    {}
}
```

Формат сочетания: модификаторы в порядке `cmd+ctrl+alt+shift`, затем клавиша в
нижнем регистре. Именованные: `enter`, `escape`, `backspace`, `delete`, `tab`,
`space`, `up`, `down`, `left`, `right`.

Команды: `note.create`, `note.trash`, `note.duplicate`, `note.move`, `note.pin`,
`note.status.{none,active,onhold,completed,dropped}`, `list.up`, `list.down`,
`list.open`.

Файл **сливается** с умолчаниями, а не заменяет их: правка одного сочетания не
отменяет остальные, а новые команды появляются у всех. Пустая строка вместо
команды снимает привязку. Испорченный файл не оставляет приложение без клавиш.

Умолчания живут в `internal/app/keymap.go` — единственный источник правды.
Добавляя команду, добавь её и туда, и в обработчик `frontend/src/App.tsx`.

Multi-stroke (`g g`, `space e`) из SPEC §8.11 **ещё не сделан**.

---

## 9. Как проверять работу

**Тесты обязательны для `internal/*`** и пишутся до реализации там, где
поведение описывается таблицей. Сейчас 498 тестов на Go и 58 на JS.

```sh
go test ./... -count=1 -race
cd frontend && npm test        # node --experimental-strip-types, без зависимостей
```

**Мутационная проверка окупается.** Сломай реализацию в одном месте и посмотри,
поймают ли тесты. За проект так нашлось около десятка настоящих дефектов, в том
числе: инкрементальный скан не замечал правку той же длины; массовая операция
обрывалась на первой ошибке; переименование тега задваивало его в файле.

Три ловушки самой проверки, на которые я уже наступал:

- мутация **не собралась** — это не «поймана», это ничего не значит;
- мутация **эквивалентна** — поведение то же, тест ни при чём (так выяснилось,
  что явный `LOCK_UN` перед `Close` не нужен);
- скрипт мутаций может **оставить дерево изменённым**, если его убить.

**Проверять надо источник правды, а не производное.** Тест на переименование
тега читал теги из индекса, где `INSERT OR IGNORE` схлопывает дубли, — и не
видел, что во frontmatter тег задвоился.

**Снимки экрана ловят то, чего не ловят тесты.** Дважды за проект: вся проза в
редакторе была зелёной (`tags.content` покрывает весь текст), и мета-строка
списка исчезала при высоте строки 76 px. Способ: собрать временную страницу с
компонентом, поднять `vite`, снять headless-браузером, страницу удалить.

---

## 10. Решения, записанные по ходу

Места, где реальность разошлась со спекой. Все правки внесены в SPEC, здесь
короткий список — чтобы не открывать заново.

| Что | Как оказалось |
| --- | --- |
| FTS5 `content=''` | нужен `contentless_delete=1`, иначе строку не удалить |
| «следим только за папками» | fsnotify держит дескриптор на **каждый файл**: 10 104 на 10 000 заметок |
| нечитаемый элемент в папке | молча глушит события всей папки, `Add` возвращает `nil` |
| `modernc` = сборка без cgo | для приложения неверно: Wails без cgo не собирается вовсе |
| `--follow` в go-git | не реализован; история читается системным git |
| автокоммит «после последнего изменения» | окно от **первой** правки, иначе не закрывается никогда |
| «папки нет — в корень» при восстановлении | папка пересоздаётся; в корень только при негодном `trashedFrom` |
| цвет тега в `tags.color` | вынесен в `tags.json`: индекс производный |

---

## 11. Чего ещё нет

Из фаз 5–8: превью markdown, палитра команд `Cmd+K`, ссылки `tasker://` с
автокомплитом по `[[`, бэклинки панелью, картинки, шаблоны, история версий в
интерфейсе, несколько окон, светлая тема, автообновление.

Помельче, из уже начатого: multi-stroke в кеймапе, иконки и ручной порядок
ноутбуков, контекстное меню на заметке, перетаскивание ноутбука в ноутбук,
`Autocommit` не подключён к приложению.

Известные ограничения, не баги: теги регистронезависимы только для латиницы
(`COLLATE NOCASE` в SQLite), подстроки короче трёх символов не находятся
(токенизатор trigram).
