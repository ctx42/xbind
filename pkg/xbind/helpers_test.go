// SPDX-FileCopyrightText: (c) 2026 Rafal Zajac
// SPDX-License-Identifier: MIT

package xbind

import (
	"bytes"
	"reflect"
	"testing"
	"time"

	"github.com/ctx42/testing/pkg/assert"
)

func Test_isNil_tabular(t *testing.T) {
	var mar Marshaler
	var pts *TString

	tt := []struct {
		testN string

		val  reflect.Value
		want bool
	}{
		{"invalid", reflect.Value{}, false},
		{"nil interface", reflect.ValueOf(&mar).Elem(), true},
		{"nil pointer", reflect.ValueOf(pts), true},
		{"pointer", reflect.ValueOf(&TString{}), false},
		{"interface", reflect.ValueOf(&[]any{1}).Elem().Index(0), false},
		{"nil map", reflect.ValueOf(map[int]int(nil)), false},
		{"value", reflect.ValueOf(1), false},
	}

	for _, tc := range tt {
		t.Run(tc.testN, func(t *testing.T) {
			// --- When ---
			have := isNil(tc.val)

			// --- Then ---
			assert.Equal(t, tc.want, have)
		})
	}
}

func Test_buildPath_tabular(t *testing.T) {
	tt := []struct {
		testN string

		pref string
		name string
		want string
	}{
		{"both empty", "", "", ""},
		{"empty prefix", "", "name", "name"},
		{"empty name", "pref", "", "pref"},
		{"both set", "pref", "name", "pref.name"},
	}

	for _, tc := range tt {
		t.Run(tc.testN, func(t *testing.T) {
			// --- When ---
			have := buildPath(tc.pref, tc.name)

			// --- Then ---
			assert.Equal(t, tc.want, have)
		})
	}
}

func Test_isZero(t *testing.T) {
	t.Run("struct read through unexported field", func(t *testing.T) {
		// --- Given ---
		s := &struct{ t TPtrZero }{TPtrZero{N: -1}}

		// --- When ---
		have := isZero(reflect.ValueOf(s).Elem().Field(0))

		// --- Then ---
		assert.False(t, have)
	})
}

func Test_isZero_tabular(t *testing.T) {
	var dmap map[string]string
	var dstr string
	var dfn func()

	stra := StrAlias("abc")

	tt := []struct {
		testN string

		val  any
		want bool
	}{
		{"func declaration", dfn, true},
		{"func inline", func() {}, false},

		{"map declaration", dmap, true},
		{"map inline", map[string]string{}, true},
		{"map non-zero", map[string]string{"a": "b"}, false},

		{"StrAlias slice", []StrAlias{}, true},
		{"StrAlias pointer slice", []*StrAlias{}, true},
		{"StrAlias pointer slice non-zero", []*StrAlias{&stra}, false},

		{"array", [4]int{}, true},
		{"array of pointers", [4]*int{}, true},
		{"array with zeros", [...]int{0, 0}, true},
		{"array non-zero", [...]int{0, 1}, false},

		{"IsZero implementor", time.Time{}, true},
		{"IsZero implementor non-zero", time.Now(), false},
		{"IsZero method wins", time.Time{}.In(waw), true},
		{"array of empty slices", [1][]int{{}}, true},
		{"struct with empty slice", struct{ S []int }{[]int{}}, true},

		{"base declaration", dstr, true},
		{"base non-zero", "abc", false},
		{"struct unexported fields", struct{ v int }{}, true},
		{"struct unexported fields non-zero", struct{ v int }{1}, false},
	}

	for _, tc := range tt {
		t.Run(tc.testN, func(t *testing.T) {
			// --- When ---
			have := isZero(reflect.ValueOf(tc.val))

			// --- Then ---
			assert.Equal(t, tc.want, have)
		})
	}
}

func Test_isZeroFields(t *testing.T) {
	t.Run("own IsZero method ignored", func(t *testing.T) {
		// --- Given ---
		val := reflect.ValueOf(&TPtrZero{N: -1}).Elem()

		// --- When ---
		have := isZeroFields(val)

		// --- Then ---
		assert.False(t, have)

		assert.True(t, isZero(val))
	})
}

func Test_isZeroFields_tabular(t *testing.T) {
	tt := []struct {
		testN string

		val  any
		want bool
	}{
		{"zero", TRequired{}, true},
		{"non-zero", TRequired{FInt: 1}, false},
	}

	for _, tc := range tt {
		t.Run(tc.testN, func(t *testing.T) {
			// --- When ---
			have := isZeroFields(reflect.ValueOf(tc.val))

			// --- Then ---
			assert.Equal(t, tc.want, have)
		})
	}
}

func Test_hasNilEmbedded(t *testing.T) {
	t.Run("embedding cycle", func(t *testing.T) {
		// --- Given ---
		cyc := &TCycle{}
		cyc.TCycle = cyc

		// --- When ---
		have := hasNilEmbedded(reflect.ValueOf(*cyc), map[uintptr]bool{})

		// --- Then ---
		assert.False(t, have)
	})
}

func Test_hasNilEmbedded_tabular(t *testing.T) {
	tt := []struct {
		testN string

		val  any
		want bool
	}{
		{"nil embedded pointer", TPromZero{}, true},
		{"embedded pointer", TPromZero{Time: &time.Time{}}, false},
		{"embedded value", Anonymous{}, false},
		{"nil named pointer", struct{ T *time.Time }{}, false},
		{"deep nil embedded pointer", struct{ TPromZero }{}, true},
		{
			"nil pointer behind embedded pointer",
			struct{ *TPromZero }{&TPromZero{}},
			true,
		},
		{
			"pointer behind embedded pointer",
			struct{ *TPromZero }{&TPromZero{Time: &time.Time{}}},
			false,
		},
	}

	for _, tc := range tt {
		t.Run(tc.testN, func(t *testing.T) {
			// --- When ---
			have := hasNilEmbedded(reflect.ValueOf(tc.val), map[uintptr]bool{})

			// --- Then ---
			assert.Equal(t, tc.want, have)
		})
	}
}

func Test_indirect_tabular(t *testing.T) {
	tt := []struct {
		testN string

		val  any
		want reflect.Type
	}{
		{"pointer to struct", &bytes.Buffer{}, reflect.TypeFor[bytes.Buffer]()},
		{"pointer to pointer", new(new(1)), reflect.TypeFor[*int]()},
		{"slice", []string{"abc"}, reflect.TypeFor[[]string]()},
		{
			"map",
			map[string]string{"key": "abc"},
			reflect.TypeFor[map[string]string](),
		},
		{"string", "abc", reflect.TypeFor[string]()},
	}

	for _, tc := range tt {
		t.Run(tc.testN, func(t *testing.T) {
			// --- When ---
			have := indirect(reflect.TypeOf(tc.val))

			// --- Then ---
			assert.Equal(t, tc.want, have)
		})
	}
}

func Test_addressable(t *testing.T) {
	// --- Given ---
	m := reflect.ValueOf(map[string]int{"a": 1})
	val := m.MapIndex(reflect.ValueOf("a"))

	// --- When ---
	have := addressable(val)

	// --- Then ---
	assert.True(t, have.CanAddr())
	assert.Equal(t, 1, have.Interface())

	assert.False(t, val.CanAddr())
}
