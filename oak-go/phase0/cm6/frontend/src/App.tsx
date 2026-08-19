import { useMemo, useState } from "react";
import { Editor, type ProbeEvent } from "./Editor";

const INITIAL = `# Проверка редактора в WKWebView

Обычный текст по-русски. Печатай сюда, следи за панелью справа.

Кавычки остаются прямыми: "двойные" и 'одинарные'.
Дефис и два дефиса: - и --. Многоточие: ...

- [ ] незакрытая задача
- [x] закрытая задача
- пункт списка

## Код

\`\`\`go
func (s *Service) RecalculateHeaderValues(ctx context.Context) error {
	// строка с "кавычками" внутри кода
	return fmt.Errorf("read note %s: %w", path, err)
}
\`\`\`

| Колонка | Значение |
| --- | --- |
| Счётчик | 42 |

> [!NOTE]
> Цитата с кириллицей — проверь выделение мышью через границу строк.
`;

const CHECKS = [
  "Мёртвые клавиши: Option+E затем E (только если пользуешься ABC Extended)",
  "Длинный текст: набери страницу по-русски и проверь, что печатать приятно",
];

export default function App() {
  const [doc, setDoc] = useState(INITIAL);
  const [events, setEvents] = useState<ProbeEvent[]>([]);
  const [mode, setMode] = useState("—");
  const [vimEnabled, setVimEnabled] = useState(true);
  const [langmap, setLangmap] = useState(true);
  const [filter, setFilter] = useState<Record<string, boolean>>({
    keydown: true,
    composition: true,
    beforeinput: true,
    input: false,
  });
  const [checked, setChecked] = useState<Record<number, boolean>>({});
  const [exLog, setExLog] = useState<string[]>([]);
  const [auto, setAuto] = useState({
    compositions: 0, // composition-события при наборе
    replacements: 0, // insertReplacementText — системная подстановка текста
    keys: 0,         // сколько клавиш вообще нажато, чтобы отличить «чисто» от «не проверялось»
  });

  const counter = useMemo(() => ({ n: 0 }), []);

  const pushEvent = (e: { type: string; detail: string }) => {
    // Счётчики считаются до фильтра: галочки в логе на них не влияют.
    if (e.type === "keydown") setAuto((a) => ({ ...a, keys: a.keys + 1 }));
    if (e.type === "compositionstart")
      setAuto((a) => ({ ...a, compositions: a.compositions + 1 }));
    if (e.type === "beforeinput" && e.detail.startsWith("insertReplacementText"))
      setAuto((a) => ({ ...a, replacements: a.replacements + 1 }));

    const group = e.type.startsWith("composition")
      ? "composition"
      : e.type === "beforeinput"
        ? "beforeinput"
        : e.type === "input"
          ? "input"
          : "keydown";
    if (!filter[group]) return;
    counter.n += 1;
    setEvents((prev) => [{ ...e, n: counter.n, ts: Date.now() }, ...prev].slice(0, 200));
  };

  // Осталось ровно то, что нельзя выключить настройкой редактора: системный
  // ввод macOS, идущий мимо модели транзакций CodeMirror.
  const rows = [
    {
      name: "Composition при наборе",
      ...(auto.keys === 0
        ? { label: "нет данных", cls: "muted", note: "напечатай что-нибудь" }
        : auto.compositions === 0
          ? {
              label: "чисто",
              cls: "good",
              note: `нажато клавиш: ${auto.keys}, composition-событий 0 — русская раскладка идёт прямым вводом`,
            }
          : {
              label: `событий: ${auto.compositions}`,
              cls: "bad",
              note: "смотри лог — это самое хрупкое место WebKit",
            }),
    },
    {
      name: "Системные подстановки текста",
      ...(auto.replacements === 0
        ? {
            label: "чисто",
            cls: "good",
            note: "insertReplacementText не приходил — автозамена и орфография заглушены на редакторе",
          }
        : {
            label: `подстановок: ${auto.replacements}`,
            cls: "bad",
            note: "система вставила текст мимо CodeMirror — проверь, что документ цел",
          }),
    },
  ];

  return (
    <div className="app">
      <div className="pane editor-pane">
        <div className="toolbar">
          <span className={`mode mode-${mode}`}>vim: {mode}</span>
          <label>
            <input
              type="checkbox"
              checked={vimEnabled}
              onChange={(e) => setVimEnabled(e.target.checked)}
            />
            vim
          </label>
          <label title="русские буквы работают как команды по позиции клавиши">
            <input
              type="checkbox"
              checked={langmap}
              onChange={(e) => setLangmap(e.target.checked)}
            />
            langmap (RU)
          </label>
          <span className="grow" />
          <span className="chars">{doc.length} симв.</span>
        </div>
        <Editor
          doc={INITIAL}
          vimEnabled={vimEnabled}
          langmap={langmap}
          onDoc={setDoc}
          onEvent={pushEvent}
          onMode={setMode}
          onEx={(cmd) =>
            setExLog((p) => [`${cmd} перехвачена приложением`, ...p].slice(0, 5))
          }
        />
      </div>

      <div className="pane diag">
        <section>
          <h2>Движок</h2>
          <div className="ua">{navigator.userAgent}</div>
        </section>

        <section>
          <h2>Автопроверки</h2>
          <table className="auto">
            <tbody>
              {rows.map((r) => (
                <tr key={r.name}>
                  <td className="auto-name">{r.name}</td>
                  <td className={`auto-verdict ${r.cls}`}>{r.label}</td>
                  <td className="auto-note">{r.note}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </section>

        {exLog.length > 0 && (
          <section>
            <h2>Ex-команды</h2>
            <ul className="chars-list">
              {exLog.map((l, i) => (
                <li key={i} className="good">
                  {l}
                </li>
              ))}
            </ul>
          </section>
        )}

        <section>
          <h2>Что осталось проверить руками</h2>
          <ul className="checklist">
            {CHECKS.map((c, i) => (
              <li key={i}>
                <label>
                  <input
                    type="checkbox"
                    checked={!!checked[i]}
                    onChange={(e) =>
                      setChecked((p) => ({ ...p, [i]: e.target.checked }))
                    }
                  />
                  <span className={checked[i] ? "done" : ""}>{c}</span>
                </label>
              </li>
            ))}
          </ul>
        </section>

        <section className="events">
          <h2>
            События ввода
            <span className="grow" />
            <button onClick={() => setEvents([])}>очистить</button>
          </h2>
          <div className="filters">
            {Object.keys(filter).map((k) => (
              <label key={k}>
                <input
                  type="checkbox"
                  checked={filter[k]}
                  onChange={(e) => setFilter((p) => ({ ...p, [k]: e.target.checked }))}
                />
                {k}
              </label>
            ))}
          </div>
          <ol className="event-log">
            {events.map((e) => (
              <li
                key={e.n}
                className={`ev ev-${e.type.startsWith("composition") ? "comp" : e.type}`}
              >
                <span className="ev-type">{e.type}</span>
                <span className="ev-detail">{e.detail}</span>
              </li>
            ))}
          </ol>
        </section>
      </div>
    </div>
  );
}
