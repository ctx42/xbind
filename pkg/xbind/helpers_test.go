// SPDX-FileCopyrightText: (c) 2026 Rafal Zajac
// SPDX-License-Identifier: MIT

package xbind

import (
	"bytes"
	"encoding"
	"math"
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

func Test_fieldPath_tabular(t *testing.T) {
	tt := []struct {
		testN string

		pref string
		name string
		want string
	}{
		{"name", "", "a", "a"},
		{"prefixed name", "p", "a", "p.a"},
		{"dotted name", "", "a.b", "[a.b]"},
		{"prefixed dotted name", "p", "a.b", "p[a.b]"},
	}

	for _, tc := range tt {
		t.Run(tc.testN, func(t *testing.T) {
			// --- When ---
			have := fieldPath(tc.pref, tc.name)

			// --- Then ---
			assert.Equal(t, tc.want, have)
		})
	}
}

func Test_keyText_tabular(t *testing.T) {
	nilItf := reflect.ValueOf(map[any]int{(*int)(nil): 1}).MapKeys()[0]

	tt := []struct {
		testN string

		key    reflect.Value
		format string
		want   string
	}{
		{"int", reflect.ValueOf(7), "%v", "7"},
		{"text marshaler", reflect.ValueOf(TTextVal('A')), "%#v", "A"},
		{"no encoder", reflect.ValueOf(TItem{Size: 1}), "%v", "{1 }"},
		{
			"no encoder Go syntax",
			reflect.ValueOf(TItem{Size: 1}),
			"%#v",
			`xbind.TItem{Size:1, Name:""}`,
		},
		{"nil pointer", reflect.ValueOf((*int)(nil)), "%v", "<nil>"},
		{"nil pointer in interface", nilItf, "%v", "<nil>"},
	}

	for _, tc := range tt {
		t.Run(tc.testN, func(t *testing.T) {
			// --- When ---
			have := keyText(tc.key, tc.format)

			// --- Then ---
			assert.Equal(t, tc.want, have)
		})
	}
}

func Test_isNaN_tabular(t *testing.T) {
	tt := []struct {
		testN string

		val  any
		want bool
	}{
		{"NaN", math.NaN(), true},
		{"float32 NaN", float32(math.NaN()), true},
		{"pointer to NaN", new(math.NaN()), true},
		{"number", 1.5, false},
		{"not a float", "NaN", false},
		{"nil pointer", (*float64)(nil), false},
	}

	for _, tc := range tt {
		t.Run(tc.testN, func(t *testing.T) {
			// --- When ---
			have := isNaN(reflect.ValueOf(tc.val))

			// --- Then ---
			assert.Equal(t, tc.want, have)
		})
	}
}

func Test_isNaNKey_tabular(t *testing.T) {
	tt := []struct {
		testN string

		val  any
		want bool
	}{
		{"NaN", math.NaN(), true},
		{"pointer to NaN", new(math.NaN()), true},
		{"struct holding NaN", TFltKey{F: math.NaN()}, true},
		{"array holding NaN", [1]float64{math.NaN()}, true},
		{"number", 1.5, false},
		{"struct", TFltKey{F: 1}, false},
	}

	for _, tc := range tt {
		t.Run(tc.testN, func(t *testing.T) {
			// --- When ---
			have := isNaNKey(reflect.ValueOf(tc.val))

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

	t.Run("IsZero method on the address", func(t *testing.T) {
		// --- Given ---
		val := reflect.ValueOf(&TPtrZero{N: -1}).Elem()

		// --- When ---
		have := isZero(val)

		// --- Then ---
		assert.True(t, have)
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
		seen := map[uintptr]bool{}

		// --- When ---
		have := hasNilEmbedded(reflect.ValueOf(*cyc), seen)

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
		{
			"nil embed in an embedded interface",
			struct{ encoding.TextMarshaler }{TPromZero{}},
			true,
		},
		{
			"nil pointer in an embedded interface",
			struct{ encoding.TextMarshaler }{(*TTextVal)(nil)},
			true,
		},
		{
			"embedded interface",
			struct{ encoding.TextMarshaler }{TTextVal('A')},
			false,
		},
	}

	for _, tc := range tt {
		t.Run(tc.testN, func(t *testing.T) {
			// --- Given ---
			seen := map[uintptr]bool{}

			// --- When ---
			have := hasNilEmbedded(reflect.ValueOf(tc.val), seen)

			// --- Then ---
			assert.Equal(t, tc.want, have)
		})
	}
}

func Test_nilPromoted_tabular(t *testing.T) {
	tt := []struct {
		testN string

		val  any
		name string
		want bool
	}{
		{"own method", TOwnText{}, "MarshalText", false},
		{"value method through nil pointer", TPromZero{}, "MarshalText", true},
		{
			"value method through pointer",
			TPromZero{Time: &time.Time{}},
			"MarshalText",
			false,
		},
		{"pointer to struct", &TPromZero{}, "MarshalText", true},
		{"past nil pointer", struct{ *TPromZero }{}, "MarshalText", true},
		{
			"pointer method through nil pointer",
			struct{ *TStructMar }{},
			"MarshalValues",
			false,
		},
		{
			"nil interface",
			struct{ encoding.TextMarshaler }{},
			"MarshalText",
			true,
		},
		{
			"interface",
			struct{ encoding.TextMarshaler }{TTextVal('A')},
			"MarshalText",
			false,
		},
		{
			"nil embed in an interface",
			struct{ encoding.TextMarshaler }{TPromZero{}},
			"MarshalText",
			true,
		},
		{
			"nil value receiver in an interface",
			struct{ encoding.TextMarshaler }{(*TTextVal)(nil)},
			"MarshalText",
			true,
		},
		{
			"nil pointer receiver in an interface",
			struct{ encoding.TextMarshaler }{(*TTextPtr)(nil)},
			"MarshalText",
			false,
		},
		{"no method", TItem{}, "MarshalText", false},
		{"not a struct", 1, "MarshalText", false},
	}

	for _, tc := range tt {
		t.Run(tc.testN, func(t *testing.T) {
			// --- When ---
			have := nilPromoted(reflect.ValueOf(tc.val), tc.name)

			// --- Then ---
			assert.Equal(t, tc.want, have)
		})
	}
}

func Test_methodPath_tabular(t *testing.T) {
	tt := []struct {
		testN string

		typ    reflect.Type
		wPath  []int
		wFound bool
	}{
		{"declared", reflect.TypeFor[TOwnText](), nil, true},
		{"promoted", reflect.TypeFor[TPromZero](), []int{0}, true},
		{
			"pointer to struct",
			reflect.TypeFor[*struct{ TPromZero }](),
			[]int{0, 0},
			true,
		},
		{
			"shallowest wins",
			reflect.TypeFor[struct {
				TPromZero
				*TTextPtr
			}](),
			[]int{1},
			true,
		},
		{
			"ambiguous",
			reflect.TypeFor[struct {
				tTextA
				tTextB
			}](),
			nil,
			false,
		},
		{"recursive type", reflect.TypeFor[TCycle](), nil, false},
		{"no method", reflect.TypeFor[TItem](), nil, false},
	}

	for _, tc := range tt {
		t.Run(tc.testN, func(t *testing.T) {
			// --- Given ---
			seen := map[reflect.Type]bool{}

			// --- When ---
			have, ok := methodPath(tc.typ, "MarshalText", seen)

			// --- Then ---
			assert.Equal(t, tc.wPath, have)
			assert.Equal(t, tc.wFound, ok)
			assert.Len(t, 0, seen)
		})
	}
}

func Test_declares_tabular(t *testing.T) {
	tt := []struct {
		testN string

		typ  reflect.Type
		want bool
	}{
		{"value receiver", reflect.TypeFor[TTextVal](), true},
		{"pointer receiver", reflect.TypeFor[TTextPtr](), true},
		{"promoted", reflect.TypeFor[TPromZero](), false},
		{"generic type", reflect.TypeFor[TGenText[int]](), true},
		{"interface", reflect.TypeFor[encoding.TextMarshaler](), true},
		{"none", reflect.TypeFor[TItem](), false},
	}

	for _, tc := range tt {
		t.Run(tc.testN, func(t *testing.T) {
			// --- When ---
			have := declares(tc.typ, "MarshalText")

			// --- Then ---
			assert.Equal(t, tc.want, have)
		})
	}
}

func Test_isEmpty_tabular(t *testing.T) {
	var nilPtr *int
	itf := reflect.ValueOf(&struct{ I any }{I: ""}).Elem().Field(0)
	itfFull := reflect.ValueOf(&struct{ I any }{I: "a"}).Elem().Field(0)
	unexp := reflect.ValueOf(struct{ t time.Time }{}).Field(0)
	var self TRecPtr
	self = &self
	var selfItf any
	selfItf = &selfItf

	tt := []struct {
		testN string

		val  reflect.Value
		want bool
	}{
		{"invalid", reflect.Value{}, true},
		{"nil pointer", reflect.ValueOf(nilPtr), true},
		{"nil map", reflect.ValueOf(map[string]int(nil)), true},
		{"nil slice", reflect.ValueOf([]int(nil)), true},
		{"nil func", reflect.ValueOf((func())(nil)), true},
		{"func", reflect.ValueOf(func() {}), false},
		{"nil chan", reflect.ValueOf((chan int)(nil)), true},
		{"empty chan", reflect.ValueOf(make(chan int, 1)), true},
		{"zero IsZero", reflect.ValueOf(time.Time{}), true},
		{"IsZero", reflect.ValueOf(time.Unix(1, 0)), false},
		{"nil embedded IsZero", reflect.ValueOf(TPromZero{}), true},
		{"pointer to nil embedded", reflect.ValueOf(&TPromZero{}), true},
		{
			"embedded IsZero",
			reflect.ValueOf(TPromZero{new(time.Unix(1, 0))}),
			false,
		},
		{"empty Valuer", reflect.ValueOf(TValuer("")), true},
		{"Valuer", reflect.ValueOf(TValuer("a")), false},
		{"Valuer error", reflect.ValueOf(TValuer("error")), false},
		{"empty Valuer of itself", reflect.ValueOf(TSelfValuer("")), true},
		{"Valuer of itself", reflect.ValueOf(TSelfValuer("a")), false},
		{"unexported zero struct", unexp, true},
		{"false", reflect.ValueOf(false), false},
		{"empty string", reflect.ValueOf(""), true},
		{"string", reflect.ValueOf("a"), false},
		{"empty array", reflect.ValueOf([0]int{}), true},
		{"array", reflect.ValueOf([1]int{}), false},
		{"empty map", reflect.ValueOf(map[string]int{}), true},
		{"empty slice", reflect.ValueOf([]int{}), true},
		{"pointer to empty", reflect.ValueOf(new("")), true},
		{"pointer", reflect.ValueOf(new("a")), false},
		{"interface to empty", itf, true},
		{"interface", itfFull, false},
		{"pointer to itself", reflect.ValueOf(self), false},
		{
			"interface holding its address",
			reflect.ValueOf(&selfItf).Elem(),
			false,
		},
		{"zero int", reflect.ValueOf(0), true},
		{"int", reflect.ValueOf(1), false},
		{"zero float", reflect.ValueOf(0.0), true},
		{"zero struct", reflect.ValueOf(TRequired{}), true},
		{"struct", reflect.ValueOf(TRequired{FInt: 1}), false},
	}

	for _, tc := range tt {
		t.Run(tc.testN, func(t *testing.T) {
			// --- When ---
			have := isEmpty(tc.val)

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

func Test_baseType_tabular(t *testing.T) {
	tt := []struct {
		testN string

		typ  reflect.Type
		want reflect.Type
	}{
		{"value", reflect.TypeFor[int](), reflect.TypeFor[int]()},
		{"pointer", reflect.TypeFor[**int](), reflect.TypeFor[int]()},
		{"slice", reflect.TypeFor[[]TItem](), reflect.TypeFor[TItem]()},
		{"array", reflect.TypeFor[[2]*TItem](), reflect.TypeFor[TItem]()},
		{"map", reflect.TypeFor[map[int]TItem](), reflect.TypeFor[TItem]()},
		{
			"nested",
			reflect.TypeFor[*map[string][]*[2]TItem](),
			reflect.TypeFor[TItem](),
		},
		{
			"recursive map",
			reflect.TypeFor[TRecMap](),
			reflect.TypeFor[TRecMap](),
		},
		{
			"recursive slice",
			reflect.TypeFor[*TRecSlice](),
			reflect.TypeFor[TRecSlice](),
		},
		{
			"recursive pointer",
			reflect.TypeFor[TRecPtr](),
			reflect.TypeFor[TRecPtr](),
		},
	}

	for _, tc := range tt {
		t.Run(tc.testN, func(t *testing.T) {
			// --- When ---
			have := baseType(tc.typ)

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
