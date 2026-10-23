// SPDX-FileCopyrightText: (c) 2026 Rafal Zajac
// SPDX-License-Identifier: MIT

package xbind

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/ctx42/testing/pkg/assert"
)

func Test_ErrReqStructPtr(t *testing.T) {
	// --- When ---
	have := fmt.Sprintf("%+v", ErrReqStructPtr)

	// --- Then ---
	assert.Equal(t, "struct pointer required (ECReqStructPtr)", have)
}

func Test_ErrNilDst(t *testing.T) {
	// --- When ---
	have := fmt.Sprintf("%+v", ErrNilDst)

	// --- Then ---
	assert.Equal(t, "nil destination (ECNilDst)", have)
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

func Test_checkTags(t *testing.T) {
	t.Run("error - malformed tag", func(t *testing.T) {
		// --- Given ---
		typ := reflect.TypeOf(badTag(`url:n`, false))

		// --- When ---
		err := checkTags("", typ, "url", map[reflect.Type]bool{})

		// --- Then ---
		assert.ErrorEqual(t, "invalid struct tag: N", err)
		assert.ErrorIs(t, ErrInvTag, err)
	})

	t.Run("error - malformed tag of nested struct", func(t *testing.T) {
		// --- Given ---
		typ := reflect.TypeOf(badTag(`url:n`, true))

		// --- When ---
		err := checkTags("p", typ, "url", map[reflect.Type]bool{})

		// --- Then ---
		assert.ErrorEqual(t, "invalid struct tag: p.F.N", err)
	})

	t.Run("error - malformed tag of other key", func(t *testing.T) {
		// --- Given ---
		typ := reflect.TypeOf(badTag(`url:"n" json:n`, false))

		// --- When ---
		err := checkTags("", typ, "url", map[reflect.Type]bool{})

		// --- Then ---
		assert.ErrorEqual(t, "invalid struct tag: N", err)
	})
}

func Test_checkTags_tabular(t *testing.T) {
	ignored := reflect.StructOf([]reflect.StructField{{
		Name: "F",
		Type: reflect.TypeOf(badTag(`url:n`, false)),
		Tag:  `url:"-"`,
	}})

	tt := []struct {
		testN string

		typ reflect.Type
	}{
		{"valid", reflect.TypeFor[TString]()},
		{"pointer", reflect.TypeFor[*TMultiEmbedded]()},
		{"recursive", reflect.TypeFor[TNode]()},
		{"unexported field", reflect.TypeFor[TNotExpEmb]()},
		{"ignored field", ignored},
	}

	for _, tc := range tt {
		t.Run(tc.testN, func(t *testing.T) {
			// --- When ---
			err := checkTags("", tc.typ, "url", map[reflect.Type]bool{})

			// --- Then ---
			assert.NoError(t, err)
		})
	}
}
