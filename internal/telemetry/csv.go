package telemetry

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"
)

// timeColumns are the column (and JSON key) names taken as the timestamp.
var timeColumns = []string{"timestamp", "time", "ts"}

// readCSV reads a header row naming the columns, then one row per time: a
// timestamp column (timestamp, time or ts, else the first) and a number per
// metric. An empty cell means no value then.
func readCSV(r io.Reader) ([]sample, error) {
	cr := csv.NewReader(r)
	cr.Comment = '#'
	cr.FieldsPerRecord = -1
	cr.TrimLeadingSpace = true

	header, err := cr.Read()
	if err != nil {
		if errors.Is(err, io.EOF) {
			return nil, nil
		}
		return nil, err
	}
	timeCol := slices.IndexFunc(header, func(h string) bool {
		return slices.Contains(timeColumns, strings.ToLower(strings.TrimSpace(h)))
	})
	if timeCol < 0 {
		timeCol = 0
	}

	var samples []sample
	for {
		row, err := cr.Read()
		if errors.Is(err, io.EOF) {
			return samples, nil
		}
		if err != nil {
			return nil, err
		}
		line, _ := cr.FieldPos(0)
		if timeCol >= len(row) {
			return nil, fmt.Errorf("line %d: no timestamp", line)
		}
		t, err := parseTime(row[timeCol])
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", line, err)
		}
		for i, cell := range row {
			cell = strings.TrimSpace(cell)
			if i == timeCol || i >= len(header) || cell == "" {
				continue
			}
			v, err := strconv.ParseFloat(cell, 64)
			if err != nil {
				return nil, fmt.Errorf("line %d: %s: %q isn't a number", line, header[i], cell)
			}
			name := strings.TrimSpace(header[i])
			samples = append(samples, sample{t: t, series: name, name: name, value: v})
		}
	}
}
