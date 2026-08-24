import { useState } from "react";

import { alertKinds, alertNames } from "../markup";
import { toolbarPosition, type SelectionRect } from "../toolbar";
import type { MarkupKind } from "./CodeMirror";

type Props = {
  /** Где стоит выделение, в координатах окна. null — тулбара нет. */
  selection: SelectionRect | null;
  /** Колонка редактора: относительно неё тулбар и позиционируется. */
  container: DOMRect | null;
  onApply: (kind: MarkupKind) => void;
};

/** Кнопки по порядку. Подпись — тем же символом, каким размечает. */
const buttons: { kind: MarkupKind; label: string; title: string }[] = [
  { kind: "bold", label: "B", title: "Жирный" },
  { kind: "italic", label: "I", title: "Курсив" },
  { kind: "strike", label: "S", title: "Зачёркнутый" },
  { kind: "mark", label: "H", title: "Выделение маркером" },
  { kind: "code", label: "‹›", title: "Код в строке" },
  { kind: "fence", label: "{ }", title: "Блок кода" },
];

// Размер нужен до отрисовки: тулбар позиционируется абсолютно, и измерять его
// после появления значит показать один кадр не на месте. Числа совпадают с
// разметкой в styles.css — при правке менять оба.
const size = { width: 268, height: 30 };

/**
 * MarkupToolbar — плавающие кнопки над выделенным текстом.
 *
 * Появляется только при непустом выделении: над кареткой ему нечего делать,
 * а висеть постоянно он не должен — это редактор markdown, и разметку здесь
 * чаще набирают руками, чем нажимают.
 */
export function MarkupToolbar({ selection, container, onApply }: Props) {
  const [alerts, setAlerts] = useState(false);

  if (!selection || !container) return null;

  // Из координат окна в координаты колонки: у неё backdrop-filter, а он
  // делает её содержащим блоком для позиционированных потомков — fixed внутри
  // отсчитывался бы всё равно от неё, только молча и неверно.
  const at = toolbarPosition(
    {
      top: selection.top - container.top,
      bottom: selection.bottom - container.top,
      left: selection.left - container.left,
    },
    { width: container.width, height: container.height },
    size,
  );

  return (
    <div
      className="markupbar"
      style={{ left: at.left, top: at.top }}
      // Нажатие не должно снимать выделение в тексте: без этого кнопка
      // размечала бы пустоту, потому что к моменту клика выделения уже нет.
      onMouseDown={(event) => event.preventDefault()}
    >
      {buttons.map((button) => (
        <button
          key={button.kind}
          className="markupbar__button"
          data-kind={button.kind}
          title={button.title}
          onClick={() => onApply(button.kind)}
        >
          {button.label}
        </button>
      ))}
      <span className="markupbar__rule" />
      <button
        className="markupbar__button"
        title="Алерт GitHub"
        aria-expanded={alerts}
        onClick={() => setAlerts((open) => !open)}
      >
        !
      </button>
      {alerts && (
        <div className="menu menu--alerts">
          {alertKinds.map((kind) => (
            <button
              key={kind}
              className="menu__item"
              onClick={() => {
                setAlerts(false);
                onApply(`alert:${kind}`);
              }}
            >
              <span className="menu__label">{alertNames[kind]}</span>
              <span className="menu__key">{kind}</span>
            </button>
          ))}
        </div>
      )}
    </div>
  );
}
