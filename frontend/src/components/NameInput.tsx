import { useEffect, useRef, useState } from "react";

type Props = {
  initial: string;
  placeholder: string;
  /** Подсказка под полем: что сделает Enter и как отменить. */
  hint?: string;
  onCommit: (value: string) => void;
  onCancel: () => void;
};

/**
 * NameInput — строка ввода имени прямо в дереве.
 *
 * Не диалог: создание и переименование ноутбука — мелкие действия, и модальное
 * окно ради одного слова сбивает больше, чем помогает. Enter подтверждает,
 * Escape и потеря фокуса отменяют.
 */
export function NameInput({ initial, placeholder, hint, onCommit, onCancel }: Props) {
  const [value, setValue] = useState(initial);
  const input = useRef<HTMLInputElement | null>(null);

  useEffect(() => {
    input.current?.focus();
    input.current?.select();
  }, []);

  return (
    <>
      <input
        ref={input}
        className="nameinput"
        value={value}
        placeholder={placeholder}
        spellCheck={false}
        autoCorrect="off"
        onChange={(event) => setValue(event.target.value)}
        onBlur={onCancel}
        onKeyDown={(event) => {
          if (event.key === "Enter") {
            event.preventDefault();
            const clean = value.trim();
            if (clean === "") onCancel();
            else onCommit(clean);
          }
          if (event.key === "Escape") {
            event.preventDefault();
            onCancel();
          }
        }}
      />
      {hint && <div className="nameinput__hint">{hint}</div>}
    </>
  );
}
