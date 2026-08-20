package notes

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func testLock(t *testing.T) *vaultLock {
	t.Helper()
	dir := t.TempDir()
	return newVaultLock(dir)
}

func TestLockExcludesWithinProcess(t *testing.T) {
	l := testLock(t)
	ctx := context.Background()

	release, err := l.acquire(ctx)
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}

	taken := make(chan struct{})
	go func() {
		r2, err := l.acquire(ctx)
		if err != nil {
			t.Errorf("второй acquire: %v", err)
			close(taken)
			return
		}
		r2()
		close(taken)
	}()

	select {
	case <-taken:
		t.Fatal("вторая блокировка взялась, пока первая держится")
	case <-time.After(60 * time.Millisecond):
	}

	release()
	select {
	case <-taken:
	case <-time.After(2 * time.Second):
		t.Fatal("вторая блокировка не взялась после освобождения")
	}
}

// holdLockEnv переключает тестовый бинарник в режим «держать блокировку». Так
// получается настоящий второй процесс без внешних утилит: flock(1) на macOS
// нет, а именно межпроцессное исключение здесь и проверяется.
const holdLockEnv = "TASKER_TEST_HOLD_LOCK"

func TestMain(m *testing.M) {
	if path := os.Getenv(holdLockEnv); path != "" {
		l := &vaultLock{path: path}
		release, err := l.acquire(context.Background())
		if err != nil {
			os.Exit(1)
		}
		fmt.Println("held")
		time.Sleep(3 * time.Second)
		release()
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestLockExcludesOtherProcess(t *testing.T) {
	dir := t.TempDir()
	lockPath := filepath.Join(dir, lockFileName)

	holder := exec.Command(os.Args[0], "-test.run=TestLockExcludesOtherProcess")
	holder.Env = append(os.Environ(), holdLockEnv+"="+lockPath)
	stdout, err := holder.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := holder.Start(); err != nil {
		t.Fatalf("не удалось запустить второй процесс: %v", err)
	}
	defer func() {
		holder.Process.Kill()
		holder.Wait()
	}()

	// Ждём подтверждения, что блокировка действительно взята.
	ready := make(chan struct{})
	go func() {
		buf := make([]byte, 4)
		if _, err := io.ReadFull(stdout, buf); err == nil && string(buf) == "held" {
			close(ready)
		}
	}()
	select {
	case <-ready:
	case <-time.After(10 * time.Second):
		t.Fatal("второй процесс не взял блокировку")
	}

	l := newVaultLock(dir)
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	if _, err := l.acquire(ctx); err == nil {
		t.Error("блокировка взялась, хотя её держит другой процесс")
	}
}

// Ожидание должно прерываться отменой контекста, а не висеть навсегда.
func TestLockRespectsContext(t *testing.T) {
	l := testLock(t)

	release, err := l.acquire(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer release()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	if _, err := l.acquire(ctx); err == nil {
		t.Fatal("acquire не вернул ошибку по истечении контекста")
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("ожидание длилось %v — контекст не соблюдён", elapsed)
	}
}

// Блокировка одного vault не мешает другому.
func TestLockIsPerVault(t *testing.T) {
	first := newVaultLock(t.TempDir())
	second := newVaultLock(t.TempDir())
	ctx := context.Background()

	r1, err := first.acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer r1()

	done := make(chan struct{})
	go func() {
		defer close(done)
		r2, err := second.acquire(ctx)
		if err != nil {
			t.Errorf("второй vault: %v", err)
			return
		}
		r2()
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Error("блокировка одного vault держит другой")
	}
}

// Последовательные операции не должны накапливать дескрипторы.
func TestLockReleasesDescriptors(t *testing.T) {
	l := testLock(t)
	ctx := context.Background()

	var wg sync.WaitGroup
	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			release, err := l.acquire(ctx)
			if err != nil {
				t.Errorf("acquire: %v", err)
				return
			}
			release()
		}()
	}
	wg.Wait()

	if _, err := os.Stat(l.path); err != nil {
		t.Errorf("файл блокировки пропал: %v", err)
	}
}
