package main

import (
	"os/exec"
	"strings"
	"testing"
)

func dumpPythonMarkdown(s string) string {
	cmd := exec.Command(
		"python",
		"-c",
		"import markdown as m; print(m.markdown('"+s+"').replace('<p>','').replace('</p>',''))")
	out, err := cmd.Output()
	if err != nil {
		return err.Error()
	}
	return strings.TrimSpace(string(out))
}

func TestMarkdown(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"bold-middle", "A **B C** D", "A \x02B C\x02 D"},
		{"bold-three", "A **B C ** D**", "A \x02B C ** D\x02"},
		{"italic-middle", "A *B C* D", "A \x1dB C\x1d D"},
		{"italic-three", "A *B C * D*", "A \x1dB C * D\x1d"},
	}

	for _, tt := range tests {
		test := tt
		f := func(t *testing.T) {
			got := strings.TrimSpace(markdownToIrc(test.in))
			if got != test.want {
				py := dumpPythonMarkdown(test.in)
				t.Fatalf("got %q, wanted %q (python: %q)", got, test.want, py)
			}
		}
		t.Run(test.name, f)
	}
}
