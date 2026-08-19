import { useEffect, useRef } from "react";
import { Compartment, EditorState } from "@codemirror/state";
import {
  EditorView,
  keymap,
  lineNumbers,
  highlightActiveLine,
  highlightActiveLineGutter,
  drawSelection,
  rectangularSelection,
  crosshairCursor,
  highlightSpecialChars,
} from "@codemirror/view";
import {
  defaultKeymap,
  history,
  historyKeymap,
  indentWithTab,
} from "@codemirror/commands";
import {
  syntaxHighlighting,
  defaultHighlightStyle,
  indentOnInput,
  bracketMatching,
} from "@codemirror/language";
import { searchKeymap, highlightSelectionMatches } from "@codemirror/search";
import { markdown, markdownLanguage } from "@codemirror/lang-markdown";
import { languages } from "@codemirror/language-data";
import { oneDark } from "@codemirror/theme-one-dark";
import { vim, Vim, getCM } from "@replit/codemirror-vim";
import { RU_LANGMAP } from "./langmap";

export type ProbeEvent = {
  n: number;
  type: string;
  detail: string;
  ts: number;
};

type Props = {
  doc: string;
  vimEnabled: boolean;
  langmap: boolean;
  onDoc: (text: string) => void;
  onEvent: (e: Omit<ProbeEvent, "n" | "ts">) => void;
  onMode: (mode: string) => void;
  onEx: (cmd: string) => void;
};

const vimCompartment = new Compartment();

export function Editor({
  doc,
  vimEnabled,
  langmap,
  onDoc,
  onEvent,
  onMode,
  onEx,
}: Props) {
  const host = useRef<HTMLDivElement | null>(null);
  const view = useRef<EditorView | null>(null);

  // Колбэки держим в ref: пересоздавать редактор на каждый рендер нельзя,
  // иначе теряется состояние вима и позиция курсора.
  const cb = useRef({ onDoc, onEvent, onMode, onEx });
  cb.current = { onDoc, onEvent, onMode, onEx };

  useEffect(() => {
    if (!host.current) return;

    // :w и :q — проверяем, что ex-команды перехватываются приложением,
    // как того требует SPEC §8.6.
    Vim.defineEx("write", "w", () => cb.current.onEx(":w"));
    Vim.defineEx("quit", "q", () => cb.current.onEx(":q"));

    const state = EditorState.create({
      doc,
      extensions: [
        // vim обязан идти первым, иначе его кеймап перекрывается дефолтным
        vimCompartment.of(vimEnabled ? vim({ status: true }) : []),
        lineNumbers(),
        highlightActiveLineGutter(),
        highlightSpecialChars(),
        history(),
        drawSelection(),
        EditorState.allowMultipleSelections.of(true),
        indentOnInput(),
        bracketMatching(),
        highlightActiveLine(),
        highlightSelectionMatches(),
        rectangularSelection(),
        crosshairCursor(),
        syntaxHighlighting(defaultHighlightStyle, { fallback: true }),
        markdown({ base: markdownLanguage, codeLanguages: languages }),
        oneDark,
        EditorView.lineWrapping,
        keymap.of([...defaultKeymap, ...historyKeymap, ...searchKeymap, indentWithTab]),
        // Системный ввод macOS полностью заглушён: прямые кавычки остаются
        // прямыми, два дефиса — двумя дефисами. Правится только здесь.
        EditorView.contentAttributes.of({
          spellcheck: "false",
          autocorrect: "off",
          autocapitalize: "off",
        }),
        EditorView.updateListener.of((u) => {
          if (u.docChanged) cb.current.onDoc(u.state.doc.toString());
          const cm: any = getCM(u.view);
          const mode = cm?.state?.vim
            ? cm.state.vim.insertMode
              ? "INSERT"
              : cm.state.vim.visualMode
                ? "VISUAL"
                : "NORMAL"
            : "—";
          cb.current.onMode(mode);
        }),
      ],
    });

    const v = new EditorView({ state, parent: host.current });
    view.current = v;

    const log = (type: string, detail: string) =>
      cb.current.onEvent({ type, detail });

    const onKeyDown = (e: KeyboardEvent) =>
      log(
        "keydown",
        `key=${JSON.stringify(e.key)} code=${e.code} keyCode=${e.keyCode} isComposing=${e.isComposing}${e.metaKey ? " meta" : ""}${e.altKey ? " alt" : ""}${e.ctrlKey ? " ctrl" : ""}`,
      );
    const onCompStart = (e: CompositionEvent) =>
      log("compositionstart", JSON.stringify(e.data));
    const onCompUpdate = (e: CompositionEvent) =>
      log("compositionupdate", JSON.stringify(e.data));
    const onCompEnd = (e: CompositionEvent) =>
      log("compositionend", JSON.stringify(e.data));
    const onBeforeInput = (e: InputEvent) =>
      log("beforeinput", `${e.inputType} data=${JSON.stringify(e.data)}`);
    const onInput = (e: Event) =>
      log("input", `${(e as InputEvent).inputType} data=${JSON.stringify((e as InputEvent).data)}`);

    const dom = v.contentDOM;
    dom.addEventListener("keydown", onKeyDown, true);
    dom.addEventListener("compositionstart", onCompStart, true);
    dom.addEventListener("compositionupdate", onCompUpdate, true);
    dom.addEventListener("compositionend", onCompEnd, true);
    dom.addEventListener("beforeinput", onBeforeInput, true);
    dom.addEventListener("input", onInput, true);

    v.focus();

    return () => {
      dom.removeEventListener("keydown", onKeyDown, true);
      dom.removeEventListener("compositionstart", onCompStart, true);
      dom.removeEventListener("compositionupdate", onCompUpdate, true);
      dom.removeEventListener("compositionend", onCompEnd, true);
      dom.removeEventListener("beforeinput", onBeforeInput, true);
      dom.removeEventListener("input", onInput, true);
      v.destroy();
      view.current = null;
    };
    // Редактор создаётся один раз. Тумблеры применяются реконфигурацией ниже.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  useEffect(() => {
    view.current?.dispatch({
      effects: vimCompartment.reconfigure(vimEnabled ? vim({ status: true }) : []),
    });
    view.current?.focus();
  }, [vimEnabled]);

  useEffect(() => {
    // Второй аргумент — переносить ли langmap на Ctrl-комбинации.
    Vim.langmap(langmap ? RU_LANGMAP : "", true);
  }, [langmap]);

  return <div className="editor-host" ref={host} />;
}
