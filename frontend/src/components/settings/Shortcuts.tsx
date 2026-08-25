import { useEffect, useRef, useState } from "react";

import {
  bindingsFor,
  checkBinding,
  commands,
  contextHints,
  contextNames,
  prettyCombo,
  type CommandContext,
  type Conflict,
} from "../../commands";
import { combination, type Keymap } from "../../keys";
import { Card } from "./controls";

type Props = {
  keymap: Keymap;
  /** Действуют ли движения вима. Выключенные надо показать выключенными. */
  vimNavigation: boolean;
  /** Сохранить раскладку целиком и перечитать её. */
  onSave: (keymap: Keymap) => void;
  onReset: () => void;
  path: string;
  onReveal: () => void;
};

const contexts: CommandContext[] = ["global", "note-list", "editor"];

/** Что сейчас записывается: команда, контекст и заменяемое сочетание. */
type Recording = {
  context: CommandContext;
  command: string;
  /** Пусто — добавляем новое сочетание, иначе заменяем это. */
  replacing: string;
};

export function Shortcuts({ keymap, vimNavigation, onSave, onReset, path, onReveal }: Props) {
  const [recording, setRecording] = useState<Recording | null>(null);
  const [conflict, setConflict] = useState<Conflict>({ kind: "none" });

  // Запись слушает окно с capture: иначе сочетание сначала поймает главный
  // обработчик приложения и, скажем, заведёт заметку прямо во время записи.
  const state = useRef({ keymap, recording, onSave });
  state.current = { keymap, recording, onSave };

  useEffect(() => {
    if (!recording) return;

    const onKey = (event: KeyboardEvent) => {
      event.preventDefault();
      event.stopPropagation();

      if (event.key === "Escape") {
        setRecording(null);
        setConflict({ kind: "none" });
        return;
      }
      // Один модификатор — ещё не сочетание: человек держит cmd и выбирает.
      if (["Meta", "Control", "Alt", "Shift"].includes(event.key)) return;

      const current = state.current.recording;
      if (!current) return;

      const combo = combination(event);
      const bindings = state.current.keymap[current.context] ?? {};
      const found = checkBinding(bindings, current.context, combo, current.command);
      if (found.kind === "taken") {
        // Занятое показываем и ждём: молча отобрать клавишу у другой команды
        // хуже, чем не назначить.
        setConflict(found);
        return;
      }

      const next: Keymap = {
        ...state.current.keymap,
        [current.context]: { ...bindings, [combo]: current.command },
      };
      if (current.replacing && current.replacing !== combo) {
        delete next[current.context][current.replacing];
      }
      state.current.onSave(next);
      setRecording(null);
      setConflict(found.kind === "vim" ? found : { kind: "none" });
    };

    window.addEventListener("keydown", onKey, true);
    return () => window.removeEventListener("keydown", onKey, true);
  }, [recording]);

  const unbind = (context: CommandContext, combo: string) => {
    const bindings = { ...(keymap[context] ?? {}) };
    delete bindings[combo];
    onSave({ ...keymap, [context]: bindings });
  };

  return (
    <>
      {conflict.kind !== "none" && (
        <div className="card__warn card__warn--loud">
          {conflict.kind === "taken" ? (
            <>
              Это сочетание занято командой <code>{conflict.command}</code>. Снимите его там или
              выберите другое.
            </>
          ) : (
            <>
              <code>{prettyCombo(conflict.key)}</code> в тексте принадлежит виму — теперь эта
              клавиша ему не достанется. Если это не то, чего вы хотели, снимите привязку.
            </>
          )}
          <button className="button button--link" onClick={() => setConflict({ kind: "none" })}>
            понятно
          </button>
        </div>
      )}

      {!vimNavigation && (
        <p className="section-note">
          Движения вима (<code>j</code>, <code>k</code>, <code>h</code>, <code>l</code>, а также
          смена колонки на <code>⌃⇧H</code> и <code>⌃⇧L</code>) сейчас выключены в разделе
          «Редактор» — привязки ниже остаются в файле, но не срабатывают. Стрелки,
          <code>⏎</code> и <code>esc</code> работают.
        </p>
      )}

      {contexts.map((context) => (
        <div key={context} className="shortcuts">
          <div className="section">
            <span className="section__label">{contextNames[context]}</span>
            <span className="section__rule" />
          </div>
          <p className="section-note">{contextHints[context]}</p>

          <Card>
            {commands
              .filter((command) => command.context === context)
              .map((command) => {
                const combos = bindingsFor(keymap, context, command.id);
                const active =
                  recording?.context === context && recording.command === command.id;
                return (
                  <div key={command.id} className="card__row card__row--tight">
                    <div className="card__label">
                      <span className="card__name">{command.label}</span>
                      <span className="card__hint">
                        <code>{command.id}</code>
                      </span>
                    </div>
                    <div className="card__control keys">
                      {combos.map((combo) => (
                        <span key={combo} className="keycap">
                          <button
                            className="keycap__combo"
                            onClick={() =>
                              setRecording({ context, command: command.id, replacing: combo })
                            }
                          >
                            {prettyCombo(combo)}
                          </button>
                          <button
                            className="keycap__remove"
                            aria-label={`снять ${combo}`}
                            onClick={() => unbind(context, combo)}
                          >
                            ×
                          </button>
                        </span>
                      ))}
                      {active ? (
                        <span className="keycap keycap--recording">нажмите сочетание · esc</span>
                      ) : (
                        <button
                          className="keycap keycap--add"
                          onClick={() =>
                            setRecording({ context, command: command.id, replacing: "" })
                          }
                        >
                          + сочетание
                        </button>
                      )}
                    </div>
                  </div>
                );
              })}
          </Card>
        </div>
      ))}

      <div className="row-actions">
        <button className="button" onClick={onReset}>
          Вернуть умолчания
        </button>
        <button className="button button--link" onClick={onReveal}>
          {path}
        </button>
      </div>
    </>
  );
}
