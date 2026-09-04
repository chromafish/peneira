package sonda

import (
	"testing"
	"time"
)

func TestJSONLinesLiftTheWellKnownKeys(t *testing.T) {
	line := `{"time":"2026-09-04T10:00:00.5Z","level":"INFO","msg":"listening","addr":":8080","retries":3,"ok":true,"source":{"function":"main.serve","file":"/src/main.go","line":42}}`
	l := Parse(line)
	if l.Format != JSON {
		t.Fatalf("format = %v, want JSON", l.Format)
	}
	if want := time.Date(2026, 9, 4, 10, 0, 0, 500_000_000, time.UTC); !l.Time.Equal(want) {
		t.Errorf("time = %v, want %v", l.Time, want)
	}
	if l.Level != "INFO" || l.Severity != Info {
		t.Errorf("level = %q %v", l.Level, l.Severity)
	}
	if l.Message != "listening" {
		t.Errorf("message = %q", l.Message)
	}
	if l.Source != (Source{Path: "/src/main.go", Line: 42, Function: "main.serve"}) {
		t.Errorf("source = %+v", l.Source)
	}
	want := []Field{{"addr", ":8080", String}, {"retries", "3", Number}, {"ok", "true", Bool}}
	if len(l.Fields) != len(want) {
		t.Fatalf("fields = %+v, want %+v", l.Fields, want)
	}
	for i := range want {
		if l.Fields[i] != want[i] {
			t.Errorf("field %d = %+v, want %+v", i, l.Fields[i], want[i])
		}
	}
	if l.Raw != line {
		t.Errorf("raw = %q", l.Raw)
	}
}

// pino writes its levels as numbers and its time in milliseconds; zap writes
// the caller as one string. All three come back in the same shape.
func TestOtherJSONLoggers(t *testing.T) {
	pino := Parse(`{"level":30,"time":1756980000123,"pid":1,"msg":"hello"}`)
	if pino.Level != "info" || pino.Severity != Info {
		t.Errorf("pino level = %q %v", pino.Level, pino.Severity)
	}
	if want := time.UnixMilli(1756980000123); !pino.Time.Equal(want) {
		t.Errorf("pino time = %v, want %v", pino.Time, want)
	}
	zap := Parse(`{"level":"error","ts":1756980000.5,"caller":"pkg/file.go:123","msg":"boom","error":"eof"}`)
	if zap.Severity != Error {
		t.Errorf("zap severity = %v", zap.Severity)
	}
	if zap.Source != (Source{Path: "pkg/file.go", Line: 123}) {
		t.Errorf("zap source = %+v", zap.Source)
	}
	if !zap.Time.Equal(time.Unix(1756980000, 500_000_000)) {
		t.Errorf("zap time = %v", zap.Time)
	}
	if len(zap.Fields) != 1 || zap.Fields[0].Key != "error" {
		t.Errorf("zap fields = %+v", zap.Fields)
	}
}

func TestLogfmt(t *testing.T) {
	l := Parse(`time=2026-09-04T10:00:00Z level=WARN msg="disk is nearly full" free=12 unit=GB`)
	if l.Format != Logfmt {
		t.Fatalf("format = %v, want logfmt", l.Format)
	}
	if l.Severity != Warn || l.Message != "disk is nearly full" {
		t.Errorf("level %v message %q", l.Severity, l.Message)
	}
	if l.Time.IsZero() {
		t.Error("the time was not parsed")
	}
	if len(l.Fields) != 2 || l.Fields[0] != (Field{"free", "12", Number}) || l.Fields[1] != (Field{"unit", "GB", String}) {
		t.Errorf("fields = %+v", l.Fields)
	}
}

// Prose with an equals sign in it is not logfmt, and neither is anything
// else that is not entirely pairs.
func TestPlainTextIsNotGuessedAt(t *testing.T) {
	for _, line := range []string{
		"2026/09/04 10:00:00 listening on :8080",
		"error: x=1 went wrong",
		`{"almost": json`,
		"INFO[0000] starting port=8080",
		"",
	} {
		l := Parse(line)
		if l.Format != Plain {
			t.Errorf("%q parsed as %v", line, l.Format)
		}
		if l.Message != line || l.Raw != line || l.Level != "" || len(l.Fields) != 0 {
			t.Errorf("%q became %+v", line, l)
		}
	}
}

func TestIndentedPlainTextContinuesAStructuredLog(t *testing.T) {
	first := Parse(`level=error msg="query failed"`)
	first.Stream = Stderr
	next := Parse("    at db.go:10")
	next.Stream = Stderr
	if !continues(&first, next) {
		t.Fatal("an indented line did not continue the log before it")
	}
	first.join(next.Raw)
	if first.Message != "query failed\n    at db.go:10" {
		t.Errorf("message = %q", first.Message)
	}
	if first.Raw != "level=error msg=\"query failed\"\n    at db.go:10" {
		t.Errorf("raw = %q", first.Raw)
	}

	// Across streams, or under a plain line, the same text stands alone.
	other := next
	other.Stream = Stdout
	if continues(&first, other) {
		t.Error("a line on another stream was joined")
	}
	plain := Parse("something plain")
	plain.Stream = Stderr
	if continues(&plain, next) {
		t.Error("a line was joined to plain text")
	}
	flush := Parse("flush left")
	flush.Stream = Stderr
	if continues(&first, flush) {
		t.Error("a flush-left line was joined to a structured log")
	}
}

// A panic and its trace stay one log rather than becoming a hundred.
func TestAPanicOpensATraceThatRunsUntilALineParses(t *testing.T) {
	lines := []string{
		"panic: runtime error: index out of range [3] with length 3",
		"",
		"goroutine 1 [running]:",
		"main.main()",
		"\t/src/main.go:10 +0x1d",
		"exit status 2",
		`{"level":"info","msg":"restarted"}`,
	}
	var logs []Log
	for _, line := range lines {
		l := Parse(line)
		if n := len(logs); n > 0 && continues(&logs[n-1], l) {
			logs[n-1].join(line)
			continue
		}
		logs = append(logs, l)
	}
	if len(logs) != 2 {
		t.Fatalf("got %d logs, want the panic and the line after it", len(logs))
	}
	if logs[0].Raw != "panic: runtime error: index out of range [3] with length 3\n\ngoroutine 1 [running]:\nmain.main()\n\t/src/main.go:10 +0x1d\nexit status 2" {
		t.Errorf("trace = %q", logs[0].Raw)
	}
	if logs[1].Format != JSON {
		t.Errorf("the line after the trace parsed as %v", logs[1].Format)
	}
}

func TestSourceFromFileAndLine(t *testing.T) {
	l := Parse(`{"level":"debug","message":"x","file":"a/b.go","line":7}`)
	if l.Source != (Source{Path: "a/b.go", Line: 7}) {
		t.Errorf("source = %+v", l.Source)
	}
	if len(l.Fields) != 0 {
		t.Errorf("fields left over: %+v", l.Fields)
	}
}

// A key of the right name but the wrong shape stays a field rather than
// being lost.
func TestAKeyOfTheWrongShapeStaysAField(t *testing.T) {
	l := Parse(`{"msg":{"nested":true},"level":"info"}`)
	if l.Message != "" {
		t.Errorf("message = %q, want none", l.Message)
	}
	if len(l.Fields) != 1 || l.Fields[0].Key != "msg" || l.Fields[0].Kind != Object {
		t.Errorf("fields = %+v", l.Fields)
	}
}
