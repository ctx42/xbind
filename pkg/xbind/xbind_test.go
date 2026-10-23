// SPDX-FileCopyrightText: (c) 2026 Rafal Zajac
// SPDX-License-Identifier: MIT

package xbind

import (
	"fmt"
	"testing"

	"github.com/ctx42/testing/pkg/assert"
	"github.com/ctx42/xrr/pkg/xrr"
)

func Test_ErrReqStructPtr(t *testing.T) {
	// --- When ---
	have := fmt.Sprintf("%+v", ErrReqStructPtr)

	// --- Then ---
	assert.Equal(t, "struct pointer required (ECReqStructPtr)", have)
}

func Test_EncloseError(t *testing.T) {
	t.Run("enclose", func(t *testing.T) {
		// --- When ---
		have := EncloseError(errTest)

		// --- Then ---
		var xe xrr.Envelope
		assert.Type(t, &xe, have)
		assert.Same(t, errTest, xe.Unwrap())
		assert.ErrorIs(t, ErrBind, have)
	})

	t.Run("nil error", func(t *testing.T) {
		// --- When ---
		have := EncloseError(nil)

		// --- Then ---
		assert.Nil(t, have)
	})
}
