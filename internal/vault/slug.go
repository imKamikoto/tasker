package vault

import "strings"

// maxSlugLen — предел длины имени файла из SPEC §4.1.
const maxSlugLen = 60

// translit — транслитерация кириллицы для имён файлов.
//
// Своя таблица вместо зависимости: правило нужно ровно одно, а любая библиотека
// принесёт с собой ещё десяток языков и своё мнение о спорных буквах. Спорные
// здесь решены так: ё как е, х как h, ц как c, щ как sch, твёрдый и мягкий
// знаки выбрасываются. Менять это после того, как в vault появятся файлы,
// нельзя — имена уже созданных заметок не переименовываются (SPEC §4.1).
var translit = map[rune]string{
	'а': "a", 'б': "b", 'в': "v", 'г': "g", 'д': "d", 'е': "e", 'ё': "e",
	'ж': "zh", 'з': "z", 'и': "i", 'й': "y", 'к': "k", 'л': "l", 'м': "m",
	'н': "n", 'о': "o", 'п': "p", 'р': "r", 'с': "s", 'т': "t", 'у': "u",
	'ф': "f", 'х': "h", 'ц': "c", 'ч': "ch", 'ш': "sh", 'щ': "sch",
	'ъ': "", 'ы': "y", 'ь': "", 'э': "e", 'ю': "yu", 'я': "ya",
}

// Slug строит имя файла из заголовка заметки: транслит кириллицы, нижний
// регистр, дефисы вместо всего остального, не длиннее maxSlugLen.
//
// Пустой результат — валидный ответ: заголовок может целиком состоять из эмодзи
// или иероглифов. Что делать в этом случае, решает вызывающий, потому что здесь
// нет доступа к ULID заметки.
func Slug(title string) string {
	var b strings.Builder
	b.Grow(len(title))

	for _, r := range strings.ToLower(title) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			if s, ok := translit[r]; ok {
				b.WriteString(s)
				continue
			}
			b.WriteByte('-')
		}
	}

	return tidySlug(b.String())
}

// tidySlug схлопывает дефисы, обрезает края и укорачивает до maxSlugLen.
func tidySlug(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	prevDash := false
	for i := 0; i < len(s); i++ {
		if s[i] == '-' {
			if prevDash {
				continue
			}
			prevDash = true
		} else {
			prevDash = false
		}
		b.WriteByte(s[i])
	}

	out := strings.Trim(b.String(), "-")
	if len(out) <= maxSlugLen {
		return out
	}

	// Режем по границе слова: обрубок в середине слова читается плохо, а имя
	// файла человек видит в git-диффах и в Finder.
	out = out[:maxSlugLen]
	if i := strings.LastIndexByte(out, '-'); i > 0 {
		out = out[:i]
	}
	return strings.Trim(out, "-")
}
