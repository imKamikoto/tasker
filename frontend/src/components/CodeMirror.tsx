import { useEffect, useRef } from "react";

import { defaultKeymap, history, historyKeymap, indentWithTab } from "@codemirror/commands";
import { languages } from "@codemirror/language-data";
import { markdown, markdownLanguage } from "@codemirror/lang-markdown";
import { bracketMatching, indentOnInput } from "@codemirror/language";
import { highlightSelectionMatches, search, searchKeymap } from "@codemirror/search";
import { EditorState } from "@codemirror/state";
import {
  EditorView,
  drawSelection,
  highlightActiveLine,
  highlightSpecialChars,
  keymap,
  rectangularSelection,
} from "@codemirror/view";
import { Vim, vim } from "@replit/codemirror-vim";

import { oakHighlight, oakTheme } from "../editorTheme";
import { continueList } from "../lists";
import { RU_LANGMAP } from "../langmap";

type Props = {
  /** Начальный текст. Дальше буфером владеет сам редактор. */
  initialDoc: string;
  onChange: (doc: string) => void;
  /** :w — сохранить немедленно. */
  onWrite: () => void;
  /** :q — закрыть заметку. */
  onQuit: () => void;
};

/**
 * CodeMirror — обёртка над редактором.
 *
 * Создаётся один раз и живёт до размонтирования: пересоздание теряет состояние
 * вима, позицию курсора и историю отмен. Заметки переключаются через key на
 * родителе, а не подменой документа — так меньше мест, где можно ошибиться.
 */
export function CodeMirror({ initialDoc, onChange, onWrite, onQuit }: Props) {
  const host = useRef<HTMLDivElement | null>(null);

  // Колбэки держим в ref: они меняются на каждом рендере родителя, а редактор
  // пересоздавать из-за этого нельзя. Классическая ловушка устаревшего
  // замыкания, если сложить их прямо в расширения.
  const callbacks = useRef({ onChange, onWrite, onQuit });
  callbacks.current = { onChange, onWrite, onQuit };

  useEffect(() => {
    if (!host.current) return;

    // Ex-команды вима привязаны к приложению (SPEC §8.6).
    Vim.defineEx("write", "w", () => callbacks.current.onWrite());
    Vim.defineEx("quit", "q", () => callbacks.current.onQuit());
    // Русские буквы работают как команды по физической позиции клавиши.
    Vim.langmap(RU_LANGMAP, true);

    const view = new EditorView({
      parent: host.current,
      state: EditorState.create({
        doc: initialDoc,
        extensions: [
          // Вим идёт первым, иначе его кеймап перекрывается дефолтным.
          vim({ status: true }),
          history(),
          drawSelection(),
          EditorState.allowMultipleSelections.of(true),
          indentOnInput(),
          bracketMatching(),
          highlightActiveLine(),
          highlightSpecialChars(),
          highlightSelectionMatches(),
          rectangularSelection(),
          search({ top: true }),
          // Подсветка кода внутри блоков грузится по требованию: тащить все
          // языки сразу — это мегабайты ради заметки, где их обычно нет.
          markdown({ base: markdownLanguage, codeLanguages: languages }),
          oakHighlight,
          oakTheme,
          EditorView.lineWrapping,
          // Продолжение списков идёт раньше умолчаний, иначе Enter заберёт
          // дефолтная привязка и до нас не дойдёт.
          keymap.of([{ key: "Enter", run: continueListOnEnter }]),
          keymap.of([...defaultKeymap, ...historyKeymap, ...searchKeymap, indentWithTab]),
          // Системный ввод macOS заглушён целиком: прямые кавычки остаются
          // прямыми, два дефиса — двумя дефисами. Решение фазы 0, менять нельзя.
          EditorView.contentAttributes.of({
            spellcheck: "false",
            autocorrect: "off",
            autocapitalize: "off",
          }),
          EditorView.updateListener.of((update) => {
            if (update.docChanged) callbacks.current.onChange(update.state.doc.toString());
          }),
        ],
      }),
    });

    view.focus();
    return () => view.destroy();
    // Пустой список зависимостей намеренно: редактор создаётся один раз.
    // initialDoc читается только при создании, дальше буфер живёт сам.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  return <div className="cm-host" ref={host} />;
}

/**
 * continueListOnEnter продолжает список, чекбокс или нумерацию.
 *
 * Срабатывает только в конце строки и при одном курсоре: посередине строки
 * Enter должен просто разрывать текст, а с несколькими курсорами дописывать
 * маркеры — почти наверняка не то, чего от него ждут.
 */
function continueListOnEnter(view: EditorView): boolean {
  const { state } = view;
  if (state.selection.ranges.length !== 1 || !state.selection.main.empty) return false;

  const pos = state.selection.main.head;
  const line = state.doc.lineAt(pos);
  if (pos !== line.to) return false;

  const action = continueList(line.text);
  if (!action) return false;

  if (action.kind === "clear") {
    view.dispatch({ changes: { from: line.from, to: line.to, insert: "" } });
    return true;
  }
  view.dispatch({
    changes: { from: pos, insert: action.insert },
    selection: { anchor: pos + action.insert.length },
    scrollIntoView: true,
  });
  return true;
}
