package sonda

import (
	"bytes"
	"encoding/json"
	"math"
	"strconv"
	"strings"
	"time"
)

// Stream says which of the process's outputs a line came from.
type Stream uint8

const (
	Stdout Stream = iota
	Stderr
	// Build marks what a failed build printed, standard output and error
	// together, which is how a failing build is read at a terminal.
	Build
)

// Format is how much of a line was understood.
type Format uint8

const (
	Plain Format = iota
	JSON
	Logfmt
)

// Severity orders levels so that a filter can ask for a level and above. A
// log with no level sits below every real one.
type Severity int8

const (
	Unlevelled Severity = iota
	Trace
	Debug
	Info
	Warn
	Error
	Fatal
	Panic
)

// Kind is the type a structured field's value had in the line.
type Kind uint8

const (
	String Kind = iota
	Number
	Bool
	Null
	Object
)

// Field is one structured key and value left over after the well-known keys
// were lifted out. Object values are kept as compact JSON.
type Field struct {
	Key   string
	Value string
	Kind  Kind
}

// Source is where in the program a line was written from.
type Source struct {
	Path     string
	Line     int
	Function string
}

// Log is one line of output, or one line and the lines that continued it.
type Log struct {
	// At is when the line was read. It is what orders a run: a program's
	// clock and its flush order are not the same thing, so a timestamp parsed
	// from the line is kept apart in Time.
	At     time.Time
	Stream Stream

	// Raw is the line as written. A log made of several lines holds all of
	// them, newline separated.
	Raw    string
	Format Format

	Time     time.Time
	Level    string
	Severity Severity
	Message  string
	Fields   []Field
	Source   Source

	// trace marks a log that opened a panic's trace, which runs until a line
	// parses again.
	trace bool
}

// parse reads one line as far as it is understood. Formats are tried in
// order and the first that matches wins; plain text always matches, so no
// line fails outright.
func parse(line string) Log {
	if l, ok := parseJSON(line); ok {
		return l
	}
	if l, ok := parseLogfmt(line); ok {
		return l
	}
	l := Log{Raw: line, Format: Plain, Message: line}
	l.trace = strings.HasPrefix(line, "panic: ") || strings.HasPrefix(line, "fatal error: ")
	return l
}

// continues reports whether a plain line belongs to the log before it. A
// panic opens a trace that runs until a line parses again; otherwise a line
// that fell through to plain text continues a structured log when it is
// indented under it.
func continues(prev *Log, l Log) bool {
	if prev == nil || l.Format != Plain || prev.Stream != l.Stream {
		return false
	}
	if prev.trace {
		return true
	}
	if prev.Format == Plain || l.Raw == "" {
		return false
	}
	return l.Raw[0] == ' ' || l.Raw[0] == '\t'
}

// join appends a continuation line.
func (l *Log) join(line string) {
	l.Raw += "\n" + line
	l.Message += "\n" + line
}

// parseJSON reads a line that is one JSON object. Keys are kept in the order
// they were written, which a map would lose.
func parseJSON(line string) (Log, bool) {
	s := strings.TrimSpace(line)
	if !strings.HasPrefix(s, "{") || !strings.HasSuffix(s, "}") {
		return Log{}, false
	}
	dec := json.NewDecoder(strings.NewReader(s))
	dec.UseNumber()
	if tok, err := dec.Token(); err != nil || tok != json.Delim('{') {
		return Log{}, false
	}
	var fields []Field
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return Log{}, false
		}
		key, ok := tok.(string)
		if !ok {
			return Log{}, false
		}
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			return Log{}, false
		}
		fields = append(fields, jsonField(key, raw))
	}
	if tok, err := dec.Token(); err != nil || tok != json.Delim('}') {
		return Log{}, false
	}
	if dec.InputOffset() != int64(len(s)) {
		return Log{}, false
	}
	l := Log{Raw: line, Format: JSON}
	l.lift(fields)
	return l, true
}

func jsonField(key string, raw json.RawMessage) Field {
	raw = bytes.TrimSpace(raw)
	f := Field{Key: key}
	switch {
	case len(raw) == 0:
		f.Kind = Null
	case raw[0] == '"':
		var s string
		if json.Unmarshal(raw, &s) == nil {
			f.Value = s
		} else {
			f.Value = string(raw)
		}
	case raw[0] == '{' || raw[0] == '[':
		var buf bytes.Buffer
		if json.Compact(&buf, raw) == nil {
			f.Value = buf.String()
		} else {
			f.Value = string(raw)
		}
		f.Kind = Object
	case raw[0] == 't' || raw[0] == 'f':
		f.Value, f.Kind = string(raw), Bool
	case raw[0] == 'n':
		f.Value, f.Kind = "null", Null
	default:
		f.Value, f.Kind = string(raw), Number
	}
	return f
}

// parseLogfmt reads a line of key=value pairs separated by spaces, with
// quoted values. Every token has to be a pair: a line with a bare word in it
// is prose with an equals sign, not logfmt, and is left to plain text.
func parseLogfmt(line string) (Log, bool) {
	var fields []Field
	i, n := 0, len(line)
	for i < n {
		for i < n && (line[i] == ' ' || line[i] == '\t') {
			i++
		}
		if i >= n {
			break
		}
		start := i
		for i < n && line[i] != '=' && line[i] != ' ' && line[i] != '\t' && line[i] != '"' {
			i++
		}
		if i >= n || line[i] != '=' || i == start {
			return Log{}, false
		}
		f := Field{Key: line[start:i]}
		i++
		if i < n && line[i] == '"' {
			value, end, ok := unquote(line, i)
			if !ok || (end < n && line[end] != ' ' && line[end] != '\t') {
				return Log{}, false
			}
			f.Value, i = value, end
		} else {
			start = i
			for i < n && line[i] != ' ' && line[i] != '\t' {
				i++
			}
			f.Value = line[start:i]
			f.Kind = bareKind(f.Value)
		}
		fields = append(fields, f)
	}
	if len(fields) == 0 {
		return Log{}, false
	}
	l := Log{Raw: line, Format: Logfmt}
	l.lift(fields)
	return l, true
}

// unquote reads a double-quoted value starting at the quote, and returns it
// with the index just past the closing quote.
func unquote(s string, i int) (string, int, bool) {
	var b strings.Builder
	for i++; i < len(s); i++ {
		switch c := s[i]; c {
		case '"':
			return b.String(), i + 1, true
		case '\\':
			if i+1 >= len(s) {
				return "", i, false
			}
			i++
			switch s[i] {
			case 'n':
				b.WriteByte('\n')
			case 't':
				b.WriteByte('\t')
			case 'r':
				b.WriteByte('\r')
			default:
				b.WriteByte(s[i])
			}
		default:
			b.WriteByte(c)
		}
	}
	return "", i, false
}

func bareKind(v string) Kind {
	switch v {
	case "true", "false":
		return Bool
	case "null", "nil", "<nil>":
		return Null
	}
	if _, err := strconv.ParseFloat(v, 64); err == nil {
		return Number
	}
	return String
}

// The well-known keys, in the order they are tried.
var (
	timeKeys    = []string{"time", "ts", "timestamp", "@timestamp"}
	levelKeys   = []string{"level", "lvl", "severity"}
	messageKeys = []string{"msg", "message"}
)

// lift moves the well-known keys from the fields into the log's own places.
// A key whose value is not of the shape expected stays a field, so nothing is
// lost by being half understood.
func (l *Log) lift(fields []Field) {
	taken := make([]bool, len(fields))
	take := func(keys []string, want func(Field) bool) {
		for _, k := range keys {
			for i, f := range fields {
				if !taken[i] && f.Key == k && want(f) {
					taken[i] = true
					return
				}
			}
		}
	}
	take(timeKeys, func(f Field) bool {
		t, ok := parseTime(f)
		l.Time = t
		return ok
	})
	take(levelKeys, func(f Field) bool {
		level, sev, ok := parseLevel(f)
		l.Level, l.Severity = level, sev
		return ok
	})
	take(messageKeys, func(f Field) bool {
		if f.Kind != String {
			return false
		}
		l.Message = f.Value
		return true
	})
	l.liftSource(fields, taken)

	for i, f := range fields {
		if !taken[i] {
			l.Fields = append(l.Fields, f)
		}
	}
}

// liftSource takes the source location from a caller string, a source object,
// or a file with a line beside it.
func (l *Log) liftSource(fields []Field, taken []bool) {
	at := func(key string) int {
		for i, f := range fields {
			if !taken[i] && f.Key == key {
				return i
			}
		}
		return -1
	}
	for _, key := range []string{"source", "caller"} {
		i := at(key)
		if i < 0 {
			continue
		}
		var src Source
		var ok bool
		switch fields[i].Kind {
		case String:
			src, ok = parseLocation(fields[i].Value)
		case Object:
			src, ok = parseSourceObject(fields[i].Value)
		}
		if ok {
			l.Source, taken[i] = src, true
			return
		}
	}
	if fi, li := at("file"), at("line"); fi >= 0 && li >= 0 && fields[fi].Kind == String {
		if n, err := strconv.Atoi(fields[li].Value); err == nil {
			l.Source = Source{Path: fields[fi].Value, Line: n}
			taken[fi], taken[li] = true, true
		}
	}
}

// parseLocation reads "path/file.go:123", with or without the number.
func parseLocation(s string) (Source, bool) {
	if s == "" {
		return Source{}, false
	}
	colon := strings.LastIndexByte(s, ':')
	if colon < 0 {
		return Source{Path: s}, true
	}
	n, err := strconv.Atoi(s[colon+1:])
	if err != nil {
		return Source{Path: s}, true
	}
	return Source{Path: s[:colon], Line: n}, true
}

func parseSourceObject(compact string) (Source, bool) {
	var obj struct {
		File     string `json:"file"`
		Line     int    `json:"line"`
		Function string `json:"function"`
		Func     string `json:"func"`
	}
	if json.Unmarshal([]byte(compact), &obj) != nil || obj.File == "" {
		return Source{}, false
	}
	src := Source{Path: obj.File, Line: obj.Line, Function: obj.Function}
	if src.Function == "" {
		src.Function = obj.Func
	}
	return src, true
}

var timeLayouts = []string{
	time.RFC3339Nano,
	"2006-01-02T15:04:05.000Z0700",
	"2006-01-02 15:04:05.999999999Z07:00",
	"2006-01-02 15:04:05.999999999",
	"2006-01-02T15:04:05.999999999",
	"2006/01/02 15:04:05",
	time.RFC1123Z,
	time.RFC1123,
}

// parseTime reads a timestamp written as text or as a number of seconds,
// milliseconds, microseconds or nanoseconds since the epoch, told apart by
// their size.
func parseTime(f Field) (time.Time, bool) {
	switch f.Kind {
	case String:
		for _, layout := range timeLayouts {
			if t, err := time.Parse(layout, f.Value); err == nil {
				return t, true
			}
		}
	case Number:
		n, err := strconv.ParseFloat(f.Value, 64)
		if err != nil || n <= 0 || math.IsInf(n, 0) {
			return time.Time{}, false
		}
		switch {
		case n < 1e11:
			sec, frac := math.Modf(n)
			return time.Unix(int64(sec), int64(frac*1e9)), true
		case n < 1e14:
			return time.UnixMilli(int64(n)), true
		case n < 1e17:
			return time.UnixMicro(int64(n)), true
		default:
			return time.Unix(0, int64(n)), true
		}
	}
	return time.Time{}, false
}

// parseLevel reads a level written as a name, or as one of the numbers pino
// and its relatives write, which come back as the name they stand for.
func parseLevel(f Field) (string, Severity, bool) {
	switch f.Kind {
	case String:
		sev := severityOf(f.Value)
		return f.Value, sev, true
	case Number:
		n, err := strconv.ParseFloat(f.Value, 64)
		if err != nil {
			return "", Unlevelled, false
		}
		var name string
		switch {
		case n < 20:
			name = "trace"
		case n < 30:
			name = "debug"
		case n < 40:
			name = "info"
		case n < 50:
			name = "warn"
		case n < 60:
			name = "error"
		default:
			name = "fatal"
		}
		return name, severityOf(name), true
	}
	return "", Unlevelled, false
}

func severityOf(level string) Severity {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "trace", "trc":
		return Trace
	case "debug", "dbg", "verbose":
		return Debug
	case "info", "inf", "information", "notice":
		return Info
	case "warn", "wrn", "warning":
		return Warn
	case "error", "err":
		return Error
	case "fatal", "ftl", "critical", "crit", "emergency", "alert":
		return Fatal
	case "panic", "pnc":
		return Panic
	}
	return Unlevelled
}

// Name is the level as a short capitalised word, for a column of fixed width.
func (s Severity) Name() string {
	switch s {
	case Trace:
		return "TRACE"
	case Debug:
		return "DEBUG"
	case Info:
		return "INFO"
	case Warn:
		return "WARN"
	case Error:
		return "ERROR"
	case Fatal:
		return "FATAL"
	case Panic:
		return "PANIC"
	}
	return ""
}
