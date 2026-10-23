// SPDX-FileCopyrightText: (c) 2026 Rafal Zajac
// SPDX-License-Identifier: MIT

package xbind

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ctx42/testing/pkg/assert"
	"github.com/ctx42/testing/pkg/must"
	"github.com/ctx42/xrr/pkg/xrr/xrrtest"
)

func Test_decFor(t *testing.T) {
	type s struct{ F int }

	t.Run("not registered", func(t *testing.T) {
		// --- When ---
		have := decFor(reflect.TypeOf(s{}), make(decReg))

		// --- Then ---
		assert.Nil(t, have)
	})

	t.Run("registered pointer but got value", func(t *testing.T) {
		// --- Given ---
		dec := func(...string) (any, error) { return nil, nil }
		reg := decReg{reflect.TypeOf(&s{}): dec}

		// --- When ---
		have := decFor(reflect.TypeOf(s{}), reg)

		// --- Then ---
		assert.Nil(t, have)
	})

	t.Run("registered value but got pointer", func(t *testing.T) {
		// --- Given ---
		dec := func(...string) (any, error) { return nil, nil }
		reg := decReg{reflect.TypeOf(s{}): dec}

		// --- When ---
		have := decFor(reflect.TypeOf(&s{}), reg)

		// --- Then ---
		assert.Nil(t, have)
	})

	t.Run("registered decoder", func(t *testing.T) {
		// --- Given ---
		dec := func(...string) (any, error) { return nil, nil }
		reg := decReg{reflect.TypeOf(&s{}): dec}

		// --- When ---
		have := decFor(reflect.TypeOf(&s{}), reg)

		// --- Then ---
		want := reflect.ValueOf(dec).Pointer()
		assert.Equal(t, want, reflect.ValueOf(have).Pointer())
	})

	t.Run("registered decoder returns error", func(t *testing.T) {
		// --- Given ---
		dec := func(...string) (any, error) { return nil, errors.New("msg") }
		reg := decReg{reflect.TypeOf(""): dec}

		// --- When ---
		have := decFor(reflect.TypeOf(""), reg)

		// --- Then ---
		_, err := have("")
		assert.ErrorEqual(t, "msg", err)
	})
}

func Test_decFor_tabular(t *testing.T) {
	tt := []struct {
		testN string

		typ  reflect.Type
		want decFn
		vls  []string
		wVal any
	}{
		{"bool", reflect.TypeOf(true), decBool, []string{"true"}, true},
		{"int", reflect.TypeOf(-1), decInt, []string{"-1"}, -1},
		{"i8", reflect.TypeOf(int8(-1)), decInt8, []string{"-2"}, int8(-2)},
		{"i16", reflect.TypeOf(int16(-1)), decInt16, []string{"-3"}, int16(-3)},
		{"i32", reflect.TypeOf(int32(-1)), decInt32, []string{"-4"}, int32(-4)},
		{"i64", reflect.TypeOf(int64(-1)), decInt64, []string{"-5"}, int64(-5)},
		{"uint", reflect.TypeOf(uint(1)), decUint, []string{"6"}, uint(6)},
		{"ui8", reflect.TypeOf(uint8(1)), decUint8, []string{"7"}, uint8(7)},
		{
			"ui16",
			reflect.TypeOf(uint16(1)),
			decUint16,
			[]string{"8"},
			uint16(8),
		},
		{
			"ui32",
			reflect.TypeOf(uint32(1)),
			decUint32,
			[]string{"9"},
			uint32(9),
		},
		{
			"ui64",
			reflect.TypeOf(uint64(1)),
			decUint64,
			[]string{"10"},
			uint64(10),
		},
		{
			"f32",
			reflect.TypeOf(float32(1.1)),
			decFloat32,
			[]string{"11.1"},
			float32(11.1),
		},
		{"f64", reflect.TypeOf(1.1), decFloat64, []string{"12.1"}, 12.1},
		{"string", reflect.TypeOf("abc"), decString, []string{"abc"}, "abc"},
		{"int_alias", reflect.TypeOf(IntAlias(1)), decInt, []string{"1"}, 1},
		{
			"float_alias",
			reflect.TypeOf(FltAlias(1.1)),
			decFloat64,
			[]string{"1.1"},
			1.1,
		},
		{
			"string_alias",
			reflect.TypeOf(StrAlias("abc")),
			decString,
			[]string{"abc"},
			"abc",
		},
		{
			"time",
			reflect.TypeOf(time.Now()),
			decTime,
			[]string{"2022-03-10T00:00:00Z"},
			time.Date(2022, 3, 10, 0, 0, 0, 0, time.UTC),
		},
		{
			"duration",
			reflect.TypeOf(time.Second),
			decDur,
			[]string{"1s"},
			time.Second,
		},
	}

	for _, tc := range tt {
		t.Run(tc.testN, func(t *testing.T) {
			// --- When ---
			have := decFor(tc.typ, make(decReg))

			// --- Then ---
			want := reflect.ValueOf(tc.want).Pointer()
			assert.Equal(t, want, reflect.ValueOf(have).Pointer())

			hVal := must.Value(have(tc.vls...))
			assert.Equal(t, tc.wVal, hVal)
		})
	}
}

func Test_decBool(t *testing.T) {
	t.Run("error - not supported value", func(t *testing.T) {
		// --- When ---
		_, err := decBool("wrong")

		// --- Then ---
		want := "invalid value: cannot convert `wrong` to bool"
		assert.ErrorEqual(t, want, err)
		assert.ErrorIs(t, ErrInvValue, err)
		xrrtest.AssertCode(t, ECInvValue, err)
	})

	t.Run("error - not supported upper case value", func(t *testing.T) {
		// --- When ---
		_, err := decBool("WRONG")

		// --- Then ---
		want := "invalid value: cannot convert `WRONG` to bool"
		assert.ErrorEqual(t, want, err)
		assert.ErrorIs(t, ErrInvValue, err)
		xrrtest.AssertCode(t, ECInvValue, err)
	})

	t.Run("error - empty value", func(t *testing.T) {
		// --- When ---
		_, err := decBool("")

		// --- Then ---
		assert.ErrorEqual(t, "invalid value: cannot convert `` to bool", err)
		assert.ErrorIs(t, ErrInvValue, err)
		xrrtest.AssertCode(t, ECInvValue, err)
	})

	t.Run("error - require single value", func(t *testing.T) {
		// --- When ---
		_, err := decBool("off", "on")

		// --- Then ---
		assert.ErrorEqual(t, "require single value: received 2", err)
		assert.ErrorIs(t, ErrReqSingleValue, err)
		xrrtest.AssertCode(t, ECReqSingle, err)
	})
}

func Test_decBool_tabular(t *testing.T) {
	tt := []struct {
		testN string

		vls  []string
		want bool
	}{
		{"true", []string{"true"}, true},
		{"1", []string{"1"}, true},
		{"t", []string{"t"}, true},
		{"T", []string{"T"}, true},
		{"TRUE", []string{"TRUE"}, true},
		{"True", []string{"True"}, true},
		{"on", []string{"on"}, true},
		{"On", []string{"On"}, true},
		{"ON", []string{"ON"}, true},

		{"false", []string{"false"}, false},
		{"0", []string{"0"}, false},
		{"f", []string{"f"}, false},
		{"F", []string{"F"}, false},
		{"FALSE", []string{"FALSE"}, false},
		{"False", []string{"False"}, false},
		{"off", []string{"off"}, false},
		{"Off", []string{"Off"}, false},
		{"OFF", []string{"OFF"}, false},
	}

	for _, tc := range tt {
		t.Run(tc.testN, func(t *testing.T) {
			// --- When ---
			have, err := decBool(tc.vls...)

			// --- Then ---
			assert.NoError(t, err)
			assert.Equal(t, tc.want, have.(bool))
		})
	}
}

func Test_decFloat32(t *testing.T) {
	t.Run("error - not supported value", func(t *testing.T) {
		// --- When ---
		_, err := decFloat32("wrong")

		// --- Then ---
		want := "invalid value: cannot convert `wrong` to float32"
		assert.ErrorEqual(t, want, err)
		assert.ErrorIs(t, ErrInvValue, err)
		xrrtest.AssertCode(t, ECInvValue, err)
	})

	t.Run("error - empty value", func(t *testing.T) {
		// --- When ---
		_, err := decFloat32("")

		// --- Then ---
		assert.ErrorEqual(t, "invalid value: cannot convert `` to float32", err)
		assert.ErrorIs(t, ErrInvValue, err)
		xrrtest.AssertCode(t, ECInvValue, err)
	})

	t.Run("error - out of range value", func(t *testing.T) {
		// --- When ---
		_, err := decFloat32("1000000000000000000000000000000000000000.1")

		// --- Then ---
		want := "" +
			"invalid value: cannot convert " +
			"`1000000000000000000000000000000000000000.1` to float32"
		assert.ErrorEqual(t, want, err)
		assert.ErrorIs(t, ErrInvValue, err)
		xrrtest.AssertCode(t, ECInvValue, err)
	})

	t.Run("error - require single value", func(t *testing.T) {
		// --- When ---
		_, err := decFloat32("1.1", "1.2")

		// --- Then ---
		assert.ErrorEqual(t, "require single value: received 2", err)
		assert.ErrorIs(t, ErrReqSingleValue, err)
		xrrtest.AssertCode(t, ECReqSingle, err)
	})
}

func Test_decFloat32_tabular(t *testing.T) {
	tt := []struct {
		testN string

		vls  []string
		want float32
	}{
		{"fraction", []string{"1.1"}, 1.1},
	}

	for _, tc := range tt {
		t.Run(tc.testN, func(t *testing.T) {
			// --- When ---
			have, err := decFloat32(tc.vls...)

			// --- Then ---
			assert.NoError(t, err)
			assert.Equal(t, tc.want, have.(float32))
		})
	}
}

func Test_decFloat64(t *testing.T) {
	t.Run("error - not supported value", func(t *testing.T) {
		// --- When ---
		_, err := decFloat64("wrong")

		// --- Then ---
		want := "invalid value: cannot convert `wrong` to float64"
		assert.ErrorEqual(t, want, err)
		assert.ErrorIs(t, ErrInvValue, err)
		xrrtest.AssertCode(t, ECInvValue, err)
	})

	t.Run("error - empty value", func(t *testing.T) {
		// --- When ---
		_, err := decFloat64("")

		// --- Then ---
		assert.ErrorEqual(t, "invalid value: cannot convert `` to float64", err)
		assert.ErrorIs(t, ErrInvValue, err)
		xrrtest.AssertCode(t, ECInvValue, err)
	})

	t.Run("error - out of range value", func(t *testing.T) {
		// --- Given ---
		outOfRange := strings.Repeat("1", 350) + ".1"

		// --- When ---
		_, err := decFloat64(outOfRange)

		// --- Then ---
		want := "invalid value: cannot convert `" + outOfRange + "` to float64"
		assert.ErrorEqual(t, want, err)
		assert.ErrorIs(t, ErrInvValue, err)
		xrrtest.AssertCode(t, ECInvValue, err)
	})

	t.Run("error - require single value", func(t *testing.T) {
		// --- When ---
		_, err := decFloat64("1.1", "1.2")

		// --- Then ---
		assert.ErrorEqual(t, "require single value: received 2", err)
		assert.ErrorIs(t, ErrReqSingleValue, err)
		xrrtest.AssertCode(t, ECReqSingle, err)
	})
}

func Test_decFloat64_tabular(t *testing.T) {
	tt := []struct {
		testN string

		vls  []string
		want float64
	}{
		{"fraction", []string{"1.1"}, 1.1},
	}

	for _, tc := range tt {
		t.Run(tc.testN, func(t *testing.T) {
			// --- When ---
			have, err := decFloat64(tc.vls...)

			// --- Then ---
			assert.NoError(t, err)
			assert.Equal(t, tc.want, have.(float64))
		})
	}
}

func Test_decInt(t *testing.T) {
	t.Run("error - not supported value", func(t *testing.T) {
		// --- When ---
		_, err := decInt("wrong")

		// --- Then ---
		want := "invalid value: cannot convert `wrong` to int"
		assert.ErrorEqual(t, want, err)
		assert.ErrorIs(t, ErrInvValue, err)
		xrrtest.AssertCode(t, ECInvValue, err)
	})

	t.Run("error - empty value", func(t *testing.T) {
		// --- When ---
		_, err := decInt("")

		// --- Then ---
		assert.ErrorEqual(t, "invalid value: cannot convert `` to int", err)
		assert.ErrorIs(t, ErrInvValue, err)
		xrrtest.AssertCode(t, ECInvValue, err)
	})

	t.Run("error - out of range value", func(t *testing.T) {
		// --- Given ---
		outOfRange := strings.Repeat("1", 20)

		// --- When ---
		_, err := decInt(outOfRange)

		// --- Then ---
		want := "invalid value: cannot convert `" + outOfRange + "` to int"
		assert.ErrorEqual(t, want, err)
		assert.ErrorIs(t, ErrInvValue, err)
		xrrtest.AssertCode(t, ECInvValue, err)
	})

	t.Run("error - require single value", func(t *testing.T) {
		// --- When ---
		_, err := decInt("1", "2")

		// --- Then ---
		assert.ErrorEqual(t, "require single value: received 2", err)
		assert.ErrorIs(t, ErrReqSingleValue, err)
		xrrtest.AssertCode(t, ECReqSingle, err)
	})
}

func Test_decInt_tabular(t *testing.T) {
	tt := []struct {
		testN string

		vls  []string
		want int
	}{
		{"positive", []string{"1"}, 1},
	}

	for _, tc := range tt {
		t.Run(tc.testN, func(t *testing.T) {
			// --- When ---
			have, err := decInt(tc.vls...)

			// --- Then ---
			assert.NoError(t, err)
			assert.Equal(t, tc.want, have.(int))
		})
	}
}

func Test_decInt8(t *testing.T) {
	t.Run("error - not supported value", func(t *testing.T) {
		// --- When ---
		_, err := decInt8("wrong")

		// --- Then ---
		want := "invalid value: cannot convert `wrong` to int8"
		assert.ErrorEqual(t, want, err)
		assert.ErrorIs(t, ErrInvValue, err)
		xrrtest.AssertCode(t, ECInvValue, err)
	})

	t.Run("error - empty value", func(t *testing.T) {
		// --- When ---
		_, err := decInt8("")

		// --- Then ---
		assert.ErrorEqual(t, "invalid value: cannot convert `` to int8", err)
		assert.ErrorIs(t, ErrInvValue, err)
		xrrtest.AssertCode(t, ECInvValue, err)
	})

	t.Run("error - out of range value", func(t *testing.T) {
		// --- When ---
		_, err := decInt8("500")

		// --- Then ---
		assert.ErrorEqual(t, "invalid value: cannot convert `500` to int8", err)
		assert.ErrorIs(t, ErrInvValue, err)
		xrrtest.AssertCode(t, ECInvValue, err)
	})

	t.Run("error - require single value", func(t *testing.T) {
		// --- When ---
		_, err := decInt8("1", "2")

		// --- Then ---
		assert.ErrorEqual(t, "require single value: received 2", err)
		assert.ErrorIs(t, ErrReqSingleValue, err)
		xrrtest.AssertCode(t, ECReqSingle, err)
	})
}

func Test_decInt8_tabular(t *testing.T) {
	tt := []struct {
		testN string

		vls  []string
		want int8
	}{
		{"positive", []string{"1"}, 1},
	}

	for _, tc := range tt {
		t.Run(tc.testN, func(t *testing.T) {
			// --- When ---
			have, err := decInt8(tc.vls...)

			// --- Then ---
			assert.NoError(t, err)
			assert.Equal(t, tc.want, have.(int8))
		})
	}
}

func Test_decInt16(t *testing.T) {
	t.Run("error - not supported value", func(t *testing.T) {
		// --- When ---
		_, err := decInt16("wrong")

		// --- Then ---
		want := "invalid value: cannot convert `wrong` to int16"
		assert.ErrorEqual(t, want, err)
		assert.ErrorIs(t, ErrInvValue, err)
		xrrtest.AssertCode(t, ECInvValue, err)
	})

	t.Run("error - empty value", func(t *testing.T) {
		// --- When ---
		_, err := decInt16("")

		// --- Then ---
		assert.ErrorEqual(t, "invalid value: cannot convert `` to int16", err)
		assert.ErrorIs(t, ErrInvValue, err)
		xrrtest.AssertCode(t, ECInvValue, err)
	})

	t.Run("error - out of range value", func(t *testing.T) {
		// --- Given ---
		outOfRange := strings.Repeat("1", 20)

		// --- When ---
		_, err := decInt16(outOfRange)

		// --- Then ---
		want := "invalid value: cannot convert `" + outOfRange + "` to int16"
		assert.ErrorEqual(t, want, err)
		assert.ErrorIs(t, ErrInvValue, err)
		xrrtest.AssertCode(t, ECInvValue, err)
	})

	t.Run("error - require single value", func(t *testing.T) {
		// --- When ---
		_, err := decInt16("1", "2")

		// --- Then ---
		assert.ErrorEqual(t, "require single value: received 2", err)
		assert.ErrorIs(t, ErrReqSingleValue, err)
		xrrtest.AssertCode(t, ECReqSingle, err)
	})
}

func Test_decInt16_tabular(t *testing.T) {
	tt := []struct {
		testN string

		vls  []string
		want int16
	}{
		{"positive", []string{"1"}, 1},
	}

	for _, tc := range tt {
		t.Run(tc.testN, func(t *testing.T) {
			// --- When ---
			have, err := decInt16(tc.vls...)

			// --- Then ---
			assert.NoError(t, err)
			assert.Equal(t, tc.want, have.(int16))
		})
	}
}

func Test_decInt32(t *testing.T) {
	t.Run("error - not supported value", func(t *testing.T) {
		// --- When ---
		_, err := decInt32("wrong")

		// --- Then ---
		want := "invalid value: cannot convert `wrong` to int32"
		assert.ErrorEqual(t, want, err)
		assert.ErrorIs(t, ErrInvValue, err)
		xrrtest.AssertCode(t, ECInvValue, err)
	})

	t.Run("error - empty value", func(t *testing.T) {
		// --- When ---
		_, err := decInt32("")

		// --- Then ---
		assert.ErrorEqual(t, "invalid value: cannot convert `` to int32", err)
		assert.ErrorIs(t, ErrInvValue, err)
		xrrtest.AssertCode(t, ECInvValue, err)
	})

	t.Run("error - out of range value", func(t *testing.T) {
		// --- Given ---
		outOfRange := strings.Repeat("1", 20)

		// --- When ---
		_, err := decInt32(outOfRange)

		// --- Then ---
		want := "invalid value: cannot convert `" + outOfRange + "` to int32"
		assert.ErrorEqual(t, want, err)
		assert.ErrorIs(t, ErrInvValue, err)
		xrrtest.AssertCode(t, ECInvValue, err)
	})

	t.Run("error - require single value", func(t *testing.T) {
		// --- When ---
		_, err := decInt32("1", "2")

		// --- Then ---
		assert.ErrorEqual(t, "require single value: received 2", err)
		assert.ErrorIs(t, ErrReqSingleValue, err)
		xrrtest.AssertCode(t, ECReqSingle, err)
	})
}

func Test_decInt32_tabular(t *testing.T) {
	tt := []struct {
		testN string

		vls  []string
		want int32
	}{
		{"positive", []string{"1"}, 1},
	}

	for _, tc := range tt {
		t.Run(tc.testN, func(t *testing.T) {
			// --- When ---
			have, err := decInt32(tc.vls...)

			// --- Then ---
			assert.NoError(t, err)
			assert.Equal(t, tc.want, have.(int32))
		})
	}
}

func Test_decInt64(t *testing.T) {
	t.Run("error - not supported value", func(t *testing.T) {
		// --- When ---
		_, err := decInt64("wrong")

		// --- Then ---
		want := "invalid value: cannot convert `wrong` to int64"
		assert.ErrorEqual(t, want, err)
		assert.ErrorIs(t, ErrInvValue, err)
		xrrtest.AssertCode(t, ECInvValue, err)
	})

	t.Run("error - empty value", func(t *testing.T) {
		// --- When ---
		_, err := decInt64("")

		// --- Then ---
		assert.ErrorEqual(t, "invalid value: cannot convert `` to int64", err)
		assert.ErrorIs(t, ErrInvValue, err)
		xrrtest.AssertCode(t, ECInvValue, err)
	})

	t.Run("error - out of range value", func(t *testing.T) {
		// --- Given ---
		outOfRange := strings.Repeat("1", 20)

		// --- When ---
		_, err := decInt64(outOfRange)

		// --- Then ---
		want := "invalid value: cannot convert `" + outOfRange + "` to int64"
		assert.ErrorEqual(t, want, err)
		assert.ErrorIs(t, ErrInvValue, err)
		xrrtest.AssertCode(t, ECInvValue, err)
	})

	t.Run("error - require single value", func(t *testing.T) {
		// --- When ---
		_, err := decInt64("1", "2")

		// --- Then ---
		assert.ErrorEqual(t, "require single value: received 2", err)
		assert.ErrorIs(t, ErrReqSingleValue, err)
		xrrtest.AssertCode(t, ECReqSingle, err)
	})
}

func Test_decInt64_tabular(t *testing.T) {
	tt := []struct {
		testN string

		vls  []string
		want int64
	}{
		{"positive", []string{"1"}, 1},
	}

	for _, tc := range tt {
		t.Run(tc.testN, func(t *testing.T) {
			// --- When ---
			have, err := decInt64(tc.vls...)

			// --- Then ---
			assert.NoError(t, err)
			assert.Equal(t, tc.want, have.(int64))
		})
	}
}

func Test_decUint(t *testing.T) {
	t.Run("error - not supported value", func(t *testing.T) {
		// --- When ---
		_, err := decUint("wrong")

		// --- Then ---
		want := "invalid value: cannot convert `wrong` to uint"
		assert.ErrorEqual(t, want, err)
		assert.ErrorIs(t, ErrInvValue, err)
		xrrtest.AssertCode(t, ECInvValue, err)
	})

	t.Run("error - negative value", func(t *testing.T) {
		// --- When ---
		_, err := decUint("-1")

		// --- Then ---
		assert.ErrorEqual(t, "invalid value: cannot convert `-1` to uint", err)
		assert.ErrorIs(t, ErrInvValue, err)
		xrrtest.AssertCode(t, ECInvValue, err)
	})

	t.Run("error - empty value", func(t *testing.T) {
		// --- When ---
		_, err := decUint("")

		// --- Then ---
		assert.ErrorEqual(t, "invalid value: cannot convert `` to uint", err)
		assert.ErrorIs(t, ErrInvValue, err)
		xrrtest.AssertCode(t, ECInvValue, err)
	})

	t.Run("error - out of range value", func(t *testing.T) {
		// --- Given ---
		outOfRange := strings.Repeat("1", 25)

		// --- When ---
		_, err := decUint(outOfRange)

		// --- Then ---
		want := "invalid value: cannot convert `" + outOfRange + "` to uint"
		assert.ErrorEqual(t, want, err)
		assert.ErrorIs(t, ErrInvValue, err)
		xrrtest.AssertCode(t, ECInvValue, err)
	})

	t.Run("error - require single value", func(t *testing.T) {
		// --- When ---
		_, err := decUint("1", "2")

		// --- Then ---
		assert.ErrorEqual(t, "require single value: received 2", err)
		assert.ErrorIs(t, ErrReqSingleValue, err)
		xrrtest.AssertCode(t, ECReqSingle, err)
	})
}

func Test_decUint_tabular(t *testing.T) {
	tt := []struct {
		testN string

		vls  []string
		want uint
	}{
		{"positive", []string{"1"}, 1},
	}

	for _, tc := range tt {
		t.Run(tc.testN, func(t *testing.T) {
			// --- When ---
			have, err := decUint(tc.vls...)

			// --- Then ---
			assert.NoError(t, err)
			assert.Equal(t, tc.want, have.(uint))
		})
	}
}

func Test_decUint8(t *testing.T) {
	t.Run("error - not supported value", func(t *testing.T) {
		// --- When ---
		_, err := decUint8("wrong")

		// --- Then ---
		want := "invalid value: cannot convert `wrong` to uint8"
		assert.ErrorEqual(t, want, err)
		assert.ErrorIs(t, ErrInvValue, err)
		xrrtest.AssertCode(t, ECInvValue, err)
	})

	t.Run("error - negative value", func(t *testing.T) {
		// --- When ---
		_, err := decUint8("-1")

		// --- Then ---
		assert.ErrorEqual(t, "invalid value: cannot convert `-1` to uint8", err)
		assert.ErrorIs(t, ErrInvValue, err)
		xrrtest.AssertCode(t, ECInvValue, err)
	})

	t.Run("error - empty value", func(t *testing.T) {
		// --- When ---
		_, err := decUint8("")

		// --- Then ---
		assert.ErrorEqual(t, "invalid value: cannot convert `` to uint8", err)
		assert.ErrorIs(t, ErrInvValue, err)
		xrrtest.AssertCode(t, ECInvValue, err)
	})

	t.Run("error - out of range value", func(t *testing.T) {
		// --- When ---
		_, err := decUint8("500")

		// --- Then ---
		want := "invalid value: cannot convert `500` to uint8"
		assert.ErrorEqual(t, want, err)
		assert.ErrorIs(t, ErrInvValue, err)
		xrrtest.AssertCode(t, ECInvValue, err)
	})

	t.Run("error - require single value", func(t *testing.T) {
		// --- When ---
		_, err := decUint8("1", "2")

		// --- Then ---
		assert.ErrorEqual(t, "require single value: received 2", err)
		assert.ErrorIs(t, ErrReqSingleValue, err)
		xrrtest.AssertCode(t, ECReqSingle, err)
	})
}

func Test_decUint8_tabular(t *testing.T) {
	tt := []struct {
		testN string

		vls  []string
		want uint8
	}{
		{"positive", []string{"1"}, 1},
	}

	for _, tc := range tt {
		t.Run(tc.testN, func(t *testing.T) {
			// --- When ---
			have, err := decUint8(tc.vls...)

			// --- Then ---
			assert.NoError(t, err)
			assert.Equal(t, tc.want, have.(uint8))
		})
	}
}

func Test_decUint16(t *testing.T) {
	t.Run("error - not supported value", func(t *testing.T) {
		// --- When ---
		_, err := decUint16("wrong")

		// --- Then ---
		want := "invalid value: cannot convert `wrong` to uint16"
		assert.ErrorEqual(t, want, err)
		assert.ErrorIs(t, ErrInvValue, err)
		xrrtest.AssertCode(t, ECInvValue, err)
	})

	t.Run("error - empty value", func(t *testing.T) {
		// --- When ---
		_, err := decUint16("")

		// --- Then ---
		assert.ErrorEqual(t, "invalid value: cannot convert `` to uint16", err)
		assert.ErrorIs(t, ErrInvValue, err)
		xrrtest.AssertCode(t, ECInvValue, err)
	})

	t.Run("error - negative value", func(t *testing.T) {
		// --- When ---
		_, err := decUint16("-1")

		// --- Then ---
		want := "invalid value: cannot convert `-1` to uint16"
		assert.ErrorEqual(t, want, err)
		assert.ErrorIs(t, ErrInvValue, err)
		xrrtest.AssertCode(t, ECInvValue, err)
	})

	t.Run("error - out of range value", func(t *testing.T) {
		// --- When ---
		_, err := decUint16("1000000000000000000000")

		// --- Then ---
		want := "" +
			"invalid value: cannot convert " +
			"`1000000000000000000000` to uint16"
		assert.ErrorEqual(t, want, err)
		assert.ErrorIs(t, ErrInvValue, err)
		xrrtest.AssertCode(t, ECInvValue, err)
	})

	t.Run("error - require single value", func(t *testing.T) {
		// --- When ---
		_, err := decUint16("1", "2")

		// --- Then ---
		assert.ErrorEqual(t, "require single value: received 2", err)
		assert.ErrorIs(t, ErrReqSingleValue, err)
		xrrtest.AssertCode(t, ECReqSingle, err)
	})
}

func Test_decUint16_tabular(t *testing.T) {
	tt := []struct {
		testN string

		vls  []string
		want uint16
	}{
		{"positive", []string{"1"}, 1},
	}

	for _, tc := range tt {
		t.Run(tc.testN, func(t *testing.T) {
			// --- When ---
			have, err := decUint16(tc.vls...)

			// --- Then ---
			assert.NoError(t, err)
			assert.Equal(t, tc.want, have.(uint16))
		})
	}
}

func Test_decUint32(t *testing.T) {
	t.Run("error - not supported value", func(t *testing.T) {
		// --- When ---
		_, err := decUint32("wrong")

		// --- Then ---
		want := "invalid value: cannot convert `wrong` to uint32"
		assert.ErrorEqual(t, want, err)
		assert.ErrorIs(t, ErrInvValue, err)
		xrrtest.AssertCode(t, ECInvValue, err)
	})

	t.Run("error - empty value", func(t *testing.T) {
		// --- When ---
		_, err := decUint32("")

		// --- Then ---
		assert.ErrorEqual(t, "invalid value: cannot convert `` to uint32", err)
		assert.ErrorIs(t, ErrInvValue, err)
		xrrtest.AssertCode(t, ECInvValue, err)
	})

	t.Run("error - negative value", func(t *testing.T) {
		// --- When ---
		_, err := decUint32("-1")

		// --- Then ---
		want := "invalid value: cannot convert `-1` to uint32"
		assert.ErrorEqual(t, want, err)
		assert.ErrorIs(t, ErrInvValue, err)
		xrrtest.AssertCode(t, ECInvValue, err)
	})

	t.Run("error - out of range value", func(t *testing.T) {
		// --- When ---
		_, err := decUint32("1000000000000000000000")

		// --- Then ---
		want := "" +
			"invalid value: cannot convert " +
			"`1000000000000000000000` to uint32"
		assert.ErrorEqual(t, want, err)
		assert.ErrorIs(t, ErrInvValue, err)
		xrrtest.AssertCode(t, ECInvValue, err)
	})

	t.Run("error - require single value", func(t *testing.T) {
		// --- When ---
		_, err := decUint32("1", "2")

		// --- Then ---
		assert.ErrorEqual(t, "require single value: received 2", err)
		assert.ErrorIs(t, ErrReqSingleValue, err)
		xrrtest.AssertCode(t, ECReqSingle, err)
	})
}

func Test_decUint32_tabular(t *testing.T) {
	tt := []struct {
		testN string

		vls  []string
		want uint32
	}{
		{"positive", []string{"1"}, 1},
	}

	for _, tc := range tt {
		t.Run(tc.testN, func(t *testing.T) {
			// --- When ---
			have, err := decUint32(tc.vls...)

			// --- Then ---
			assert.NoError(t, err)
			assert.Equal(t, tc.want, have.(uint32))
		})
	}
}

func Test_decUint64(t *testing.T) {
	t.Run("error - not supported value", func(t *testing.T) {
		// --- When ---
		_, err := decUint64("wrong")

		// --- Then ---
		want := "invalid value: cannot convert `wrong` to uint64"
		assert.ErrorEqual(t, want, err)
		assert.ErrorIs(t, ErrInvValue, err)
		xrrtest.AssertCode(t, ECInvValue, err)
	})

	t.Run("error - empty value", func(t *testing.T) {
		// --- When ---
		_, err := decUint64("")

		// --- Then ---
		assert.ErrorEqual(t, "invalid value: cannot convert `` to uint64", err)
		assert.ErrorIs(t, ErrInvValue, err)
		xrrtest.AssertCode(t, ECInvValue, err)
	})

	t.Run("error - negative value", func(t *testing.T) {
		// --- When ---
		_, err := decUint64("-1")

		// --- Then ---
		want := "invalid value: cannot convert `-1` to uint64"
		assert.ErrorEqual(t, want, err)
		assert.ErrorIs(t, ErrInvValue, err)
		xrrtest.AssertCode(t, ECInvValue, err)
	})

	t.Run("error - out of range value", func(t *testing.T) {
		// --- When ---
		_, err := decUint64("1000000000000000000000")

		// --- Then ---
		want := "" +
			"invalid value: cannot convert " +
			"`1000000000000000000000` to uint64"
		assert.ErrorEqual(t, want, err)
		assert.ErrorIs(t, ErrInvValue, err)
		xrrtest.AssertCode(t, ECInvValue, err)
	})

	t.Run("error - require single value", func(t *testing.T) {
		// --- When ---
		_, err := decUint64("1", "2")

		// --- Then ---
		assert.ErrorEqual(t, "require single value: received 2", err)
		assert.ErrorIs(t, ErrReqSingleValue, err)
		xrrtest.AssertCode(t, ECReqSingle, err)
	})
}

func Test_decUint64_tabular(t *testing.T) {
	tt := []struct {
		testN string

		vls  []string
		want uint64
	}{
		{"zero", []string{"0"}, 0},
		{"max", []string{"18446744073709551615"}, 18446744073709551615},
	}

	for _, tc := range tt {
		t.Run(tc.testN, func(t *testing.T) {
			// --- When ---
			have, err := decUint64(tc.vls...)

			// --- Then ---
			assert.NoError(t, err)
			assert.Equal(t, tc.want, have.(uint64))
		})
	}
}

func Test_decTime(t *testing.T) {
	t.Run("error - invalid format", func(t *testing.T) {
		// --- When ---
		_, err := decTime("202203-15T11:43:01Z")

		// --- Then ---
		want := "" +
			"invalid value: cannot convert " +
			"`202203-15T11:43:01Z` to RFC3339 time"
		assert.ErrorEqual(t, want, err)
		assert.ErrorIs(t, ErrInvValue, err)
		xrrtest.AssertCode(t, ECInvValue, err)
	})

	t.Run("error - empty value", func(t *testing.T) {
		// --- When ---
		_, err := decTime("")

		// --- Then ---
		want := "invalid value: cannot convert `` to RFC3339 time"
		assert.ErrorEqual(t, want, err)
		assert.ErrorIs(t, ErrInvValue, err)
		xrrtest.AssertCode(t, ECInvValue, err)
	})

	t.Run("error - require single value", func(t *testing.T) {
		// --- When ---
		_, err := decTime("2000-01-02T03:04:05Z", "2001-01-02T03:04:05Z")

		// --- Then ---
		assert.ErrorEqual(t, "require single value: received 2", err)
		assert.ErrorIs(t, ErrReqSingleValue, err)
		xrrtest.AssertCode(t, ECReqSingle, err)
	})
}

func Test_decTime_tabular(t *testing.T) {
	tt := []struct {
		testN string

		vls  []string
		wTim time.Time
		wOff int
	}{
		{
			"utc",
			[]string{"2022-03-15T11:43:01Z"},
			time.Date(2022, 3, 15, 11, 43, 1, 0, time.UTC),
			0,
		},
		{
			"offset",
			[]string{"2022-03-15T11:43:01+02:00"},
			time.Date(2022, 3, 15, 10, 43, 1, 0, waw),
			2 * 60 * 60,
		},
	}

	for _, tc := range tt {
		t.Run(tc.testN, func(t *testing.T) {
			// --- When ---
			have, err := decTime(tc.vls...)

			// --- Then ---
			assert.NoError(t, err)
			tim := have.(time.Time)
			assert.Time(t, tc.wTim, tim)
			_, off := tim.Zone()
			assert.Equal(t, tc.wOff, off)
		})
	}
}

func Test_decDur(t *testing.T) {
	t.Run("error - invalid format", func(t *testing.T) {
		// --- When ---
		_, err := decDur("1abc")

		// --- Then ---
		want := "invalid value: cannot convert `1abc` to duration"
		assert.ErrorEqual(t, want, err)
		assert.ErrorIs(t, ErrInvValue, err)
		xrrtest.AssertCode(t, ECInvValue, err)
	})

	t.Run("error - empty value", func(t *testing.T) {
		// --- When ---
		_, err := decDur("")

		// --- Then ---
		want := "invalid value: cannot convert `` to duration"
		assert.ErrorEqual(t, want, err)
		assert.ErrorIs(t, ErrInvValue, err)
		xrrtest.AssertCode(t, ECInvValue, err)
	})

	t.Run("error - require single value", func(t *testing.T) {
		// --- When ---
		_, err := decDur("1s", "2s")

		// --- Then ---
		assert.ErrorEqual(t, "require single value: received 2", err)
		assert.ErrorIs(t, ErrReqSingleValue, err)
		xrrtest.AssertCode(t, ECReqSingle, err)
	})
}

func Test_decDur_tabular(t *testing.T) {
	tt := []struct {
		testN string

		vls  []string
		want time.Duration
	}{
		{"compound", []string{"1s10ms"}, time.Second + 10*time.Millisecond},
	}

	for _, tc := range tt {
		t.Run(tc.testN, func(t *testing.T) {
			// --- When ---
			have, err := decDur(tc.vls...)

			// --- Then ---
			assert.NoError(t, err)
			assert.Duration(t, tc.want, have.(time.Duration))
		})
	}
}

func Test_decString(t *testing.T) {
	t.Run("single value", func(t *testing.T) {
		// --- When ---
		have, err := decString("message")

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, "message", have)
	})

	t.Run("error - require single value", func(t *testing.T) {
		// --- When ---
		_, err := decString("a", "b")

		// --- Then ---
		assert.ErrorEqual(t, "require single value: received 2", err)
		assert.ErrorIs(t, ErrReqSingleValue, err)
		xrrtest.AssertCode(t, ECReqSingle, err)
	})
}

func Test_single(t *testing.T) {
	t.Run("one value", func(t *testing.T) {
		// --- Given ---
		vls := []string{"abc"}

		// --- When ---
		have, err := single(vls)

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, "abc", have)
	})

	t.Run("error - zero values", func(t *testing.T) {
		// --- When ---
		have, err := single(nil)

		// --- Then ---
		assert.ErrorEqual(t, "require single value: received 0", err)
		assert.ErrorIs(t, ErrReqSingleValue, err)
		xrrtest.AssertCode(t, ECReqSingle, err)
		assert.Empty(t, have)
	})

	t.Run("error - two values", func(t *testing.T) {
		// --- Given ---
		vls := []string{"a", "b"}

		// --- When ---
		have, err := single(vls)

		// --- Then ---
		assert.ErrorEqual(t, "require single value: received 2", err)
		assert.ErrorIs(t, ErrReqSingleValue, err)
		xrrtest.AssertCode(t, ECReqSingle, err)
		assert.Empty(t, have)
	})
}

func Test_convertErr(t *testing.T) {
	t.Run("plain value", func(t *testing.T) {
		// --- When ---
		err := convertErr("abc", "int")

		// --- Then ---
		assert.ErrorEqual(t, "invalid value: cannot convert `abc` to int", err)
		assert.ErrorIs(t, ErrInvValue, err)
		xrrtest.AssertCode(t, ECInvValue, err)
	})

	t.Run("value with backtick", func(t *testing.T) {
		// --- When ---
		err := convertErr("a`b", "int")

		// --- Then ---
		want := "invalid value: cannot convert \"a`b\" to int"
		assert.ErrorEqual(t, want, err)
		assert.ErrorIs(t, ErrInvValue, err)
		xrrtest.AssertCode(t, ECInvValue, err)
	})
}
