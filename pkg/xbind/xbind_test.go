// SPDX-FileCopyrightText: (c) 2026 Rafal Zajac
// SPDX-License-Identifier: MIT

package xbind

import (
	"fmt"
	"testing"

	"github.com/ctx42/testing/pkg/assert"
)

func Test_ErrReqStructPtr(t *testing.T) {
	// --- When ---
	have := fmt.Sprintf("%+v", ErrReqStructPtr)

	// --- Then ---
	assert.Equal(t, "struct pointer required (ECReqStructPtr)", have)
}

func Test_ErrDecNotFound(t *testing.T) {
	// --- When ---
	have := fmt.Sprintf("%+v", ErrDecNotFound)

	// --- Then ---
	assert.Equal(t, "decoder not found (ECDecNotFound)", have)
}

func Test_ErrEncNotFound(t *testing.T) {
	// --- When ---
	have := fmt.Sprintf("%+v", ErrEncNotFound)

	// --- Then ---
	assert.Equal(t, "encoder not found (ECEncNotFound)", have)
}
