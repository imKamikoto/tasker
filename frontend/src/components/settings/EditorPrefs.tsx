import { limits, type UISettings } from "../../settings";
import { Card, Row, Slider, Toggle } from "./controls";

type Props = {
  settings: UISettings;
  onChange: (patch: Partial<UISettings>) => void;
};

export function EditorPrefs({ settings, onChange }: Props) {
  return (
    <>
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
              label="Движения j, k, h, l в списке и сайдбаре"
              onChange={(vimNavigation) => onChange({ vimNavigation })}
            />
            <span className="card__note">
              Выключенный вим делает редактор обычным текстовым полем: текст печатается сразу,
              режимов и ex-команд <code>:w</code> и <code>:q</code> нет. Движения снимаются
              отдельно — стрелки, <code>⏎</code> и остальные команды работают в любом случае.
              Клавиши правятся в разделе «Шоткаты».
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
