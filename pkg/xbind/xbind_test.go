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

func Test_ErrUnkKeyPath_Error(t *testing.T) {
	// --- When ---
	have := ErrUnkKeyPath("path").Error()

	// --- Then ---
	assert.Equal(t, "invalid field path", have)
}

func Test_ErrUnkKeyPath_ErrorCode(t *testing.T) {
	// --- When ---
	have := ErrUnkKeyPath("path").ErrorCode()

	// --- Then ---
	assert.Equal(t, "ECUnkFieldPath", have)
}

func Test_ErrUnkKeyPath_Format_tabular(t *testing.T) {
	tt := []struct {
		testN string

		format string
		want   string
	}{
		{"v", "%v", "invalid field path"},
		{"plus v", "%+v", "invalid field path (ECUnkFieldPath)"},
		{"s", "%s", "invalid field path"},
		{"q", "%q", `"invalid field path"`},
	}

	for _, tc := range tt {
		t.Run(tc.testN, func(t *testing.T) {
			// --- When ---
			have := fmt.Sprintf(tc.format, ErrUnkKeyPath("path"))

			// --- Then ---
			assert.Equal(t, tc.want, have)
		})
	}
}

func Test_ErrDecNotFound_Error(t *testing.T) {
	// --- When ---
	have := ErrDecNotFound("type").Error()

	// --- Then ---
	assert.Equal(t, "decoder not found for `type`", have)
}

func Test_ErrDecNotFound_ErrorCode(t *testing.T) {
	// --- When ---
	have := ErrDecNotFound("type").ErrorCode()

	// --- Then ---
	assert.Equal(t, "ECDecoderNotFound", have)
}

func Test_ErrDecNotFound_Format_tabular(t *testing.T) {
	tt := []struct {
		testN string

		format string
		want   string
	}{
		{"v", "%v", "decoder not found for `type`"},
		{"plus v", "%+v", "decoder not found for `type` (ECDecoderNotFound)"},
		{"s", "%s", "decoder not found for `type`"},
		{"q", "%q", "\"decoder not found for `type`\""},
	}

	for _, tc := range tt {
		t.Run(tc.testN, func(t *testing.T) {
			// --- When ---
			have := fmt.Sprintf(tc.format, ErrDecNotFound("type"))

			// --- Then ---
			assert.Equal(t, tc.want, have)
		})
	}
}

func Test_ErrEncNotFound_Error(t *testing.T) {
	// --- When ---
	have := ErrEncNotFound("type").Error()

	// --- Then ---
	assert.Equal(t, "encoder not found for `type`", have)
}

func Test_ErrEncNotFound_ErrorCode(t *testing.T) {
	// --- When ---
	have := ErrEncNotFound("type").ErrorCode()

	// --- Then ---
	assert.Equal(t, "ECEncoderNotFound", have)
}

func Test_ErrEncNotFound_Format_tabular(t *testing.T) {
	tt := []struct {
		testN string

		format string
		want   string
	}{
		{"v", "%v", "encoder not found for `type`"},
		{"plus v", "%+v", "encoder not found for `type` (ECEncoderNotFound)"},
		{"s", "%s", "encoder not found for `type`"},
		{"q", "%q", "\"encoder not found for `type`\""},
	}

	for _, tc := range tt {
		t.Run(tc.testN, func(t *testing.T) {
			// --- When ---
			have := fmt.Sprintf(tc.format, ErrEncNotFound("type"))

			// --- Then ---
			assert.Equal(t, tc.want, have)
		})
	}
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
