package app

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"tasker/internal/vault"
)

// serveVault поднимает раздатчик поверх временного хранилища.
func serveVault(t *testing.T) (http.Handler, string) {
	t.Helper()
	root := t.TempDir()
	v, err := vault.Open(root)
	if err != nil {
		t.Fatalf("vault.Open: %v", err)
	}
	// Заглушка вместо фронтенда: до неё доходит всё, что не про хранилище.
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})
	return VaultAssets(v)(next), root
}

func write(t *testing.T, root, rel, body string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func get(t *testing.T, h http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

func TestVaultAssetsServesAttachment(t *testing.T) {
	h, root := serveVault(t)
	write(t, root, "attachments/2026/08/ABCDEFGH.png", "картинка")

	rec := get(t, h, "/vault/attachments/2026/08/ABCDEFGH.png")
	if rec.Code != http.StatusOK {
		t.Fatalf("код %d", rec.Code)
	}
	if rec.Body.String() != "картинка" {
		t.Errorf("тело %q", rec.Body.String())
	}
}

// Пути с кириллицей и пробелами приезжают закодированными: ноутбуки называют
// по-русски, и без раскодирования файл просто не нашёлся бы.
func TestVaultAssetsDecodesPath(t *testing.T) {
	h, root := serveVault(t)
	write(t, root, "Работа/моя схема.png", "схема")

	rec := get(t, h, "/vault/%D0%A0%D0%B0%D0%B1%D0%BE%D1%82%D0%B0/%D0%BC%D0%BE%D1%8F%20%D1%81%D1%85%D0%B5%D0%BC%D0%B0.png")
	if rec.Code != http.StatusOK {
		t.Fatalf("код %d, тело %q", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != "схема" {
		t.Errorf("тело %q", rec.Body.String())
	}
}

// Всё, что не про хранилище, уходит дальше по цепочке — к фронтенду.
func TestVaultAssetsPassesEverythingElse(t *testing.T) {
	h, _ := serveVault(t)
	if rec := get(t, h, "/index.html"); rec.Code != http.StatusTeapot {
		t.Errorf("код %d, ожидалось, что запрос уйдёт дальше", rec.Code)
	}
}

// Путь приходит из markdown, то есть из файла, который правят чем угодно.
func TestVaultAssetsRefusesToLeaveVault(t *testing.T) {
	h, root := serveVault(t)
	// Файл рядом с хранилищем, но снаружи.
	outside := filepath.Join(filepath.Dir(root), "секрет.txt")
	if err := os.WriteFile(outside, []byte("нельзя"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Remove(outside) })

	for _, path := range []string{
		"/vault/../секрет.txt",
		"/vault/attachments/../../секрет.txt",
		"/vault/%2e%2e/секрет.txt",
	} {
		rec := get(t, h, path)
		if rec.Code == http.StatusOK && rec.Body.String() == "нельзя" {
			t.Errorf("%s отдал файл снаружи хранилища", path)
		}
	}
}

// Скрытые каталоги — это .git и .tasker: содержимое индекса и историю вебвью
// показывать незачем.
func TestVaultAssetsRefusesHiddenPaths(t *testing.T) {
	h, root := serveVault(t)
	write(t, root, ".tasker/config.json", "{}")

	if rec := get(t, h, "/vault/.tasker/config.json"); rec.Code == http.StatusOK {
		t.Errorf("отдал файл из скрытого каталога: %q", rec.Body.String())
	}
}
