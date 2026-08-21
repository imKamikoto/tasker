import { HighlightStyle, syntaxHighlighting } from "@codemirror/language";
import { EditorView } from "@codemirror/view";
import { tags } from "@lezer/highlight";

/**
 * Тема редактора на тех же переменных, что и остальной интерфейс.
 *
 * Своя, а не готовая: любая покупная тема тащит собственную палитру, и
 * редактор начинает выглядеть чужим внутри собственного окна.
 */
export const taskerTheme = EditorView.theme({
  "&": {
    color: "var(--color-fg)",
    backgroundColor: "transparent",
    height: "100%",
    fontSize: "var(--editor-font-size)",
  },
  ".cm-content": {
    fontFamily: "var(--font-mono)",
    lineHeight: "var(--editor-line-height)",
    padding: "0",
    caretColor: "var(--color-accent)",
  },
  ".cm-scroller": {
    fontFamily: "var(--font-mono)",
    lineHeight: "var(--editor-line-height)",
    overflow: "auto",
  },
  ".cm-line": { padding: "0" },
  "&.cm-focused": { outline: "none" },
  ".cm-cursor, .cm-dropCursor": { borderLeftColor: "var(--color-accent)" },
  "&.cm-focused .cm-selectionBackground, .cm-selectionBackground, ::selection": {
    backgroundColor: "var(--color-bg-selected)",
  },
  // Подсветка текущей строки почти незаметна: она подсказывает, где курсор,
  // а не спорит с выделением.
  ".cm-activeLine": { backgroundColor: "hsl(var(--hsl-fg) / 0.03)" },
  ".cm-gutters": {
    backgroundColor: "transparent",
    color: "var(--color-fg-faint)",
    border: "none",
    minWidth: "34px",
  },
  ".cm-lineNumbers .cm-gutterElement": { padding: "0 14px 0 0", minWidth: "34px" },
  ".cm-activeLineGutter": { backgroundColor: "transparent", color: "var(--color-fg-dim)" },
  ".cm-searchMatch": { backgroundColor: "var(--color-accent-soft)" },
  ".cm-searchMatch.cm-searchMatch-selected": {
    backgroundColor: "var(--color-accent)",
    color: "hsl(0 0% 8%)",
  },
  ".cm-panels": {
    backgroundColor: "var(--color-bg-elevated)",
    color: "var(--color-fg)",
    borderColor: "var(--color-border)",
  },
  ".cm-panels input, .cm-panels button": {
    backgroundColor: "var(--color-bg)",
    color: "var(--color-fg)",
    border: "1px solid var(--color-border)",
    borderRadius: "4px",
  },
});

/**
 * Разметка подсвечивается прямо в тексте, а не прячется: это первая из трёх
 * причин, ради которых приложение вообще пишется (SPEC §1).
 *
 * Цветов ровно три — акцент, зелёный статуса и приглушённый серый. Раскрашивать
 * разметку в радугу здесь нечем и незачем: палитра нейтральная, и заметка
 * должна читаться как текст, а не как подсвеченный исходник.
 */
export const taskerHighlight = syntaxHighlighting(
  HighlightStyle.define([
    { tag: tags.heading1, color: "var(--color-fg-heading)", fontWeight: "600", fontSize: "1.25em" },
    { tag: tags.heading2, color: "var(--color-fg-heading)", fontWeight: "600", fontSize: "1.12em" },
    {
      tag: [tags.heading3, tags.heading4, tags.heading5, tags.heading6],
      color: "var(--color-fg-heading)",
      fontWeight: "600",
    },
    { tag: tags.strong, fontWeight: "600", color: "var(--color-fg-heading)" },
    { tag: tags.emphasis, fontStyle: "italic" },
    { tag: tags.strikethrough, textDecoration: "line-through", color: "var(--color-fg-dim)" },
    { tag: tags.link, color: "var(--color-accent)", textDecoration: "underline" },
    { tag: tags.url, color: "var(--color-accent)" },
    { tag: tags.quote, color: "var(--color-fg-muted)", fontStyle: "italic" },
    // Маркеры списков и чекбоксов приглушены: они каркас, а не содержание.
    { tag: [tags.list, tags.labelName], color: "var(--color-fg-dim)" },
    // Только моноширинный — не tags.content: тот покрывает вообще весь
    // текст документа, и вся проза уезжает в цвет кода.
    { tag: tags.monospace, color: "var(--color-accent)" },
    { tag: tags.processingInstruction, color: "var(--color-fg-dim)" },
    { tag: tags.comment, color: "var(--color-fg-dim)", fontStyle: "italic" },
    { tag: [tags.keyword, tags.controlKeyword], color: "var(--color-accent)" },
    { tag: [tags.string, tags.special(tags.string)], color: "var(--color-status-active)" },
    { tag: [tags.number, tags.bool, tags.null], color: "var(--color-status-hold)" },
    {
      tag: [tags.function(tags.variableName), tags.definition(tags.variableName)],
      color: "var(--color-fg-heading)",
    },
    { tag: [tags.typeName, tags.className], color: "var(--color-accent)" },
    { tag: tags.operator, color: "var(--color-fg-dim)" },
  ]),
);

