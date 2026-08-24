import { useEffect, useRef } from "react";

import {
  defaultKeymap,
  history,
  historyKeymap,
  indentLess,
  indentMore,
  indentWithTab,
} from "@codemirror/commands";
import { languages } from "@codemirror/language-data";
import { markdown, markdownLanguage } from "@codemirror/lang-markdown";
import { bracketMatching, indentOnInput } from "@codemirror/language";
import { highlightSelectionMatches, search, searchKeymap } from "@codemirror/search";
import { EditorState, Prec } from "@codemirror/state";
import {
  EditorView,
  drawSelection,
  highlightActiveLine,
  highlightSpecialChars,
  keymap,
  lineNumbers,
  rectangularSelection,
} from "@codemirror/view";
import { Vim, getCM, vim } from "@replit/codemirror-vim";

import { checkboxHighlight } from "../checkboxes";
import { taskerHighlight, taskerTheme } from "../editorTheme";
import { spansLines } from "../indent";
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
  /** Растёт, когда список просит передать фокус в текст. */
  focusToken: number;
  /** Состояние для строки статуса: режим вима и позиция курсора. */
  onStatus: (status: EditorStatus) => void;
  /** Вим-режим. Выключенный делает редактор обычным текстовым полем. */
  vimEnabled: boolean;
  /** Показывать номера строк. */
  lineNumbers: boolean;
  /** Переносить длинные строки. */
  lineWrap: boolean;
};

/**
 * indentBlock двигает выделение вправо, если оно задевает больше одной строки.
 *
 * Возвращает false на выделении внутри строки — нажатие уходит дальше по
 * цепочке, и Tab печатает, как печатал.
 */
function indentBlock(shift: boolean) {
  return (view: EditorView): boolean => {
    const { state } = view;
    const ranges = state.selection.ranges.map((range) => ({
      fromLine: state.doc.lineAt(range.from).number,
      toLine: state.doc.lineAt(range.to).number,
    }));
    if (!spansLines(ranges)) return false;
    return shift ? indentLess(view) : indentMore(view);
  };
}

/** Что показывает строка статуса под текстом. */
export type EditorStatus = {
  /**
   * NORMAL, INSERT, VISUAL — как их называет сам вим. Пустая строка означает,
   * что вим выключен: режимов нет, и показывать в строке статуса нечего.
   */
  mode: string;
  line: number;
  column: number;
};

/**
 * CodeMirror — обёртка над редактором.
 *
 * Создаётся один раз и живёт до размонтирования: пересоздание теряет состояние
 * вима, позицию курсора и историю отмен. Заметки переключаются через key на
 * родителе, а не подменой документа — так меньше мест, где можно ошибиться.
 */
export function CodeMirror({
  initialDoc,
  onChange,
  onWrite,
  onQuit,
  focusToken,
  onStatus,
  vimEnabled,
  lineNumbers: showLineNumbers,
  lineWrap,
}: Props) {
  const host = useRef<HTMLDivElement | null>(null);
  const view = useRef<EditorView | null>(null);
  // Режим держим здесь, а не в state: он меняется на каждое нажатие, и
  // перерисовывать из-за него редактор нельзя.
  const mode = useRef(vimEnabled ? "NORMAL" : "");

  const reportPosition = (editor: EditorView) => {
    const head = editor.state.selection.main.head;
    const line = editor.state.doc.lineAt(head);
    callbacks.current.onStatus({
      mode: mode.current,
      line: line.number,
      column: head - line.from + 1,
    });
  };

  // Колбэки держим в ref: они меняются на каждом рендере родителя, а редактор
  // пересоздавать из-за этого нельзя. Классическая ловушка устаревшего
  // замыкания, если сложить их прямо в расширения.
  const callbacks = useRef({ onChange, onWrite, onQuit, onStatus });
  callbacks.current = { onChange, onWrite, onQuit, onStatus };

  useEffect(() => {
    if (!host.current) return;

    if (vimEnabled) {
      // Ex-команды вима привязаны к приложению (SPEC §8.6).
      Vim.defineEx("write", "w", () => callbacks.current.onWrite());
      Vim.defineEx("quit", "q", () => callbacks.current.onQuit());
      // Русские буквы работают как команды по физической позиции клавиши.
      Vim.langmap(RU_LANGMAP, true);
    }

    const editor = new EditorView({
      parent: host.current,
      state: EditorState.create({
        doc: initialDoc,
        extensions: [
          // Отступ блока — выше вима, иначе до него не доходит: кеймап вима
          // стоит первым и Tab забирает себе. Cmd-скобки работают в любом
          // режиме и не спорят ни с вимом, ни с набором; Tab перехватывается
          // только когда выделение задевает больше одной строки — там он
          // означает «сдвинуть», а не «вставить табуляцию».
          Prec.highest(
            keymap.of([
              // Cmd-скобки двигают всегда, даже одну строку под кареткой:
              // это привычная пара из редакторов macOS, и «ничего не выделено»
              // там не повод не сдвинуть.
              { key: "Mod-]", run: indentMore },
              { key: "Mod-[", run: indentLess },
              { key: "Tab", run: indentBlock(false) },
              { key: "Shift-Tab", run: indentBlock(true) },
            ]),
          ),
          // Вим идёт первым, иначе его кеймап перекрывается дефолтным.
          // status: false — свою панель он рисует внутри редактора, а нам
          // нужна одна строка на всю ширину колонки, с режимом и позицией.
          //
          // Выключённый вим не подменяется заглушкой, а просто не добавляется:
          // тогда остаётся обычный CodeMirror с defaultKeymap, где текст
          // печатается сразу и стрелки работают как везде.
          ...(vimEnabled ? [vim({ status: false })] : []),
          // Расширения включаются списком, а не переключаются на лету:
          // редактор всё равно пересоздаётся, когда настройка меняется.
          ...(showLineNumbers ? [lineNumbers()] : []),
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
          taskerHighlight,
          taskerTheme,
          checkboxHighlight,
          ...(lineWrap ? [EditorView.lineWrapping] : []),
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
            if (update.docChanged || update.selectionSet) reportPosition(update.view);
          }),
        ],
      }),
    });

    // Режим вима приходит событием: своей панели у него больше нет, а знать,
    // в каком он режиме, — половина смысла вим-режима. Без вима подписываться
    // не на что: события никто не шлёт, и режим остаётся пустым.
    const cm = vimEnabled ? getCM(editor) : null;
    const onModeChange = (event: { mode: string; subMode?: string }) => {
      mode.current = (event.subMode || event.mode || "normal").toUpperCase();
      reportPosition(editor);
    };
    cm?.on("vim-mode-change", onModeChange);
    reportPosition(editor);

    // Каретку сюда не забираем: кто владеет фокусом, решает App. Иначе
    // щелчок по заметке выбрасывал бы из списка в текст, и j/k переставали
    // работать сразу после выбора.
    view.current = editor;
    return () => {
      cm?.off("vim-mode-change", onModeChange);
      editor.destroy();
      view.current = null;
    };
    // Пустой список зависимостей намеренно: редактор создаётся один раз.
    // initialDoc читается только при создании, дальше буфер живёт сам.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // Enter в списке передаёт фокус сюда. Через число, а не через ref наружу:
  // так фокус — это событие, а не состояние, и повторный запрос сработает.
  //
  // Значение при монтировании запоминается и пропускается. Редактор
  // пересоздаётся на каждой заметке, а эффект нового компонента срабатывает
  // сразу с текущим числом — и после первого же Enter каждое переключение
  // заметки в списке утаскивало бы каретку в текст. Фокус — это событие,
  // и «событием» здесь считается только изменение, а не сам факт монтирования.
  const requested = useRef(focusToken);
  useEffect(() => {
    if (focusToken === requested.current) return;
    requested.current = focusToken;
    view.current?.focus();
  }, [focusToken]);

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
