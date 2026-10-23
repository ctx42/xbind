// SPDX-FileCopyrightText: (c) 2026 Rafal Zajac
// SPDX-License-Identifier: MIT

package xbindtest

import (
	"net/url"
	"testing"

	"github.com/ctx42/testing/pkg/assert"
)

func Test_MustEncode(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		// --- Given ---
		s := struct{ FStr string }{FStr: "abc"}

		// --- When ---
		have := MustEncode(s)

		// --- Then ---
		assert.Equal(t, url.Values{"FStr": {"abc"}}, have)
	})

	t.Run("error - no encoder", func(t *testing.T) {
		// --- Given ---
		s := struct{ Fn func() }{Fn: func() {}}

		// --- When ---
		have := assert.PanicMsg(t, func() { MustEncode(s) })

		// --- Then ---
		assert.Equal(t, "Fn: encoder not found for `func()`", *have)
	})
}

func Test_MustDecode(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		// --- Given ---
		s := &struct{ FStr string }{}
		vs := url.Values{"FStr": {"abc"}}

		// --- When ---
		MustDecode(s, vs)

		// --- Then ---
		assert.Equal(t, "abc", s.FStr)
	})

	t.Run("error - not pointer", func(t *testing.T) {
		// --- Given ---
		s := struct{ FStr string }{}
		vs := url.Values{"FStr": {"abc"}}

		// --- When ---
		have := assert.PanicMsg(t, func() { MustDecode(s, vs) })

		// --- Then ---
		assert.Equal(t, "struct pointer required", *have)
	})
}
