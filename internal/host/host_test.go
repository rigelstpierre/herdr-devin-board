package host_test

import (
	"reflect"
	"testing"

	"github.com/rigelstpierre/herdr-devin-board/internal/host"
)

type recorder struct {
	calls   [][]string
	outputs [][]byte
}

func (r *recorder) run(name string, args ...string) ([]byte, error) {
	r.calls = append(r.calls, append([]string{name}, args...))
	if len(r.outputs) == 0 {
		return nil, nil
	}
	out := r.outputs[0]
	r.outputs = r.outputs[1:]
	return out, nil
}

func TestOpenURLUsesOpenOnMac(t *testing.T) {
	r := &recorder{}

	if err := host.New(r.run, "darwin", "", "").OpenURL("https://example.test"); err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(r.calls, [][]string{{"open", "https://example.test"}}) {
		t.Fatalf("calls %v", r.calls)
	}
}

func TestOpenURLUsesXdgOpenOnLinux(t *testing.T) {
	r := &recorder{}

	if err := host.New(r.run, "linux", "", "").OpenURL("https://example.test"); err != nil {
		t.Fatal(err)
	}

	if r.calls[0][0] != "xdg-open" {
		t.Fatalf("calls %v", r.calls)
	}
}

func TestSSHSplitsBesideBoardAndRunsDevinSSH(t *testing.T) {
	r := &recorder{outputs: [][]byte{[]byte(`{"result":{"pane":{"pane_id":"w1:p9"}}}`)}}

	if err := host.New(r.run, "darwin", "w1:p2", "/opt/herdr").SSH("4f1232c9d91c4214"); err != nil {
		t.Fatal(err)
	}

	want := [][]string{
		{"/opt/herdr", "pane", "split", "--direction", "right", "--pane", "w1:p2"},
		{"/opt/herdr", "pane", "run", "w1:p9", "devin ssh 4f1232c9d91c4214"},
	}
	if !reflect.DeepEqual(r.calls, want) {
		t.Fatalf("calls %v", r.calls)
	}
}

func TestSSHFallsBackToCurrentPaneAndHerdrOnPath(t *testing.T) {
	r := &recorder{outputs: [][]byte{[]byte(`{"result":{"pane":{"pane_id":"p1"}}}`)}}

	if err := host.New(r.run, "darwin", "", "").SSH("abc"); err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(r.calls[0], []string{"herdr", "pane", "split", "--direction", "right", "--current"}) {
		t.Fatalf("calls %v", r.calls)
	}
}

func TestSSHRejectsUnsafeSessionID(t *testing.T) {
	r := &recorder{}

	if err := host.New(r.run, "darwin", "", "").SSH("abc; rm -rf ~"); err == nil {
		t.Fatal("want error")
	}
	if len(r.calls) != 0 {
		t.Fatalf("ran %v", r.calls)
	}
}

func TestSSHErrorsWhenSplitReturnsNoPane(t *testing.T) {
	r := &recorder{outputs: [][]byte{[]byte(`{"result":{}}`)}}

	if err := host.New(r.run, "darwin", "", "").SSH("abc"); err == nil {
		t.Fatal("want error")
	}
}
