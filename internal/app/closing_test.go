package app

import (
	"sync/atomic"
	"testing"
	"time"
)

func TestClosingWaitsForInterface(t *testing.T) {
	var asked atomic.Bool
	closing := NewClosing(func() { asked.Store(true) }, time.Second)

	go func() {
		// Интерфейс отвечает не мгновенно: ему надо записать файл.
		time.Sleep(30 * time.Millisecond)
		closing.Ready()
	}()

	start := time.Now()
	if !closing.Wait() {
		t.Fatal("не дождались ответа, хотя он был")
	}
	if !asked.Load() {
		t.Error("интерфейс не попросили сохраниться")
	}
	if elapsed := time.Since(start); elapsed > 500*time.Millisecond {
		t.Errorf("ждали %v — ответ не забрали сразу", elapsed)
	}
}

// Молчащий интерфейс не должен подвешивать окно: закрываемся по таймауту.
func TestClosingGivesUpOnTimeout(t *testing.T) {
	closing := NewClosing(func() {}, 60*time.Millisecond)

	start := time.Now()
	if closing.Wait() {
		t.Error("Wait сообщил об ответе, которого не было")
	}
	elapsed := time.Since(start)
	if elapsed < 50*time.Millisecond {
		t.Errorf("сдались через %v — таймаут не выдержан", elapsed)
	}
	if elapsed > time.Second {
		t.Errorf("ждали %v — слишком долго для закрывающегося окна", elapsed)
	}
}

// Опоздавший ответ не должен засчитаться следующей попытке закрыться.
func TestClosingDiscardsStaleAnswer(t *testing.T) {
	closing := NewClosing(func() {}, 40*time.Millisecond)

	if closing.Wait() {
		t.Fatal("первая попытка не должна была получить ответ")
	}
	// Ответ приходит с опозданием, уже после таймаута.
	closing.Ready()

	if closing.Wait() {
		t.Error("вторая попытка засчитала прошлогодний ответ")
	}
}

// Ready до Wait не должен теряться: интерфейс мог успеть раньше.
func TestClosingReadyBeforeWait(t *testing.T) {
	closing := NewClosing(func() {}, time.Second)
	closing.Ready()
	// Wait выбрасывает прошлые ответы, поэтому этот не считается — и это
	// правильно: он относился к прошлому закрытию, а не к текущему.
	if closing.Wait() {
		t.Error("ответ, пришедший до просьбы, засчитан")
	}
}

func TestClosingNeverBlocksOnReady(t *testing.T) {
	closing := NewClosing(func() {}, time.Second)
	done := make(chan struct{})
	go func() {
		for i := 0; i < 100; i++ {
			closing.Ready()
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Ready заблокировался")
	}
}
