import { noteCount } from "../format";
import { statusGlyphs, statuses, type Status } from "../statuses";

type Props = {
  count: number;
  /** Закреплена ли первая заметка выделения: от неё считается, что делать. */
  pinned: boolean;
  onStatus: (status: Status) => void;
  onPin: () => void;
  onMove: () => void;
  onTrash: () => void;
  onClear: () => void;
};

/**
 * BulkPane — что можно сделать с выделенной пачкой.
 *
 * До сих пор массовые операции жили только на клавиатуре, и о них нельзя было
 * узнать, не читая документацию. Панель называет их вместе с шоткатами: она
 * же и подсказка, которую видно ровно тогда, когда она нужна.
 */
export function BulkPane({ count, pinned, onStatus, onPin, onMove, onTrash, onClear }: Props) {
  return (
    <div className="bulk">
      <div className="bulk__head">
        <span>Выбрано: {noteCount(count)}</span>
        <span className="bulk__rule" />
        <button className="bulk__esc" onClick={onClear}>
          esc — снять
        </button>
      </div>

      <div className="bulk__grid">
        {/* Статусы разложены в ряд: выбрать один — самое частое, ради чего
            вообще выделяют несколько заметок. Подпись общая на весь ряд —
            пять глифов подряд без неё читаются как ребус. */}
        <div className="bulk__wide bulk__row">
          <span className="bulk__caption">статус</span>
          <span className="menu__key">⌘⌃1..5</span>
        </div>
        <div className="bulk__wide bulk__grid" style={{ gridTemplateColumns: "repeat(5, 1fr)" }}>
          {statuses.map((status, i) => (
            <button
              key={status}
              className="bulk__button"
              title={`${status} (⌘⌃${i + 1})`}
              onClick={() => onStatus(status)}
            >
              <span className="menu__glyph" data-status={status}>
                {statusGlyphs[status]}
              </span>
            </button>
          ))}
        </div>

        <button className="bulk__button" onClick={onMove}>
          <span>→ перенести</span>
          <span className="menu__key">m</span>
        </button>
        <button className="bulk__button" onClick={onPin}>
          <span>{pinned ? "☆ открепить" : "★ закрепить"}</span>
          <span className="menu__key">p</span>
        </button>
        <button className="bulk__button bulk__button--danger bulk__wide" onClick={onTrash}>
          <span>▚ в корзину</span>
          <span className="menu__key">⌘⌫</span>
        </button>
      </div>
    </div>
  );
}
