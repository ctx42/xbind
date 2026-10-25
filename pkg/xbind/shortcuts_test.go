// SPDX-FileCopyrightText: (c) 2026 Rafal Zajac
// SPDX-License-Identifier: MIT

package xbind

import (
	"net/url"
	"reflect"
	"testing"
	"time"

	"github.com/ctx42/testing/pkg/assert"
	"github.com/ctx42/xrr/pkg/xrr"
	"github.com/ctx42/xrr/pkg/xrr/xrrtest"
)

// tFast is a struct for the field-driven decode tests.
type tFast struct {
	A   int            `url:"a"`
	P   *int           `url:"p"`
	L   TLevel         `url:"l"`
	S   []string       `url:"s"`
	In  TItem          `url:"in"`
	Dot int            `url:"x.y"`
	Ptr *TItem         `url:"ptr"`
	M   map[string]int `url:"m"`
}

func Test_Decoder_decodeFast(t *testing.T) {
	t.Run("decodes", func(t *testing.T) {
		// --- Given ---
		dst := &tFast{}
		src := url.Values{
			"a":       {"1"},
			"p":       {"2"},
			"l":       {"2"},
			"s":       {"x", "y"},
			"in.size": {"3"},
			"[x.y]":   {"4"},
		}

		// --- When ---
		ok, err := NewDecoder().decodeFast(reflect.ValueOf(dst).Elem(), src)

		// --- Then ---
		assert.True(t, ok)
		assert.NoError(t, err)
		assert.Equal(t, 1, dst.A)
		assert.Equal(t, 2, *dst.P)
		assert.Equal(t, TLevel(2), dst.L)
		assert.Equal(t, []string{"x", "y"}, dst.S)
		assert.Equal(t, 3, dst.In.Size)
		assert.Equal(t, 4, dst.Dot)
	})

	t.Run("errors", func(t *testing.T) {
		// --- Given ---
		dst := &tFast{}
		src := url.Values{"a": {"x"}, "p": {"y"}, "in.name": {"n"}}

		// --- When ---
		ok, err := NewDecoder().decodeFast(reflect.ValueOf(dst).Elem(), src)

		// --- Then ---
		assert.True(t, ok)
		want := "" +
			"a: invalid value: cannot convert `x` to int (ECInvValue); " +
			"p: invalid value: cannot convert `y` to int (ECInvValue)"
		xrrtest.AssertFieldsEqual(t, want, err)
		assert.Nil(t, dst.P) // A failing key changes nothing.
		assert.Equal(t, "n", dst.In.Name)
	})

	t.Run("error - field errors of a decoder are filtered", func(t *testing.T) {
		// --- Given ---
		dec := NewDecoder(WithDecFunc(func(string) (TPoint, error) {
			empty := xrr.NewFieldErrors(map[string]error{})
			ers := map[string]error{"x": errTest, "y": empty}
			return TPoint{}, xrr.NewFieldErrors(ers)
		}))
		dst := &struct {
			P TPoint `url:"p"`
		}{}
		src := url.Values{"p": {"1,2"}}

		// --- When ---
		ok, err := dec.decodeFast(reflect.ValueOf(dst).Elem(), src)

		// --- Then ---
		assert.True(t, ok)
		have := xrr.GetFields(xrr.GetFields(err)["p"])
		assert.Equal(t, map[string]error{"x": errTest}, have)
	})

	t.Run("many fields for few keys", func(t *testing.T) {
		// --- Given ---
		dst := &TFuzzDec{}
		src := url.Values{"f_str": {"a"}}

		// --- When ---
		ok, err := NewDecoder().decodeFast(reflect.ValueOf(dst).Elem(), src)

		// --- Then ---
		assert.False(t, ok)
		assert.NoError(t, err)
	})
}

func Test_Decoder_decodeFast_tabular(t *testing.T) {
	tt := []struct {
		testN string

		key string
	}{
		{"through a pointer", "ptr.size"},
		{"map entry", "m[k]"},
		{"other spelling", "[a]"},
		{"struct", "in"},
		{"unknown", "unknown"},
	}

	for _, tc := range tt {
		t.Run(tc.testN, func(t *testing.T) {
			// --- Given ---
			dst := &tFast{}
			src := url.Values{"a": {"1"}, tc.key: {"2"}}

			// --- When ---
			ok, err := NewDecoder().decodeFast(reflect.ValueOf(dst).Elem(), src)

			// --- Then ---
			assert.False(t, ok)
			assert.NoError(t, err)
			assert.Equal(t, tFast{}, *dst) // Nothing changed.
		})
	}
}

func Test_Decoder_fastFields(t *testing.T) {
	t.Run("fields", func(t *testing.T) {
		// --- Given ---
		dec := NewDecoder()

		// --- When ---
		have := dec.fastFields(reflect.TypeFor[tFast]())

		// --- Then ---
		keys := map[string][]int{}
		for _, ff := range have {
			keys[ff.key] = ff.idx
		}
		want := map[string][]int{
			"a":       {0},
			"p":       {1},
			"l":       {2},
			"s":       {3},
			"in.size": {4, 0},
			"in.name": {4, 1},
			"[x.y]":   {5},
		}
		assert.Equal(t, want, keys)
	})

	t.Run("cached", func(t *testing.T) {
		// --- Given ---
		dec := NewDecoder()
		want := dec.fastFields(reflect.TypeFor[tFast]())

		// --- When ---
		have := dec.fastFields(reflect.TypeFor[tFast]())

		// --- Then ---
		assert.Same(t, &want[0], &have[0])
	})
}

func Test_Decoder_fastIn(t *testing.T) {
	t.Run("depth limit", func(t *testing.T) {
		// --- Given ---
		dec := NewDecoder(WithDecMaxDepth(1))

		// --- When ---
		have := dec.fastIn(reflect.TypeFor[tFast](), "", nil, 0, nil)

		// --- Then ---
		for _, ff := range have {
			assert.Len(t, 1, ff.idx) // No in.size, two steps deep.
		}
		assert.Len(t, 5, have)
	})

	t.Run("embedded value struct", func(t *testing.T) {
		// --- Given ---
		type T struct {
			TItem
			B int `url:"b"`
		}

		// --- When ---
		have := NewDecoder().fastIn(reflect.TypeFor[T](), "", nil, 0, nil)

		// --- Then ---
		assert.Len(t, 3, have)
	})

	t.Run("embedded pointer", func(t *testing.T) {
		// --- Given ---
		type T struct {
			*TItem
			B int `url:"b"`
		}

		// --- When ---
		have := NewDecoder().fastIn(reflect.TypeFor[T](), "", nil, 0, nil)

		// --- Then ---
		assert.Len(t, 1, have)
		assert.Equal(t, "b", have[0].key)
	})
}

func Test_valuePath_tabular(t *testing.T) {
	type tPtr struct{ *TItem }
	type tVal struct{ TItem }

	tt := []struct {
		testN string

		typ  reflect.Type
		idx  []int
		want bool
	}{
		{"field", reflect.TypeFor[TItem](), []int{0}, true},
		{"through value", reflect.TypeFor[tVal](), []int{0, 0}, true},
		{"through pointer", reflect.TypeFor[tPtr](), []int{0, 0}, false},
	}

	for _, tc := range tt {
		t.Run(tc.testN, func(t *testing.T) {
			// --- When ---
			have := valuePath(tc.typ, tc.idx)

			// --- Then ---
			assert.Equal(t, tc.want, have)
		})
	}
}

func Test_Decoder_direct_tabular(t *testing.T) {
	type tEmb struct {
		E int `url:"e"`
	}
	type T struct {
		tEmb
		A int            `url:"a"`
		M map[string]int `url:"m"`
		S []int          `url:"s"`
		D int            `url:"x.y"`
	}

	tt := []struct {
		testN string

		dec  *Decoder
		key  string
		want []int
	}{
		{"leaf field", NewDecoder(), "a", []int{1}},
		{"slice field", NewDecoder(), "s", []int{3}},
		{"map field", NewDecoder(), "m", nil},
		{"promoted field", NewDecoder(), "e", nil},
		{"unknown field", NewDecoder(), "b", nil},
		{"dotted key", NewDecoder(), "x.y", nil},
		{"bracketed key", NewDecoder(), "[a]", nil},
		{"empty key", NewDecoder(), "", nil},
		{"depth limit 0", NewDecoder(WithDecMaxDepth(0)), "a", nil},
		{"depth limit 1", NewDecoder(WithDecMaxDepth(1)), "a", []int{1}},
	}

	for _, tc := range tt {
		t.Run(tc.testN, func(t *testing.T) {
			// --- Given ---
			stt := newState()
			stt.tc = tc.dec.cache

			// --- When ---
			have, ok := tc.dec.direct(reflect.TypeFor[T](), tc.key, stt)

			// --- Then ---
			assert.Equal(t, tc.want != nil, ok)
			assert.Equal(t, tc.want, have.idx)
		})
	}
}

func Test_Decoder_mapFactsOf_tabular(t *testing.T) {
	tt := []struct {
		testN string

		typ  reflect.Type
		want bool
	}{
		{"string to int", reflect.TypeFor[map[string]int](), true},
		{"int to time", reflect.TypeFor[map[int]time.Time](), true},
		{"float keys", reflect.TypeFor[map[float64]int](), false},
		{"struct values", reflect.TypeFor[map[string]TItem](), false},
		{"pointer values", reflect.TypeFor[map[string]*int](), false},
		{"unmarshaler values", reflect.TypeFor[map[string]TTextPtr](), false},
	}

	for _, tc := range tt {
		t.Run(tc.testN, func(t *testing.T) {
			// --- When ---
			have := NewDecoder().mapFactsOf(tc.typ)

			// --- Then ---
			assert.Equal(t, tc.want, have != nil)
		})
	}
}

func Test_Decoder_directMap(t *testing.T) {
	type T struct {
		M  map[string]int   `url:"m"`
		MI map[int]string   `url:"mi"`
		MS map[string]TItem `url:"ms"`
	}

	t.Run("sets the entry", func(t *testing.T) {
		// --- Given ---
		dst := &T{}
		dec := NewDecoder()
		stt := dec.state()
		stt.track = false
		val := reflect.ValueOf(dst).Elem()

		// --- When ---
		ok, err := dec.directMap(val, "m[a]", []string{"1"}, stt)

		// --- Then ---
		assert.True(t, ok)
		assert.NoError(t, err)
		assert.Equal(t, map[string]int{"a": 1}, dst.M)
	})

	t.Run("adds to the map", func(t *testing.T) {
		// --- Given ---
		dst := &T{MI: map[int]string{1: "x"}}
		dec := NewDecoder()
		stt := dec.state()
		stt.track = false
		val := reflect.ValueOf(dst).Elem()

		// --- When ---
		ok, err := dec.directMap(val, "mi[2]", []string{"y"}, stt)

		// --- Then ---
		assert.True(t, ok)
		assert.NoError(t, err)
		assert.Equal(t, map[int]string{1: "x", 2: "y"}, dst.MI)
	})

	t.Run("error - value", func(t *testing.T) {
		// --- Given ---
		dst := &T{}
		dec := NewDecoder()
		stt := dec.state()
		stt.track = false
		val := reflect.ValueOf(dst).Elem()

		// --- When ---
		ok, err := dec.directMap(val, "m[a]", []string{"x"}, stt)

		// --- Then ---
		assert.True(t, ok)
		assert.ErrorIs(t, ErrInvValue, err)
		assert.Nil(t, dst.M)
	})

	t.Run("error - key", func(t *testing.T) {
		// --- Given ---
		dst := &T{}
		dec := NewDecoder()
		stt := dec.state()
		stt.track = false
		val := reflect.ValueOf(dst).Elem()

		// --- When ---
		ok, err := dec.directMap(val, "mi[x]", []string{"y"}, stt)

		// --- Then ---
		assert.True(t, ok)
		assert.ErrorIs(t, ErrInvValue, err)
		assert.Nil(t, dst.MI)
	})

	t.Run("tracked paths", func(t *testing.T) {
		// --- Given ---
		dst := &T{}
		dec := NewDecoder()
		stt := dec.state()

		// --- When ---
		val := reflect.ValueOf(dst).Elem()
		ok, err := dec.directMap(val, "m[a]", []string{"1"}, stt)

		// --- Then ---
		assert.False(t, ok)
		assert.NoError(t, err)
	})
}

func Test_Decoder_directMap_tabular(t *testing.T) {
	type T struct {
		M  map[string]int   `url:"m"`
		MI map[int]string   `url:"mi"`
		MS map[string]TItem `url:"ms"`
		D  map[string]int   `url:"d.m"`
	}

	tt := []struct {
		testN string

		key string
	}{
		{"other key text", "mi[01]"},
		{"dotted", "m.a"},
		{"dotted field name", "d.m[a]"},
		{"more segments", "m[a].b"},
		{"empty key", "m[]"},
		{"bracketed field", "[m][a]"},
		{"struct values", "ms[a]"},
		{"unknown field", "x[a]"},
		{"no key", "m"},
	}

	for _, tc := range tt {
		t.Run(tc.testN, func(t *testing.T) {
			// --- Given ---
			dst := &T{}
			dec := NewDecoder()
			stt := dec.state()
			stt.track = false
			val := reflect.ValueOf(dst).Elem()

			// --- When ---
			ok, err := dec.directMap(val, tc.key, []string{"1"}, stt)

			// --- Then ---
			assert.False(t, ok)
			assert.NoError(t, err)
			assert.Equal(t, T{}, *dst)
		})
	}
}
