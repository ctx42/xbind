// SPDX-FileCopyrightText: (c) 2026 Rafal Zajac
// SPDX-License-Identifier: MIT

//go:build ignore

// Command report reads "go test -bench -benchmem -v" output of this package
// on stdin and prints Markdown tables comparing the libraries with xbind.
package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

// base is the library the others are compared with.
const base = "xbind"

// units are the metrics reported, in order.
var units = []string{"ns/op", "B/op", "allocs/op"}

var (
	// resultRe matches a benchmark result line.
	resultRe = regexp.MustCompile(
		`^Benchmark_(\w+)/([^/\s]+)/(\S+?)(?:-\d+)?\s+\d+\s+(.+)$`,
	)

	// skipRe matches the line closing a skipped benchmark.
	skipRe = regexp.MustCompile(`^--- SKIP: Benchmark_(\w+)/([^/\s]+)/(\S+)`)

	// reasonRe matches a line logged by a benchmark.
	reasonRe = regexp.MustCompile(`^\s+\S+_test\.go:\d+: (.*)$`)
)

// cell identifies one library on one workload of one operation.
type cell struct{ op, work, lib string }

// report holds the parsed benchmark output.
type report struct {
	header  []string                      // The goos, goarch, cpu lines.
	ops     []string                      // Operations in input order.
	works   map[string][]string           // Workloads per operation.
	libs    map[string][]string           // Libraries per operation.
	samples map[cell]map[string][]float64 // Values per cell and unit.
	skips   map[cell]string               // Skip reasons per cell.
}

func main() {
	rep, err := parse(os.Stdin)
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	rep.write(os.Stdout)
}

// parse reads benchmark output from r.
func parse(r io.Reader) (*report, error) {
	rep := &report{
		works:   map[string][]string{},
		libs:    map[string][]string{},
		samples: map[cell]map[string][]float64{},
		skips:   map[cell]string{},
	}
	var reason string
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		line := sc.Text()
		if m := reasonRe.FindStringSubmatch(line); m != nil {
			reason = m[1]
			continue
		}
		if m := skipRe.FindStringSubmatch(line); m != nil {
			rep.skips[rep.add(m[1], m[2], m[3])] = reason
			continue
		}
		if strings.HasPrefix(line, "goos:") ||
			strings.HasPrefix(line, "goarch:") ||
			strings.HasPrefix(line, "cpu:") {
			if !slices.Contains(rep.header, line) {
				rep.header = append(rep.header, line)
			}
			continue
		}
		m := resultRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		cel := rep.add(m[1], m[2], m[3])
		fields := strings.Fields(m[4])
		for i := 0; i+1 < len(fields); i += 2 {
			val, err := strconv.ParseFloat(fields[i], 64)
			if err != nil {
				return nil, fmt.Errorf("parse %q: %w", line, err)
			}
			if rep.samples[cel] == nil {
				rep.samples[cel] = map[string][]float64{}
			}
			unit := fields[i+1]
			rep.samples[cel][unit] = append(rep.samples[cel][unit], val)
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if len(rep.samples) == 0 {
		return nil, fmt.Errorf("no benchmark results on stdin")
	}
	return rep, nil
}

// add records the operation, workload and library in input order and returns
// their cell.
func (rep *report) add(op, work, lib string) cell {
	if !slices.Contains(rep.ops, op) {
		rep.ops = append(rep.ops, op)
	}
	if !slices.Contains(rep.works[op], work) {
		rep.works[op] = append(rep.works[op], work)
	}
	if !slices.Contains(rep.libs[op], lib) {
		rep.libs[op] = append(rep.libs[op], lib)
	}
	return cell{op: op, work: work, lib: lib}
}

// median returns the median of the cell's values in unit, and false when the
// cell has none.
func (rep *report) median(cel cell, unit string) (float64, bool) {
	vals := slices.Clone(rep.samples[cel][unit])
	if len(vals) == 0 {
		return 0, false
	}
	slices.Sort(vals)
	mid := len(vals) / 2
	if len(vals)%2 == 0 {
		return (vals[mid-1] + vals[mid]) / 2, true
	}
	return vals[mid], true
}

// timed returns the libraries with at least one result for op.
func (rep *report) timed(op string) []string {
	var libs []string
	for _, lib := range rep.libs[op] {
		for _, work := range rep.works[op] {
			if _, ok := rep.samples[cell{op, work, lib}]; ok {
				libs = append(libs, lib)
				break
			}
		}
	}
	return libs
}

// write prints the report to w.
func (rep *report) write(w io.Writer) {
	_, _ = fmt.Fprintln(w, "# xbind compared with other form libraries")
	_, _ = fmt.Fprintln(w)
	for _, line := range rep.header {
		_, _ = fmt.Fprintf(w, "- %s\n", line)
	}
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, "Each cell is the median of the runs, then the "+
		"ratio to xbind; below 1 the library beats xbind.")
	for _, op := range rep.ops {
		libs := rep.timed(op)
		for _, unit := range units {
			rows := [][]string{append([]string{"workload"}, libs...)}
			for _, work := range rep.works[op] {
				ref, _ := rep.median(cell{op, work, base}, unit)
				row := []string{work}
				for _, lib := range libs {
					val, ok := rep.median(cell{op, work, lib}, unit)
					switch {
					case !ok:
						row = append(row, "—")
					case lib == base || ref == 0:
						row = append(row, number(val))
					default:
						txt := fmt.Sprintf("%s ×%.2f", number(val), val/ref)
						row = append(row, txt)
					}
				}
				rows = append(rows, row)
			}
			_, _ = fmt.Fprintf(w, "\n## %s %s\n\n", op, unit)
			table(w, rows)
		}
	}
	rep.writeTrails(w)
	rep.writeSkips(w)
}

// writeTrails prints the cases where another library beats xbind, the
// largest gap first.
func (rep *report) writeTrails(w io.Writer) {
	type trail struct {
		op, work, unit, lib string
		have, best          float64
	}
	var trails []trail
	for _, op := range rep.ops {
		for _, work := range rep.works[op] {
			for _, unit := range units {
				have, ok := rep.median(cell{op, work, base}, unit)
				if !ok {
					continue
				}
				trl := trail{op: op, work: work, unit: unit, have: have}
				for _, lib := range rep.libs[op] {
					val, ok := rep.median(cell{op, work, lib}, unit)
					if ok && lib != base && val < have &&
						(trl.lib == "" || val < trl.best) {
						trl.lib, trl.best = lib, val
					}
				}
				if trl.lib != "" {
					trails = append(trails, trl)
				}
			}
		}
	}
	sort.SliceStable(trails, func(i, j int) bool {
		return gap(trails[i].have, trails[i].best) >
			gap(trails[j].have, trails[j].best)
	})
	_, _ = fmt.Fprintln(w, "\n## Where xbind trails")
	_, _ = fmt.Fprintln(w)
	if len(trails) == 0 {
		_, _ = fmt.Fprintln(w, "Nowhere: xbind is best on every metric.")
		return
	}
	rows := [][]string{{
		"op", "workload", "metric", "xbind", "best", "library", "xbind ÷ best",
	}}
	for _, trl := range trails {
		rows = append(rows, []string{
			trl.op,
			trl.work,
			trl.unit,
			number(trl.have),
			number(trl.best),
			trl.lib,
			ratio(trl.have, trl.best),
		})
	}
	table(w, rows)
}

// writeSkips prints why libraries were not timed on some workloads.
func (rep *report) writeSkips(w io.Writer) {
	if len(rep.skips) == 0 {
		return
	}
	_, _ = fmt.Fprintln(w, "\n## Not timed")
	_, _ = fmt.Fprintln(w)
	rows := [][]string{{"op", "workload", "library", "reason"}}
	for _, op := range rep.ops {
		for _, work := range rep.works[op] {
			for _, lib := range rep.libs[op] {
				// A library with no decoder or no encoder has no column.
				reason, ok := rep.skips[cell{op, work, lib}]
				if !ok || reason == "no decoder" || reason == "no encoder" {
					continue
				}
				if utf8.RuneCountInString(reason) > 70 {
					reason = string([]rune(reason)[:69]) + "…"
				}
				reason = strings.ReplaceAll(reason, "|", `\|`)
				rows = append(rows, []string{op, work, lib, reason})
			}
		}
	}
	table(w, rows)
}

// gap returns how many times have is larger than best.
func gap(have, best float64) float64 {
	if best == 0 {
		return have + 1e9
	}
	return have / best
}

// ratio formats how many times have is larger than best.
func ratio(have, best float64) string {
	if best == 0 {
		return "∞"
	}
	return fmt.Sprintf("%.1f×", have/best)
}

// number formats val with thousands separators and one decimal below 100.
func number(val float64) string {
	if val < 100 && val != float64(int64(val)) {
		return strconv.FormatFloat(val, 'f', 1, 64)
	}
	txt := strconv.FormatInt(int64(val+0.5), 10)
	for i := len(txt) - 3; i > 0; i -= 3 {
		txt = txt[:i] + "," + txt[i:]
	}
	return txt
}

// table prints rows as a Markdown table, the first row as its header, with
// every column padded to its widest cell.
func table(w io.Writer, rows [][]string) {
	widths := make([]int, len(rows[0]))
	for _, row := range rows {
		for i, txt := range row {
			widths[i] = max(widths[i], utf8.RuneCountInString(txt))
		}
	}
	line := func(row []string) {
		var sb strings.Builder
		for i, txt := range row {
			pad := widths[i] - utf8.RuneCountInString(txt)
			sb.WriteString("| " + txt + strings.Repeat(" ", pad) + " ")
		}
		_, _ = fmt.Fprintln(w, sb.String()+"|")
	}
	line(rows[0])
	dashes := make([]string, len(widths))
	for i, width := range widths {
		dashes[i] = strings.Repeat("-", width)
	}
	_, _ = fmt.Fprintln(w, "|-"+strings.Join(dashes, "-|-")+"-|")
	for _, row := range rows[1:] {
		line(row)
	}
}
