type Props = {
  /** Путь заметки относительно корня vault. */
  path: string;
  /** Заметку в последний раз трогал агент — это стоит сказать прямо. */
  fromAgent: boolean;
  onReload: () => void;
  onKeepMine: () => void;
};

/**
 * ConflictModal — файл изменился на диске, пока в буфере лежит несохранённое.
 *
 * Модалка, а не плашка: молча взять любую из сторон нельзя (SPEC §5.3), обе —
 * чья-то работа, и решение принимает человек. Стороны показаны рядом, а не
 * одной строкой с двумя кнопками, потому что выбирают между ними, а не
 * соглашаются с сообщением.
 */
export function ConflictModal({ path, fromAgent, onReload, onKeepMine }: Props) {
  return (
    <div className="overlay">
      <div className="modal conflict">
        <div className="modal__head">
          <span className="banner__glyph">⚠</span>
          <span>Файл изменён снаружи</span>
          <span className="key">{path}</span>
        </div>

        <div className="conflict__sides">
          <div className="conflict__side">
            <div className="conflict__label">
              Версия на диске
              {fromAgent && <span className="conflict__tag">◆ изменена агентом</span>}
            </div>
            <div className="conflict__fact">
              Кто-то записал файл, пока он был открыт здесь.
              <br />
              Взять её — значит выбросить несохранённое из редактора.
            </div>
          </div>

          <div className="conflict__divider" />

          <div className="conflict__side">
            <div className="conflict__label">
              Ваша версия
              <span className="conflict__tag">● в редакторе</span>
            </div>
            <div className="conflict__fact">
              Правки в буфере, на диск ещё не уехали.
              <br />
              Оставить её — значит перезаписать то, что на диске, при
              следующем сохранении.
            </div>
          </div>
        </div>

        <div className="modal__foot">
          <span>esc — оставить своё</span>
          <span className="modal__foot-actions">
            <button className="button" onClick={onKeepMine}>
              Оставить свою
            </button>
            <button className="button button--accent" onClick={onReload}>
              Взять с диска
            </button>
          </span>
        </div>
      </div>
    </div>
  );
}
