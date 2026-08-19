package vault

import (
	"errors"
	"testing"
)

func TestParseStatus(t *testing.T) {
	cases := []struct {
		in      string
		want    Status
		wantErr bool
	}{
		{"", StatusNone, false},
		{"none", StatusNone, false},
		{"active", StatusActive, false},
		{"onHold", StatusOnHold, false},
		{"completed", StatusCompleted, false},
		{"dropped", StatusDropped, false},
		{"onhold", "", true}, // регистр значим: в файле пишется ровно onHold
		{"Active", "", true},
		{"почтиГотово", "", true},
	}
	for _, c := range cases {
		got, err := ParseStatus(c.in)
		if c.wantErr {
			if !errors.Is(err, ErrInvalidStatus) {
				t.Errorf("ParseStatus(%q): ожидалась ErrInvalidStatus, получено %v", c.in, err)
			}
			continue
		}
		if err != nil || got != c.want {
			t.Errorf("ParseStatus(%q) = %q, %v; ожидалось %q", c.in, got, err, c.want)
		}
	}
}

func TestParseOrigin(t *testing.T) {
	cases := []struct {
		in      string
		want    Origin
		wantErr bool
	}{
		{"", OriginUser, false},
		{"user", OriginUser, false},
		{"agent", OriginAgent, false},
		{"Agent", "", true},
		{"робот", "", true},
	}
	for _, c := range cases {
		got, err := ParseOrigin(c.in)
		if c.wantErr {
			if !errors.Is(err, ErrInvalidOrigin) {
				t.Errorf("ParseOrigin(%q): ожидалась ErrInvalidOrigin, получено %v", c.in, err)
			}
			continue
		}
		if err != nil || got != c.want {
			t.Errorf("ParseOrigin(%q) = %q, %v; ожидалось %q", c.in, got, err, c.want)
		}
	}
}
