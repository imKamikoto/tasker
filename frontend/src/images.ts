import { RangeSetBuilder } from "@codemirror/state";
import {
  Decoration,
  ViewPlugin,
  WidgetType,
  type DecorationSet,
  type EditorView,
} from "@codemirror/view";

/** Найденная картинка в тексте: где стоит, чем подписана, откуда берётся. */
export type ImageRef = { from: number; to: number; alt: string; src: string };

/**
 * Картинка markdown: `![подпись](путь)`.
 *
 * Путь без пробелов и без закрывающей скобки — ровно то, что кладёт Go
 * (attachments/2026/08/ABCDEFGH.png). Разбирать полный синтаксис ссылок
 * markdown здесь незачем: показать надо свои вложения, а не всё возможное.
 */
const image = /!\[([^\]]*)\]\(([^()\s]+)\)/g;

/**
 * findImages находит картинки в куске текста.
 *
 * offset — позиция куска в документе, чтобы вернуть абсолютные координаты.
 * Чистой функцией: пустая подпись, подпись со скобками, две картинки в строке,
 * ссылка без восклицательного знака — всё это границы, и проверяются они
 * таблицей.
 */
export function findImages(text: string, offset = 0): ImageRef[] {
  const out: ImageRef[] = [];
  const re = new RegExp(image.source, "g");
  let found: RegExpExecArray | null;
  while ((found = re.exec(text)) !== null) {
    out.push({
      from: offset + found.index,
      to: offset + found.index + found[0].length,
      alt: found[1],
      src: found[2],
    });
  }
  return out;
}

/**
 * imageURL превращает путь из markdown в адрес, по которому вебвью получит файл.
 *
 * Путь в заметке — от корня хранилища (SPEC §4.4), а хранилище отдаётся по
 * своему префиксу. Внешние адреса остаются как есть: они и так абсолютные, и
 * подставлять им префикс значит сломать.
 */
export function imageURL(src: string): string {
  if (/^[a-z]+:\/\//i.test(src) || src.startsWith("data:")) return src;
  // Каждый сегмент кодируется отдельно: в путях есть кириллица и пробелы, а
  // слэши между сегментами должны остаться слэшами.
  const encoded = src
    .replace(/^\/+/, "")
    .split("/")
    .map((part) => encodeURIComponent(part))
    .join("/");
  return `/vault/${encoded}`;
}

/**
 * Виджет с самой картинкой.
 *
 * Отдельным блоком под строкой, а не заменой разметки: это редактор исходника,
 * и подменять `![](...)` картинкой значит прятать то, что человек правит.
 * Строка остаётся строкой, картинка появляется под ней.
 */
class ImageWidget extends WidgetType {
  // Обычными полями, а не свойствами-параметрами конструктора: тесты в этом
  // проекте гоняются node в режиме strip-only, а он такой синтаксис не берёт.
  readonly src: string;
  readonly alt: string;

  constructor(src: string, alt: string) {
    super();
    this.src = src;
    this.alt = alt;
  }

  // Без этого CodeMirror пересоздаёт виджет на каждую перерисовку, и картинка
  // мигает при наборе в соседней строке.
  eq(other: ImageWidget): boolean {
    return other.src === this.src && other.alt === this.alt;
  }

  toDOM(view: EditorView): HTMLElement {
    const wrap = document.createElement("div");
    wrap.className = "cm-image";

    const img = document.createElement("img");
    img.src = imageURL(this.src);
    img.alt = this.alt;
    // Высота виджета меняется, когда картинка догрузилась: CodeMirror считает
    // геометрию заранее и без пересчёта оставит под ней дырку или наложение.
    img.addEventListener("load", () => view.requestMeasure());
    img.addEventListener("error", () => {
      wrap.classList.add("cm-image--broken");
      wrap.textContent = "картинка не найдена: " + this.src;
      view.requestMeasure();
    });

    wrap.appendChild(img);
    return wrap;
  }

  // Щелчки по картинке редактору не нужны, но выделение текста мышью через
  // неё проходить должно.
  ignoreEvent(): boolean {
    return false;
  }
}

/**
 * imagePreview показывает картинки под строками, в которых они упомянуты.
 *
 * Только видимые строки — как и у чекбоксов: проход по всему документу на
 * каждую правку заметен на длинной заметке.
 */
export const imagePreview = ViewPlugin.fromClass(
  class {
    decorations: DecorationSet;

    constructor(view: EditorView) {
      this.decorations = buildImages(view);
    }

    update(update: { view: EditorView; docChanged: boolean; viewportChanged: boolean }) {
      if (update.docChanged || update.viewportChanged) this.decorations = buildImages(update.view);
    }
  },
  { decorations: (plugin) => plugin.decorations },
);

function buildImages(view: EditorView): DecorationSet {
  const builder = new RangeSetBuilder<Decoration>();
  for (const { from, to } of view.visibleRanges) {
    const start = view.state.doc.lineAt(from).from;
    const text = view.state.doc.sliceString(start, to);
    for (const found of findImages(text, start)) {
      const line = view.state.doc.lineAt(found.from);
      builder.add(
        line.to,
        line.to,
        Decoration.widget({ widget: new ImageWidget(found.src, found.alt), block: true, side: 1 }),
      );
    }
  }
  return builder.finish();
}
