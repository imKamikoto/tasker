package app

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func newVaults(t *testing.T, pick func() (string, error)) *Vaults {
	t.Helper()
	v, err := NewVaults(t.TempDir(), pick)
	if err != nil {
		t.Fatalf("NewVaults: %v", err)
	}
	return v
}

// dirs создаёт нужное число папок и отдаёт их пути.
func dirs(t *testing.T, n int) []string {
	t.Helper()
	base := t.TempDir()
	out := make([]string, n)
	for i := range out {
		out[i] = filepath.Join(base, string(rune('a'+i)))
		if err := os.MkdirAll(out[i], 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return out
}

func TestVaultsEmptyUntilChosen(t *testing.T) {
	v := newVaults(t, nil)
	if got := v.Current(); got != "" {
		t.Errorf("Current = %q, ожидалась пустая строка", got)
	}
	if got := v.Recent(); len(got) != 0 {
		t.Errorf("Recent = %v, ожидался пустой список", got)
	}
}

func TestVaultsSwitchRemembers(t *testing.T) {
	paths := dirs(t, 2)
	v := newVaults(t, nil)

	if err := v.Switch(paths[0]); err != nil {
		t.Fatalf("Switch: %v", err)
	}
	if got := v.Current(); got != paths[0] {
		t.Errorf("Current = %q, ожидалось %q", got, paths[0])
	}

	if err := v.Switch(paths[1]); err != nil {
		t.Fatalf("Switch: %v", err)
	}
	// Прежняя текущая уехала в недавние.
	if got := v.Recent(); !reflect.DeepEqual(got, []string{paths[0]}) {
		t.Errorf("Recent = %v, ожидалось %v", got, []string{paths[0]})
	}
}

func TestVaultsSurvivesRestart(t *testing.T) {
	home := t.TempDir()
	paths := dirs(t, 1)

	first, err := NewVaults(home, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := first.Switch(paths[0]); err != nil {
		t.Fatal(err)
	}

	// Второй экземпляр поверх того же home — то же самое, что новый запуск.
	second, err := NewVaults(home, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := second.Current(); got != paths[0] {
		t.Errorf("после перезапуска Current = %q, ожидалось %q", got, paths[0])
	}
}

func TestVaultsRejectsBadPath(t *testing.T) {
	v := newVaults(t, nil)
	file := filepath.Join(t.TempDir(), "note.md")
	if err := os.WriteFile(file, []byte("тело"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := v.Switch(file); !errors.Is(err, ErrNotDirectory) {
		t.Errorf("на файле Switch вернул %v, ожидался ErrNotDirectory", err)
	}
	if err := v.Switch(filepath.Join(t.TempDir(), "нет-такой")); err == nil {
		t.Error("на несуществующем пути Switch не вернул ошибку")
	}
	if err := v.Switch("   "); !errors.Is(err, ErrNoVaultChosen) {
		t.Errorf("на пустом пути Switch вернул %v, ожидался ErrNoVaultChosen", err)
	}

	// Ни одна неудача не должна была ничего записать.
	if got := v.Current(); got != "" {
		t.Errorf("после неудачных попыток Current = %q", got)
	}
}

func TestVaultsChooseCancelled(t *testing.T) {
	v := newVaults(t, func() (string, error) { return "", nil })
	if _, err := v.Choose(); !errors.Is(err, ErrNoVaultChosen) {
		t.Errorf("Choose при закрытом диалоге вернул %v, ожидался ErrNoVaultChosen", err)
	}
}

func TestVaultsChooseStores(t *testing.T) {
	paths := dirs(t, 1)
	v := newVaults(t, func() (string, error) { return paths[0], nil })

	got, err := v.Choose()
	if err != nil {
		t.Fatalf("Choose: %v", err)
	}
	if got != paths[0] || v.Current() != paths[0] {
		t.Errorf("Choose = %q, Current = %q, ожидалось %q", got, v.Current(), paths[0])
	}
}

func TestVaultsForgetKeepsCurrent(t *testing.T) {
	paths := dirs(t, 2)
	v := newVaults(t, nil)
	if err := v.Switch(paths[0]); err != nil {
		t.Fatal(err)
	}
	if err := v.Switch(paths[1]); err != nil {
		t.Fatal(err)
	}

	// Текущую забыть нельзя — иначе приложению не с чем будет запуститься.
	if err := v.Forget(paths[1]); err != nil {
		t.Fatal(err)
	}
	if v.Current() != paths[1] {
		t.Errorf("текущая забылась: %q", v.Current())
	}

	if err := v.Forget(paths[0]); err != nil {
		t.Fatal(err)
	}
	if got := v.Recent(); len(got) != 0 {
		t.Errorf("Recent = %v, ожидался пустой список", got)
	}
}

func TestVaultsBrokenFileFallsBackToEmpty(t *testing.T) {
	v := newVaults(t, nil)
	if err := os.WriteFile(v.Path(), []byte("{не json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := v.Current(); got != "" {
		t.Errorf("на испорченном файле Current = %q, ожидалась пустая строка", got)
	}

	// И перезаписывается первой же удачной сменой.
	paths := dirs(t, 1)
	if err := v.Switch(paths[0]); err != nil {
		t.Fatal(err)
	}
	if v.Current() != paths[0] {
		t.Errorf("после смены Current = %q", v.Current())
	}
}

func TestPromote(t *testing.T) {
	cases := []struct {
		name   string
		recent []string
		prev   string
		next   string
		want   []string
	}{
		{"первый выбор", nil, "", "/a", nil},
		{"прежняя уходит в начало", []string{"/b"}, "/a", "/c", []string{"/a", "/b"}},
		{"новая не остаётся в недавних", []string{"/c", "/b"}, "/a", "/c", []string{"/a", "/b"}},
		{"повтора не возникает", []string{"/a", "/b"}, "/a", "/c", []string{"/a", "/b"}},
		{"возврат туда же ничего не меняет", []string{"/b"}, "/a", "/a", []string{"/b"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := promote(c.recent, c.prev, c.next)
			if len(got) == 0 && len(c.want) == 0 {
				return
			}
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("promote = %v, ожидалось %v", got, c.want)
			}
		})
	}
}

func TestPromoteHasCeiling(t *testing.T) {
	recent := make([]string, 0, 20)
	for i := 0; i < 20; i++ {
		recent = append(recent, filepath.Join("/vault", string(rune('a'+i))))
	}
	got := promote(recent, "/prev", "/next")
	if len(got) != maxRecent {
		t.Errorf("длина %d, ожидалось %d", len(got), maxRecent)
	}
	if got[0] != "/prev" {
		t.Errorf("первой должна быть прежняя текущая, а не %q", got[0])
	}
}

func TestAppBundle(t *testing.T) {
	cases := []struct {
		binary string
		want   string
		ok     bool
	}{
		{"/Applications/Tasker.app/Contents/MacOS/tasker", "/Applications/Tasker.app", true},
		{"/Users/me/work/tasker/bin/tasker", "", false},
		// Папка с таким именем, но не бандл: расширения .app нет.
		{"/Users/me/Tasker/Contents/MacOS/tasker", "", false},
	}
	for _, c := range cases {
		got, ok := appBundle(c.binary)
		if ok != c.ok || got != c.want {
			t.Errorf("appBundle(%q) = %q, %v; ожидалось %q, %v", c.binary, got, ok, c.want, c.ok)
		}
	}
}
