import { accentNames, accents, limits, type Theme, type UISettings } from "../../settings";
import { Card, Row, Slider, Toggle } from "./controls";

type Props = {
  settings: UISettings;
  onChange: (patch: Partial<UISettings>) => void;
  /** Машина сейчас на батарее — тогда прозрачность выключена принудительно. */
  onBattery: boolean;
};

/** Три оформления в порядке макета. */
const themeOptions: { value: Theme; label: string }[] = [
  { value: "auto", label: "Авто" },
  { value: "light", label: "Светлая" },
  { value: "dark", label: "Тёмная" },
];

export function Appearance({ settings, onChange, onBattery }: Props) {
  // Прозрачность может быть выставлена, но не действовать: об этом надо
  // сказать прямо, иначе ползунок выглядит сломанным.
  const muted = settings.opaqueOnBattery && onBattery && settings.transparency > 0;

  return (
    <>
      <Card>
        <Row label="Оформление">
          <div className="themes">
            {themeOptions.map((option) => (
              <button
                key={option.value}
                className="theme"
                aria-selected={settings.theme === option.value}
                onClick={() => onChange({ theme: option.value })}
              >
                {/* Превью рисуется теми же тремя колонками, что и окно: так
                    понятно, что именно поменяется. */}
                <span className="theme__preview" data-kind={option.value}>
                  <span className="theme__bar" />
                  <span className="theme__side" />
                  <span className="theme__body" />
                </span>
                <span className="theme__label">{option.label}</span>
              </button>
            ))}
          </div>
        </Row>

        <Row
          label="Масштаб текста"
          hint="Растёт только кегль — колонки сохраняют ширину"
        >
          <Slider
            value={settings.textScale}
            min={limits.textScale.min}
            max={limits.textScale.max}
            step={limits.textScale.step}
            format={(value) => `${Math.round(value * 100)}%`}
            onChange={(textScale) => onChange({ textScale })}
          />
        </Row>

        <Row label="Акцент" hint="Красит выделение, ссылки, чеклисты и метку агента">
          <div className="accents">
            {accents.map((accent) => (
              <button
                key={accent}
                className="accent"
                aria-selected={settings.accent === accent}
                onClick={() => onChange({ accent })}
              >
                <span
                  className="accent__dot"
                  data-custom={accent === "custom"}
                  data-accent={accent}
                  style={
                    accent === "custom"
                      ? { ["--hsl-accent" as string]: `${settings.accentHue} 28% 64%` }
                      : undefined
                  }
                >
                  {accent === "custom" ? "+" : ""}
                </span>
                <span className="accent__label">{accentNames[accent]}</span>
              </button>
            ))}
          </div>
        </Row>

        {settings.accent === "custom" && (
          <Row label="Оттенок" hint="Насыщенность и светлота подбираются под тему сами">
            <Slider
              value={settings.accentHue}
              min={limits.accentHue.min}
              max={limits.accentHue.max}
              step={limits.accentHue.step}
              format={(value) => `${value}°`}
              onChange={(accentHue) => onChange({ accentHue })}
            />
          </Row>
        )}
      </Card>

      <Card>
        <Row label="Прозрачность фона" hint="Сайдбар и список пропускают обои">
          <Slider
            value={settings.transparency}
            min={limits.transparency.min}
            max={limits.transparency.max}
            step={limits.transparency.step}
            format={(value) => `${value}%`}
            onChange={(transparency) => onChange({ transparency })}
          />
        </Row>

        <Row label="Размытие" hint="Действует только при ненулевой прозрачности">
          <Slider
            value={settings.blur}
            min={limits.blur.min}
            max={limits.blur.max}
            step={limits.blur.step}
            format={(value) => `${value}%`}
            onChange={(blur) => onChange({ blur })}
          />
        </Row>

        <Row label="Экономия">
          <div className="stack">
            <Toggle
              checked={settings.opaqueOnBattery}
              label="Отключать прозрачность на батарее"
              onChange={(opaqueOnBattery) => onChange({ opaqueOnBattery })}
            />
            <Toggle
              checked={settings.dither}
              label="Дизер-текстура на панелях"
              onChange={(dither) => onChange({ dither })}
            />
            {muted && (
              <span className="card__note">
                Сейчас машина на батарее — панели непрозрачны, пока её не подключат.
              </span>
            )}
          </div>
        </Row>
      </Card>

      <p className="section-note">
        Редактор всегда плотнее списка на 30 пунктов: сквозь текст, который правят, обои
        просвечивать не должны.
      </p>
    </>
  );
}
