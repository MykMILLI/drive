package fbhttp

import "testing"

func TestProjectSlugRe(t *testing.T) {
	valid := []string{"1552-migrate-tenants", "a", "q4-report", "0"}
	for _, s := range valid {
		if !projectSlugRe.MatchString(s) {
			t.Errorf("expected %q to be a valid project id", s)
		}
	}

	invalid := []string{"", "-lead", "UPPER", "has space", "../etc", "a/b", "dot.name", ".hidden"}
	for _, s := range invalid {
		if projectSlugRe.MatchString(s) {
			t.Errorf("expected %q to be rejected", s)
		}
	}

	long := make([]byte, 65)
	for i := range long {
		long[i] = 'a'
	}
	if projectSlugRe.MatchString(string(long)) {
		t.Error("expected 65-character id to be rejected")
	}
}

func TestProjectPath(t *testing.T) {
	if got, want := projectPath("1552-x"), "/Projects/1552-x/.project.json"; got != want {
		t.Fatalf("projectPath = %q, want %q", got, want)
	}
}
