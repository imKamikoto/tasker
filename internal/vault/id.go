package vault

import (
	"strings"

	"github.com/oklog/ulid/v2"
)

// idLen — длина ULID в Crockford base32.
const idLen = 26

// NewID возвращает новый ULID заметки.
//
// ulid.Make берёт монотонный источник энтропии под мьютексом, поэтому его можно
// звать из нескольких горутин: заметки создают и приложение, и tasker-mcp.
func NewID() string {
	return ulid.Make().String()
}

// ValidID проверяет, что строка — корректный ULID в каноническом виде.
//
// Регистр важен: id попадает в имена ссылок вида tasker://note/<id>, и два
// написания одного идентификатора сломали бы их сравнение.
func ValidID(s string) bool {
	if len(s) != idLen || s != strings.ToUpper(s) {
		return false
	}
	_, err := ulid.ParseStrict(s)
	return err == nil
}
