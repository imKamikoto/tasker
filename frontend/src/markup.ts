/**
 * Разметка выделенного текста: болд, курсив, код, блоки, алерты.
 *
 * Чистыми функциями над строкой, а не командами CodeMirror: вся сложность
 * здесь в границах — выделение, уже обёрнутое маркерами; маркеры снаружи
 * выделения; пустое выделение; блок, начатый с середины строки. Проверять это
 * таблицей дешевле и надёжнее, чем щёлкать в окне.
 */

/** Что заменить в тексте и где оказаться выделению после замены. */
export type Replacement = {
  from: number;
  to: number;
  insert: string;
  /** Границы выделения после замены, в координатах уже изменённого текста. */
  select: { from: number; to: number };
};

/** Парные маркеры строчной разметки. */
export const inlineMarkers = {
  bold: "**",
  italic: "*",
  strike: "~~",
  mark: "==",
  code: "`",
} as const;

export type InlineKind = keyof typeof inlineMarkers;

/**
 * toggleInline оборачивает выделение маркерами или снимает их.
 *
 * Снимает в двух случаях: маркеры внутри выделения и маркеры сразу за его
 * границами. Второй важнее первого — двойной щелчок по слову выделяет слово,
 * а не звёздочки вокруг него, и без этого снять болд было бы нельзя.
 */
export function toggleInline(
  doc: string,
  from: number,
  to: number,
  kind: InlineKind,
): Replacement {
  const marker = inlineMarkers[kind];
  const width = marker.length;
  const selected = doc.slice(from, to);

  // Маркеры снаружи выделения.
  const before = doc.slice(Math.max(0, from - width), from);
  const after = doc.slice(to, to + width);
  if (before === marker && after === marker) {
    return {
      from: from - width,
      to: to + width,
      insert: selected,
      select: { from: from - width, to: from - width + selected.length },
    };
  }

  // Маркеры внутри выделения.
  if (selected.length >= width * 2 && selected.startsWith(marker) && selected.endsWith(marker)) {
    const inner = selected.slice(width, selected.length - width);
    return { from, to, insert: inner, select: { from, to: from + inner.length } };
  }

  const wrapped = marker + selected + marker;
  return {
    from,
    to,
    insert: wrapped,
    // Пустое выделение — каретка между маркерами: человек нажал болд, чтобы
    // печатать жирным, а не чтобы получить четыре звёздочки и искать середину.
    select:
      selected === ""
        ? { from: from + width, to: from + width }
        : { from: from + width, to: from + width + selected.length },
  };
}

/**
 * lineRange расширяет границы до целых строк.
 *
 * Блочная разметка иначе разрежет строку пополам: ограда посреди абзаца — это
 * не блок кода, а испорченный текст.
 */
export function lineRange(doc: string, from: number, to: number): { from: number; to: number } {
  let start = from;
  while (start > 0 && doc[start - 1] !== "\n") start--;
  let end = to;
  while (end < doc.length && doc[end] !== "\n") end++;
  return { from: start, to: end };
}

/**
 * toggleFence заворачивает выделенные строки в ограду блока кода или снимает её.
 *
 * Ограда из трёх обратных кавычек, как во всём GFM. Язык не подставляется:
 * угадать его нельзя, а пустая подсказка честнее неверной.
 */
export function toggleFence(doc: string, from: number, to: number): Replacement {
  const range = lineRange(doc, from, to);
  const text = doc.slice(range.from, range.to);
  const lines = text.split("\n");

  // Ограда внутри выделения.
  const fenced =
    lines.length >= 2 &&
    lines[0].trimEnd().startsWith("```") &&
    lines[lines.length - 1].trimEnd() === "```";

  if (fenced) {
    const inner = lines.slice(1, -1).join("\n");
    return {
      from: range.from,
      to: range.to,
      insert: inner,
      select: { from: range.from, to: range.from + inner.length },
    };
  }

  // Ограда снаружи выделения — тот же случай, что и у строчных маркеров, и
  // более частый: выделяют код, а не ограду вокруг него.
  const outer = outerFence(doc, range);
  if (outer !== null) {
    return {
      from: outer.from,
      to: outer.to,
      insert: text,
      select: { from: outer.from, to: outer.from + text.length },
    };
  }

  const insert = "```\n" + text + "\n```";
  // Выделение остаётся на самом тексте, а не на ограде: следующее нажатие
  // должно снимать блок, а не заворачивать ограду в ограду.
  return {
    from: range.from,
    to: range.to,
    insert,
    select: { from: range.from + 4, to: range.from + 4 + text.length },
  };
}

/**
 * outerFence находит ограду, стоящую вокруг выделенных строк.
 *
 * Возвращает границы вместе с обеими строками ограды или null, если её нет.
 */
function outerFence(doc: string, range: { from: number; to: number }): { from: number; to: number } | null {
  if (range.from === 0 || range.to === doc.length) return null;

  const prev = lineRange(doc, range.from - 1, range.from - 1);
  const next = lineRange(doc, range.to + 1, range.to + 1);
  if (!doc.slice(prev.from, prev.to).trimEnd().startsWith("```")) return null;
  if (doc.slice(next.from, next.to).trimEnd() !== "```") return null;
  return { from: prev.from, to: next.to };
}

/** Виды алертов GitHub. Порядок — как в их документации. */
export const alertKinds = ["NOTE", "TIP", "IMPORTANT", "WARNING", "CAUTION"] as const;
export type AlertKind = (typeof alertKinds)[number];

/** Человеческие имена для меню. */
export const alertNames: Record<AlertKind, string> = {
  NOTE: "Заметка",
  TIP: "Совет",
  IMPORTANT: "Важно",
  WARNING: "Предупреждение",
  CAUTION: "Осторожно",
};

/**
 * applyAlert превращает выделенные строки в алерт GitHub.
 *
 * Уже размеченный блок меняет вид, а не обрастает вторым уровнем цитаты:
 * нажать «Важно» на «Совете» — это «пусть будет важно», а не «процитируй».
 */
export function applyAlert(doc: string, from: number, to: number, kind: AlertKind): Replacement {
  const range = lineRange(doc, from, to);
  const text = doc.slice(range.from, range.to);
  const lines = text.split("\n");

  // Снимаем прежнюю разметку цитаты, если она есть, вместе со старым заголовком.
  const stripped = lines.map((line) => line.replace(/^>\s?/, ""));
  if (stripped.length > 0 && /^\[![A-Z]+\]\s*$/.test(stripped[0].trim())) {
    stripped.shift();
  }

  const body = stripped.map((line) => (line === "" ? ">" : `> ${line}`));
  const insert = [`> [!${kind}]`, ...body].join("\n");
  return {
    from: range.from,
    to: range.to,
    insert,
    select: { from: range.from, to: range.from + insert.length },
  };
}
