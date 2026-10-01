package host_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/rigelstpierre/herdr-devin-board/internal/host"
)

type recorder struct {
	calls   [][]string
	outputs [][]byte
	failOn  string
}

func (r *recorder) run(name string, args ...string) ([]byte, error) {
	r.calls = append(r.calls, append([]string{name}, args...))
	if r.failOn != "" && strings.HasPrefix(strings.Join(args, " "), r.failOn) {
		return nil, errors.New("exit status 1")
	}
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

func TestOpenURLRejectsNonHTTPTargets(t *testing.T) {
	for _, target := range []string{"file:///etc/passwd", "/Applications/Calculator.app", "-a Terminal"} {
		r := &recorder{}

		if err := host.New(r.run, "darwin", "", "").OpenURL(target); err == nil {
			t.Errorf("%q: want error", target)
		}
		if len(r.calls) != 0 {
			t.Errorf("%q: ran %v", target, r.calls)
		}
	}
}

const tabCreated = `{"result":{"tab":{"tab_id":"w1:t9"},"root_pane":{"pane_id":"w1:p9"}}}`

func TestAttachCreatesANamedTabAndResumesTheCloudSession(t *testing.T) {
	r := &recorder{outputs: [][]byte{[]byte(tabCreated)}}
	h := host.New(r.run, "darwin", "w1:p2", "herdr").InWorkspace("w1")

	if err := h.Attach("25f09dbe", "Fix IR-7159"); err != nil {
		t.Fatal(err)
	}

	want := [][]string{
		{"herdr", "tab", "create", "--label", "Devin · Fix IR-7159", "--focus", "--workspace", "w1"},
		{"herdr", "pane", "run", "w1:p9", "devin --cloud --resume 25f09dbe"},
	}
	if !reflect.DeepEqual(r.calls, want) {
		t.Fatalf("calls %v", r.calls)
	}
}

func TestAttachAgainFocusesTheExistingTab(t *testing.T) {
	r := &recorder{outputs: [][]byte{[]byte(tabCreated)}}
	h := host.New(r.run, "darwin", "", "herdr")
	if err := h.Attach("abc", "Title"); err != nil {
		t.Fatal(err)
	}
	r.calls = nil

	if err := h.Attach("abc", "Title"); err != nil {
		t.Fatal(err)
	}

	want := [][]string{{"herdr", "tab", "get", "w1:t9"}, {"herdr", "tab", "focus", "w1:t9"}}
	if !reflect.DeepEqual(r.calls, want) {
		t.Fatalf("calls %v", r.calls)
	}
}

func TestAttachReopensWhenTheTabWasClosed(t *testing.T) {
	r := &recorder{outputs: [][]byte{[]byte(tabCreated)}}
	h := host.New(r.run, "darwin", "", "herdr")
	if err := h.Attach("abc", "Title"); err != nil {
		t.Fatal(err)
	}
	r.calls, r.failOn = nil, "tab get"
	r.outputs = [][]byte{[]byte(`{"result":{"tab":{"tab_id":"w1:tA"},"root_pane":{"pane_id":"w1:pA"}}}`)}

	if err := h.Attach("abc", "Title"); err != nil {
		t.Fatal(err)
	}

	if len(r.calls) != 3 || r.calls[1][2] != "create" || r.calls[2][3] != "w1:pA" {
		t.Fatalf("calls %v", r.calls)
	}
}

func TestAttachTruncatesLongTitles(t *testing.T) {
	r := &recorder{outputs: [][]byte{[]byte(tabCreated)}}

	if err := host.New(r.run, "darwin", "", "herdr").Attach("abc", strings.Repeat("x", 80)); err != nil {
		t.Fatal(err)
	}

	if label := r.calls[0][4]; len([]rune(label)) > 32 || !strings.HasSuffix(label, "…") {
		t.Fatalf("label %q", label)
	}
}

func TestAttachRejectsUnsafeSessionID(t *testing.T) {
	r := &recorder{}

	if err := host.New(r.run, "darwin", "", "").Attach("abc; rm -rf ~", "t"); err == nil {
		t.Fatal("want error")
	}
	if len(r.calls) != 0 {
		t.Fatalf("ran %v", r.calls)
	}
}

func TestAttachOpensTheTabInTheWorkspaceDirectory(t *testing.T) {
	r := &recorder{outputs: [][]byte{[]byte(tabCreated)}}
	h := host.New(r.run, "darwin", "", "herdr").InWorkspace("w1").InDirectory("/Users/me/repo")

	if err := h.Attach("abc", "T"); err != nil {
		t.Fatal(err)
	}

	want := []string{"herdr", "tab", "create", "--label", "Devin · T", "--focus", "--workspace", "w1", "--cwd", "/Users/me/repo"}
	if !reflect.DeepEqual(r.calls[0], want) {
		t.Fatalf("create %v", r.calls[0])
	}
}
