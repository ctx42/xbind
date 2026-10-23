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

func Test_ErrReqStruct(t *testing.T) {
	// --- When ---
	have := fmt.Sprintf("%+v", ErrReqStruct)

	// --- Then ---
	want := "struct or struct pointer required (ECReqStruct)"
	assert.Equal(t, want, have)
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

	t.Run("error - malformed tag of slice element struct", func(t *testing.T) {
		// --- Given ---
		elm := reflect.TypeOf(badTag(`url:n`, false)).Elem()
		typ := reflect.StructOf([]reflect.StructField{
			{Name: "L", Type: reflect.SliceOf(elm)},
		})

		// --- When ---
		err := checkTags("", typ, "url", map[reflect.Type]bool{})

		// --- Then ---
		assert.ErrorEqual(t, "invalid struct tag: L.N", err)
	})

	t.Run("error - bracket in name of map element struct", func(t *testing.T) {
		// --- Given ---
		type item struct {
			A string `url:"a[b]"`
		}
		typ := reflect.TypeFor[struct{ M map[string][]*item }]()

		// --- When ---
		err := checkTags("", typ, "url", map[reflect.Type]bool{})

		// --- Then ---
		assert.ErrorEqual(t, "invalid struct tag: M.A: bracket in name", err)
	})

	t.Run("error - name on embedded struct", func(t *testing.T) {
		// --- Given ---
		typ := reflect.TypeFor[struct {
			*TItem `url:"item"`
		}]()

		// --- When ---
		err := checkTags("", typ, "url", map[reflect.Type]bool{})

		// --- Then ---
		want := "invalid struct tag: TItem: name on embedded struct"
		assert.ErrorEqual(t, want, err)
		assert.ErrorIs(t, ErrInvTag, err)
	})

	t.Run("options on embedded struct", func(t *testing.T) {
		// --- Given ---
		typ := reflect.TypeFor[struct {
			TItem `url:",omitempty"`
		}]()

		// --- When ---
		err := checkTags("", typ, "url", map[reflect.Type]bool{})

		// --- Then ---
		assert.NoError(t, err)
	})

	t.Run("name on embedded value", func(t *testing.T) {
		// --- Given ---
		typ := reflect.TypeFor[struct {
			TLevel `url:"level"`
		}]()

		// --- When ---
		err := checkTags("", typ, "url", map[reflect.Type]bool{})

		// --- Then ---
		assert.NoError(t, err)
	})
}

func Test_checkTags_tabular(t *testing.T) {
	ignored := reflect.StructOf([]reflect.StructField{{
		Name: "F",
		Type: reflect.TypeOf(badTag(`url:n`, false)),
		Tag:  `url:"-"`,
	}})
	unexported := reflect.StructOf([]reflect.StructField{{
		Name:    "n",
		PkgPath: "github.com/ctx42/xbind/pkg/xbind",
		Type:    reflect.TypeFor[string](),
		Tag:     `url:n`,
	}})

	tt := []struct {
		testN string

		typ reflect.Type
	}{
		{"valid", reflect.TypeFor[TString]()},
		{"pointer", reflect.TypeFor[*TMultiEmbedded]()},
		{"recursive", reflect.TypeFor[TNode]()},
		{"recursive map", reflect.TypeFor[struct{ M TRecMap }]()},
		{"unexported embedded struct", reflect.TypeFor[TNotExpEmb]()},
		{"unexported field", unexported},
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

func Test_newLevel_tabular(t *testing.T) {
	tt := []struct {
		testN string

		typ  reflect.Type
		want map[string][]int
	}{
		{
			"fields",
			reflect.TypeFor[TNode](),
			map[string][]int{"val": {0}, "req": {1}, "next": {2}},
		},
		{
			"outer field shadows embedded",
			reflect.TypeFor[*struct {
				TRequired
				FStr string `url:"f_str"`
			}](),
			map[string][]int{"f_str": {1}, "f_int": {0, 1}},
		},
		{"ambiguous", reflect.TypeFor[TAmbig](), map[string][]int{"a": {0, 1}}},
		{
			"tagged wins",
			reflect.TypeFor[TTagged](),
			map[string][]int{"X": {0, 0}},
		},
		{"same type twice", reflect.TypeFor[TDouble](), map[string][]int{}},
		{"embedding cycle", reflect.TypeFor[TCycle](), map[string][]int{}},
		{
			"skipped fields",
			reflect.TypeFor[struct {
				fmt.Stringer
				A string `url:"-"`
				b string
			}](),
			map[string][]int{},
		},
	}

	for _, tc := range tt {
		t.Run(tc.testN, func(t *testing.T) {
			// --- When ---
			have := newLevel(tc.typ, "url")

			// --- Then ---
			assert.Equal(t, tc.want, have.vis)
			assert.Nil(t, have.idx)
		})
	}
}

func Test_level_names(t *testing.T) {
	// --- Given ---
	lvl := level{vis: map[string][]int{"a": {0, 1}}, idx: []int{0}}

	// --- When ---
	have := lvl.names("a", 1)

	// --- Then ---
	assert.True(t, have)
	assert.False(t, lvl.names("a", 0))
	assert.False(t, lvl.names("b", 1))
}

func Test_level_embedded(t *testing.T) {
	// --- Given ---
	lvl := level{vis: map[string][]int{"a": {0, 1}}, idx: []int{0}}

	// --- When ---
	have := lvl.embedded(1)

	// --- Then ---
	assert.Equal(t, []int{0, 1}, have.idx)
	assert.Same(t, lvl.vis, have.vis)
	assert.Equal(t, []int{0}, lvl.idx)
}
