# Tasker MCP — контракт для агента (вариант Б: Go)

Это главная причина, по которой приложение вообще пишется. Всё остальное — редактор вокруг этого.

---

## 1. Что решаем

Работаешь с Claude Code над задачей A. По ходу дела всплывает что-то, что нельзя чинить прямо сейчас: баг в соседнем модуле, недочёт в архитектуре, «тут надо переписать, но не в этом PR». Сейчас это уходит либо в TODO-комментарий, который никто не прочитает, либо в отдельное окно Notion, куда надо переключиться и потерять контекст.

Нужно, чтобы работало так:

> — Claude, заведи задачу: в `RequestHeader` счётчик не пересчитывается после ручной правки. Вернёмся после текущей.
> — Готово, завёл «Счётчик перерасчёта не обновляется после ручной правки» в «Работа/Баги», статус active, тег armz-frontend. Ссылка проставлена на текущую задачу.

И чтобы заметка сразу была в приложении — без импорта, без перезапуска, и даже если приложение закрыто.

---

## 2. Архитектура

`tasker-mcp` — **отдельный бинарник** из `cmd/tasker-mcp`, поднимаемый Claude Code по stdio. Он не общается с приложением: он импортирует те же `internal/vault` и `internal/index` и работает с vault напрямую.

```
Claude Code ──stdio──> tasker-mcp ──> internal/vault ──> ~/Notes/*.md
                                └─> internal/index ──> .tasker/index.sqlite
                                                             ↑
                                 tasker.app (fsnotify) ─────────┘  видит изменения,
                                                                шлёт tasker:notes-changed
```

Три следствия:

1. **Приложение может быть закрыто** — агент всё равно пишет заметки.
2. **Никаких API-ключей.** Агент — это твой Claude Code, он уже авторизован. Приложение об AI не знает ничего.
3. **Нет дублирования логики.** Валидация frontmatter, ULID, обновление индекса, git-коммит — один и тот же код.

Отдельный бонус Go: `tasker-mcp` — **один статический бинарник без зависимостей**. Ни рантайма, ни `node_modules`, ни путей до интерпретатора. Скопировал файл — работает.

Библиотека: **`github.com/modelcontextprotocol/go-sdk`** — официальный SDK, v1 с обещанием обратной совместимости.

Установка:

```sh
go build -o ~/bin/tasker-mcp ./cmd/tasker-mcp
claude mcp add tasker --scope user -- ~/bin/tasker-mcp --vault ~/Notes
```

Конкурентная запись с приложением снимается WAL + `busy_timeout=5000` + атомарной записью файлов (временный файл рядом → `rename`).

---

## 3. Инструменты

Схемы описываются через дженерики Go SDK: параметры — структуры с `json` и `jsonschema` тегами, SDK генерирует JSON Schema сам. Не писать схемы руками.

### `search_notes`

Основной способ агенту что-то найти.

```go
type SearchParams struct {
    Query       string `json:"query" jsonschema:"язык запросов: 'счётчик tag:баг -status:completed'"`
    Limit       int    `json:"limit,omitempty" jsonschema:"по умолчанию 20, максимум 100"`
    IncludeBody bool   `json:"includeBody,omitempty"`
}
// → { total int, notes []NoteSummary }

type NoteSummary struct {
    ID       string   `json:"id"`
    Title    string   `json:"title"`
    Notebook string   `json:"notebook"`
    Status   string   `json:"status"`
    Tags     []string `json:"tags"`
    Pinned   bool     `json:"pinned"`
    Created  string   `json:"created"`   // RFC 3339
    Updated  string   `json:"updated"`
    Excerpt  string   `json:"excerpt"`
    Body     string   `json:"body,omitempty"`
}
```

### `get_note`

```go
type GetParams struct{ ID string `json:"id"` }
// → NoteSummary + Body string + Backlinks []NoteRef + Links []NoteRef
```

### `create_note`

```go
type CreateParams struct {
    Title    string   `json:"title"`
    Body     string   `json:"body,omitempty"`
    Notebook string   `json:"notebook,omitempty"` // 'Работа/Баги'; создаётся, если нет
    Tags     []string `json:"tags,omitempty"`     // несуществующие создаются
    Status   string   `json:"status,omitempty"`   // по умолчанию 'none'
    Pinned   bool     `json:"pinned,omitempty"`
    LinkTo   string   `json:"linkTo,omitempty"`   // id: добавит взаимные ссылки
    Context  *Context `json:"context,omitempty"`  // repo, branch, commit, file
}
// → { id, path, url }   url = 'tasker://note/<id>'
```

Заметка помечается `origin: agent` — чтобы в приложении было видно, что её завёл не ты.

### `update_note`

```go
type UpdateParams struct {
    ID         string   `json:"id"`
    Title      *string  `json:"title,omitempty"`
    Body       *string  `json:"body,omitempty"`    // полная замена
    Append     *string  `json:"append,omitempty"`  // взаимоисключающе с Body
    Prepend    *string  `json:"prepend,omitempty"`
    Status     *string  `json:"status,omitempty"`
    AddTags    []string `json:"addTags,omitempty"`
    RemoveTags []string `json:"removeTags,omitempty"`
    Pinned     *bool    `json:"pinned,omitempty"`
    Notebook   *string  `json:"notebook,omitempty"` // перемещение
}
```

Указатели, а не значения — иначе не отличить «не передано» от «передан ноль». Это единственное место, где на этом легко обжечься.

`append` — самая частая операция в работе: агент дописывает найденное в существующую заметку, не переписывая её целиком. Отделяется пустой строкой.

### `set_status`

Отдельно, потому что самая частая точечная операция и не хочется дёргать `update_note` с риском задеть что-то ещё.

```go
type StatusParams struct {
    ID     string `json:"id"`
    Status string `json:"status" jsonschema:"enum=none,enum=active,enum=onHold,enum=completed,enum=dropped"`
}
```

### `list_tasks`

Сахар над поиском с явной семантикой «что у меня в работе».

```go
type TasksParams struct {
    Status   []string `json:"status,omitempty"`   // по умолчанию ['active','onHold']
    Notebook string   `json:"notebook,omitempty"`
    Tag      string   `json:"tag,omitempty"`
    Limit    int      `json:"limit,omitempty"`
}
```

Сортировка: закреплённые сверху, дальше по дате изменения по убыванию.

### `list_notebooks` · `list_tags` · `trash_note`

```go
// list_notebooks → []{ path string, count int, children []string }
// list_tags      → []{ name string, count int, color string }
// trash_note     → { id string, trashed bool }   только в корзину
```

---

## 4. Чего агенту не дано

Осознанные ограничения, а не недоделки:

- **Удалять навсегда.** Только корзина.
- **Массово удалять или переименовывать ноутбуки и теги.** Слишком легко устроить погром одним вызовом.
- **Писать в `.tasker/config.json`.**
- **Трогать git напрямую.** Коммиты делает `internal/gitstore` по своим правилам.
- **Читать что-либо вне vault.** Путь задан флагом `--vault`; после `filepath.EvalSymlinks` любой выход за пределы — ошибка. Проверять именно так, а не через `strings.HasPrefix`, иначе симлинк из vault наружу открывает весь диск.

---

## 5. Skill для Claude Code

Положить в `~/.claude/skills/tasker-notes/SKILL.md`.

```markdown
---
name: tasker-notes
description: >
  Ведение рабочих заметок и задач в Tasker. Использовать, когда пользователь просит
  записать задачу, завести баг, зафиксировать находку «на потом», посмотреть что
  в работе, отметить задачу выполненной, или когда по ходу работы над кодом
  обнаружено что-то, что явно не относится к текущей задаче.
---

# Работа с заметками Tasker

## Когда заводить заметку сам

Если во время работы над задачей находится проблема, которая **не относится** к
текущей задаче — сначала спроси одной фразой, заводить ли задачу. Не заводи
молча и не заводи по каждому мелкому замечанию.

## Как заводить

1. Заголовок конкретный и самодостаточный. Не «Баг в хедере», а
   «Счётчик перерасчёта не обновляется после ручной правки значения».
2. Тело по структуре: что не так → где (файл, функция) → как воспроизвести →
   почему не чиним сейчас.
3. `notebook` — «Работа/Баги» для дефектов, «Работа/Задачи» для остального.
4. `tags` — имя репозитория обязательно.
5. `status: active`, если возвращаться скоро; `onHold` — если «когда-нибудь».
6. `context` — repo, branch, commit, файл. Заполняй всегда.
7. `linkTo` — id заметки текущей задачи, если известен.

## Формат ссылок

Ссылки на заметки — `[Заголовок](tasker://note/<ULID>)`. Wiki-ссылки не поддерживаются.

## Что не делать

- Не удалять заметки без явной просьбы.
- Не ставить `completed` самостоятельно — это решает пользователь.
- Не переписывать тело существующей заметки целиком; дописывай через `append`.
```

---

## 6. Приёмка

Сценарий, который должен работать от начала до конца:

1. Приложение закрыто. В Claude Code: «заведи задачу: в парсере ломается экранирование кавычек, вернёмся позже».
2. Агент зовёт `create_note` с заголовком, телом, ноутбуком «Работа/Баги», тегом репозитория, `status: active`, заполненным `context`.
3. На диске появляется `~/Notes/Работа/Баги/lomaetsya-ekranirovanie-kavychek.md` с корректным frontmatter, индекс обновлён, git-коммит `agent: create "..."` создан.
4. Открываю приложение — заметка в списке «Активные», помечена как заведённая агентом.
5. **Приложение открыто, агент создаёт вторую заметку** — она появляется в списке без перезапуска, через `tasker:notes-changed`.
6. Дописываю руками пару строк, ставлю `completed` по `Cmd+Ctrl+4` — уходит из списка.
7. В Claude Code: «что у меня в работе?» → `list_tasks` её не возвращает.
8. `git log` показывает и агентские коммиты, и мои.

Шаг 5 — самый важный: он проверяет, что связка «чужой процесс пишет файл → watcher → индекс → событие → UI» работает целиком.
