package notes

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

// lockFileName — файл блокировки в служебном каталоге vault.
const lockFileName = "vault.lock"

// retryInterval — как часто пробуем взять занятую блокировку. Операции короткие
// (запись файла, коммит), так что ожидание измеряется миллисекундами.
const retryInterval = 5 * time.Millisecond

// vaultLock — межпроцессная блокировка записи в vault.
//
// Она нужна не для SQLite: там WAL и busy_timeout. Она нужна для git —
// одновременный коммит из приложения и из tasker-mcp упирается в index.lock и
// падает, — и для операций «прочитать, изменить, записать», которые иначе
// перемежаются между процессами.
//
// Под ней flock: он снимается сам, когда процесс умирает, поэтому упавшее
// приложение не оставляет vault заблокированным навсегда. Это главная причина
// не делать блокировку через файл-маркер.
type vaultLock struct {
	path string
}

func newVaultLock(configDir string) *vaultLock {
	return &vaultLock{path: filepath.Join(configDir, lockFileName)}
}

// acquire берёт блокировку и возвращает функцию освобождения.
func (l *vaultLock) acquire(ctx context.Context) (func(), error) {
	f, err := os.OpenFile(l.path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, fmt.Errorf("lock vault: %w", err)
	}

	for {
		err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			// Закрытие дескриптора само снимает flock — отдельный LOCK_UN
			// ничего не добавляет.
			return func() { f.Close() }, nil
		}
		if err != syscall.EWOULDBLOCK {
			f.Close()
			return nil, fmt.Errorf("lock vault: %w", err)
		}

		// Занято другим процессом. Ждём, поглядывая на контекст: блокирующий
		// flock его не умеет, а бросать вызывающего без возможности отмениться
		// нельзя.
		select {
		case <-ctx.Done():
			f.Close()
			return nil, fmt.Errorf("lock vault: %w", ctx.Err())
		case <-time.After(retryInterval):
		}
	}
}
