package source

import (
	"context"
	"strings"
	"testing"
	"time"
)

// --- summarizeDiff unit tests ---

func TestSummarizeDiff_Empty(t *testing.T) {
	got := summarizeDiff("")
	if got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

func TestSummarizeDiff_WhitespaceOnly(t *testing.T) {
	got := summarizeDiff("   \n\t\n")
	if got != "" {
		t.Errorf("expected empty for whitespace-only diff, got %q", got)
	}
}

func TestSummarizeDiff_SingleFileAddition(t *testing.T) {
	diff := `diff --git a/foo.go b/foo.go
--- a/foo.go
+++ b/foo.go
@@ -0,0 +1,5 @@
+package main
+
+func Hello() string {
+	return "hello"
+}
`
	got := summarizeDiff(diff)
	if !strings.Contains(got, "foo.go") {
		t.Errorf("expected foo.go in summary, got %q", got)
	}
	// Should mention added lines or "신규 추가"
	if !strings.Contains(got, "+") && !strings.Contains(got, "신규") {
		t.Errorf("expected addition indicator in summary, got %q", got)
	}
	// Must NOT contain actual code content
	if strings.Contains(got, `return "hello"`) {
		t.Errorf("summary must not contain source code content, got %q", got)
	}
}

func TestSummarizeDiff_FunctionIdentifierExtracted(t *testing.T) {
	diff := `diff --git a/bar.go b/bar.go
--- a/bar.go
+++ b/bar.go
@@ -1,3 +1,6 @@
 package bar
+
+func NewBar() *Bar {
+	return &Bar{}
+}
`
	got := summarizeDiff(diff)
	if !strings.Contains(got, "NewBar") {
		t.Errorf("expected function name NewBar in summary, got %q", got)
	}
}

func TestSummarizeDiff_RemovalDetected(t *testing.T) {
	diff := `diff --git a/old.go b/old.go
--- a/old.go
+++ b/old.go
@@ -1,5 +1,0 @@
-package old
-
-func Dead() {}
-
-var x = 1
`
	got := summarizeDiff(diff)
	if !strings.Contains(got, "old.go") {
		t.Errorf("expected old.go in summary, got %q", got)
	}
	// net is negative → should show negative line count or 삭제
	if !strings.Contains(got, "-") && !strings.Contains(got, "삭제") {
		t.Errorf("expected deletion indicator in summary, got %q", got)
	}
}

func TestSummarizeDiff_MultipleFiles(t *testing.T) {
	diff := `diff --git a/a.go b/a.go
--- a/a.go
+++ b/a.go
@@ -1 +1,3 @@
+func Foo() {}
+func Bar() {}
diff --git a/b.go b/b.go
--- a/b.go
+++ b/b.go
@@ -1,2 +1 @@
-func Old() {}
`
	got := summarizeDiff(diff)
	if !strings.Contains(got, "a.go") {
		t.Errorf("expected a.go in summary, got %q", got)
	}
	if !strings.Contains(got, "b.go") {
		t.Errorf("expected b.go in summary, got %q", got)
	}
}

func TestSummarizeDiff_NoCodeContentLeaked(t *testing.T) {
	diff := `diff --git a/secret.go b/secret.go
--- a/secret.go
+++ b/secret.go
@@ -1 +1,3 @@
+const apiKey = "SUPER_SECRET_KEY_12345"
+const dbPass = "hunter2"
+func connect() error { return nil }
`
	got := summarizeDiff(diff)
	if strings.Contains(got, "SUPER_SECRET_KEY_12345") {
		t.Errorf("summary must not contain secret value, got %q", got)
	}
	if strings.Contains(got, "hunter2") {
		t.Errorf("summary must not contain secret value, got %q", got)
	}
}

// --- extractIdentifier unit tests ---

func TestExtractIdentifier_Func(t *testing.T) {
	cases := []struct {
		line string
		want string
	}{
		{"func Hello() string {", "Hello"},
		{"func (r *Recv) Method(arg int) error {", "Method"},
		{"type Foo struct {", "Foo"},
		{"const MaxRetries = 3", "MaxRetries"},
		{"var ErrNotFound = errors.New(\"not found\")", "ErrNotFound"},
		{"	x := 42", ""},                  // assignment — not an identifier
		{"\treturn someValue", ""},           // return — not an identifier
		{"// comment line", ""},             // comment
	}
	for _, tc := range cases {
		got := extractIdentifier(tc.line)
		if got != tc.want {
			t.Errorf("extractIdentifier(%q) = %q, want %q", tc.line, got, tc.want)
		}
	}
}

// --- dedup unit tests ---

func TestDedup_RemovesDuplicates(t *testing.T) {
	in := []string{"A", "B", "A", "C", "B"}
	got := dedup(in)
	want := []string{"A", "B", "C"}
	if len(got) != len(want) {
		t.Fatalf("dedup(%v) = %v, want %v", in, got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("dedup[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestDedup_CapsAtThree(t *testing.T) {
	in := []string{"A", "B", "C", "D", "E"}
	got := dedup(in)
	if len(got) != 3 {
		t.Errorf("dedup should cap at 3, got %d: %v", len(got), got)
	}
}

// --- DiffSource.RecentMessages integration test ---

func TestDiffSource_RecentMessages_ReturnsSummary(t *testing.T) {
	diff := `diff --git a/engine.go b/engine.go
--- a/engine.go
+++ b/engine.go
@@ -1,3 +1,6 @@
 package pop
+
+func NewEngine() *Engine {
+	return &Engine{}
+}
`
	ds := NewDiffSource(diff)
	msgs, err := ds.RecentMessages(context.Background(), time.Time{}, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	msg := msgs[0]
	if msg.ID == "" {
		t.Error("message ID must not be empty")
	}
	if msg.Content == "" {
		t.Error("message content must not be empty")
	}
	if strings.Contains(msg.Content, "return &Engine{}") {
		t.Errorf("content must not contain source code: %q", msg.Content)
	}
}

func TestDiffSource_RecentMessages_EmptyDiff(t *testing.T) {
	ds := NewDiffSource("")
	msgs, err := ds.RecentMessages(context.Background(), time.Time{}, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(msgs) != 0 {
		t.Errorf("expected 0 messages for empty diff, got %d", len(msgs))
	}
}
