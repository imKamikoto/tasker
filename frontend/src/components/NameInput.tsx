import { useEffect, useRef, useState } from "react";

type Props = {
  initial: string;
  placeholder: string;
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
export function NameInput({ initial, placeholder, onCommit, onCancel }: Props) {
  const [value, setValue] = useState(initial);
  const input = useRef<HTMLInputElement | null>(null);

  useEffect(() => {
    input.current?.focus();
    input.current?.select();
  }, []);

  return (
    <input
      ref={input}
      className="nameinput"
      value={value}
      placeholder={placeholder}
      spellCheck={false}
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
  );
}
