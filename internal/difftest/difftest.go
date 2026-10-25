// SPDX-FileCopyrightText: (c) 2026 Rafal Zajac
// SPDX-License-Identifier: MIT

// Package difftest checks that xbind decodes and encodes random inputs as
// package ref, a copy of xbind before its decode and encode shortcuts, does:
// the same values, the same errors. It is a module of its own, so
// "go test ./..." run from the repository root does not reach it.
//
// Run from this directory:
//
//	go test ./...
//
// A behavior change made to xbind on purpose fails the tests until package
// ref gets the same change.
package difftest
