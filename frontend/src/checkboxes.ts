import { RangeSetBuilder } from "@codemirror/state";
import { Decoration, ViewPlugin, type DecorationSet, type EditorView } from "@codemirror/view";

/** Один найденный чекбокс: где стоит маркер и закрыт ли он. */
export type Checkbox = { from: number; to: number; done: boolean };

/**
 * Маркер пункта списка с чекбоксом. Только в начале строки и только с пробелом
 * после скобок: `- [x]` посреди текста — это текст, а не задача.
 */
const marker = /^([ \t]*)([-*+][ \t]+\[[ xX]\])(?=[ \t])/;

/**
 * findCheckboxes находит маркеры чекбоксов в куске текста.
 *
 * offset — позиция куска в документе, чтобы вернуть абсолютные координаты.
 * Функция чистая: подсветка чекбоксов легко ломается на краях (табы, вложенные
 * списки, `[x]` внутри ссылки), и проверять это надо таблицей, а не глазами.
 */
export function findCheckboxes(text: string, offset = 0): Checkbox[] {
  const out: Checkbox[] = [];
  let at = 0;

  for (const line of text.split("\n")) {
    const found = marker.exec(line);
    if (found) {
      const start = at + found[1].length;
      out.push({
        from: offset + start,
        to: offset + start + found[2].length,
        done: found[2].endsWith("[x]") || found[2].endsWith("[X]"),
      });
    }
    at += line.length + 1;
  }
  return out;
}

// Инлайн-стилем, а не классом: подсветка синтаксиса красит тот же диапазон
// как маркер списка, и кто победит из двух классов, решает порядок в
// сгенерированной таблице стилей — то есть случай.
const done = Decoration.mark({ attributes: { style: "color: var(--color-status-active)" } });
const open = Decoration.mark({ attributes: { style: "color: var(--color-fg-dim)" } });

/**
 * checkboxHighlight красит маркеры чекбоксов.
 *
 * Отдельным плагином, а не через HighlightStyle: lezer разбирает `- [x]` как
 * обычный элемент списка, и отличить закрытую задачу от открытой его тегами
 * нельзя. Обрабатываются только видимые строки — в заметке на тысячу пунктов
 * проход по всему документу на каждую правку заметен.
 */
export const checkboxHighlight = ViewPlugin.fromClass(
  class {
    decorations: DecorationSet;

    constructor(view: EditorView) {
      this.decorations = build(view);
    }

    update(update: { view: EditorView; docChanged: boolean; viewportChanged: boolean }) {
      if (update.docChanged || update.viewportChanged) this.decorations = build(update.view);
    }
  },
  { decorations: (plugin) => plugin.decorations },
);

function build(view: EditorView): DecorationSet {
  const builder = new RangeSetBuilder<Decoration>();
  for (const { from, to } of view.visibleRanges) {
    // Читаем от начала строки: маркер привязан к её началу, а видимый
    // диапазон может начаться с середины.
    const start = view.state.doc.lineAt(from).from;
    const text = view.state.doc.sliceString(start, to);
    for (const box of findCheckboxes(text, start)) {
      builder.add(box.from, box.to, box.done ? done : open);
    }
  }
  return builder.finish();
}
