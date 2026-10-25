// SPDX-FileCopyrightText: (c) 2026 Rafal Zajac
// SPDX-License-Identifier: MIT

// Package bench compares the speed and memory use of xbind with other Go form
// libraries, decoding and encoding the same workloads. It is a module of its
// own, so the libraries it pins in its go.mod stay out of the xbind module;
// "go test ./..." run from the repository root does not reach it.
//
// Run from this directory:
//
//	go test -run '^$' -bench . -benchmem -count 5 -v > ../../tmp/bench.txt
//	go run report.go < ../../tmp/bench.txt
//
// The report prints, for decoding and encoding, a table per metric (ns/op,
// B/op, allocs/op) with each library's median and its ratio to xbind, then
// the cases where a library beats xbind, largest gap first, and why each
// skipped library was not timed; it needs the -v output for the reasons.
//
// Each library gets the workload's keys in its own syntax (dotted, bracketed
// or bare) and runs with its default options. A benchmark first checks that
// the library decodes the keys to the expected value, or encodes the value to
// the expected keys, and skips the library when it does not, so a library is
// timed only on work it gets right. Decoding allocates a new destination per
// operation for every library.
package bench
