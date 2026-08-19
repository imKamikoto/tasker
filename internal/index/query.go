package index

import (
	"errors"
	"fmt"
	"strings"

	"tasker/internal/vault"
)

// Ошибки разбора запроса.
var (
	// ErrOnlyNegations — в запросе нет ни одного положительного условия.
	// «Всё, кроме черновиков» — это весь vault минус немного, и почти всегда
	// не то, что человек имел в виду (SPEC §8.5).
	ErrOnlyNegations = errors.New("query has only negations")
	// ErrUnknownValue — значение вне закрытого перечисления.
	ErrUnknownValue = errors.New("unknown filter value")
	// ErrEmptyValue — у фильтра нет значения.
	ErrEmptyValue = errors.New("empty filter value")
)

// TermKind — что именно ограничивает условие.
type TermKind int

const (
	// TermText — полнотекстовый поиск. Field сужает его до title или body.
	TermText TermKind = iota
	TermNotebook
	TermTag
	TermStatus
	TermPinned
	TermTask
)

// Term — одно условие запроса.
type Term struct {
	Kind    TermKind
	Field   string // "title" или "body" для TermText, иначе пусто
	Value   string
	Negated bool
}

// Query — разобранный запрос. Условия соединяются через И (SPEC §8.5).
type Query struct {
	Terms []Term
}

// ParseQuery разбирает язык запросов из SPEC §8.5.
//
// Неизвестный префикс считается обычным текстом: двоеточие в поиске по коду или
// по ссылке встречается чаще, чем опечатка в имени фильтра. А вот значения
// закрытых перечислений (status, is, has) проверяются — там опечатку надо
// показать, иначе человек будет смотреть на пустой список и гадать.
func ParseQuery(input string) (Query, error) {
	var q Query

	for _, tok := range tokenize(input) {
		term, err := parseToken(tok)
		if err != nil {
			return Query{}, err
		}
		q.Terms = append(q.Terms, term)
	}

	if len(q.Terms) > 0 && !hasPositive(q.Terms) {
		return Query{}, fmt.Errorf("parse query %q: %w", input, ErrOnlyNegations)
	}
	return q, nil
}

func hasPositive(terms []Term) bool {
	for _, t := range terms {
		if !t.Negated {
			return true
		}
	}
	return false
}

// token — кусок запроса до разбора.
//
// Отрицание снимается здесь, а не при разборе: только тут видно, стоял ли минус
// снаружи кавычек. В -"точная фраза" он отрицание, в "-дефис внутри" — часть
// искомого текста.
type token struct {
	raw     string
	negated bool
}

// tokenize режет ввод по пробелам, не трогая пробелы внутри кавычек.
// Незакрытая кавычка тянется до конца ввода: человек ещё печатает, и ронять
// запрос на каждом промежуточном состоянии нельзя.
func tokenize(input string) []token {
	var (
		out     []token
		cur     strings.Builder
		inQuote bool
		negated bool
		started bool
	)
	flush := func() {
		if started {
			raw := cur.String()
			if negated && raw == "" {
				// Одинокий минус — это искомый текст, а не отрицание пустоты.
				out = append(out, token{raw: "-"})
			} else {
				out = append(out, token{raw: raw, negated: negated})
			}
		}
		cur.Reset()
		negated = false
		started = false
	}

	for _, r := range input {
		switch {
		case r == '"':
			inQuote = !inQuote
			started = true
		case !inQuote && (r == ' ' || r == '\t' || r == '\n'):
			flush()
		// Открывающая кавычка уже помечает токен начатым, поэтому минус после
		// неё сюда не попадает и остаётся частью искомого текста.
		case r == '-' && !started:
			negated = true
			started = true
		default:
			cur.WriteRune(r)
			started = true
		}
	}
	flush()
	return out
}

// prefixes — известные фильтры. Имя префикса регистронезависимо, значение нет.
var prefixes = map[string]TermKind{
	"book":   TermNotebook,
	"tag":    TermTag,
	"status": TermStatus,
	"title":  TermText,
	"body":   TermText,
	"is":     TermPinned,
	"has":    TermTask,
}

func parseToken(tok token) (Term, error) {
	raw, negated := tok.raw, tok.negated

	name, value, found := strings.Cut(raw, ":")
	kind, known := prefixes[strings.ToLower(name)]
	if !found || !known {
		// Обычный текст. Двоеточие внутри остаётся частью искомого.
		return Term{Kind: TermText, Value: raw, Negated: negated}, nil
	}

	if strings.TrimSpace(value) == "" {
		return Term{}, fmt.Errorf("parse %q: %w", tok.raw, ErrEmptyValue)
	}

	switch strings.ToLower(name) {
	case "title", "body":
		return Term{Kind: TermText, Field: strings.ToLower(name), Value: value, Negated: negated}, nil
	case "status":
		if _, err := vault.ParseStatus(value); err != nil {
			return Term{}, fmt.Errorf("parse %q: %w", tok.raw, ErrUnknownValue)
		}
	case "is":
		if !strings.EqualFold(value, "pinned") {
			return Term{}, fmt.Errorf("parse %q: %w", tok.raw, ErrUnknownValue)
		}
	case "has":
		if !strings.EqualFold(value, "task") {
			return Term{}, fmt.Errorf("parse %q: %w", tok.raw, ErrUnknownValue)
		}
	}

	return Term{Kind: kind, Value: value, Negated: negated}, nil
}
