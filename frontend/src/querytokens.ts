/**
 * Разбор строки поиска для подсветки.
 *
 * Это не второй парсер запросов: исполняет их по-прежнему Go, а здесь ровно
 * столько знания, сколько нужно, чтобы покрасить набранное и показать опечатку
 * в закрытом перечислении до отправки запроса. Правила повторяют
 * internal/index/query.go и обязаны меняться вместе с ним.
 */

/** Как красить кусок запроса. */
export type TokenKind =
  /** Обычный текст и фильтры, не сужающие набор по имени. */
  | "plain"
  /** Фильтр по ноутбуку или тегу — то, чем чаще всего отбирают. */
  | "accent"
  /** Значение вне закрытого перечисления или пустое: запрос не исполнится. */
  | "bad";

export type QueryToken = {
  text: string;
  kind: TokenKind;
};

/** Значения закрытых перечислений. Совпадают с проверками в query.go. */
const enums: Record<string, string[]> = {
  status: ["none", "active", "onhold", "completed", "dropped"],
  is: ["pinned", "agent"],
  has: ["task"],
};

/** Фильтры, которые красятся акцентом: ими отбирают, а не уточняют текст. */
const accented = new Set(["book", "tag"]);

/** Все известные префиксы. Незнакомый — это обычный текст с двоеточием. */
const known = new Set([...Object.keys(enums), ...accented, "title", "body"]);

/**
 * splitQuery режет строку на куски вместе с пробелами между ними.
 *
 * Пробелы сохраняются в выдаче: она накладывается поверх поля ввода символ
 * в символ, и потерянный пробел сдвинул бы всю подсветку.
 */
export function splitQuery(input: string): QueryToken[] {
  const out: QueryToken[] = [];
  let word = "";
  let space = "";
  let inQuote = false;

  const flush = () => {
    if (space !== "") {
      out.push({ text: space, kind: "plain" });
      space = "";
    }
    if (word !== "") {
      out.push({ text: word, kind: classify(word) });
      word = "";
    }
  };

  for (const char of input) {
    if (char === '"') inQuote = !inQuote;
    if (!inQuote && (char === " " || char === "\t")) {
      if (word !== "") flush();
      space += char;
      continue;
    }
    if (space !== "") {
      out.push({ text: space, kind: "plain" });
      space = "";
    }
    word += char;
  }
  flush();
  return out;
}

/** classify решает, каким цветом красить один токен. */
export function classify(token: string): TokenKind {
  const raw = token.startsWith("-") ? token.slice(1) : token;
  const colon = raw.indexOf(":");
  if (colon < 0) return "plain";

  const name = raw.slice(0, colon).toLowerCase();
  if (!known.has(name)) return "plain";

  const value = raw.slice(colon + 1).replace(/"/g, "");
  // Пустое значение — не опечатка, а недопечатанное. Красить его красным,
  // пока человек ещё набирает, значит ругаться на каждый второй символ.
  if (value === "") return accented.has(name) ? "accent" : "plain";

  const allowed = enums[name];
  if (allowed && !allowed.includes(value.toLowerCase())) return "bad";
  return accented.has(name) ? "accent" : "plain";
}

/** hasBadToken — есть ли в запросе кусок, из-за которого он не исполнится. */
export function hasBadToken(input: string): boolean {
  return splitQuery(input).some((token) => token.kind === "bad");
}

/** allowedValues возвращает значения фильтра для подсказки в пустой выдаче. */
export function allowedValues(name: string): string[] {
  return enums[name.toLowerCase()] ?? [];
}

/** badTokens отдаёт сами испорченные куски — их показывают в объяснении. */
export function badTokens(input: string): string[] {
  return splitQuery(input)
    .filter((token) => token.kind === "bad")
    .map((token) => token.text);
}
