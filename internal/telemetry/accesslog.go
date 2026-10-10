package telemetry

import (
	"bufio"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// The Common and Combined Log Formats, Apache's and nginx's defaults.
const (
	commonLogFormat   = `%h %l %u %t "%r" %>s %b`
	combinedLogFormat = `%h %l %u %t "%r" %>s %b "%{Referer}i" "%{User-agent}i"`
)

// readAccessLog counts a web server's access log, a line per request, into
// per-second metrics:
//
//	requests                          requests that second
//	bytes                             bytes sent (%b, %B or %O)
//	status_1xx ... status_5xx         requests by status class
//	method_get, method_post, ...      requests by method, from %r or %m
//	                                  (method_other for the rest)
//	clients                           distinct clients (%h or %a)
//	response_ms, response_ms_max      mean and slowest response time,
//	                                  if the log has one (%D or %T)
//
// logFormat is an Apache LogFormat; with none, each line is tried as the
// Combined and then the Common Log Format. Lines may run on past the
// format, as nginx's often do. Lines that don't fit are skipped, and
// counted, unless most don't fit.
func readAccessLog(r io.Reader, logFormat string) ([]sample, int, error) {
	var formats []*accessFormat
	if logFormat == "" {
		for _, f := range []string{combinedLogFormat, commonLogFormat} {
			af, err := compileLogFormat(f)
			if err != nil {
				return nil, 0, err
			}
			formats = append(formats, af)
		}
	} else {
		af, err := compileLogFormat(logFormat)
		if err != nil {
			return nil, 0, fmt.Errorf("logformat: %w", err)
		}
		formats = append(formats, af)
	}

	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 16*1024*1024)
	t := newTally()
	var lines, skipped int
	var firstBad string
	for line := 1; sc.Scan(); line++ {
		text := sc.Text()
		if strings.TrimSpace(text) == "" {
			continue
		}
		lines++
		ok := false
		for _, f := range formats {
			if f.count(text, t) {
				ok = true
				break
			}
		}
		if !ok {
			skipped++
			if firstBad == "" {
				firstBad = fmt.Sprintf("line %d: %.120s", line, text)
			}
		}
	}
	if err := sc.Err(); err != nil {
		return nil, 0, err
	}
	if lines > 0 && skipped*2 > lines {
		return nil, 0, fmt.Errorf("%d of %d lines don't fit the log format (%s); give the server's LogFormat as logformat=", skipped, lines, firstBad)
	}
	return t.samples(), skipped, nil
}

// accessFormat is a compiled LogFormat: a pattern, and which of its groups
// hold the fields counted.
type accessFormat struct {
	re *regexp.Regexp
	// group index of each field, or 0 if the format hasn't got it
	time, status, client, request, method, bytes, duration int
	// durationScale turns the duration field into milliseconds.
	durationScale float64
}

// directive matches one LogFormat directive: %, then any condition
// (!400,501) and < or >, then any {argument}, then its letter.
var directive = regexp.MustCompile(`%!?[0-9,]*[<>]?(?:\{([^}]*)\})?([a-zA-Z%])`)

// compileLogFormat turns a LogFormat into a pattern. It needs a time (%t)
// and a status (%s or %>s); a time with a custom strftime format isn't
// understood.
func compileLogFormat(format string) (*accessFormat, error) {
	af := &accessFormat{}
	var pattern strings.Builder
	pattern.WriteString("^")
	group := 0
	last := 0
	for _, m := range directive.FindAllStringSubmatchIndex(format, -1) {
		literal := format[last:m[0]]
		last = m[1]
		pattern.WriteString(literalPattern(literal))

		letter := format[m[4]:m[5]]
		arg := ""
		if m[2] >= 0 {
			arg = format[m[2]:m[3]]
		}
		if letter == "%" {
			pattern.WriteString("%")
			continue
		}

		quoted := strings.HasSuffix(literal, `"`)
		group++
		switch {
		case letter == "t" && arg != "":
			return nil, fmt.Errorf("%%{%s}t: only the default time format, %%t, is understood", arg)
		case letter == "t":
			pattern.WriteString(`\[([^\]]+)\]`)
			af.time = group
			continue
		case quoted:
			pattern.WriteString(`((?:[^"\\]|\\.)*)`)
		default:
			pattern.WriteString(`(\S*)`)
		}

		switch letter {
		case "s":
			// %>s, the final status, wins over %s, the original.
			if af.status == 0 || strings.Contains(format[m[0]:m[1]], ">") {
				af.status = group
			}
		case "h", "a":
			if af.client == 0 {
				af.client = group
			}
		case "r":
			af.request = group
		case "m":
			af.method = group
		case "b", "B", "O":
			if af.bytes == 0 {
				af.bytes = group
			}
		case "D":
			af.duration, af.durationScale = group, 1e-3
		case "T":
			scale := map[string]float64{"": 1e3, "s": 1e3, "ms": 1, "us": 1e-3}[arg]
			if scale == 0 {
				return nil, fmt.Errorf("%%{%s}T: want %%T, %%{ms}T, %%{us}T or %%{s}T", arg)
			}
			af.duration, af.durationScale = group, scale
		}
	}
	pattern.WriteString(literalPattern(format[last:]))

	if af.time == 0 {
		return nil, fmt.Errorf("%q has no time (%%t)", format)
	}
	if af.status == 0 {
		return nil, fmt.Errorf("%q has no status (%%s or %%>s)", format)
	}
	re, err := regexp.Compile(pattern.String())
	if err != nil {
		return nil, err
	}
	af.re = re
	return af, nil
}

// literalPattern matches literal text from a LogFormat, any run of spaces
// as one or more.
func literalPattern(s string) string {
	return spaces.ReplaceAllString(regexp.QuoteMeta(s), " +")
}

var spaces = regexp.MustCompile(` +`)

// count adds a line to the tally, or returns false if it doesn't fit.
func (af *accessFormat) count(line string, t *tally) bool {
	m := af.re.FindStringSubmatch(line)
	if m == nil {
		return false
	}
	when, err := time.Parse("02/Jan/2006:15:04:05 -0700", m[af.time])
	if err != nil {
		return false
	}
	status, err := strconv.Atoi(m[af.status])
	if err != nil || status < 100 || status > 599 {
		return false
	}
	at := float64(when.Unix())

	t.add(at, "requests", 1)
	t.add(at, fmt.Sprintf("status_%dxx", status/100), 1)
	if af.bytes > 0 {
		if n, err := strconv.ParseFloat(m[af.bytes], 64); err == nil { // "-" is none
			t.add(at, "bytes", n)
		}
	}
	if af.client > 0 && m[af.client] != "-" {
		t.count(at, "clients", m[af.client])
	}

	method := ""
	if af.method > 0 {
		method = m[af.method]
	} else if af.request > 0 {
		method, _, _ = strings.Cut(m[af.request], " ")
	}
	if method != "" && method != "-" {
		switch method = strings.ToLower(method); method {
		case "get", "post", "head", "put", "delete", "patch", "options":
		default:
			method = "other"
		}
		t.add(at, "method_"+method, 1)
	}

	if af.duration > 0 {
		if d, err := strconv.ParseFloat(m[af.duration], 64); err == nil {
			t.observe(at, "response_ms", d*af.durationScale)
		}
	}
	return true
}
