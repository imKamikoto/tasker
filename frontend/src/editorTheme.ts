import { HighlightStyle, syntaxHighlighting } from "@codemirror/language";
import { EditorView } from "@codemirror/view";
import { tags } from "@lezer/highlight";

/**
 * Тема редактора на тех же переменных, что и остальной интерфейс.
 *
 * Своя, а не готовая: любая покупная тема тащит собственную палитру, и
 * редактор начинает выглядеть чужим внутри собственного окна.
 */
export const oakTheme = EditorView.theme(
  {
    "&": {
      color: "var(--color-fg)",
      backgroundColor: "transparent",
      height: "100%",
      fontSize: "13px",
    },
    ".cm-content": {
      fontFamily: "var(--font-mono)",
      lineHeight: "1.65",
      padding: "0",
      caretColor: "var(--color-oak-600)",
    },
    ".cm-scroller": { fontFamily: "var(--font-mono)", overflow: "auto" },
    ".cm-line": { padding: "0" },
    "&.cm-focused": { outline: "none" },
    ".cm-cursor, .cm-dropCursor": { borderLeftColor: "var(--color-oak-600)" },
    "&.cm-focused .cm-selectionBackground, .cm-selectionBackground, ::selection": {
      backgroundColor: "var(--color-oak-200)",
    },
    ".cm-activeLine": { backgroundColor: "hsl(var(--hsl-oak-100) / 0.5)" },
    ".cm-gutters": {
      backgroundColor: "transparent",
      color: "var(--color-fg-muted)",
      border: "none",
    },
    ".cm-activeLineGutter": { backgroundColor: "transparent", color: "var(--color-oak-600)" },
    ".cm-searchMatch": { backgroundColor: "var(--color-oak-200)" },
    ".cm-searchMatch.cm-searchMatch-selected": { backgroundColor: "var(--color-oak-400)" },
    // Строка состояния вима: он рисует её сам, наше дело — не дать ей выбиваться.
    ".cm-vim-panel": {
      backgroundColor: "var(--color-bg-shadow)",
      color: "var(--color-fg-muted)",
      fontFamily: "var(--font-mono)",
      padding: "2px 8px",
    },
    ".cm-vim-panel input": { color: "var(--color-fg)" },
    ".cm-panels": { backgroundColor: "var(--color-bg-shadow)", color: "var(--color-fg)" },
    ".cm-panels input, .cm-panels button": {
      backgroundColor: "var(--color-bg-elevated)",
      color: "var(--color-fg)",
      border: "1px solid var(--color-border)",
    },
  },
  { dark: true },
);

/**
 * Разметка подсвечивается прямо в тексте, а не прячется: это первая из трёх
 * причин, ради которых приложение вообще пишется (SPEC §1).
 */
export const oakHighlight = syntaxHighlighting(
  HighlightStyle.define([
    { tag: tags.heading1, color: "var(--color-oak-800)", fontWeight: "600", fontSize: "1.25em" },
    { tag: tags.heading2, color: "var(--color-oak-800)", fontWeight: "600", fontSize: "1.12em" },
    { tag: [tags.heading3, tags.heading4, tags.heading5, tags.heading6], color: "var(--color-oak-800)", fontWeight: "600" },
    { tag: tags.strong, fontWeight: "600", color: "var(--color-fg)" },
    { tag: tags.emphasis, fontStyle: "italic" },
    { tag: tags.strikethrough, textDecoration: "line-through", color: "var(--color-fg-muted)" },
    { tag: tags.link, color: "var(--color-oak-600)", textDecoration: "underline" },
    { tag: tags.url, color: "var(--color-oak-600)" },
    { tag: tags.quote, color: "var(--color-fg-muted)", fontStyle: "italic" },
    { tag: [tags.list, tags.labelName], color: "var(--color-oak)" },
    // Только моноширинный — не tags.content: тот покрывает вообще весь
    // текст документа, и вся проза уезжает в цвет кода.
    { tag: tags.monospace, color: "var(--color-green)" },
    { tag: tags.processingInstruction, color: "var(--color-oak)" },
    { tag: tags.comment, color: "var(--color-fg-muted)", fontStyle: "italic" },
    { tag: [tags.keyword, tags.controlKeyword], color: "var(--color-oak-600)" },
    { tag: [tags.string, tags.special(tags.string)], color: "var(--color-green)" },
    { tag: [tags.number, tags.bool, tags.null], color: "var(--color-yellow)" },
    { tag: [tags.function(tags.variableName), tags.definition(tags.variableName)], color: "var(--color-oak-800)" },
    { tag: [tags.typeName, tags.className], color: "var(--color-oak-600)" },
    { tag: tags.operator, color: "var(--color-fg-muted)" },
  ]),
);
