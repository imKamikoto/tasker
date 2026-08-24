package app

import (
	"net/http"
	"net/url"
	"strings"

	"tasker/internal/vault"
)

// VaultPrefix — по этому пути вебвью просит файлы из хранилища.
//
// Отдельный префикс, а не корень: собранный фронтенд и содержимое хранилища
// живут в одном адресном пространстве, и без разделителя заметка с именем
// index.html подменила бы приложение.
const VaultPrefix = "/vault/"

// VaultAssets отдаёт вебвью файлы из хранилища — картинки в заметках.
//
// Через раздатчик, а не data-URI: картинка на несколько мегабайт, вшитая в
// текст документа, перерисовывается вместе с ним на каждое нажатие клавиши.
//
// Путь приходит из markdown, то есть из файла, который человек мог поправить
// чем угодно, — поэтому он проверяется тем же правилом, что и всё остальное в
// vault (Inside): не выходить за пределы хранилища и не лезть в скрытые
// каталоги, где лежат .git и служебные файлы.
func VaultAssets(v *vault.Vault) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rel, ok := strings.CutPrefix(r.URL.Path, VaultPrefix)
			if !ok {
				next.ServeHTTP(w, r)
				return
			}

			// Путь приезжает закодированным: в именах ноутбуков кириллица и
			// пробелы, и без раскодирования файл просто не найдётся.
			decoded, err := url.PathUnescape(rel)
			if err != nil {
				http.Error(w, "bad path", http.StatusBadRequest)
				return
			}

			abs, err := v.Inside(decoded)
			if err != nil {
				// Одинаковый ответ на «наружу», «в скрытый каталог» и «нет
				// такого»: разные коды подсказывали бы, что где лежит.
				http.NotFound(w, r)
				return
			}
			http.ServeFile(w, r, abs)
		})
	}
}
