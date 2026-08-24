import { RangeSetBuilder } from "@codemirror/state";
import { Decoration, ViewPlugin, type DecorationSet, type EditorView } from "@codemirror/view";

/** Найденная ссылка на заметку: где стоит и куда ведёт. */
export type NoteLink = { from: number; to: number; id: string };

/**
 * Ссылка на заметку — та же, что кладёт Go: `tasker://note/<ULID>`.
 *
 * Класс символов совпадает с reNoteURL в internal/index/record.go нарочно.
 * Crockford base32 строже (в нём нет I, L, O, U), но ссылку считает ссылкой
 * именно Go, и подчёркивать здесь надо ровно то, что он проиндексировал —
 * иначе часть ссылок окажется некликабельной без всякой видимой причины.
 */
const link = /tasker:\/\/note\/([0-9A-Z]{26})/g;

/**
 * findNoteLinks находит ссылки на заметки в куске текста.
 *
 * offset — позиция куска в документе, чтобы вернуть абсолютные координаты.
 * Чистой функцией: границы здесь и есть вся суть — ссылка в конце строки, две
 * подряд, обрезанный по длине идентификатор, — и проверяются они таблицей.
 */
export function findNoteLinks(text: string, offset = 0): NoteLink[] {
  const out: NoteLink[] = [];
  // lastIndex у глобального регекспа переживает вызовы, поэтому свой на каждый.
  const re = new RegExp(link.source, "g");
  let found: RegExpExecArray | null;
  while ((found = re.exec(text)) !== null) {
    out.push({
      from: offset + found.index,
      to: offset + found.index + found[0].length,
      id: found[1],
    });
  }
  return out;
}

/**
 * linkAt отвечает, на какую заметку ведёт ссылка под этой позицией.
 *
 * Отдельно от поиска, потому что вопрос другой: не «где ссылки», а «попал ли
 * щелчок в одну из них». Пустая строка — не попал.
 */
export function linkAt(links: NoteLink[], pos: number): string {
  for (const item of links) {
    if (pos >= item.from && pos <= item.to) return item.id;
  }
  return "";
}

// Инлайн-стилем по той же причине, что и у чекбоксов: подсветка синтаксиса
// красит тот же диапазон как часть ссылки markdown, и кто победит из двух
// классов, решает порядок в сгенерированной таблице стилей — то есть случай.
const mark = Decoration.mark({
  attributes: {
    style: "text-decoration: underline; text-underline-offset: 2px; cursor: pointer",
    title: "⌘+щелчок — открыть заметку",
  },
});

/**
 * noteLinkHighlight подчёркивает ссылки на заметки.
 *
 * Подчёркивание здесь не украшение: редактор показывает исходник, и без него
 * ссылка неотличима от текста, а значит никто не догадается по ней щёлкнуть.
 * Обрабатываются только видимые строки — как и у чекбоксов, проход по всему
 * документу на каждую правку заметен на длинной заметке.
 */
export const noteLinkHighlight = ViewPlugin.fromClass(
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
    const start = view.state.doc.lineAt(from).from;
    const text = view.state.doc.sliceString(start, to);
    for (const item of findNoteLinks(text, start)) {
      builder.add(item.from, item.to, mark);
    }
  }
  return builder.finish();
}

/** visibleNoteLinks собирает ссылки из видимой части документа. */
export function visibleNoteLinks(view: EditorView): NoteLink[] {
  const out: NoteLink[] = [];
  for (const { from, to } of view.visibleRanges) {
    const start = view.state.doc.lineAt(from).from;
    out.push(...findNoteLinks(view.state.doc.sliceString(start, to), start));
  }
  return out;
}
