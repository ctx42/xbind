// SPDX-FileCopyrightText: (c) 2026 Rafal Zajac
// SPDX-License-Identifier: MIT

package xbind

import (
	"reflect"
	"testing"
	"time"

	"github.com/ctx42/testing/pkg/assert"
	"github.com/ctx42/testing/pkg/must"
)

func Test_encFor(t *testing.T) {
	type s struct{ F int }

	t.Run("not registered", func(t *testing.T) {
		// --- When ---
		have := encFor(reflect.TypeOf(s{}), nil)

		// --- Then ---
		assert.Nil(t, have)
	})

	t.Run("time", func(t *testing.T) {
		// --- When ---
		have := encFor(reflect.TypeOf(time.Now()), nil)

		// --- Then ---
		assert.Nil(t, have)
	})

	t.Run("pointer to registered value", func(t *testing.T) {
		// --- Given ---
		enc := func(reflect.Value) (string, error) { return "called", nil }
		reg := encReg{reflect.TypeOf(s{}): enc}

		// --- When ---
		have := encFor(reflect.TypeOf(&s{}), reg)

		// --- Then ---
		assert.NotNil(t, have)

		assert.Equal(t, "called", must.Value(have(reflect.ValueOf(&s{}))))
	})

	t.Run("value of registered pointer", func(t *testing.T) {
		// --- Given ---
		enc := func(reflect.Value) (string, error) { return "called", nil }
		reg := encReg{reflect.TypeOf(&s{}): enc}

		// --- When ---
		have := encFor(reflect.TypeOf(s{}), reg)

		// --- Then ---
		assert.Nil(t, have)
	})

	t.Run("registered", func(t *testing.T) {
		// --- Given ---
		enc := func(reflect.Value) (string, error) { return "called", nil }
		reg := encReg{reflect.TypeOf(&s{}): enc}

		// --- When ---
		have := encFor(reflect.TypeOf(&s{}), reg)

		// --- Then ---
		assert.NotNil(t, have)

		assert.Equal(t, "called", must.Value(have(reflect.ValueOf((*s)(nil)))))
	})
}

func Test_encFor_tabular(t *testing.T) {
	tt := []struct {
		testN string

		val  any
		want string
	}{
		{"bool", true, "true"},
		{"int", -1, "-1"},
		{"int8", int8(-1), "-1"},
		{"int16", int16(-1), "-1"},
		{"int32", int32(-1), "-1"},
		{"int64", int64(-1), "-1"},
		{"uint", uint(1), "1"},
		{"uint8", uint8(1), "1"},
		{"uint16", uint16(1), "1"},
		{"uint32", uint32(1), "1"},
		{"uint64", uint64(1), "1"},
		{"float32", float32(1.1), "1.1"},
		{"float64", 1.123456789, "1.123456789"},
		{"string", "abc", "abc"},
		{"int alias", IntAlias(1), "1"},
		{"float alias", FltAlias(1.123456789), "1.123456789"},
		{"string alias", StrAlias("abc"), "abc"},
		{"duration", time.Second, "1s"},
		{"pointer bool", new(true), "true"},
		{"pointer int", new(-1), "-1"},
		{"pointer int8", new(int8(-2)), "-2"},
		{"pointer int16", new(int16(-3)), "-3"},
		{"pointer int32", new(int32(-4)), "-4"},
		{"pointer int64", new(int64(-5)), "-5"},
		{"pointer uint", new(uint(6)), "6"},
		{"pointer uint8", new(uint8(7)), "7"},
		{"pointer uint16", new(uint16(8)), "8"},
		{"pointer uint32", new(uint32(9)), "9"},
		{"pointer uint64", new(uint64(10)), "10"},
		{"pointer float32", new(float32(11.1)), "11.1"},
		{"pointer float64", new(12.1), "12.1"},
		{"pointer string", new("abc"), "abc"},
		{"pointer duration", new(time.Second), "1s"},
	}

	for _, tc := range tt {
		t.Run(tc.testN, func(t *testing.T) {
			// --- When ---
			have := encFor(reflect.TypeOf(tc.val), nil)

			// --- Then ---
			assert.NotNil(t, have)

			assert.Equal(t, tc.want, must.Value(have(reflect.ValueOf(tc.val))))
		})
	}
}

func Test_encBool(t *testing.T) {
	t.Run("true", func(t *testing.T) {
		// --- When ---
		have, err := encBool(reflect.ValueOf(true))

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, "true", have)
	})

	t.Run("false", func(t *testing.T) {
		// --- When ---
		have, err := encBool(reflect.ValueOf(false))

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, "false", have)
	})
}

func Test_encInt(t *testing.T) {
	t.Run("int", func(t *testing.T) {
		// --- When ---
		have, err := encInt(reflect.ValueOf(-1))

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, "-1", have)
	})

	t.Run("int8", func(t *testing.T) {
		// --- When ---
		have, err := encInt(reflect.ValueOf(int8(-1)))

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, "-1", have)
	})

	t.Run("int16", func(t *testing.T) {
		// --- When ---
		have, err := encInt(reflect.ValueOf(int16(-1)))

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, "-1", have)
	})

	t.Run("int32", func(t *testing.T) {
		// --- When ---
		have, err := encInt(reflect.ValueOf(int32(-1)))

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, "-1", have)
	})

	t.Run("int64", func(t *testing.T) {
		// --- When ---
		have, err := encInt(reflect.ValueOf(int64(-1)))

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, "-1", have)
	})
}

func Test_encUint(t *testing.T) {
	t.Run("uint", func(t *testing.T) {
		// --- When ---
		have, err := encUint(reflect.ValueOf(uint(1)))

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, "1", have)
	})

	t.Run("uint8", func(t *testing.T) {
		// --- When ---
		have, err := encUint(reflect.ValueOf(uint8(1)))

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, "1", have)
	})

	t.Run("uint16", func(t *testing.T) {
		// --- When ---
		have, err := encUint(reflect.ValueOf(uint16(1)))

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, "1", have)
	})

	t.Run("uint32", func(t *testing.T) {
		// --- When ---
		have, err := encUint(reflect.ValueOf(uint32(1)))

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, "1", have)
	})

	t.Run("uint64", func(t *testing.T) {
		// --- When ---
		have, err := encUint(reflect.ValueOf(uint64(1)))

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, "1", have)
	})
}

func Test_encFloat(t *testing.T) {
	t.Run("float 64", func(t *testing.T) {
		// --- When ---
		have, err := encFloat(reflect.ValueOf(1.1), 64)

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, "1.1", have)
	})
}

func Test_encFloat32(t *testing.T) {
	t.Run("float 32", func(t *testing.T) {
		// --- When ---
		have, err := encFloat32(reflect.ValueOf(float32(1.1)))

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, "1.1", have)
	})

	t.Run("float 64", func(t *testing.T) {
		// --- When ---
		have, err := encFloat32(reflect.ValueOf(1.12))

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, "1.12", have)
	})
}

func Test_encFloat64(t *testing.T) {
	t.Run("float", func(t *testing.T) {
		// --- When ---
		have, err := encFloat64(reflect.ValueOf(1.1))

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, "1.1", have)
	})
}

func Test_encString(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		// --- When ---
		have, err := encString(reflect.ValueOf("abc"))

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, "abc", have)
	})
}

func Test_encDur(t *testing.T) {
	t.Run("duration", func(t *testing.T) {
		// --- When ---
		have, err := encDur(reflect.ValueOf(time.Second))

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, "1s", have)
	})

	t.Run("zero duration", func(t *testing.T) {
		// --- When ---
		have, err := encDur(reflect.ValueOf(time.Duration(0)))

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, "0s", have)
	})

	t.Run("not duration", func(t *testing.T) {
		// --- When ---
		have, err := encDur(reflect.ValueOf(123))

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, "", have)
	})
}
