import { limits, type UISettings } from "../../settings";
import { Card, Row, Slider, Toggle } from "./controls";

type Props = {
  settings: UISettings;
  onChange: (patch: Partial<UISettings>) => void;
};

/**
 * Образец текста для превью.
 *
 * Не «Съешь ещё этих мягких булок»: настраивают здесь чтение кода и списков,
 * и показывать надо ровно то, из чего заметки и состоят. Длинная строка нужна
 * специально — на ней видно, что делает перенос.
 */
const sample = [
  "## Заголовок заметки",
  "",
  "- [x] сделанный пункт",
  "- [ ] длинная строка, на которой видно, переносится текст по ширине колонки или уезжает вбок за край",
  "",
  "`код в строке` и обычный текст",
];

export function EditorPrefs({ settings, onChange }: Props) {
  return (
    <>
      {/* Превью прямо в настройках: иначе, чтобы увидеть кегль и интерлиньяж,
          приходится закрыть окно, посмотреть и открыть заново. Переменные те
          же, что и у настоящего редактора, — App выставляет их на :root, и
          образец меняется вместе с ползунком. */}
      <div className="preview" data-numbers={settings.lineNumbers} data-wrap={settings.lineWrap}>
        {sample.map((line, i) => (
          <div key={i} className="preview__line">
            {settings.lineNumbers && <span className="preview__number">{i + 1}</span>}
            <span className="preview__text">{line}</span>
          </div>
        ))}
      </div>

      <Card>
        <Row label="Кегль" hint="Тело заметки и код">
          <Slider
            value={settings.fontSize}
            min={limits.fontSize.min}
            max={limits.fontSize.max}
            step={limits.fontSize.step}
            format={(value) => `${value} px`}
            onChange={(fontSize) => onChange({ fontSize })}
          />
        </Row>
        <Row label="Интерлиньяж">
          <Slider
            value={settings.lineHeight}
            min={limits.lineHeight.min}
            max={limits.lineHeight.max}
            step={limits.lineHeight.step}
            format={(value) => value.toFixed(2)}
            onChange={(lineHeight) => onChange({ lineHeight })}
          />
        </Row>
        <Row label="Показ">
          <div className="stack">
            <Toggle
              checked={settings.lineNumbers}
              label="Номера строк"
              onChange={(lineNumbers) => onChange({ lineNumbers })}
            />
            <Toggle
              checked={settings.lineWrap}
              label="Переносить длинные строки"
              onChange={(lineWrap) => onChange({ lineWrap })}
            />
          </div>
        </Row>
      </Card>

      <Card>
        <Row label="Автосохранение" hint="Сколько ждать после последней правки">
          <div className="stack">
            <Slider
              value={settings.saveDelay}
              min={limits.saveDelay.min}
              max={limits.saveDelay.max}
              step={limits.saveDelay.step}
              format={(value) => `${value} мс`}
              onChange={(saveDelay) => onChange({ saveDelay })}
            />
            <span className="card__note">
              Заметка всё равно уезжает на диск при переключении на другую и при закрытии окна —
              задержка решает только, как часто пишется файл во время набора.
            </span>
          </div>
        </Row>
      </Card>

      <Card>
        <Row label="Vim" hint="Включён по умолчанию — это одна из трёх причин, по которым приложение существует">
          <div className="stack">
            <Toggle
              checked={settings.vim}
              label="Вим-режим в редакторе"
              onChange={(vim) => onChange({ vim })}
            />
            <Toggle
              checked={settings.vimNavigation}
              label="Движения j, k, h, l — в списке, сайдбаре и при смене колонки"
              onChange={(vimNavigation) => onChange({ vimNavigation })}
            />
            <span className="card__note">
              Выключенный вим делает редактор обычным текстовым полем: текст печатается сразу,
              режимов и ex-команд <code>:w</code> и <code>:q</code> нет. Движения снимаются
              отдельно и вместе со сменой колонки на <code>⌃⇧H</code> и <code>⌃⇧L</code> — она
              переезжает на <code>⌃⇧←</code> и <code>⌃⇧→</code>. Стрелки, <code>⏎</code> и
              остальные команды работают в любом случае. Клавиши правятся в разделе «Шоткаты».
            </span>
          </div>
        </Row>
      </Card>

      <div className="card card--flat">
        <div className="card__row">
          <div className="card__label">
            <span className="card__name">Системный ввод</span>
          </div>
          <div className="card__control">
            <span className="card__note">
              Автозамена кавычек, автокапитализация и проверка орфографии выключены насовсем:
              они ломают прямые кавычки в коде и мешают виму. Это решение фазы 0.
            </span>
          </div>
        </div>
      </div>
    </>
  );
}
