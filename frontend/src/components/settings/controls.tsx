import type { ReactNode } from "react";

/**
 * Общие контролы экрана настроек.
 *
 * Карточка со строками в стиле System Settings: слева подпись фиксированной
 * ширины с пояснением, справа контрол. Ширина подписи одна на все строки —
 * иначе контролы встают лесенкой и карточка перестаёт читаться столбцом.
 */

export function Card({ children }: { children: ReactNode }) {
  return <div className="card">{children}</div>;
}

type RowProps = {
  label: string;
  hint?: string;
  children: ReactNode;
};

export function Row({ label, hint, children }: RowProps) {
  return (
    <div className="card__row">
      <div className="card__label">
        <span className="card__name">{label}</span>
        {hint && <span className="card__hint">{hint}</span>}
      </div>
      <div className="card__control">{children}</div>
    </div>
  );
}

type SliderProps = {
  value: number;
  min: number;
  max: number;
  step: number;
  /** Как показать значение справа от дорожки. */
  format: (value: number) => string;
  onChange: (value: number) => void;
};

export function Slider({ value, min, max, step, format, onChange }: SliderProps) {
  // Доля пройденного считается здесь и уезжает в CSS переменной: WebKit не
  // красит пройденную часть дорожки сам.
  const fill = max > min ? ((value - min) / (max - min)) * 100 : 0;

  return (
    <div className="slider">
      <input
        className="slider__input"
        type="range"
        value={value}
        min={min}
        max={max}
        step={step}
        style={{ ["--slider-fill" as string]: `${fill}%` }}
        onChange={(event) => onChange(Number(event.target.value))}
      />
      <span className="slider__value">{format(value)}</span>
    </div>
  );
}

type ToggleProps = {
  checked: boolean;
  label: string;
  disabled?: boolean;
  onChange: (checked: boolean) => void;
};

export function Toggle({ checked, label, disabled, onChange }: ToggleProps) {
  return (
    <label className="toggle" data-disabled={disabled === true}>
      <input
        type="checkbox"
        checked={checked}
        disabled={disabled}
        onChange={(event) => onChange(event.target.checked)}
      />
      <span className="toggle__track" aria-hidden="true">
        <span className="toggle__knob" />
      </span>
      {label}
    </label>
  );
}

type ChoiceProps<T extends string> = {
  value: T;
  options: { value: T; label: string }[];
  onChange: (value: T) => void;
};

/** Сегментированный переключатель для двух-трёх вариантов. */
export function Choice<T extends string>({ value, options, onChange }: ChoiceProps<T>) {
  return (
    <div className="choice">
      {options.map((option) => (
        <button
          key={option.value}
          className="choice__item"
          aria-selected={option.value === value}
          onClick={() => onChange(option.value)}
        >
          {option.label}
        </button>
      ))}
    </div>
  );
}

/** Строка с путём и кнопкой «показать в Finder». */
export function PathLine({ path, onReveal }: { path: string; onReveal?: () => void }) {
  return (
    <div className="pathline">
      {/* Целиком путь лежит в подсказке: в строку он не всегда влезает, а
          обрезанный конец надо уметь посмотреть. */}
      <span className="pathline__path" title={path}>
        {path || "—"}
      </span>
      {path && onReveal && (
        <button className="button button--link" onClick={onReveal}>
          в Finder
        </button>
      )}
    </div>
  );
}

/** Пара «что» — «сколько» для сводок. */
export function Fact({ label, value }: { label: string; value: ReactNode }) {
  return (
    <div className="fact">
      <span className="fact__label">{label}</span>
      <span className="fact__value">{value}</span>
    </div>
  );
}
