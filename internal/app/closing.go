package app

import (
	"sync"
	"time"
)

// EventBeforeClose просит интерфейс дописать буфер на диск: окно закрывается.
const EventBeforeClose = "tasker:before-close"

// defaultCloseTimeout — сколько ждать ответа от интерфейса.
//
// Полсекунды с запасом хватает на запись файла и коммит. Больше ждать нельзя:
// человек нажал закрыть, и окно, висящее секундами, читается как зависшее.
const defaultCloseTimeout = 500 * time.Millisecond

// Closing — сервис Wails, через который интерфейс сообщает, что буфер записан.
//
// Нужен, потому что несохранённое живёт в вебвью, и Go его оттуда не достанет:
// единственный способ — попросить и дождаться (SPEC §6, хук WindowClosing).
type Closing struct {
	request func()
	timeout time.Duration

	mu    sync.Mutex
	ready chan struct{}
}

// NewClosing создаёт координатор. request просит интерфейс сохраниться.
func NewClosing(request func(), timeout time.Duration) *Closing {
	if timeout <= 0 {
		timeout = defaultCloseTimeout
	}
	return &Closing{request: request, timeout: timeout, ready: make(chan struct{}, 1)}
}

// Ready вызывается из интерфейса, когда буфер записан.
//
// Экспортируется в биндинги — это единственный способ для вебвью ответить.
func (c *Closing) Ready() {
	c.mu.Lock()
	defer c.mu.Unlock()
	// Неблокирующе: ответ может прийти и после того, как истёк таймаут, и
	// подвешивать на этом вебвью незачем.
	select {
	case c.ready <- struct{}{}:
	default:
	}
}

// Wait просит интерфейс сохраниться и ждёт ответа, но не дольше таймаута.
//
// Возвращает false, если ответа не дождались: тогда закрываемся всё равно.
// Потерять при этом можно только правки последних миллисекунд, а зависшее окно
// пользователь видит сразу.
func (c *Closing) Wait() bool {
	c.drain()
	c.request()

	select {
	case <-c.ready:
		return true
	case <-time.After(c.timeout):
		return false
	}
}

// drain выбрасывает ответ, оставшийся от прошлой попытки закрыться.
func (c *Closing) drain() {
	c.mu.Lock()
	defer c.mu.Unlock()
	select {
	case <-c.ready:
	default:
	}
}
