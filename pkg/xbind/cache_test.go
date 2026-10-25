// SPDX-FileCopyrightText: (c) 2026 Rafal Zajac
// SPDX-License-Identifier: MIT

package xbind

import (
	"encoding"
	"reflect"
	"sync"
	"testing"

	"github.com/ctx42/testing/pkg/assert"
)

func Test_typeKey(t *testing.T) {
	t.Run("same type", func(t *testing.T) {
		// --- When ---
		have := typeKey(reflect.TypeFor[int]())

		// --- Then ---
		assert.Equal(t, typeKey(reflect.TypeOf(0)), have)
	})

	t.Run("other type", func(t *testing.T) {
		// --- When ---
		have := typeKey(reflect.TypeFor[int]())

		// --- Then ---
		assert.NotEqual(t, typeKey(reflect.TypeFor[int64]()), have)
	})
}

func Test_typeMap_Load(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		// --- Given ---
		var tm typeMap[string]

		// --- When ---
		have, ok := tm.Load(reflect.TypeFor[int]())

		// --- Then ---
		assert.False(t, ok)
		assert.Equal(t, "", have)
	})

	t.Run("stored", func(t *testing.T) {
		// --- Given ---
		var tm typeMap[string]
		tm.Store(reflect.TypeFor[int](), "int")

		// --- When ---
		have, ok := tm.Load(reflect.TypeFor[int]())

		// --- Then ---
		assert.True(t, ok)
		assert.Equal(t, "int", have)
	})

	t.Run("other type", func(t *testing.T) {
		// --- Given ---
		var tm typeMap[string]
		tm.Store(reflect.TypeFor[int](), "int")

		// --- When ---
		_, ok := tm.Load(reflect.TypeFor[string]())

		// --- Then ---
		assert.False(t, ok)
	})
}

func Test_typeMap_Store(t *testing.T) {
	t.Run("replaces the value", func(t *testing.T) {
		// --- Given ---
		var tm typeMap[string]
		tm.Store(reflect.TypeFor[int](), "a")

		// --- When ---
		tm.Store(reflect.TypeFor[int](), "b")

		// --- Then ---
		have, _ := tm.Load(reflect.TypeFor[int]())
		assert.Equal(t, "b", have)
	})

	t.Run("keeps the old map", func(t *testing.T) {
		// --- Given ---
		var tm typeMap[string]
		tm.Store(reflect.TypeFor[int](), "a")
		old := tm.m.Load()

		// --- When ---
		tm.Store(reflect.TypeFor[string](), "b")

		// --- Then ---
		assert.Len(t, 1, *old)
		assert.Len(t, 2, *tm.m.Load())
	})

	t.Run("concurrent", func(t *testing.T) {
		// --- Given ---
		var tm typeMap[int]
		types := []reflect.Type{
			reflect.TypeFor[int](),
			reflect.TypeFor[string](),
			reflect.TypeFor[bool](),
			reflect.TypeFor[float64](),
		}
		var wg sync.WaitGroup

		// --- When ---
		for i, typ := range types {
			wg.Go(func() { tm.Store(typ, i) })
			wg.Go(func() { tm.Load(typ) })
		}
		wg.Wait()

		// --- Then ---
		for i, typ := range types {
			have, ok := tm.Load(typ)
			assert.True(t, ok)
			assert.Equal(t, i, have)
		}
	})
}

func Test_typeMap_LoadOrStore(t *testing.T) {
	t.Run("stores", func(t *testing.T) {
		// --- Given ---
		var tm typeMap[string]

		// --- When ---
		have, loaded := tm.LoadOrStore(reflect.TypeFor[int](), "a")

		// --- Then ---
		assert.False(t, loaded)
		assert.Equal(t, "a", have)
		stored, _ := tm.Load(reflect.TypeFor[int]())
		assert.Equal(t, "a", stored)
	})

	t.Run("loads", func(t *testing.T) {
		// --- Given ---
		var tm typeMap[string]
		tm.Store(reflect.TypeFor[int](), "a")

		// --- When ---
		have, loaded := tm.LoadOrStore(reflect.TypeFor[int](), "b")

		// --- Then ---
		assert.True(t, loaded)
		assert.Equal(t, "a", have)
	})
}

func Test_typeMap_store(t *testing.T) {
	// --- Given ---
	var tm typeMap[string]

	// --- When ---
	tm.store(reflect.TypeFor[int](), "a")

	// --- Then ---
	want := map[uintptr]string{typeKey(reflect.TypeFor[int]()): "a"}
	assert.Equal(t, want, *tm.m.Load())
}

func Test_keyField_unmFact(t *testing.T) {
	t.Run("unmarshals", func(t *testing.T) {
		// --- When ---
		have := keyField{unm: true}.unmFact()

		// --- Then ---
		assert.Equal(t, unmYes, have)
	})

	t.Run("does not unmarshal", func(t *testing.T) {
		// --- When ---
		have := keyField{}.unmFact()

		// --- Then ---
		assert.Equal(t, unmNo, have)
	})
}

func Test_newTypeInfo_tabular(t *testing.T) {
	tt := []struct {
		testN string

		typ  reflect.Type
		want typeInfo
	}{
		{"int", reflect.TypeFor[int](), typeInfo{}},
		{
			"Marshaler value receiver",
			reflect.TypeFor[TMarValVal](),
			typeInfo{mar: true, pmar: true},
		},
		{
			"Marshaler pointer receiver",
			reflect.TypeFor[TMarValPtr](),
			typeInfo{pmar: true},
		},
		{
			"TextMarshaler and TextUnmarshaler",
			reflect.TypeFor[TLevel](),
			typeInfo{text: true, ptext: true, punm: true},
		},
		{
			"pointer type",
			reflect.TypeFor[*TTextPtr](),
			typeInfo{text: true, unm: true},
		},
		{
			"interface",
			reflect.TypeFor[encoding.TextMarshaler](),
			typeInfo{text: true},
		},
	}

	for _, tc := range tt {
		t.Run(tc.testN, func(t *testing.T) {
			// --- When ---
			have := newTypeInfo(tc.typ)

			// --- Then ---
			assert.Equal(t, tc.want, have)
		})
	}
}

func Test_typeCache_info(t *testing.T) {
	t.Run("nil cache", func(t *testing.T) {
		// --- When ---
		have := (*typeCache)(nil).info(reflect.TypeFor[TLevel]())

		// --- Then ---
		assert.Equal(t, newTypeInfo(reflect.TypeFor[TLevel]()), have)
	})

	t.Run("stores the info", func(t *testing.T) {
		// --- Given ---
		tc := &typeCache{}
		typ := reflect.TypeFor[TLevel]()

		// --- When ---
		have := tc.info(typ)

		// --- Then ---
		assert.Equal(t, newTypeInfo(typ), have)
		stored, ok := tc.infos.Load(typ)
		assert.True(t, ok)
		assert.Equal(t, have, stored)
	})

	t.Run("returns the stored info", func(t *testing.T) {
		// --- Given ---
		tc := &typeCache{}
		typ := reflect.TypeFor[int]()
		tc.infos.Store(typ, typeInfo{mar: true})

		// --- When ---
		have := tc.info(typ)

		// --- Then ---
		assert.Equal(t, typeInfo{mar: true}, have)
	})
}

func Test_typeCache_isMarshaler_tabular(t *testing.T) {
	tt := []struct {
		testN string

		typ  reflect.Type
		want bool
	}{
		{"Marshaler", reflect.TypeFor[TMarValVal](), true},
		{"TextMarshaler", reflect.TypeFor[TTextVal](), true},
		{"Marshaler pointer receiver", reflect.TypeFor[TMarValPtr](), false},
		{
			"pointer to Marshaler pointer receiver",
			reflect.TypeFor[*TMarValPtr](),
			true,
		},
		{"TextMarshaler pointer receiver", reflect.TypeFor[TTextPtr](), false},
		{
			"pointer to TextMarshaler pointer receiver",
			reflect.TypeFor[*TTextPtr](),
			true,
		},
		{"int", reflect.TypeFor[int](), false},
	}

	for _, tc := range tt {
		t.Run(tc.testN, func(t *testing.T) {
			// --- When ---
			have := (&typeCache{}).isMarshaler(tc.typ)

			// --- Then ---
			assert.Equal(t, tc.want, have)
		})
	}
}

func Test_typeCache_textUnmarshaler_tabular(t *testing.T) {
	tt := []struct {
		testN string

		typ  reflect.Type
		want bool
	}{
		{"named byte", reflect.TypeFor[TTextVal](), false},
		{"pointer to named byte", reflect.TypeFor[*TTextVal](), false},
		{"level", reflect.TypeFor[TLevel](), true},
		{"pointer to level", reflect.TypeFor[*TLevel](), true},
		{"text pointer", reflect.TypeFor[TTextPtr](), true},
		{"float key", reflect.TypeFor[TFltKey](), false},
		{"named string", reflect.TypeFor[tIDText](), false},
		{"struct", reflect.TypeFor[tSumText](), false},
		{"int", reflect.TypeFor[int](), false},
	}

	for _, tc := range tt {
		t.Run(tc.testN, func(t *testing.T) {
			// --- When ---
			have := (&typeCache{}).textUnmarshaler(tc.typ)

			// --- Then ---
			assert.Equal(t, tc.want, have)
		})
	}
}

func Test_typeCache_checkTags(t *testing.T) {
	t.Run("nil cache", func(t *testing.T) {
		// --- Given ---
		leaf := func(reflect.Type) bool { return false }

		// --- When ---
		err := (*typeCache)(nil).checkTags(reflect.TypeFor[TBox](), "url", leaf)

		// --- Then ---
		assert.NoError(t, err)
	})

	t.Run("stores the result", func(t *testing.T) {
		// --- Given ---
		tc := &typeCache{}
		typ := reflect.TypeOf(badTag(`url:"a`, true)).Elem()
		leaf := func(reflect.Type) bool { return false }

		// --- When ---
		err := tc.checkTags(typ, "url", leaf)

		// --- Then ---
		assert.ErrorIs(t, ErrInvTag, err)
		stored, ok := tc.tags.Load(typ)
		assert.True(t, ok)
		assert.Equal(t, tagCheck{err: err}, stored)
	})

	t.Run("returns the stored result", func(t *testing.T) {
		// --- Given ---
		tc := &typeCache{}
		typ := reflect.TypeFor[TBox]()
		tc.tags.Store(typ, tagCheck{err: errTest})
		leaf := func(reflect.Type) bool { return false }

		// --- When ---
		err := tc.checkTags(typ, "url", leaf)

		// --- Then ---
		assert.Same(t, errTest, err)
	})
}

func Test_typeCache_checkDst(t *testing.T) {
	t.Run("nil cache", func(t *testing.T) {
		// --- Given ---
		var tc *typeCache
		dec := NewDecoder()

		// --- When ---
		have, err := tc.checkDst(
			reflect.TypeFor[TNode](),
			"url",
			dec.isLeafType,
			dec.hasChecks,
		)

		// --- Then ---
		assert.NoError(t, err)
		assert.True(t, have)
	})

	t.Run("stores the result", func(t *testing.T) {
		// --- Given ---
		tc := &typeCache{}
		dec := NewDecoder()

		// --- When ---
		have, err := tc.checkDst(
			reflect.TypeFor[TBox](),
			"url",
			dec.isLeafType,
			dec.hasChecks,
		)

		// --- Then ---
		assert.NoError(t, err)
		assert.False(t, have)
		stored, ok := tc.tags.Load(reflect.TypeFor[TBox]())
		assert.True(t, ok)
		assert.Equal(t, tagCheck{}, stored)
	})

	t.Run("returns the stored result", func(t *testing.T) {
		// --- Given ---
		tc := &typeCache{}
		tc.tags.Store(reflect.TypeFor[TBox](), tagCheck{checks: true})
		dec := NewDecoder()

		// --- When ---
		have, err := tc.checkDst(
			reflect.TypeFor[TBox](),
			"url",
			dec.isLeafType,
			dec.hasChecks,
		)

		// --- Then ---
		assert.NoError(t, err)
		assert.True(t, have)
	})

	t.Run("error - tags", func(t *testing.T) {
		// --- Given ---
		var tc *typeCache
		typ := reflect.TypeOf(badTag(`url:"a`, true)).Elem()
		var calls int
		checks := func(reflect.Type) bool { calls++; return true }

		// --- When ---
		have, err := tc.checkDst(typ, "url", NewDecoder().isLeafType, checks)

		// --- Then ---
		assert.ErrorIs(t, ErrInvTag, err)
		assert.False(t, have)
		assert.Equal(t, 0, calls)
	})
}

func Test_typeCache_leaf(t *testing.T) {
	t.Run("nil cache calls fn every time", func(t *testing.T) {
		// --- Given ---
		var calls int
		fn := func(reflect.Type) bool { calls++; return true }
		var tc *typeCache
		tc.leaf(reflect.TypeFor[int](), fn)

		// --- When ---
		have := tc.leaf(reflect.TypeFor[int](), fn)

		// --- Then ---
		assert.True(t, have)
		assert.Equal(t, 2, calls)
	})

	t.Run("cache calls fn once", func(t *testing.T) {
		// --- Given ---
		var calls int
		fn := func(reflect.Type) bool { calls++; return true }
		tc := &typeCache{}
		tc.leaf(reflect.TypeFor[int](), fn)

		// --- When ---
		have := tc.leaf(reflect.TypeFor[int](), fn)

		// --- Then ---
		assert.True(t, have)
		assert.Equal(t, 1, calls)
		_, ok := tc.values.Load(reflect.TypeFor[int]())
		assert.False(t, ok)
	})
}

func Test_typeCache_value(t *testing.T) {
	t.Run("nil cache calls fn every time", func(t *testing.T) {
		// --- Given ---
		var calls int
		fn := func(reflect.Type) bool { calls++; return true }
		var tc *typeCache
		tc.value(reflect.TypeFor[int](), fn)

		// --- When ---
		have := tc.value(reflect.TypeFor[int](), fn)

		// --- Then ---
		assert.True(t, have)
		assert.Equal(t, 2, calls)
	})

	t.Run("cache calls fn once", func(t *testing.T) {
		// --- Given ---
		var calls int
		fn := func(reflect.Type) bool { calls++; return true }
		tc := &typeCache{}
		tc.value(reflect.TypeFor[int](), fn)

		// --- When ---
		have := tc.value(reflect.TypeFor[int](), fn)

		// --- Then ---
		assert.True(t, have)
		assert.Equal(t, 1, calls)
		_, ok := tc.leaves.Load(reflect.TypeFor[int]())
		assert.False(t, ok)
	})
}

func Test_typeCache_plainKey(t *testing.T) {
	t.Run("nil cache calls fn every time", func(t *testing.T) {
		// --- Given ---
		var calls int
		fn := func(reflect.Type) bool { calls++; return true }
		var tc *typeCache
		tc.plainKey(reflect.TypeFor[int](), fn)

		// --- When ---
		have := tc.plainKey(reflect.TypeFor[int](), fn)

		// --- Then ---
		assert.True(t, have)
		assert.Equal(t, 2, calls)
	})

	t.Run("cache calls fn once", func(t *testing.T) {
		// --- Given ---
		var calls int
		fn := func(reflect.Type) bool { calls++; return false }
		tc := &typeCache{}
		tc.plainKey(reflect.TypeFor[int](), fn)

		// --- When ---
		have := tc.plainKey(reflect.TypeFor[int](), fn)

		// --- Then ---
		assert.False(t, have)
		assert.Equal(t, 1, calls)
	})
}

func Test_typeCache_composite(t *testing.T) {
	t.Run("nil cache calls fn every time", func(t *testing.T) {
		// --- Given ---
		var calls int
		fn := func(reflect.Type) bool { calls++; return true }
		var tc *typeCache
		tc.composite(reflect.TypeFor[TItem](), fn)

		// --- When ---
		have := tc.composite(reflect.TypeFor[TItem](), fn)

		// --- Then ---
		assert.True(t, have)
		assert.Equal(t, 2, calls)
	})

	t.Run("cache calls fn once", func(t *testing.T) {
		// --- Given ---
		var calls int
		fn := func(reflect.Type) bool { calls++; return false }
		tc := &typeCache{}
		tc.composite(reflect.TypeFor[TItem](), fn)

		// --- When ---
		have := tc.composite(reflect.TypeFor[TItem](), fn)

		// --- Then ---
		assert.False(t, have)
		assert.Equal(t, 1, calls)
	})
}

func Test_typeCache_addr(t *testing.T) {
	t.Run("nil cache calls fn every time", func(t *testing.T) {
		// --- Given ---
		var calls int
		fn := func(reflect.Type) bool { calls++; return true }
		var tc *typeCache
		tc.addr(reflect.TypeFor[int](), fn)

		// --- When ---
		have := tc.addr(reflect.TypeFor[int](), fn)

		// --- Then ---
		assert.True(t, have)
		assert.Equal(t, 2, calls)
	})

	t.Run("cache calls fn once", func(t *testing.T) {
		// --- Given ---
		var calls int
		fn := func(reflect.Type) bool { calls++; return false }
		tc := &typeCache{}
		tc.addr(reflect.TypeFor[int](), fn)

		// --- When ---
		have := tc.addr(reflect.TypeFor[int](), fn)

		// --- Then ---
		assert.False(t, have)
		assert.Equal(t, 1, calls)
	})
}

func Test_typeCache_loop(t *testing.T) {
	t.Run("nil cache calls fn every time", func(t *testing.T) {
		// --- Given ---
		var calls int
		fn := func(reflect.Type) bool { calls++; return true }
		var tc *typeCache
		tc.loop(reflect.TypeFor[int](), fn)

		// --- When ---
		have := tc.loop(reflect.TypeFor[int](), fn)

		// --- Then ---
		assert.True(t, have)
		assert.Equal(t, 2, calls)
	})

	t.Run("cache calls fn once", func(t *testing.T) {
		// --- Given ---
		var calls int
		fn := func(reflect.Type) bool { calls++; return false }
		tc := &typeCache{}
		tc.loop(reflect.TypeFor[int](), fn)

		// --- When ---
		have := tc.loop(reflect.TypeFor[int](), fn)

		// --- Then ---
		assert.False(t, have)
		assert.Equal(t, 1, calls)
	})
}

func Test_typeCache_level(t *testing.T) {
	t.Run("nil cache", func(t *testing.T) {
		// --- When ---
		have := (*typeCache)(nil).level(reflect.TypeFor[TBox](), "url")

		// --- Then ---
		assert.Equal(t, newLevel(reflect.TypeFor[TBox](), "url"), have)
	})

	t.Run("stores the names", func(t *testing.T) {
		// --- Given ---
		tc := &typeCache{}
		typ := reflect.TypeFor[TBox]()

		// --- When ---
		have := tc.level(typ, "url")

		// --- Then ---
		assert.Equal(t, newLevel(typ, "url"), have)
		stored, ok := tc.levels.Load(typ)
		assert.True(t, ok)
		assert.Same(t, have.nms, stored)
	})

	t.Run("returns the stored names", func(t *testing.T) {
		// --- Given ---
		tc := &typeCache{}
		typ := reflect.TypeFor[TBox]()
		nms := newLevel(reflect.TypeFor[TAmbig](), "url").nms
		tc.levels.Store(typ, nms)

		// --- When ---
		have := tc.level(typ, "url")

		// --- Then ---
		assert.Same(t, nms, have.nms)
		assert.Nil(t, have.idx)
	})
}

func Test_typeCache_keyFields(t *testing.T) {
	type T struct {
		A int    `url:"a"`
		B []TBox `url:"b"`
		C string `url:"-"`
		D *int   `url:"d"`
		E int    `url:"e"`
	}
	t.Run("nil cache", func(t *testing.T) {
		// --- Given ---
		var tc *typeCache
		dec := NewDecoder()

		// --- When ---
		have := tc.keyFields(
			reflect.TypeFor[T](),
			"url",
			dec.isLeafType,
			dec.isUnmType,
			dec.reg,
		)

		// --- Then ---
		assert.Len(t, 4, have)
		assert.Equal(t, []int{0}, have["a"].idx)
		assert.Equal(t, reflect.TypeFor[int](), have["a"].typ)
		assert.True(t, have["a"].leaf)
		assert.NotNil(t, have["a"].set)
		assert.Equal(t, []int{1}, have["b"].idx)
		assert.False(t, have["b"].leaf)
		assert.Nil(t, have["b"].set)
		assert.True(t, have["d"].leaf)
		assert.Nil(t, have["d"].set)
	})

	t.Run("registered type has no setter", func(t *testing.T) {
		// --- Given ---
		var tc *typeCache
		dec := NewDecoder(WithDecFunc(func(string) (int, error) {
			return 0, nil
		}))

		// --- When ---
		have := tc.keyFields(
			reflect.TypeFor[T](),
			"url",
			dec.isLeafType,
			dec.isUnmType,
			dec.reg,
		)

		// --- Then ---
		assert.Nil(t, have["a"].set)
		assert.Nil(t, have["e"].set)
	})

	t.Run("stores the fields", func(t *testing.T) {
		// --- Given ---
		tc := &typeCache{}
		dec := NewDecoder()

		// --- When ---
		have := tc.keyFields(
			reflect.TypeFor[T](),
			"url",
			dec.isLeafType,
			dec.isUnmType,
			dec.reg,
		)

		// --- Then ---
		assert.Len(t, 4, have)
		stored, ok := tc.fields.Load(reflect.TypeFor[T]())
		assert.True(t, ok)
		assert.Len(t, 4, stored)
	})

	t.Run("returns the stored fields", func(t *testing.T) {
		// --- Given ---
		tc := &typeCache{}
		kfs := keyFields{"x": {idx: []int{0}}}
		tc.fields.Store(reflect.TypeFor[T](), kfs)
		dec := NewDecoder()

		// --- When ---
		have := tc.keyFields(
			reflect.TypeFor[T](),
			"url",
			dec.isLeafType,
			dec.isUnmType,
			dec.reg,
		)

		// --- Then ---
		assert.Equal(t, kfs, have)
	})
}

func Test_newKeyField(t *testing.T) {
	t.Run("leaf", func(t *testing.T) {
		// --- Given ---
		typ := reflect.TypeFor[struct{ F *int }]()
		leaf, unm := NewDecoder().isLeafType, NewDecoder().isUnmType

		// --- When ---
		have := newKeyField(typ, []int{0}, leaf, unm)

		// --- Then ---
		want := keyField{
			idx:  []int{0},
			typ:  reflect.TypeFor[*int](),
			leaf: true,
		}
		assert.Equal(t, want, have)
	})

	t.Run("embedded field", func(t *testing.T) {
		// --- Given ---
		typ := reflect.TypeFor[TAmbig]()
		leaf, unm := NewDecoder().isLeafType, NewDecoder().isUnmType

		// --- When ---
		have := newKeyField(typ, []int{0, 1}, leaf, unm)

		// --- Then ---
		want := keyField{
			idx:  []int{0, 1},
			typ:  reflect.TypeFor[string](),
			leaf: true,
		}
		assert.Equal(t, want, have)
	})

	t.Run("not a leaf", func(t *testing.T) {
		// --- Given ---
		typ := reflect.TypeFor[struct{ F map[string]TBox }]()
		leaf, unm := NewDecoder().isLeafType, NewDecoder().isUnmType

		// --- When ---
		have := newKeyField(typ, []int{0}, leaf, unm)

		// --- Then ---
		assert.False(t, have.leaf)
	})
}

func Test_memo(t *testing.T) {
	t.Run("stores the result", func(t *testing.T) {
		// --- Given ---
		var m typeMap[bool]
		fn := func(reflect.Type) bool { return true }

		// --- When ---
		have := memo(&m, reflect.TypeFor[int](), fn)

		// --- Then ---
		assert.True(t, have)
		stored, ok := m.Load(reflect.TypeFor[int]())
		assert.True(t, ok)
		assert.True(t, stored)
	})

	t.Run("returns the stored result", func(t *testing.T) {
		// --- Given ---
		var m typeMap[bool]
		m.Store(reflect.TypeFor[int](), false)
		fn := func(reflect.Type) bool { return true }

		// --- When ---
		have := memo(&m, reflect.TypeFor[int](), fn)

		// --- Then ---
		assert.False(t, have)
	})
}
