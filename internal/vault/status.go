package vault

import (
	"errors"
	"fmt"
)

// ErrInvalidStatus и ErrInvalidOrigin возвращаются, когда в frontmatter лежит
// значение вне перечисления. Файл правится руками, поэтому такое бывает, и
// вызывающий должен уметь отличить это от ошибки чтения.
var (
	ErrInvalidStatus = errors.New("invalid status")
	ErrInvalidOrigin = errors.New("invalid origin")
)

// Status — состояние заметки-задачи (SPEC §8.3).
type Status string

const (
	StatusNone      Status = "none"
	StatusActive    Status = "active"
	StatusOnHold    Status = "onHold"
	StatusCompleted Status = "completed"
	StatusDropped   Status = "dropped"
)

// ParseStatus разбирает значение поля status.
//
// Пустая строка — валидный ввод: отсутствие поля эквивалентно none (SPEC §4.2).
// Регистр значим, потому что в файле статус пишется ровно так, как здесь: если
// принимать "onhold", в vault заведутся два написания одного и того же.
func ParseStatus(s string) (Status, error) {
	switch Status(s) {
	case "", StatusNone:
		return StatusNone, nil
	case StatusActive, StatusOnHold, StatusCompleted, StatusDropped:
		return Status(s), nil
	default:
		return "", fmt.Errorf("parse status %q: %w", s, ErrInvalidStatus)
	}
}

// Origin — кто завёл заметку. Агент помечает свои, чтобы в приложении было
// видно, что заметка появилась не от руки (SPEC §4.2).
type Origin string

const (
	OriginUser  Origin = "user"
	OriginAgent Origin = "agent"
)

// ParseOrigin разбирает значение поля origin. Пустая строка — это user.
func ParseOrigin(s string) (Origin, error) {
	switch Origin(s) {
	case "", OriginUser:
		return OriginUser, nil
	case OriginAgent:
		return OriginAgent, nil
	default:
		return "", fmt.Errorf("parse origin %q: %w", s, ErrInvalidOrigin)
	}
}

// Context — откуда пришла заметка: репозиторий, ветка, коммит, файл.
// Заполняет агент через MCP, приложение только показывает (docs/MCP.md §3).
type Context struct {
	Repo   string `yaml:"repo,omitempty"`
	Branch string `yaml:"branch,omitempty"`
	Commit string `yaml:"commit,omitempty"`
	File   string `yaml:"file,omitempty"`
}
