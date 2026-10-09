package telemetry

import (
	"bufio"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"slices"
	"strconv"
)

// WriteFormats are the formats a Writer writes.
var WriteFormats = []string{"jsonl", "csv"}

// Writer records each tick's raw metrics, for the file source to replay.
//
// A live run reads a metric missing from a tick as 0 (Wikipedia, say, only
// reports what happened that second), but a replay holds a series' last
// value, so once a metric has turned up, every later tick records it,
// with 0 when it's missing. A recording then plays as the run did.
type Writer struct {
	path   string
	format string
	f      *os.File
	w      *bufio.Writer
	seen   []string // every metric so far, sorted; a CSV's columns
	err    error
}

// CreateWriter starts a recording at path in format: "jsonl" or "csv", or
// "" to go by the extension.
func CreateWriter(path, format string) (*Writer, error) {
	if err := CheckWriter(path, format); err != nil {
		return nil, err
	}
	if format == "" {
		format = FormatOf(path)
	}
	f, err := os.Create(path)
	if err != nil {
		return nil, err
	}
	w := &Writer{path: path, format: format, f: f, w: bufio.NewWriter(f)}
	if format == "csv" {
		_, err = w.w.WriteString("timestamp\n")
	}
	return w, err
}

// CheckWriter reports whether CreateWriter would accept path and format,
// without creating anything.
func CheckWriter(path, format string) error {
	if format == "" {
		format = FormatOf(path)
	}
	if !slices.Contains(WriteFormats, format) {
		if format == "" {
			return fmt.Errorf("%s: can't tell the format from the extension; use .jsonl or .csv, or add format=jsonl|csv", path)
		}
		return fmt.Errorf("%s: telemetry is recorded as jsonl or csv, not %s", path, format)
	}
	return nil
}

// Write records one tick. Values that aren't finite are recorded as 0,
// as neither format can hold them. A metric new to a CSV adds a column,
// which means rewriting what's been written so far, with 0s in it.
func (w *Writer) Write(timestamp int64, metrics map[string]float64) error {
	if w.err != nil {
		return w.err
	}
	var added []string
	for name := range metrics {
		if _, found := slices.BinarySearch(w.seen, name); !found {
			added = append(added, name)
		}
	}
	if len(added) > 0 {
		w.seen = append(w.seen, added...)
		slices.Sort(w.seen)
		if w.format == "csv" {
			w.err = w.rewriteCSV()
		}
	}

	if w.err == nil {
		if w.format == "jsonl" {
			w.err = w.writeJSON(timestamp, metrics)
		} else {
			w.err = w.writeCSVRow(timestamp, metrics)
		}
	}
	if w.err == nil {
		// A tick at a time, so a recording cut short still has everything
		// up to then.
		w.err = w.w.Flush()
	}
	if w.err != nil {
		w.err = fmt.Errorf("%s: %w", w.path, w.err)
	}
	return w.err
}

// value is a metric's value to record: 0 when it's missing or not finite.
func value(metrics map[string]float64, name string) string {
	v := metrics[name]
	if math.IsNaN(v) || math.IsInf(v, 0) {
		v = 0
	}
	return strconv.FormatFloat(v, 'g', -1, 64)
}

func (w *Writer) writeJSON(timestamp int64, metrics map[string]float64) error {
	fmt.Fprintf(w.w, `{"timestamp":%d`, timestamp)
	for _, name := range w.seen {
		key, _ := json.Marshal(name)
		fmt.Fprintf(w.w, ",%s:%s", key, value(metrics, name))
	}
	_, err := w.w.WriteString("}\n")
	return err
}

func (w *Writer) writeCSVRow(timestamp int64, metrics map[string]float64) error {
	row := make([]string, len(w.seen)+1)
	row[0] = strconv.FormatInt(timestamp, 10)
	for i, name := range w.seen {
		row[i+1] = value(metrics, name)
	}
	c := csv.NewWriter(w.w)
	c.Write(row)
	c.Flush()
	return c.Error()
}

// rewriteCSV writes the file again with a column for every metric seen,
// the new ones 0 in the rows so far.
func (w *Writer) rewriteCSV() error {
	if err := w.w.Flush(); err != nil {
		return err
	}
	if _, err := w.f.Seek(0, io.SeekStart); err != nil {
		return err
	}
	old, err := csv.NewReader(w.f).ReadAll()
	if err != nil {
		return err
	}
	header, rows := old[0], old[1:]

	if err := w.f.Truncate(0); err != nil {
		return err
	}
	if _, err := w.f.Seek(0, io.SeekStart); err != nil {
		return err
	}
	w.w.Reset(w.f)
	c := csv.NewWriter(w.w)
	c.Write(append([]string{"timestamp"}, w.seen...))
	for _, row := range rows {
		was := make(map[string]string, len(header))
		for i, name := range header {
			if i < len(row) {
				was[name] = row[i]
			}
		}
		out := make([]string, len(w.seen)+1)
		out[0] = was["timestamp"]
		for i, name := range w.seen {
			if v, ok := was[name]; ok {
				out[i+1] = v
			} else {
				out[i+1] = "0"
			}
		}
		c.Write(out)
	}
	c.Flush()
	return c.Error()
}

// Close finishes the file.
func (w *Writer) Close() error {
	err := errors.Join(w.w.Flush(), w.f.Close())
	if err != nil {
		return fmt.Errorf("%s: %w", w.path, err)
	}
	return nil
}
