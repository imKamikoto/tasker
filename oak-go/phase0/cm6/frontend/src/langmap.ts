// Vim-совместимый langmap для ЙЦУКЕН: в NORMAL/VISUAL клавиша с русской буквой
// выполняет команду, соответствующую её физической позиции на клавиатуре.
// Строка собирается из позиционных рядов, а не пишется руками: списки должны
// быть одной длины, иначе парсер вима молча выбрасывает весь кусок.

// Ряды клавиатуры в одном и том же порядке.
const EN = "qwertyuiop[]asdfghjkl;'zxcvbnm,./";
const RU = "йцукенгшщзхъфывапролджэячсмитьбю.";

const EN_SHIFT = "QWERTYUIOP{}ASDFGHJKL:\"ZXCVBNM<>?";
const RU_SHIFT = "ЙЦУКЕНГШЩЗХЪФЫВАПРОЛДЖЭЯЧСМИТЬБЮ,";

// В формате langmap запятая, точка с запятой и обратный слеш экранируются.
const esc = (s: string) => s.replace(/([\\,;])/g, "\\$1");

if (EN.length !== RU.length || EN_SHIFT.length !== RU_SHIFT.length) {
  throw new Error("langmap: ряды разной длины, парсер молча выбросит их целиком");
}

export const RU_LANGMAP =
  `${esc(RU)};${esc(EN)},${esc(RU_SHIFT)};${esc(EN_SHIFT)}`;
