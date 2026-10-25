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

func Test_decReg_get(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		// --- When ---
		have := decReg(nil).get(reflect.TypeFor[int]())

		// --- Then ---
		assert.Nil(t, have)
	})

	t.Run("registered", func(t *testing.T) {
		// --- Given ---
		reg := decReg{reflect.TypeFor[int](): decInt}

		// --- When ---
		have := reg.get(reflect.TypeFor[int]())

		// --- Then ---
		assert.NotNil(t, have)
	})

	t.Run("not registered", func(t *testing.T) {
		// --- Given ---
		reg := decReg{reflect.TypeFor[int](): decInt}

		// --- When ---
		have := reg.get(reflect.TypeFor[string]())

		// --- Then ---
		assert.Nil(t, have)
	})
}

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

	t.Run("registered decoder over the built-in one", func(t *testing.T) {
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
		wFn  decFn
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
			want := reflect.ValueOf(tc.wFn).Pointer()
			assert.Equal(t, want, reflect.ValueOf(have).Pointer())

			assert.Equal(t, tc.wVal, must.Value(have(tc.vls...)))
		})
	}
}

func Test_isBuiltin_tabular(t *testing.T) {
	tt := []struct {
		testN string

		typ  reflect.Type
		want bool
	}{
		{"bool", reflect.TypeFor[bool](), true},
		{"string", reflect.TypeFor[string](), true},
		{"int8", reflect.TypeFor[int8](), true},
		{"uint64", reflect.TypeFor[uint64](), true},
		{"float32", reflect.TypeFor[float32](), true},
		{"named int", reflect.TypeFor[IntAlias](), true},
		{"time", reflect.TypeFor[time.Time](), true},
		{"duration", reflect.TypeFor[time.Duration](), true},
		{"uintptr", reflect.TypeFor[uintptr](), false},
		{"pointer", reflect.TypeFor[*int](), false},
		{"slice", reflect.TypeFor[[]int](), false},
		{"struct", reflect.TypeFor[struct{}](), false},
	}

	for _, tc := range tt {
		t.Run(tc.testN, func(t *testing.T) {
			// --- When ---
			have := isBuiltin(tc.typ)

			// --- Then ---
			assert.Equal(t, tc.want, have)
		})
	}
}

func Test_builtinSetter_tabular(t *testing.T) {
	type tAll struct {
		B   bool
		I   int16
		U   uint
		F   float32
		S   string
		T   time.Time
		D   time.Duration
		N   IntAlias
		P   *int
		Sli []int
	}
	tim := time.Date(2000, 1, 2, 3, 4, 5, 0, time.UTC)

	tt := []struct {
		testN string

		field string
		vls   []string
		want  any
	}{
		{"bool", "B", []string{"off"}, false},
		{"int16", "I", []string{"-16"}, int16(-16)},
		{"uint", "U", []string{"7"}, uint(7)},
		{"float32", "F", []string{"0.5"}, float32(0.5)},
		{"string", "S", []string{"x"}, "x"},
		{"time", "T", []string{"2000-01-02T03:04:05Z"}, tim},
		{"duration", "D", []string{"2s"}, 2 * time.Second},
		{"named int", "N", []string{"9"}, IntAlias(9)},
	}

	for _, tc := range tt {
		t.Run(tc.testN, func(t *testing.T) {
			// --- Given ---
			dst := reflect.ValueOf(&tAll{}).Elem().FieldByName(tc.field)
			set := builtinSetter(dst.Type())

			// --- When ---
			err := set(dst, tc.vls)

			// --- Then ---
			assert.NoError(t, err)
			assert.Equal(t, tc.want, dst.Interface())
		})
	}
}

func Test_builtinSetter(t *testing.T) {
	t.Run("error - value", func(t *testing.T) {
		// --- Given ---
		dst := reflect.ValueOf(&struct{ F int8 }{F: 1}).Elem().Field(0)
		set := builtinSetter(dst.Type())

		// --- When ---
		err := set(dst, []string{"300"})

		// --- Then ---
		want := "invalid value: cannot convert `300` to int8"
		assert.ErrorEqual(t, want, err)
		assert.Equal(t, int8(1), dst.Interface())
	})

	t.Run("not a built-in type", func(t *testing.T) {
		// --- When ---
		have := builtinSetter(reflect.TypeFor[*int]())

		// --- Then ---
		assert.Nil(t, have)
	})
}

func Test_setBuiltin_tabular(t *testing.T) {
	type tAll struct {
		B   bool
		I   int
		I8  int8
		U   uint
		U16 uint16
		F32 float32
		F64 float64
		S   string
		T   time.Time
		D   time.Duration
		N   IntAlias
	}
	tim := time.Date(2000, 1, 2, 3, 4, 5, 0, time.UTC)

	tt := []struct {
		testN string

		field string
		vls   []string
		want  any
	}{
		{"bool", "B", []string{"on"}, true},
		{"int", "I", []string{"-1"}, -1},
		{"int8", "I8", []string{"-8"}, int8(-8)},
		{"uint", "U", []string{"1"}, uint(1)},
		{"uint16", "U16", []string{"16"}, uint16(16)},
		{"float32", "F32", []string{"1.5"}, float32(1.5)},
		{"float64", "F64", []string{"2.5"}, 2.5},
		{"string", "S", []string{"abc"}, "abc"},
		{"time", "T", []string{"2000-01-02T03:04:05Z"}, tim},
		{"duration", "D", []string{"1s"}, time.Second},
		{"named int", "N", []string{"3"}, IntAlias(3)},
	}

	for _, tc := range tt {
		t.Run(tc.testN, func(t *testing.T) {
			// --- Given ---
			dst := reflect.ValueOf(&tAll{}).Elem().FieldByName(tc.field)

			// --- When ---
			ok, err := setBuiltin(dst, tc.vls)

			// --- Then ---
			assert.NoError(t, err)
			assert.True(t, ok)
			assert.Equal(t, tc.want, dst.Interface())
		})
	}
}

func Test_setBuiltin(t *testing.T) {
	t.Run("error - value does not convert", func(t *testing.T) {
		// --- Given ---
		dst := reflect.ValueOf(&struct{ F uint8 }{F: 7}).Elem().Field(0)

		// --- When ---
		ok, err := setBuiltin(dst, []string{"256"})

		// --- Then ---
		assert.True(t, ok)
		want := "invalid value: cannot convert `256` to uint8"
		assert.ErrorEqual(t, want, err)
		assert.Equal(t, uint8(7), dst.Interface())
	})

	t.Run("error - time", func(t *testing.T) {
		// --- Given ---
		dst := reflect.ValueOf(&struct{ F time.Time }{}).Elem().Field(0)

		// --- When ---
		ok, err := setBuiltin(dst, []string{"x"})

		// --- Then ---
		assert.True(t, ok)
		assert.ErrorIs(t, ErrTimeFormat, err)
		assert.Zero(t, dst.Interface())
	})

	t.Run("error - duration", func(t *testing.T) {
		// --- Given ---
		dst := reflect.ValueOf(&struct{ F time.Duration }{}).Elem().Field(0)

		// --- When ---
		ok, err := setBuiltin(dst, []string{"x"})

		// --- Then ---
		assert.True(t, ok)
		assert.ErrorIs(t, ErrDurFormat, err)
	})

	t.Run("error - many values", func(t *testing.T) {
		// --- Given ---
		dst := reflect.ValueOf(&struct{ F string }{}).Elem().Field(0)

		// --- When ---
		ok, err := setBuiltin(dst, []string{"a", "b"})

		// --- Then ---
		assert.True(t, ok)
		assert.ErrorIs(t, ErrReqSingleValue, err)
		assert.Equal(t, "", dst.Interface())
	})

	t.Run("no built-in decoder", func(t *testing.T) {
		// --- Given ---
		dst := reflect.ValueOf(&struct{ F []int }{}).Elem().Field(0)

		// --- When ---
		ok, err := setBuiltin(dst, []string{"1"})

		// --- Then ---
		assert.NoError(t, err)
		assert.False(t, ok)
	})

	t.Run("does not allocate", func(t *testing.T) {
		// --- Given ---
		dst := reflect.ValueOf(&struct{ F time.Time }{}).Elem().Field(0)
		vls := []string{"2000-01-02T03:04:05Z"}

		// --- When ---
		have := testing.AllocsPerRun(10, func() { _, _ = setBuiltin(dst, vls) })

		// --- Then ---
		assert.Equal(t, 0.0, have)
	})
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
			assert.Equal(t, tc.want, have)
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
			assert.Equal(t, tc.want, have)
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
			assert.Equal(t, tc.want, have)
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
			assert.Equal(t, tc.want, have)
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
		{"min", []string{"-128"}, -128},
		{"max", []string{"127"}, 127},
	}

	for _, tc := range tt {
		t.Run(tc.testN, func(t *testing.T) {
			// --- When ---
			have, err := decInt8(tc.vls...)

			// --- Then ---
			assert.NoError(t, err)
			assert.Equal(t, tc.want, have)
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
		// --- When ---
		_, err := decInt16("32768")

		// --- Then ---
		want := "invalid value: cannot convert `32768` to int16"
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
		{"min", []string{"-32768"}, -32768},
		{"max", []string{"32767"}, 32767},
	}

	for _, tc := range tt {
		t.Run(tc.testN, func(t *testing.T) {
			// --- When ---
			have, err := decInt16(tc.vls...)

			// --- Then ---
			assert.NoError(t, err)
			assert.Equal(t, tc.want, have)
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
		// --- When ---
		_, err := decInt32("2147483648")

		// --- Then ---
		want := "invalid value: cannot convert `2147483648` to int32"
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
		{"min", []string{"-2147483648"}, -2147483648},
		{"max", []string{"2147483647"}, 2147483647},
	}

	for _, tc := range tt {
		t.Run(tc.testN, func(t *testing.T) {
			// --- When ---
			have, err := decInt32(tc.vls...)

			// --- Then ---
			assert.NoError(t, err)
			assert.Equal(t, tc.want, have)
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
			assert.Equal(t, tc.want, have)
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
			assert.Equal(t, tc.want, have)
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
		{"max", []string{"255"}, 255},
	}

	for _, tc := range tt {
		t.Run(tc.testN, func(t *testing.T) {
			// --- When ---
			have, err := decUint8(tc.vls...)

			// --- Then ---
			assert.NoError(t, err)
			assert.Equal(t, tc.want, have)
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
		_, err := decUint16("65536")

		// --- Then ---
		want := "invalid value: cannot convert `65536` to uint16"
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
		{"max", []string{"65535"}, 65535},
	}

	for _, tc := range tt {
		t.Run(tc.testN, func(t *testing.T) {
			// --- When ---
			have, err := decUint16(tc.vls...)

			// --- Then ---
			assert.NoError(t, err)
			assert.Equal(t, tc.want, have)
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
		_, err := decUint32("4294967296")

		// --- Then ---
		want := "invalid value: cannot convert `4294967296` to uint32"
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
		{"max", []string{"4294967295"}, 4294967295},
	}

	for _, tc := range tt {
		t.Run(tc.testN, func(t *testing.T) {
			// --- When ---
			have, err := decUint32(tc.vls...)

			// --- Then ---
			assert.NoError(t, err)
			assert.Equal(t, tc.want, have)
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
			assert.Equal(t, tc.want, have)
		})
	}
}

func Test_decTime(t *testing.T) {
	t.Run("error - invalid format", func(t *testing.T) {
		// --- When ---
		_, err := decTime("202203-15T11:43:01Z")

		// --- Then ---
		assert.ErrorEqual(t, "invalid RFC3339 time: `202203-15T11:43:01Z`", err)
		assert.ErrorIs(t, ErrTimeFormat, err)
		xrrtest.AssertCode(t, ECInvTimeFormat, err)
	})

	t.Run("error - empty value", func(t *testing.T) {
		// --- When ---
		_, err := decTime("")

		// --- Then ---
		assert.ErrorEqual(t, "invalid RFC3339 time: ``", err)
		assert.ErrorIs(t, ErrTimeFormat, err)
		xrrtest.AssertCode(t, ECInvTimeFormat, err)
	})

	t.Run("error - offset hour out of range", func(t *testing.T) {
		// --- When ---
		_, err := decTime("2022-01-01T00:00:00+24:00")

		// --- Then ---
		want := "invalid RFC3339 time: `2022-01-01T00:00:00+24:00`"
		assert.ErrorEqual(t, want, err)
		assert.ErrorIs(t, ErrTimeFormat, err)
	})

	t.Run("error - offset minute out of range", func(t *testing.T) {
		// --- When ---
		_, err := decTime("2022-01-01T00:00:00-10:60")

		// --- Then ---
		want := "invalid RFC3339 time: `2022-01-01T00:00:00-10:60`"
		assert.ErrorEqual(t, want, err)
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
		{
			"largest offset",
			[]string{"2022-03-15T23:59:00+23:59"},
			time.Date(2022, 3, 15, 0, 0, 0, 0, time.UTC),
			23*60*60 + 59*60,
		},
	}

	for _, tc := range tt {
		t.Run(tc.testN, func(t *testing.T) {
			// --- When ---
			have, err := decTime(tc.vls...)

			// --- Then ---
			assert.NoError(t, err)
			tim, _ := have.(time.Time) // A zero time fails the checks below.
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
		assert.ErrorEqual(t, "invalid duration string: `1abc`", err)
		assert.ErrorIs(t, ErrDurFormat, err)
		xrrtest.AssertCode(t, ECInvDurFormat, err)
	})

	t.Run("error - empty value", func(t *testing.T) {
		// --- When ---
		_, err := decDur("")

		// --- Then ---
		assert.ErrorEqual(t, "invalid duration string: ``", err)
		assert.ErrorIs(t, ErrDurFormat, err)
		xrrtest.AssertCode(t, ECInvDurFormat, err)
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
			assert.Equal(t, tc.want, have)
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

func Test_parseBool_tabular(t *testing.T) {
	tt := []struct {
		testN string

		val  string
		want bool
	}{
		{"on", "On", true},
		{"off", "OFF", false},
		{"true", "TRUE", true},
		{"false", "0", false},
	}

	for _, tc := range tt {
		t.Run(tc.testN, func(t *testing.T) {
			// --- When ---
			have, err := parseBool([]string{tc.val})

			// --- Then ---
			assert.NoError(t, err)
			assert.Equal(t, tc.want, have)
		})
	}
}

func Test_parseBool(t *testing.T) {
	t.Run("error - not a bool", func(t *testing.T) {
		// --- When ---
		have, err := parseBool([]string{"Yes"})

		// --- Then ---
		assert.ErrorEqual(t, "invalid value: cannot convert `Yes` to bool", err)
		assert.False(t, have)
	})

	t.Run("error - no value", func(t *testing.T) {
		// --- When ---
		_, err := parseBool(nil)

		// --- Then ---
		assert.ErrorIs(t, ErrReqSingleValue, err)
	})
}

func Test_parseInt(t *testing.T) {
	t.Run("in range", func(t *testing.T) {
		// --- When ---
		have, err := parseInt([]string{"-128"}, reflect.Int8)

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, int64(-128), have)
	})

	t.Run("error - out of range", func(t *testing.T) {
		// --- When ---
		have, err := parseInt([]string{"128"}, reflect.Int8)

		// --- Then ---
		assert.ErrorEqual(t, "invalid value: cannot convert `128` to int8", err)
		assert.Equal(t, int64(0), have)
	})

	t.Run("error - no value", func(t *testing.T) {
		// --- When ---
		_, err := parseInt(nil, reflect.Int)

		// --- Then ---
		assert.ErrorIs(t, ErrReqSingleValue, err)
	})
}

func Test_parseUint(t *testing.T) {
	t.Run("in range", func(t *testing.T) {
		// --- When ---
		have, err := parseUint([]string{"255"}, reflect.Uint8)

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, uint64(255), have)
	})

	t.Run("error - out of range", func(t *testing.T) {
		// --- When ---
		have, err := parseUint([]string{"-1"}, reflect.Uint)

		// --- Then ---
		assert.ErrorEqual(t, "invalid value: cannot convert `-1` to uint", err)
		assert.Equal(t, uint64(0), have)
	})

	t.Run("error - no value", func(t *testing.T) {
		// --- When ---
		_, err := parseUint(nil, reflect.Uint)

		// --- Then ---
		assert.ErrorIs(t, ErrReqSingleValue, err)
	})
}

func Test_parseFloat(t *testing.T) {
	t.Run("float32", func(t *testing.T) {
		// --- When ---
		have, err := parseFloat([]string{"0.1"}, reflect.Float32)

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, float64(float32(0.1)), have)
	})

	t.Run("error - not a number", func(t *testing.T) {
		// --- When ---
		have, err := parseFloat([]string{"x"}, reflect.Float64)

		// --- Then ---
		want := "invalid value: cannot convert `x` to float64"
		assert.ErrorEqual(t, want, err)
		assert.Equal(t, 0.0, have)
	})

	t.Run("error - no value", func(t *testing.T) {
		// --- When ---
		_, err := parseFloat(nil, reflect.Float64)

		// --- Then ---
		assert.ErrorIs(t, ErrReqSingleValue, err)
	})
}

func Test_bitSize_tabular(t *testing.T) {
	tt := []struct {
		testN string

		kind reflect.Kind
		want int
	}{
		{"int", reflect.Int, 0},
		{"uint", reflect.Uint, 0},
		{"int8", reflect.Int8, 8},
		{"uint8", reflect.Uint8, 8},
		{"int16", reflect.Int16, 16},
		{"uint16", reflect.Uint16, 16},
		{"int32", reflect.Int32, 32},
		{"uint32", reflect.Uint32, 32},
		{"float32", reflect.Float32, 32},
		{"int64", reflect.Int64, 64},
		{"uint64", reflect.Uint64, 64},
		{"float64", reflect.Float64, 64},
	}

	for _, tc := range tt {
		t.Run(tc.testN, func(t *testing.T) {
			// --- When ---
			have := bitSize(tc.kind)

			// --- Then ---
			assert.Equal(t, tc.want, have)
		})
	}
}

func Test_parseTime(t *testing.T) {
	t.Run("keeps the offset", func(t *testing.T) {
		// --- When ---
		have, err := parseTime([]string{"2000-01-02T03:04:05+01:00"})

		// --- Then ---
		assert.NoError(t, err)
		_, off := have.Zone()
		assert.Equal(t, 3600, off)
	})

	t.Run("error - offset hour over 23", func(t *testing.T) {
		// --- When ---
		have, err := parseTime([]string{"2000-01-02T03:04:05+24:00"})

		// --- Then ---
		want := "invalid RFC3339 time: `2000-01-02T03:04:05+24:00`"
		assert.ErrorEqual(t, want, err)
		assert.Zero(t, have)
	})

	t.Run("error - no value", func(t *testing.T) {
		// --- When ---
		_, err := parseTime(nil)

		// --- Then ---
		assert.ErrorIs(t, ErrReqSingleValue, err)
	})
}

func Test_parseDur(t *testing.T) {
	t.Run("duration", func(t *testing.T) {
		// --- When ---
		have, err := parseDur([]string{"1m"})

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, time.Minute, have)
	})

	t.Run("error - not a duration", func(t *testing.T) {
		// --- When ---
		have, err := parseDur([]string{"x"})

		// --- Then ---
		assert.ErrorEqual(t, "invalid duration string: `x`", err)
		assert.Equal(t, time.Duration(0), have)
	})

	t.Run("error - no value", func(t *testing.T) {
		// --- When ---
		_, err := parseDur(nil)

		// --- Then ---
		assert.ErrorIs(t, ErrReqSingleValue, err)
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
