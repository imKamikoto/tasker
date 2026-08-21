import assert from "node:assert/strict";
import { test } from "node:test";

import { defaultSettings, limits, nextZoom, parseSettings } from "./settings.ts";

test("нормальные настройки читаются как есть", () => {
  const got = parseSettings(
    JSON.stringify({
      sidebarWidth: 240,
      listWidth: 400,
      sortField: "title",
      sortReversed: true,
      collapsed: ["Работа"],
    }),
  );
  assert.equal(got.sidebarWidth, 240);
  assert.equal(got.listWidth, 400);
  assert.equal(got.sortField, "title");
  assert.equal(got.sortReversed, true);
  assert.deepEqual(got.collapsed, ["Работа"]);
});

test("мусор вместо файла даёт умолчания", () => {
  for (const raw of ["", "не json", "null", "[]", "42", '"строка"']) {
    assert.deepEqual(parseSettings(raw), defaultSettings, `на вводе ${raw}`);
  }
});

test("одно испорченное поле не утаскивает остальные", () => {
  const got = parseSettings(
    JSON.stringify({ sidebarWidth: "широкий", sortField: "выдуманное", collapsed: "нет" }),
  );
  assert.equal(got.sidebarWidth, defaultSettings.sidebarWidth);
  assert.equal(got.sortField, defaultSettings.sortField);
  assert.deepEqual(got.collapsed, []);
});

test("ширины зажимаются в разумные пределы", () => {
  assert.equal(parseSettings(JSON.stringify({ sidebarWidth: -500 })).sidebarWidth, limits.sidebarWidth.min);
  assert.equal(parseSettings(JSON.stringify({ sidebarWidth: 99999 })).sidebarWidth, limits.sidebarWidth.max);
  assert.equal(parseSettings(JSON.stringify({ listWidth: 10 })).listWidth, limits.listWidth.min);
  assert.equal(parseSettings(JSON.stringify({ listWidth: 10000 })).listWidth, limits.listWidth.max);
});

test("в списке свёрнутых остаются только строки", () => {
  const got = parseSettings(JSON.stringify({ collapsed: ["Работа", 42, null, "Личное"] }));
  assert.deepEqual(got.collapsed, ["Работа", "Личное"]);
});

test("тема и акцент читаются, мусор в них даёт умолчание", () => {
  const got = parseSettings(JSON.stringify({ theme: "light", accent: "sage" }));
  assert.equal(got.theme, "light");
  assert.equal(got.accent, "sage");

  const bad = parseSettings(JSON.stringify({ theme: "неоновая", accent: "розовый" }));
  assert.equal(bad.theme, defaultSettings.theme);
  assert.equal(bad.accent, defaultSettings.accent);
});

test("свой акцент — это оттенок в градусах", () => {
  const got = parseSettings(JSON.stringify({ accent: "custom", accentHue: 300 }));
  assert.equal(got.accent, "custom");
  assert.equal(got.accentHue, 300);
  // Круг не замыкаем: 400° — это опечатка, а не 40°.
  assert.equal(parseSettings(JSON.stringify({ accentHue: 400 })).accentHue, limits.accentHue.max);
  assert.equal(parseSettings(JSON.stringify({ accentHue: -20 })).accentHue, limits.accentHue.min);
});

test("прозрачность и размытие — проценты", () => {
  const got = parseSettings(JSON.stringify({ transparency: 55, blur: 80 }));
  assert.equal(got.transparency, 55);
  assert.equal(got.blur, 80);
  assert.equal(parseSettings(JSON.stringify({ transparency: 150 })).transparency, 100);
  assert.equal(parseSettings(JSON.stringify({ blur: -5 })).blur, 0);
});

test("тумблеры принимают только настоящий boolean", () => {
  // "true" строкой и 1 — частые опечатки при правке файла руками.
  const got = parseSettings(JSON.stringify({ dither: "true", lineNumbers: 1, lineWrap: false }));
  assert.equal(got.dither, defaultSettings.dither);
  assert.equal(got.lineNumbers, defaultSettings.lineNumbers);
  assert.equal(got.lineWrap, false);
});

test("настройки редактора зажимаются", () => {
  assert.equal(parseSettings(JSON.stringify({ fontSize: 3 })).fontSize, limits.fontSize.min);
  assert.equal(parseSettings(JSON.stringify({ fontSize: 200 })).fontSize, limits.fontSize.max);
  assert.equal(parseSettings(JSON.stringify({ saveDelay: 1 })).saveDelay, limits.saveDelay.min);
  assert.equal(parseSettings(JSON.stringify({ saveDelay: 999999 })).saveDelay, limits.saveDelay.max);
});

test("дробный интерлиньяж притягивается к шагу", () => {
  // Ползунок легко даёт 1.7000000000000002 — в файле такому не место.
  const got = parseSettings(JSON.stringify({ lineHeight: 1.7000000000000002 }));
  assert.equal(got.lineHeight, 1.7);
  assert.equal(parseSettings(JSON.stringify({ lineHeight: 1.63 })).lineHeight, 1.65);
});

test("окно автокоммита не выходит за потолок Go", () => {
  // Потолок совпадает с maxCommitWindow в internal/app/git.go: иначе интерфейс
  // предложит значение, на котором Go вернёт ошибку.
  assert.equal(parseSettings(JSON.stringify({ commitWindow: 99999 })).commitWindow, 1800);
  assert.equal(parseSettings(JSON.stringify({ commitWindow: -10 })).commitWindow, 0);
  assert.equal(parseSettings(JSON.stringify({ commitWindow: 90 })).commitWindow, 90);
});

test("умолчания сами проходят разбор без изменений", () => {
  // Иначе первое же сохранение перепишет файл другими значениями.
  assert.deepEqual(parseSettings(JSON.stringify(defaultSettings)), defaultSettings);
});

test("каждое числовое умолчание лежит внутри своих границ", () => {
  for (const [key, bound] of Object.entries(limits)) {
    const value = defaultSettings[key as keyof typeof limits];
    assert.ok(
      value >= bound.min && value <= bound.max,
      `${key} = ${value} вне [${bound.min}, ${bound.max}]`,
    );
  }
});

test("масштаб растёт и падает шагом настройки", () => {
  assert.equal(nextZoom(1, "view.zoom.in"), 1.1);
  assert.equal(nextZoom(1, "view.zoom.out"), 0.9);
});

test("масштаб упирается в те же границы, что и ползунок", () => {
  // Иначе клавиатура доедет туда, куда ползунок не пускает, и значение
  // молча зажмётся при записи — «настройка не сохранилась».
  assert.equal(nextZoom(limits.textScale.max, "view.zoom.in"), limits.textScale.max);
  assert.equal(nextZoom(limits.textScale.min, "view.zoom.out"), limits.textScale.min);
});

test("сброс возвращает исходный размер", () => {
  assert.equal(nextZoom(1.7, "view.zoom.reset"), defaultSettings.textScale);
  assert.equal(nextZoom(0.7, "view.zoom.reset"), defaultSettings.textScale);
});

test("дробный шаг не тащит хвост в файл", () => {
  // 1.1 + 0.1 в двоичной арифметике даёт 1.2000000000000002.
  let zoom = 1;
  for (let i = 0; i < 5; i++) zoom = nextZoom(zoom, "view.zoom.in");
  assert.equal(zoom, 1.5);
  for (let i = 0; i < 5; i++) zoom = nextZoom(zoom, "view.zoom.out");
  assert.equal(zoom, 1);
});

test("масштаб текста зажимается границами", () => {
  // Ниже 0.8 метаполосы нечитаемы, выше 1.6 в колонку по умолчанию не
  // влезает ни строчки.
  assert.equal(parseSettings(JSON.stringify({ textScale: 0.1 })).textScale, limits.textScale.min);
  assert.equal(parseSettings(JSON.stringify({ textScale: 99 })).textScale, limits.textScale.max);
  assert.equal(parseSettings(JSON.stringify({ textScale: 1.2 })).textScale, 1.2);
  // Дробь от арифметики шага не должна попадать в файл как есть.
  assert.equal(parseSettings(JSON.stringify({ textScale: 1.1000000000000003 })).textScale, 1.1);
});

test("свёрнутый сайдбар запоминается", () => {
  assert.equal(parseSettings(JSON.stringify({ sidebarHidden: true })).sidebarHidden, true);
  assert.equal(parseSettings(JSON.stringify({ sidebarHidden: "да" })).sidebarHidden, false);
});
