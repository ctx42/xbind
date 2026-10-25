// SPDX-FileCopyrightText: (c) 2026 Rafal Zajac
// SPDX-License-Identifier: MIT

package xbind

import (
	"encoding"
	"fmt"
	"io"
	"math"
	"net/url"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ctx42/testing/pkg/assert"
	"github.com/ctx42/testing/pkg/must"
	"github.com/ctx42/testkit/pkg/reflectkit"
	"github.com/ctx42/xrr/pkg/xrr"
	"github.com/ctx42/xrr/pkg/xrr/xrrtest"

	"github.com/ctx42/mirror/pkg/mirror"
)

func Test_WithDecTag(t *testing.T) {
	// --- Given ---
	dec := &Decoder{}

	// --- When ---
	WithDecTag("abc")(dec)

	// --- Then ---
	assert.Equal(t, "abc", dec.tag)
}

func Test_WithDecAllowUnknown(t *testing.T) {
	// --- Given ---
	dec := &Decoder{}

	// --- When ---
	WithDecAllowUnknown()(dec)

	// --- Then ---
	assert.True(t, dec.allowUnknown)
}

func Test_WithDecMaxIndex(t *testing.T) {
	// --- Given ---
	dec := &Decoder{}

	// --- When ---
	WithDecMaxIndex(5)(dec)

	// --- Then ---
	assert.Equal(t, 5, *dec.maxIndex)
}

func Test_WithDecMaxDepth(t *testing.T) {
	// --- Given ---
	dec := &Decoder{}

	// --- When ---
	WithDecMaxDepth(5)(dec)

	// --- Then ---
	assert.Equal(t, 5, *dec.maxDepth)
}

func Test_WithDecFunc(t *testing.T) {
	// --- Given ---
	dec := &Decoder{}

	// --- When ---
	WithDecFunc(decPoint)(dec)

	// --- Then ---
	fn := dec.reg[reflect.TypeFor[TPoint]()]
	assert.Equal(t, TPoint{X: 1, Y: 2}, must.Value(fn("1,2")))

	_, err := fn("1,2", "3,4")
	assert.ErrorIs(t, ErrReqSingleValue, err)
}

func Test_NewDecoder(t *testing.T) {
	t.Run("defaults", func(t *testing.T) {
		// --- When ---
		have := NewDecoder()

		// --- Then ---
		assert.Equal(t, "url", have.tag)
		assert.False(t, have.allowUnknown)
		assert.Nil(t, have.reg)
	})

	t.Run("with options", func(t *testing.T) {
		// --- When ---
		have := NewDecoder(
			WithDecTag("abc"),
			WithDecAllowUnknown(),
			WithDecFunc(decPoint),
		)

		// --- Then ---
		assert.Equal(t, "abc", have.tag)
		assert.True(t, have.allowUnknown)
		assert.HasKey(t, reflect.TypeFor[TPoint](), have.reg)
	})
}

func Test_Decoder_Decode(t *testing.T) {
	t.Run("error - destination not struct", func(t *testing.T) {
		// --- Given ---
		dst := map[string]string{}
		src := url.Values{}

		// --- When ---
		err := NewDecoder().Decode(dst, src)

		// --- Then ---
		xrrtest.AssertEqual(t, "struct pointer required (ECReqStructPtr)", err)
	})

	t.Run("struct", func(t *testing.T) {
		// --- Given ---
		dst := &TString{}
		src := url.Values{
			"f_str":         {"FStr"},
			"f_p_str":       {"FpStr"},
			"f_s_str":       {"", "abc", "", "xyz"},
			"f_s_p_str":     {"", "xyz", "", "abc"},
			"f_m_int_str.1": {"abc"},
			"f_m_int_str.3": {"xyz"},
			"f_a_str":       {"abc", "xyz"},
			"f_a_p_str":     {"xyz", "abc"},
		}

		// --- When ---
		err := NewDecoder().Decode(dst, src)

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, "FStr", dst.FStr)
		assert.Equal(t, "FpStr", *dst.FpStr)
		assert.Equal(t, []string{"", "abc", "", "xyz"}, dst.FsStr)
		want := []*string{new(""), new("xyz"), new(""), new("abc")}
		assert.Equal(t, want, dst.FspStr)
		assert.Equal(t, map[int]string{1: "abc", 3: "xyz"}, dst.FmStr)
		assert.Equal(t, [2]string{"abc", "xyz"}, dst.FaStr)
		assert.Equal(t, [2]*string{new("xyz"), new("abc")}, dst.FapStr)
	})

	t.Run("sub struct", func(t *testing.T) {
		// --- Given ---
		dst := &struct {
			Sub0 TString `url:"sub0"`
			FInt int     `url:"f_int"`
		}{}
		src := url.Values{
			"sub0.f_str":         {"FStr0"},
			"sub0.f_p_str":       {"FpStr0"},
			"sub0.f_s_str":       {"", "abc0", "", "xyz0"},
			"sub0.f_s_p_str":     {"", "xyz0", "", "abc0"},
			"sub0.f_m_int_str.1": {"abc0"},
			"sub0.f_m_int_str.3": {"xyz0"},
			"sub0.f_a_str":       {"abc0", "xyz0"},
			"sub0.f_a_p_str":     {"xyz0", "abc0"},
			"f_int":              {"10"},
		}

		// --- When ---
		err := NewDecoder().Decode(dst, src)

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, "FStr0", dst.Sub0.FStr)
		assert.Equal(t, new("FpStr0"), dst.Sub0.FpStr)
		assert.Equal(t, []string{"", "abc0", "", "xyz0"}, dst.Sub0.FsStr)
		want := []*string{new(""), new("xyz0"), new(""), new("abc0")}
		assert.Equal(t, want, dst.Sub0.FspStr)
		assert.Equal(t, map[int]string{1: "abc0", 3: "xyz0"}, dst.Sub0.FmStr)
		assert.Equal(t, [2]string{"abc0", "xyz0"}, dst.Sub0.FaStr)
		assert.Equal(t, [2]*string{new("xyz0"), new("abc0")}, dst.Sub0.FapStr)
		assert.Equal(t, 10, dst.FInt)
	})

	t.Run("sub struct pointer", func(t *testing.T) {
		// --- Given ---
		dst := &struct {
			Sub0 *TString `url:"sub0"`
			FInt int      `url:"f_int"`
		}{}
		src := url.Values{
			"sub0.f_str":         {"FStr0"},
			"sub0.f_p_str":       {"FpStr0"},
			"sub0.f_s_str":       {"", "abc0", "", "xyz0"},
			"sub0.f_s_p_str":     {"", "xyz0", "", "abc0"},
			"sub0.f_m_int_str.1": {"abc0"},
			"sub0.f_m_int_str.3": {"xyz0"},
			"sub0.f_a_str":       {"abc0", "xyz0"},
			"sub0.f_a_p_str":     {"xyz0", "abc0"},
			"f_int":              {"10"},
		}

		// --- When ---
		err := NewDecoder().Decode(dst, src)

		// --- Then ---
		assert.NoError(t, err)
		assert.NotNil(t, dst.Sub0)
		assert.Equal(t, "FStr0", dst.Sub0.FStr)
		assert.Equal(t, new("FpStr0"), dst.Sub0.FpStr)
		assert.Equal(t, []string{"", "abc0", "", "xyz0"}, dst.Sub0.FsStr)
		want := []*string{new(""), new("xyz0"), new(""), new("abc0")}
		assert.Equal(t, want, dst.Sub0.FspStr)
		assert.Equal(t, map[int]string{1: "abc0", 3: "xyz0"}, dst.Sub0.FmStr)
		assert.Equal(t, [2]string{"abc0", "xyz0"}, dst.Sub0.FaStr)
		assert.Equal(t, [2]*string{new("xyz0"), new("abc0")}, dst.Sub0.FapStr)
		assert.Equal(t, 10, dst.FInt)
	})

	t.Run("error - slice of structs", func(t *testing.T) {
		// --- Given ---
		dst := &struct {
			FsS []TString `url:"f_s_s"`
		}{}
		src := url.Values{"f_s_s": {"a", "b"}}

		// --- When ---
		err := NewDecoder().Decode(dst, src)

		// --- Then ---
		xrrtest.AssertFieldCnt(t, 1, err)
		want := "decoder not found for `xbind.TString`"
		xrrtest.AssertFieldEqual(t, "f_s_s", want, err)
		xrrtest.AssertFieldCode(t, "f_s_s", ECDecNotFound, err)
	})

	t.Run("struct with a time field", func(t *testing.T) {
		// --- Given ---
		dst := &TWithTime{}
		src := url.Values{"f_time": {"2022-03-04T05:06:07Z"}}

		// --- When ---
		err := NewDecoder().Decode(dst, src)

		// --- Then ---
		assert.NoError(t, err)
		assert.Time(t, "2022-03-04T05:06:07Z", dst.FTime)
	})

	t.Run("struct with an anonymous field", func(t *testing.T) {
		// --- Given ---
		dst := &Anonymous{}
		src := url.Values{"f_time": {"2022-03-04T05:06:07Z"}}

		// --- When ---
		err := NewDecoder().Decode(dst, src)

		// --- Then ---
		assert.NoError(t, err)
		assert.Time(t, "2022-03-04T05:06:07Z", dst.FTime)
	})

	t.Run("struct with an anonymous pointer field", func(t *testing.T) {
		// --- Given ---
		dst := &AnonymousPtr{}
		src := url.Values{"f_time": {"2022-03-04T05:06:07Z"}}

		// --- When ---
		err := NewDecoder().Decode(dst, src)

		// --- Then ---
		assert.NoError(t, err)
		assert.Time(t, "2022-03-04T05:06:07Z", dst.FTime)
	})

	t.Run("unmarshaler fields", func(t *testing.T) {
		// --- Given ---
		dst := &TSWithUnm{}
		src := url.Values{
			"f_um":     {"1"},
			"f_p_um":   {"2"},
			"f_a_p_um": {"3"},
			"f_s_p_um": {"4", "5"},
		}

		// --- When ---
		err := NewDecoder().Decode(dst, src)

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, TTextPtr(1), dst.FUm)
		assert.Equal(t, new(TTextPtr(2)), dst.FpUm)
		assert.Equal(t, [1]*TTextPtr{new(TTextPtr(3))}, dst.FapUm)
		want := []*TTextPtr{new(TTextPtr(4)), new(TTextPtr(5))}
		assert.Equal(t, want, dst.FspUm)
	})

	t.Run("error - embedded interface has no key", func(t *testing.T) {
		// --- Given ---
		dst := &struct {
			io.Reader
			FStr string `url:"f_str"`
		}{}
		src := url.Values{"Reader": {"x"}, "f_str": {"abc"}}

		// --- When ---
		err := NewDecoder().Decode(dst, src)

		// --- Then ---
		want := "Reader: invalid field path (ECUnkFieldPath)"
		xrrtest.AssertFieldsEqual(t, want, err)
		assert.Equal(t, "abc", dst.FStr)
	})

	t.Run("array implementing UnmarshalText", func(t *testing.T) {
		// --- Given ---
		dst := &struct{ F ArrImp }{}
		src := url.Values{"F": {"abc"}}

		// --- When ---
		err := NewDecoder().Decode(dst, src)

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, ArrImp{'A', 'B', 'C'}, dst.F)
	})

	t.Run("error - prefix not modified by struct fields", func(t *testing.T) {
		// --- Given ---
		dst := &struct {
			F1 time.Time `url:"f_1"`
			F2 int       `url:"f_2,required"`
		}{}
		src := url.Values{"f_1": {"2000-01-02T03:04:05"}}

		// --- When ---
		err := NewDecoder().Decode(dst, src)

		// --- Then ---
		want := "" +
			"f_1: invalid RFC3339 time: `2000-01-02T03:04:05` " +
			"(ECInvTimeFormat); " +
			"f_2: cannot be blank (ECRequired)"
		xrrtest.AssertFieldsEqual(t, want, err)
	})

	t.Run("field names overlap but no tag name overlap", func(t *testing.T) {
		// --- Given ---
		dst := &struct {
			TRequired
			FStr  string    `url:"f_mix_str"`
			FTime time.Time `url:"f_mix_time"`
			FInt  int       `url:"f_mix_int"`
		}{}
		src := url.Values{
			"f_str":      {"abc"},
			"f_int":      {"123"},
			"f_mix_str":  {"def"},
			"f_mix_time": {"2000-01-02T03:04:05Z"},
			"f_mix_int":  {"456"},
		}

		// --- When ---
		err := NewDecoder().Decode(dst, src)

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, "abc", dst.TRequired.FStr)
		assert.Equal(t, 123, dst.TRequired.FInt)
		assert.Equal(t, "def", dst.FStr)
		assert.Exact(t, "2000-01-02T03:04:05Z", dst.FTime)
		assert.Equal(t, 456, dst.FInt)
	})

	t.Run("outer field shadows embedded field", func(t *testing.T) {
		// --- Given ---
		dst := &struct {
			TRequired
			FStr  string    `url:"f_str"`
			FTime time.Time `url:"f_mix_time"`
			FInt  int       `url:"f_int"`
		}{}
		src := url.Values{
			"f_str":      {"abc"},
			"f_int":      {"123"},
			"f_mix_time": {"2000-01-02T03:04:05Z"},
		}

		// --- When ---
		err := NewDecoder().Decode(dst, src)

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, "", dst.TRequired.FStr)
		assert.Equal(t, 0, dst.TRequired.FInt)
		assert.Equal(t, "abc", dst.FStr)
		assert.Exact(t, "2000-01-02T03:04:05Z", dst.FTime)
		assert.Equal(t, 123, dst.FInt)
	})

	t.Run("outer field declared first shadows", func(t *testing.T) {
		// --- Given ---
		dst := &struct {
			FStr string `url:"f_str"`
			TRequired
		}{}
		src := url.Values{"f_str": {"abc"}, "f_int": {"1"}}

		// --- When ---
		err := NewDecoder().Decode(dst, src)

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, "abc", dst.FStr)
		assert.Equal(t, "", dst.TRequired.FStr)
		assert.Equal(t, 1, dst.FInt)
	})

	t.Run("error - ambiguous fields at the same depth", func(t *testing.T) {
		// --- Given ---
		dst := &TAmbig{}
		src := url.Values{"x": {"1"}, "a": {"2"}}

		// --- When ---
		err := NewDecoder().Decode(dst, src)

		// --- Then ---
		want := "x: invalid field path (ECUnkFieldPath)"
		xrrtest.AssertFieldsEqual(t, want, err)
		assert.Equal(t, "2", dst.A)
	})

	t.Run("error - ambiguous fields with an ignored one", func(t *testing.T) {
		// --- Given ---
		type ign struct {
			x int `url:"-"`
		}
		dst := &struct {
			TAmbA
			TAmbB
			ign
		}{ign: ign{x: 1}}
		src := url.Values{"x": {"1"}}

		// --- When ---
		err := NewDecoder().Decode(dst, src)

		// --- Then ---
		want := "x: invalid field path (ECUnkFieldPath)"
		xrrtest.AssertFieldsEqual(t, want, err)
	})

	t.Run("tagged field wins at the same depth", func(t *testing.T) {
		// --- Given ---
		dst := &TTagged{}
		src := url.Values{"X": {"1"}}

		// --- When ---
		err := NewDecoder().Decode(dst, src)

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, "1", dst.TTagA.X)
		assert.Equal(t, "", dst.TTagB.X)
	})

	t.Run("do not set an ignored field", func(t *testing.T) {
		// --- Given ---
		dst := &struct {
			FStr string
			FInt int `url:"-"`
		}{}
		src := url.Values{
			"FStr": {"abc"},
			"FInt": {"123"},
		}

		// --- When ---
		err := NewDecoder().Decode(dst, src)

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, "abc", dst.FStr)
		assert.Equal(t, 0, dst.FInt)
	})

	t.Run("map of string slices", func(t *testing.T) {
		// --- Given ---
		dst := &struct {
			Fm map[string][]string
		}{}
		src := url.Values{
			"Fm.A": {"1", "2"},
			"Fm.B": {"3", "4", "5"},
		}

		// --- When ---
		err := NewDecoder().Decode(dst, src)

		// --- Then ---
		assert.NoError(t, err)
		want := map[string][]string{
			"A": {"1", "2"},
			"B": {"3", "4", "5"},
		}
		assert.Equal(t, want, dst.Fm)
	})

	t.Run("error - decoding to not exported field", func(t *testing.T) {
		// --- Given ---
		dst := &struct{ field int }{field: 42}
		src := url.Values{"field": {"1"}}

		// --- When ---
		err := NewDecoder().Decode(dst, src)

		// --- Then ---
		assert.ErrorEqual(t, "field: invalid field path", err)
		assert.Equal(t, 42, dst.field)
	})

	t.Run("struct with a struct implementing unmarshal", func(t *testing.T) {
		// --- Given ---
		dst := &TComplex{}
		src := url.Values{
			"f_itf": {"A"},
			"f_int": {"1"},
		}

		// --- When ---
		err := NewDecoder().Decode(dst, src)

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, &TComplex{FItf: TItf{Str: "A"}, FInt: 1}, dst)
	})

	t.Run("error - required unmarshaler struct not set", func(t *testing.T) {
		// --- Given ---
		dst := &TComplex{}
		src := url.Values{"f_int": {"1"}}

		// --- When ---
		err := NewDecoder().Decode(dst, src)

		// --- Then ---
		xrrtest.AssertFieldCnt(t, 1, err)
		xrrtest.AssertFieldIs(t, "f_itf", ErrRequired, err)
	})

	t.Run("error - key into unexported nil pointer", func(t *testing.T) {
		// --- Given ---
		dst := &TUnexpPtr{}
		src := url.Values{"f_int": {"1"}, "f_str": {"a"}}

		// --- When ---
		err := NewDecoder().Decode(dst, src)

		// --- Then ---
		want := "f_str: unexported field (ECUnexported)"
		xrrtest.AssertFieldsEqual(t, want, err)
		assert.Equal(t, 1, dst.FInt)
		assert.Nil(t, dst.tStr)
	})

	t.Run("error - nested field errors keyed by path", func(t *testing.T) {
		// --- Given ---
		dst := &struct {
			Sub0 TRequired `url:"sub0"`
			Sub1 TRequired `url:"sub1"`
		}{}
		src := url.Values{
			"sub0.f_str": {"a"},
			"sub0.f_int": {"x"},
			"sub1.f_str": {"b"},
			"sub1.f_int": {"y"},
		}

		// --- When ---
		err := NewDecoder().Decode(dst, src)

		// --- Then ---
		xrrtest.AssertFieldCnt(t, 2, err)
		want := "invalid value: cannot convert `x` to int"
		xrrtest.AssertFieldEqual(t, "sub0.f_int", want, err)
		want = "invalid value: cannot convert `y` to int"
		xrrtest.AssertFieldEqual(t, "sub1.f_int", want, err)
	})

	t.Run("anonymous struct in named struct", func(t *testing.T) {
		// --- Given ---
		dst := &struct {
			Sub struct{ TRequired } `url:"sub"`
		}{}
		src := url.Values{"sub.f_str": {"a"}}

		// --- When ---
		err := NewDecoder().Decode(dst, src)

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, "a", dst.Sub.FStr)
	})

	t.Run("ignored struct field", func(t *testing.T) {
		// --- Given ---
		dst := &struct {
			Sub TRequired `url:"-"`
		}{}
		src := url.Values{"Sub.f_str": {"a"}}

		// --- When ---
		err := NewDecoder().Decode(dst, src)

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, "", dst.Sub.FStr)
	})

	t.Run("required unexported fields", func(t *testing.T) {
		// --- Given ---
		dst := &struct {
			val int       `url:"val,required"`
			sub TRequired `url:"sub"`
		}{}
		src := url.Values{}

		// --- When ---
		err := NewDecoder().Decode(dst, src)

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, 0, dst.val)
		assert.Equal(t, "", dst.sub.FStr)
	})

	t.Run("error - unmarshaler interface field", func(t *testing.T) {
		// --- Given ---
		dst := &struct{ F encoding.TextUnmarshaler }{}
		src := url.Values{"F": {"x"}}

		// --- When ---
		err := NewDecoder().Decode(dst, src)

		// --- Then ---
		want := "decoder not found for `encoding.TextUnmarshaler`"
		xrrtest.AssertFieldEqual(t, "F", want, err)
	})

	t.Run("pointer to pointer implementing unmarshaler", func(t *testing.T) {
		// --- Given ---
		dst := &struct{ F **TTextPtr }{}
		src := url.Values{"F": {"2A"}}

		// --- When ---
		err := NewDecoder().Decode(dst, src)

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, TTextPtr(42), **dst.F)
	})

	t.Run("error - path continues past a field", func(t *testing.T) {
		// --- Given ---
		dst := &TString{}
		src := url.Values{"f_str.junk": {"a"}}

		// --- When ---
		err := NewDecoder().Decode(dst, src)

		// --- Then ---
		xrrtest.AssertFieldEqual(t, "f_str.junk", "invalid field path", err)
		assert.Equal(t, "", dst.FStr)
	})

	t.Run("error - path continues past a map key", func(t *testing.T) {
		// --- Given ---
		dst := &TString{}
		src := url.Values{"f_m_int_str.1.junk": {"a"}}

		// --- When ---
		err := NewDecoder().Decode(dst, src)

		// --- Then ---
		want := "invalid value: cannot convert `1.junk` to int"
		xrrtest.AssertFieldEqual(t, "f_m_int_str.1.junk", want, err)
		assert.Len(t, 0, dst.FmStr)
	})

	t.Run("map key with dots", func(t *testing.T) {
		// --- Given ---
		dst := &struct{ M map[string]string }{}
		src := url.Values{"M.a.b": {"x"}}

		// --- When ---
		err := NewDecoder().Decode(dst, src)

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, map[string]string{"a.b": "x"}, dst.M)
	})

	t.Run("error - map with empty key", func(t *testing.T) {
		// --- Given ---
		dst := &struct{ M map[string]string }{}
		src := url.Values{"M.": {"a"}}

		// --- When ---
		err := NewDecoder().Decode(dst, src)

		// --- Then ---
		xrrtest.AssertFieldEqual(t, "M.", "invalid field path", err)
		assert.Nil(t, dst.M)
	})

	t.Run("error - map without a key", func(t *testing.T) {
		// --- Given ---
		dst := &TString{}
		src := url.Values{"f_m_int_str": {"a"}}

		// --- When ---
		err := NewDecoder().Decode(dst, src)

		// --- Then ---
		xrrtest.AssertFieldEqual(t, "f_m_int_str", "invalid field path", err)
		assert.Len(t, 0, dst.FmStr)
	})

	t.Run("slice of pointers to pointers", func(t *testing.T) {
		// --- Given ---
		dst := &struct{ F []**string }{}
		src := url.Values{"F": {"a"}}

		// --- When ---
		err := NewDecoder().Decode(dst, src)

		// --- Then ---
		assert.NoError(t, err)
		assert.Len(t, 1, dst.F)
		assert.Equal(t, "a", **dst.F[0])
	})

	t.Run("error - many values for unmarshaler", func(t *testing.T) {
		// --- Given ---
		dst := &TSWithUnm{}
		src := url.Values{"f_um": {"1", "2"}}

		// --- When ---
		err := NewDecoder().Decode(dst, src)

		// --- Then ---
		want := "require single value: received 2"
		xrrtest.AssertFieldEqual(t, "f_um", want, err)
	})

	t.Run("embedded non-struct type", func(t *testing.T) {
		// --- Given ---
		dst := &struct{ IntAlias }{}
		src := url.Values{"IntAlias": {"5"}}

		// --- When ---
		err := NewDecoder().Decode(dst, src)

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, IntAlias(5), dst.IntAlias)
	})

	t.Run("zero value decoder", func(t *testing.T) {
		// --- Given ---
		dst := &struct {
			A int       `url:"a"`
			T time.Time `url:"t"`
		}{}
		src := url.Values{"a": {"1"}, "t": {"2022-03-15T11:43:01Z"}}

		// --- When ---
		err := (&Decoder{}).Decode(dst, src)

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, 1, dst.A)
		assert.Equal(t, "2022-03-15T11:43:01Z", dst.T.Format(time.RFC3339))
	})

	t.Run("multi embedded first struct", func(t *testing.T) {
		// --- Given ---
		dst := &TMultiEmbedded{}
		src := url.Values{"f_str": {"abc"}}

		// --- When ---
		err := NewDecoder().Decode(dst, src)

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, "abc", dst.FStr)
	})

	t.Run("multi embedded both structs", func(t *testing.T) {
		// --- Given ---
		dst := &TMultiEmbedded{}
		src := url.Values{
			"f_str":       {"abc"},
			"f_int_alias": {"1"},
		}

		// --- When ---
		err := NewDecoder().Decode(dst, src)

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, "abc", dst.FStr)
		assert.Equal(t, IntAlias(1), dst.FIntAlias)
	})

	t.Run("embedded struct and field", func(t *testing.T) {
		// --- Given ---
		dst := &struct {
			TString
			FNumber int `url:"f_number"`
		}{}
		src := url.Values{
			"f_str":    {"abc"},
			"f_number": {"1"},
		}

		// --- When ---
		err := NewDecoder().Decode(dst, src)

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, "abc", dst.FStr)
		assert.Equal(t, 1, dst.FNumber)
	})

	t.Run("embedded struct and field only field set", func(t *testing.T) {
		// --- Given ---
		dst := &struct {
			TString
			FNumber int `url:"f_number"`
		}{}
		src := url.Values{"f_number": {"1"}}

		// --- When ---
		err := NewDecoder().Decode(dst, src)

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, 1, dst.FNumber)
	})

	t.Run("error - required field not set", func(t *testing.T) {
		// --- Given ---
		dst := &TRequired{}
		src := url.Values{"f_int": {"123"}}

		// --- When ---
		err := NewDecoder().Decode(dst, src)

		// --- Then ---
		xrrtest.AssertFieldCnt(t, 1, err)
		xrrtest.AssertFieldIs(t, "f_str", ErrRequired, err)
		assert.Equal(t, "", dst.FStr)
		assert.Equal(t, 123, dst.FInt)
	})

	t.Run("error - nested required field not set", func(t *testing.T) {
		// --- Given ---
		dst := &struct {
			Sub0 *TRequired `url:"sub0"`
			Sub1 *TRequired `url:"sub1"`
		}{}
		src := url.Values{"sub0.f_int": {"123"}}

		// --- When ---
		err := NewDecoder().Decode(dst, src)

		// --- Then ---
		xrrtest.AssertFieldCnt(t, 1, err)
		xrrtest.AssertFieldIs(t, "sub0.f_str", ErrRequired, err)
		assert.Equal(t, "", dst.Sub0.FStr)
		assert.Equal(t, 123, dst.Sub0.FInt)
		assert.Nil(t, dst.Sub1)
	})

	t.Run("error - unknown keys not allowed by default", func(t *testing.T) {
		// --- Given ---
		dst := &struct {
			Sub0 *TRequired `url:"sub0"`
			Sub1 *TRequired `url:"sub1"`
		}{}
		src := url.Values{
			"sub0.f_str": {"abc"},
			"sub0.unk":   {"unk"},
			"sub1.f_str": {"xyz"},
			"sub1.unk":   {"unk"},
			"unk":        {"unk"},
		}

		// --- When ---
		err := NewDecoder().Decode(dst, src)

		// --- Then ---
		xrrtest.AssertFieldCnt(t, 3, err)
		xrrtest.AssertFieldEqual(t, "unk", "invalid field path", err)
		xrrtest.AssertFieldCode(t, "unk", "ECUnkFieldPath", err)
		xrrtest.AssertFieldEqual(t, "sub0.unk", "invalid field path", err)
		xrrtest.AssertFieldCode(t, "sub0.unk", "ECUnkFieldPath", err)
		xrrtest.AssertFieldEqual(t, "sub1.unk", "invalid field path", err)
		xrrtest.AssertFieldCode(t, "sub1.unk", "ECUnkFieldPath", err)
	})

	t.Run("unknown keys allowed", func(t *testing.T) {
		// --- Given ---
		dst := &struct {
			Sub0 *TRequired `url:"sub0"`
			Sub1 *TRequired `url:"sub1"`
		}{}
		src := url.Values{
			"sub0.f_str": {"abc"},
			"sub0.unk":   {"unk"},
			"sub1.f_str": {"xyz"},
			"sub1.unk":   {"unk"},
			"unk":        {"unk"},
		}

		// --- When ---
		err := NewDecoder(WithDecAllowUnknown()).Decode(dst, src)

		// --- Then ---
		assert.NoError(t, err)
	})

	t.Run("pointer to pointer to time field", func(t *testing.T) {
		// --- Given ---
		dst := &struct {
			T **time.Time `url:"t"`
		}{}
		src := url.Values{"t": {"2022-03-15T11:43:01Z"}}

		// --- When ---
		err := NewDecoder().Decode(dst, src)

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, "2022-03-15T11:43:01Z", (**dst.T).Format(time.RFC3339))
	})

	t.Run("error - pointer to pointer to struct field", func(t *testing.T) {
		// --- Given ---
		dst := &struct {
			F **TString `url:"f"`
		}{}
		src := url.Values{"f.f_str": {"a"}}

		// --- When ---
		err := NewDecoder().Decode(dst, src)

		// --- Then ---
		xrrtest.AssertFieldEqual(t, "f.f_str", "invalid field path", err)
	})

	t.Run("map type implementing unmarshaler", func(t *testing.T) {
		// --- Given ---
		dst := &struct{ M TUnmMap }{}
		src := url.Values{"M": {"a=b"}}

		// --- When ---
		err := NewDecoder().Decode(dst, src)

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, TUnmMap{"a": "b"}, dst.M)
	})

	t.Run("error - unmarshaler map with key", func(t *testing.T) {
		// --- Given ---
		dst := &struct{ M TUnmMap }{}
		src := url.Values{"M.a": {"b"}}

		// --- When ---
		err := NewDecoder().Decode(dst, src)

		// --- Then ---
		xrrtest.AssertFieldEqual(t, "M.a", "invalid field path", err)
	})

	t.Run("pointer to map field", func(t *testing.T) {
		// --- Given ---
		dst := &struct{ M *map[string]int }{}
		src := url.Values{"M.a": {"1"}}

		// --- When ---
		err := NewDecoder().Decode(dst, src)

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, map[string]int{"a": 1}, *dst.M)
	})

	t.Run("ignored field in embedded struct", func(t *testing.T) {
		// --- Given ---
		type ign struct {
			FInt int `url:"-"`
			FStr string
		}
		dst := &struct{ ign }{}
		src := url.Values{"FInt": {"1"}, "FStr": {"a"}}

		// --- When ---
		err := NewDecoder().Decode(dst, src)

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, 0, dst.FInt)
		assert.Equal(t, "a", dst.FStr)
	})

	t.Run("error - nil embedded unexported unmarshaler", func(t *testing.T) {
		// --- Given ---
		dst := &struct {
			*tUnmInner
			Y int `url:"y"`
		}{}
		src := url.Values{"x": {"1"}}

		// --- When ---
		err := NewDecoder().Decode(dst, src)

		// --- Then ---
		xrrtest.AssertFieldsEqual(t, "x: unexported field (ECUnexported)", err)
	})

	t.Run("error - unmarshaler from nil interface", func(t *testing.T) {
		// --- Given ---
		dst := &struct {
			W struct{ encoding.TextUnmarshaler } `url:"w"`
		}{}
		src := url.Values{"w": {"x"}}

		// --- When ---
		err := NewDecoder().Decode(dst, src)

		// --- Then ---
		xrrtest.AssertFieldCnt(t, 1, err)
		xrrtest.AssertFieldIs(t, "w", ErrDecNotFound, err)
	})

	t.Run("unmarshaler from shallower of two nil pointers", func(t *testing.T) {
		// --- Given ---
		type wrap struct{ *TTextPtr }
		dst := &struct {
			F struct {
				wrap
				*TTextPtr
			} `url:"f"`
		}{}
		src := url.Values{"f": {"2A"}}

		// --- When ---
		err := NewDecoder().Decode(dst, src)

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, TTextPtr(42), *dst.F.TTextPtr)
	})

	t.Run("unmarshaler behind an embedded interface", func(t *testing.T) {
		// --- Given ---
		type wrap struct{ *TItf }
		type itf = struct{ encoding.TextUnmarshaler }
		in := &wrap{}
		dst := &struct {
			F itf `url:"f"`
		}{F: itf{in}}
		src := url.Values{"f": {"x"}}

		// --- When ---
		err := NewDecoder().Decode(dst, src)

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, "x", in.Str)
	})

	t.Run("own unmarshaler leaves embeds nil", func(t *testing.T) {
		// --- Given ---
		dst := &struct {
			F TOwnUnm `url:"f"`
		}{}
		src := url.Values{"f": {"x"}}

		// --- When ---
		err := NewDecoder().Decode(dst, src)

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, "x", dst.F.S)
		assert.Nil(t, dst.F.TItem)
	})

	t.Run("unmarshaler skips unexported nil pointer", func(t *testing.T) {
		// --- Given ---
		dst := &struct {
			F struct {
				TOwnUnm
				*tStr
			} `url:"f"`
		}{}
		src := url.Values{"f": {"x"}}

		// --- When ---
		err := NewDecoder().Decode(dst, src)

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, "x", dst.F.S)
		assert.Nil(t, dst.F.tStr)
	})

	t.Run("ignored field does not hide a tagged field", func(t *testing.T) {
		// --- Given ---
		dst := &struct {
			Name  string `url:"-"`
			Other string `url:"Name,required"`
		}{}
		src := url.Values{"Name": {"x"}}

		// --- When ---
		err := NewDecoder().Decode(dst, src)

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, "", dst.Name)
		assert.Equal(t, "x", dst.Other)
	})

	t.Run("unexported embedded non-struct is skipped", func(t *testing.T) {
		// --- Given ---
		type myInt int
		dst := &struct {
			myInt `url:"m,required"`
		}{}
		src := url.Values{}

		// --- When ---
		err := NewDecoder(WithDecAllowUnknown()).Decode(dst, src)

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, myInt(0), dst.myInt)
	})

	t.Run("error - array unmarshaler", func(t *testing.T) {
		// --- Given ---
		dst := &struct{ F ArrImp }{}
		src := url.Values{"F": {"abcd"}}

		// --- When ---
		err := NewDecoder().Decode(dst, src)

		// --- Then ---
		xrrtest.AssertFieldEqual(t, "F", "string too long", err)
	})

	t.Run("pre-filled slice and array", func(t *testing.T) {
		// --- Given ---
		dst := &struct {
			S []string `url:"s"`
			A [2]int   `url:"a"`
			D []string `url:"d"`
		}{
			S: []string{"default"},
			A: [2]int{7, 8},
			D: []string{"default"},
		}
		src := url.Values{"s": {"x"}, "a": {"1"}}

		// --- When ---
		err := NewDecoder().Decode(dst, src)

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, []string{"x"}, dst.S)
		assert.Equal(t, [2]int{1, 0}, dst.A)
		assert.Equal(t, []string{"default"}, dst.D)
	})

	t.Run("error - path ends at a struct field", func(t *testing.T) {
		// --- Given ---
		dst := &struct {
			Sub TString `url:"sub"`
		}{}
		src := url.Values{"sub": {"a"}}

		// --- When ---
		err := NewDecoder().Decode(dst, src)

		// --- Then ---
		xrrtest.AssertFieldEqual(t, "sub", "invalid field path", err)
	})

	t.Run("error - ignored embedded struct", func(t *testing.T) {
		// --- Given ---
		dst := &struct {
			TRequired `url:"-"`
		}{}
		src := url.Values{"f_int": {"1"}}

		// --- When ---
		err := NewDecoder().Decode(dst, src)

		// --- Then ---
		xrrtest.AssertFieldEqual(t, "f_int", "invalid field path", err)
		assert.Equal(t, 0, dst.FInt)
	})

	t.Run("ignored embedded non-struct", func(t *testing.T) {
		// --- Given ---
		type MyInt int
		dst := &struct {
			MyInt `url:"-"`
		}{}
		src := url.Values{"MyInt": {"1"}}

		// --- When ---
		err := NewDecoder().Decode(dst, src)

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, MyInt(0), dst.MyInt)
	})

	t.Run("error - required pointer to unmarshaler", func(t *testing.T) {
		// --- Given ---
		dst := &struct {
			F *TItf `url:"f,required"`
		}{}
		src := url.Values{}

		// --- When ---
		err := NewDecoder().Decode(dst, src)

		// --- Then ---
		xrrtest.AssertFieldIs(t, "f", ErrRequired, err)
	})

	t.Run("nested unexported nil pointer", func(t *testing.T) {
		// --- Given ---
		dst := &struct {
			Sub TUnexpPtr `url:"sub"`
		}{}
		src := url.Values{"sub.f_int": {"1"}}

		// --- When ---
		err := NewDecoder().Decode(dst, src)

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, 1, dst.Sub.FInt)
		assert.Nil(t, dst.Sub.tStr)
	})

	t.Run("nil nested pointer is not allocated", func(t *testing.T) {
		// --- Given ---
		dst := &struct {
			Name string     `url:"name"`
			Sub  *TRequired `url:"sub"`
		}{}
		src := url.Values{"name": {"a"}}

		// --- When ---
		err := NewDecoder().Decode(dst, src)

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, "a", dst.Name)
		assert.Nil(t, dst.Sub)
	})

	t.Run("error - required in present nested struct", func(t *testing.T) {
		// --- Given ---
		dst := &struct {
			Sub *TRequired `url:"sub"`
			Val TRequired  `url:"val"`
		}{}
		src := url.Values{"sub.f_int": {"1"}}

		// --- When ---
		err := NewDecoder().Decode(dst, src)

		// --- Then ---
		want := "" +
			"sub.f_str: cannot be blank (ECRequired); " +
			"val.f_str: cannot be blank (ECRequired)"
		xrrtest.AssertFieldsEqual(t, want, err)
		assert.Equal(t, 1, dst.Sub.FInt)
	})

	t.Run("error - required nested struct not set", func(t *testing.T) {
		// --- Given ---
		dst := &struct {
			P *TRequired `url:"p,required"`
			V TRequired  `url:"v,required"`
		}{}
		src := url.Values{}

		// --- When ---
		err := NewDecoder().Decode(dst, src)

		// --- Then ---
		want := "" +
			"p: cannot be blank (ECRequired); " +
			"v: cannot be blank (ECRequired); " +
			"v.f_str: cannot be blank (ECRequired)"
		xrrtest.AssertFieldsEqual(t, want, err)
		assert.Nil(t, dst.P)
	})

	t.Run("required nested struct set by a key", func(t *testing.T) {
		// --- Given ---
		dst := &struct {
			P *TRequired `url:"p,required"`
		}{}
		src := url.Values{"p.f_str": {"a"}}

		// --- When ---
		err := NewDecoder().Decode(dst, src)

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, "a", dst.P.FStr)
	})

	t.Run("error - required empty values", func(t *testing.T) {
		// --- Given ---
		dst := &struct {
			S string   `url:"s,required"`
			N int      `url:"n,required"`
			L []string `url:"l,required"`
		}{}
		src := url.Values{"s": {""}, "n": {"0"}, "l": {}}

		// --- When ---
		err := NewDecoder().Decode(dst, src)

		// --- Then ---
		want := "" +
			"l: cannot be blank (ECRequired); " +
			"n: cannot be blank (ECRequired); " +
			"s: cannot be blank (ECRequired)"
		xrrtest.AssertFieldsEqual(t, want, err)
	})

	t.Run("required non-empty values", func(t *testing.T) {
		// --- Given ---
		dst := &struct {
			B bool   `url:"b,required"`
			S string `url:"s,required"`
		}{S: "set"}
		src := url.Values{}

		// --- When ---
		err := NewDecoder().Decode(dst, src)

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, "set", dst.S)
	})

	t.Run("error - value error kept over required", func(t *testing.T) {
		// --- Given ---
		dst := &struct {
			N int `url:"n,required"`
		}{}
		src := url.Values{"n": {"abc"}}

		// --- When ---
		err := NewDecoder().Decode(dst, src)

		// --- Then ---
		xrrtest.AssertFieldIs(t, "n", ErrInvValue, err)
	})

	t.Run("error - present fields not sent", func(t *testing.T) {
		// --- Given ---
		dst := &struct {
			B bool   `url:"b,present"`
			N *int   `url:"n,present"`
			S string `url:"s,present"`
			R int    `url:"r,present,required"`
		}{}
		src := url.Values{"s": {""}}

		// --- When ---
		err := NewDecoder().Decode(dst, src)

		// --- Then ---
		want := "" +
			"b: is missing (ECMissing); " +
			"n: is missing (ECMissing); " +
			"r: is missing (ECMissing)"
		xrrtest.AssertFieldsEqual(t, want, err)
	})

	t.Run("present fields sent with zero values", func(t *testing.T) {
		// --- Given ---
		dst := &struct {
			B bool   `url:"b,present"`
			N *int   `url:"n,present"`
			P *TItem `url:"p,present"`
		}{}
		src := url.Values{"b": {"false"}, "n": {"0"}, "p.size": {"0"}}

		// --- When ---
		err := NewDecoder().Decode(dst, src)

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, 0, *dst.N)
		assert.NotNil(t, dst.P)
	})

	t.Run("error - present field with a failing key", func(t *testing.T) {
		// --- Given ---
		dst := &struct {
			N int `url:"n,present"`
		}{}
		src := url.Values{"n": {"x"}}

		// --- When ---
		err := NewDecoder().Decode(dst, src)

		// --- Then ---
		want := "n: invalid value: cannot convert `x` to int (ECInvValue)"
		xrrtest.AssertFieldsEqual(t, want, err)
	})

	t.Run("error - present field behind a nil embed", func(t *testing.T) {
		// --- Given ---
		type base struct {
			ID int `url:"id,present"`
		}
		dst := &struct {
			*base
			N int `url:"n"`
		}{}
		src := url.Values{"n": {"1"}}

		// --- When ---
		err := NewDecoder().Decode(dst, src)

		// --- Then ---
		xrrtest.AssertFieldsEqual(t, "id: is missing (ECMissing)", err)
	})

	t.Run("error - present field behind a self embed", func(t *testing.T) {
		// --- Given ---
		type node struct {
			*node     //nolint:unused
			X     int `url:"x,present"`
		}
		dst := &struct{ node }{}
		src := url.Values{}

		// --- When ---
		err := NewDecoder(WithDecAllowUnknown()).Decode(dst, src)

		// --- Then ---
		xrrtest.AssertFieldsEqual(t, "x: is missing (ECMissing)", err)
	})

	t.Run("error - present field with a rejected key", func(t *testing.T) {
		// --- Given ---
		type node struct {
			Next *node `url:"next,present"`
			V    int   `url:"v"`
		}
		dst := &struct {
			IDs  []int `url:"ids,present"`
			Node node  `url:"node"`
		}{}
		src := url.Values{
			"ids[1001]":             {"1"},
			"node.next.next.next.v": {"2"},
		}

		// --- When ---
		err := NewDecoder(WithDecMaxDepth(3)).Decode(dst, src)

		// --- Then ---
		want := "" +
			"ids[1001]: invalid value: index 1001 over the limit 1000 " +
			"(ECInvValue); " +
			"node.next.next.next.v: invalid value: key depth over the " +
			"limit 3 (ECInvValue)"
		xrrtest.AssertFieldsEqual(t, want, err)
	})

	t.Run("error - present fields in reached elements", func(t *testing.T) {
		// --- Given ---
		type item struct {
			A int `url:"a,present"`
			B int `url:"b"`
		}
		dst := &struct {
			L []item          `url:"l"`
			M map[string]item `url:"m"`
		}{M: map[string]item{"x": {}}}
		src := url.Values{"l[0].b": {"1"}, "l[1].a": {"2"}, "m[y].b": {"3"}}

		// --- When ---
		err := NewDecoder().Decode(dst, src)

		// --- Then ---
		want := "" +
			"l[0].a: is missing (ECMissing); " +
			"m[y].a: is missing (ECMissing)"
		xrrtest.AssertFieldsEqual(t, want, err)
	})

	t.Run("registered decoder", func(t *testing.T) {
		// --- Given ---
		dec := NewDecoder(
			WithDecFunc(func(s string) (time.Time, error) {
				return time.Parse(time.DateOnly, s)
			}),
			WithDecFunc(decPoint),
			WithDecFunc(func(s string) (TLevel, error) {
				n, err := strconv.Atoi(s)
				return TLevel(n + 100), err // UnmarshalText gives 9.
			}),
		)
		dst := &struct {
			T  time.Time            `url:"t"`
			TP *time.Time           `url:"tp"`
			TS []time.Time          `url:"ts"`
			TM map[string]time.Time `url:"tm"`
			P  TPoint               `url:"p"`
			PP *TPoint              `url:"pp"`
			L  TLevel               `url:"l"`
		}{}
		src := url.Values{
			"t":    {"2026-01-02"},
			"tp":   {"2026-01-03"},
			"ts":   {"2026-01-04", "2026-01-05"},
			"tm.a": {"2026-01-06"},
			"p":    {"1,2"},
			"pp":   {"3,4"},
			"l":    {"9"},
		}

		// --- When ---
		err := dec.Decode(dst, src)

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, 2, dst.T.Day())
		assert.Equal(t, 3, dst.TP.Day())
		assert.Equal(t, 5, dst.TS[1].Day())
		assert.Equal(t, 6, dst.TM["a"].Day())
		assert.Equal(t, TPoint{X: 1, Y: 2}, dst.P)
		assert.Equal(t, TPoint{X: 3, Y: 4}, *dst.PP)
		assert.Equal(t, TLevel(109), dst.L)
	})

	t.Run("error - registered decoder", func(t *testing.T) {
		// --- Given ---
		dst := &struct {
			P TPoint `url:"p"`
		}{}
		src := url.Values{"p": {"x"}}

		// --- When ---
		err := NewDecoder(WithDecFunc(decPoint)).Decode(dst, src)

		// --- Then ---
		xrrtest.AssertFieldCnt(t, 1, err)
		assert.ErrorContain(t, "expected integer", err)
	})

	t.Run("registered decoder for a pointer type", func(t *testing.T) {
		// --- Given ---
		dec := NewDecoder(WithDecFunc(func(s string) (*TPoint, error) {
			pnt, err := decPoint(s)
			return &pnt, err
		}))
		dst := &struct {
			P  TPoint  `url:"p"`
			PP *TPoint `url:"pp"`
		}{}
		src := url.Values{"p": {"1,2"}, "pp": {"3,4"}}

		// --- When ---
		err := dec.Decode(dst, src)

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, TPoint{X: 1, Y: 2}, dst.P)
		assert.Equal(t, TPoint{X: 3, Y: 4}, *dst.PP)
	})

	t.Run("error - pointer decoder", func(t *testing.T) {
		// --- Given ---
		dec := NewDecoder(WithDecFunc(func(string) (*TPoint, error) {
			return nil, errTest
		}))
		dst := &struct {
			P TPoint `url:"p"`
		}{}
		src := url.Values{"p": {"1,2"}}

		// --- When ---
		err := dec.Decode(dst, src)

		// --- Then ---
		xrrtest.AssertFieldCnt(t, 1, err)
		xrrtest.AssertFieldIs(t, "p", errTest, err)
	})

	t.Run("error - nil from pointer decoder", func(t *testing.T) {
		// --- Given ---
		dec := NewDecoder(WithDecFunc(func(string) (*TPoint, error) {
			return nil, nil
		}))
		dst := &struct {
			P TPoint `url:"p"`
		}{}
		src := url.Values{"p": {"1,2"}}

		// --- When ---
		err := dec.Decode(dst, src)

		// --- Then ---
		want := "" +
			"p: invalid value: decoder returned nil `*xbind.TPoint` " +
			"(ECInvValue)"
		xrrtest.AssertFieldsEqual(t, want, err)
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
		src := url.Values{"[p]": {"1,2"}} // A key decodeFast does not take.

		// --- When ---
		err := dec.Decode(dst, src)

		// --- Then ---
		have := xrr.GetFields(xrr.GetFields(err)["[p]"])
		assert.Equal(t, map[string]error{"x": errTest}, have)
	})

	t.Run("nil from an interface decoder", func(t *testing.T) {
		// --- Given ---
		dec := NewDecoder(WithDecFunc(func(string) (fmt.Stringer, error) {
			return nil, nil
		}))
		dst := &struct {
			S fmt.Stringer `url:"s"`
		}{S: time.Second}
		src := url.Values{"s": {"x"}}

		// --- When ---
		err := dec.Decode(dst, src)

		// --- Then ---
		assert.NoError(t, err)
		assert.Nil(t, dst.S)
	})

	t.Run("error - malformed struct tag", func(t *testing.T) {
		// --- Given ---
		src := url.Values{}

		// --- When ---
		err := NewDecoder().Decode(badTag(`url:n`, false), src)

		// --- Then ---
		assert.ErrorEqual(t, "invalid struct tag: N", err)
		assert.ErrorIs(t, ErrInvTag, err)
		xrrtest.AssertCode(t, ECInvTag, err)
	})

	t.Run("tags of a type decoded as one value", func(t *testing.T) {
		// --- Given ---
		dst := &struct {
			C TBadTagText `url:"c"`
		}{}
		src := url.Values{"c": {"ff"}}

		// --- When ---
		err := NewDecoder().Decode(dst, src)

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, "ff", dst.C.Hex)
	})

	t.Run("tags inside containers of one-value types", func(t *testing.T) {
		// --- Given ---
		dst := &struct {
			L []TBadRows          `url:"l"`
			M map[string]TBadRows `url:"m"`
		}{}
		src := url.Values{"l[0]": {"1"}, "m[a]": {"2"}}

		// --- When ---
		err := NewDecoder().Decode(dst, src)

		// --- Then ---
		assert.NoError(t, err)
		assert.Len(t, 1, dst.L[0])
		assert.Len(t, 2, dst.M["a"])
	})

	t.Run("error - malformed tag in nil nested struct", func(t *testing.T) {
		// --- Given ---
		src := url.Values{}

		// --- When ---
		err := NewDecoder().Decode(badTag(`url:n`, true), src)

		// --- Then ---
		assert.ErrorEqual(t, "invalid struct tag: F.N", err)
	})

	t.Run("map of structs", func(t *testing.T) {
		// --- Given ---
		dst := &TBox{}
		src := url.Values{
			"items[a].size":   {"1"},
			"items[a].name":   {"x"},
			"items.b.size":    {"2"},
			"items[c.d].size": {"3"},
		}

		// --- When ---
		err := NewDecoder().Decode(dst, src)

		// --- Then ---
		assert.NoError(t, err)
		want := map[string]TItem{
			"a":   {Size: 1, Name: "x"},
			"b":   {Size: 2},
			"c.d": {Size: 3},
		}
		assert.Equal(t, want, dst.Items)
	})

	t.Run("map of struct pointers", func(t *testing.T) {
		// --- Given ---
		dst := &TBox{}
		src := url.Values{"pitems[a].size": {"1"}, "pitems[a].name": {"x"}}

		// --- When ---
		err := NewDecoder().Decode(dst, src)

		// --- Then ---
		assert.NoError(t, err)
		want := map[string]*TItem{"a": {Size: 1, Name: "x"}}
		assert.Equal(t, want, dst.PItems)
	})

	t.Run("map merges into existing entries", func(t *testing.T) {
		// --- Given ---
		dst := &TBox{
			Items: map[string]TItem{"a": {Size: 1, Name: "x"}, "b": {Size: 2}},
			Attr:  map[string]string{"k": "v"},
		}
		src := url.Values{"items[a].size": {"9"}, "attr[a.b]": {"y"}}

		// --- When ---
		err := NewDecoder().Decode(dst, src)

		// --- Then ---
		assert.NoError(t, err)
		want := map[string]TItem{"a": {Size: 9, Name: "x"}, "b": {Size: 2}}
		assert.Equal(t, want, dst.Items)
		assert.Equal(t, map[string]string{"k": "v", "a.b": "y"}, dst.Attr)
	})

	t.Run("map value pointer is replaced", func(t *testing.T) {
		// --- Given ---
		def := "d"
		dst := &struct {
			M map[string]*string `url:"m"`
		}{M: map[string]*string{"a": &def, "b": &def}}
		src := url.Values{"m[a]": {"x"}}

		// --- When ---
		err := NewDecoder().Decode(dst, src)

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, "x", *dst.M["a"])
		assert.Equal(t, "d", *dst.M["b"])
		assert.Equal(t, "d", def)
	})

	t.Run("map key spellings name one entry", func(t *testing.T) {
		// --- Given ---
		dst := &struct {
			M map[int][]TItem `url:"m"`
		}{}
		src := url.Values{"m[1][0].size": {"1"}, "m[01][1].size": {"2"}}

		// --- When ---
		err := NewDecoder().Decode(dst, src)

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, map[int][]TItem{1: {{Size: 1}, {Size: 2}}}, dst.M)
	})

	t.Run("pointer map keys name one entry", func(t *testing.T) {
		// --- Given ---
		dst := &struct {
			M map[*string]TItem `url:"m"`
		}{}
		src := url.Values{"m[a].size": {"1"}, "m[a].name": {"x"}}

		// --- When ---
		err := NewDecoder().Decode(dst, src)

		// --- Then ---
		assert.NoError(t, err)
		assert.Len(t, 1, dst.M)
		for key, val := range dst.M {
			assert.Equal(t, "a", *key)
			assert.Equal(t, TItem{Size: 1, Name: "x"}, val)
		}
	})

	t.Run("prefilled pointer map key names its entry", func(t *testing.T) {
		// --- Given ---
		key := "a"
		dst := &struct {
			M map[*string]TItem `url:"m"`
		}{M: map[*string]TItem{&key: {Size: 1}}}
		src := url.Values{"m[a].name": {"x"}}

		// --- When ---
		err := NewDecoder().Decode(dst, src)

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, map[*string]TItem{&key: {Size: 1, Name: "x"}}, dst.M)
	})

	t.Run("prefilled key holding a pointer", func(t *testing.T) {
		// --- Given ---
		ts := time.Date(2024, 1, 1, 1, 0, 0, 0, time.FixedZone("CET", 3600))
		dst := &struct {
			M map[time.Time]int `url:"m"`
		}{M: map[time.Time]int{ts: 1}}
		src := url.Values{"m[2024-01-01T01:00:00+01:00]": {"5"}}

		// --- When ---
		err := NewDecoder().Decode(dst, src)

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, map[time.Time]int{ts: 5}, dst.M)
	})

	t.Run("prefilled value key of another type", func(t *testing.T) {
		// --- Given ---
		dec := NewDecoder(WithDecFunc(func(s string) (any, error) {
			return s, nil
		}))
		dst := &struct {
			M map[any]int `url:"m"`
		}{M: map[any]int{1: 10}}
		src := url.Values{"m[1]": {"5"}}

		// --- When ---
		err := dec.Decode(dst, src)

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, map[any]int{1: 10, "1": 5}, dst.M)
	})

	t.Run("prefilled pointer keys with one text", func(t *testing.T) {
		// --- Given ---
		k1, k2 := "a", "a"
		dst := &struct {
			M map[*string]int `url:"m"`
		}{M: map[*string]int{&k1: 1, &k2: 2}}
		src := url.Values{"m[a]": {"5"}}

		// --- When ---
		err := NewDecoder().Decode(dst, src)

		// --- Then ---
		assert.NoError(t, err)
		assert.Len(t, 3, dst.M)
		assert.Equal(t, 1, dst.M[&k1])
		assert.Equal(t, 2, dst.M[&k2])
	})

	t.Run("error - NaN map key", func(t *testing.T) {
		// --- Given ---
		dst := &struct {
			M map[float64]int `url:"m"`
		}{}
		src := url.Values{"m[NaN]": {"1"}}

		// --- When ---
		err := NewDecoder().Decode(dst, src)

		// --- Then ---
		want := "m[NaN]: invalid value: NaN map key (ECInvValue)"
		xrrtest.AssertFieldsEqual(t, want, err)
		assert.Len(t, 0, dst.M)
	})

	t.Run("error - failed key leaves a nil map nil", func(t *testing.T) {
		// --- Given ---
		dst := &struct {
			K map[int]int `url:"k"`
			V map[int]int `url:"v"`
		}{}
		src := url.Values{"k[x]": {"1"}, "v[1]": {"x"}}

		// --- When ---
		err := NewDecoder().Decode(dst, src)

		// --- Then ---
		want := "" +
			"k[x]: invalid value: cannot convert `x` to int (ECInvValue); " +
			"v[1]: invalid value: cannot convert `x` to int (ECInvValue)"
		xrrtest.AssertFieldsEqual(t, want, err)
		assert.Nil(t, dst.K)
		assert.Nil(t, dst.V)
	})

	t.Run("error - map key holding NaN", func(t *testing.T) {
		// --- Given ---
		dst := &struct {
			M map[[1]float64]TItem `url:"m"`
		}{}
		src := url.Values{"m[NaN].size": {"1"}}

		// --- When ---
		err := NewDecoder().Decode(dst, src)

		// --- Then ---
		want := "m[NaN].size: invalid value: NaN map key (ECInvValue)"
		xrrtest.AssertFieldsEqual(t, want, err)
		assert.Len(t, 0, dst.M)
	})

	t.Run("error - conflict among many keys", func(t *testing.T) {
		// --- Given ---
		dst := &TBox{}
		src := url.Values{"attr[a]": {"1"}, "attr.a": {"2"}}
		for i := range 20 {
			src["list["+strconv.Itoa(i)+"].size"] = []string{strconv.Itoa(i)}
		}

		// --- When ---
		err := NewDecoder().Decode(dst, src)

		// --- Then ---
		want := "" +
			"attr[a]: invalid value: " +
			"another key sets the value (ECInvValue)"
		xrrtest.AssertFieldsEqual(t, want, err)
		assert.Equal(t, map[string]string{"a": "2"}, dst.Attr)
		assert.Len(t, 20, dst.List)
		assert.Equal(t, 19, dst.List[19].Size)
	})

	t.Run("error - negative zero map key", func(t *testing.T) {
		// --- Given ---
		dst := &struct {
			M map[float64]string `url:"m"`
		}{}
		src := url.Values{"m[-0]": {"a"}, "m[0]": {"b"}}

		// --- When ---
		err := NewDecoder().Decode(dst, src)

		// --- Then ---
		want := "m[0]: invalid value: another key sets the value (ECInvValue)"
		xrrtest.AssertFieldsEqual(t, want, err)
		assert.Equal(t, map[float64]string{0: "a"}, dst.M)
	})

	t.Run("error - negative zero array map key", func(t *testing.T) {
		// --- Given ---
		dst := &struct {
			M map[[1]float64]string `url:"m"`
		}{}
		src := url.Values{"m[-0]": {"a"}, "m[0]": {"b"}}

		// --- When ---
		err := NewDecoder().Decode(dst, src)

		// --- Then ---
		want := "m[0]: invalid value: another key sets the value (ECInvValue)"
		xrrtest.AssertFieldsEqual(t, want, err)
		assert.Equal(t, map[[1]float64]string{{0}: "a"}, dst.M)
	})

	t.Run("error - negative zero struct map key", func(t *testing.T) {
		// --- Given ---
		type key struct{ F float64 }
		dec := NewDecoder(WithDecFunc(func(s string) (key, error) {
			f, err := strconv.ParseFloat(s, 64)
			return key{f}, err
		}))
		dst := &struct {
			M map[key]string `url:"m"`
		}{}
		src := url.Values{"m[-0]": {"a"}, "m[0]": {"b"}}

		// --- When ---
		err := dec.Decode(dst, src)

		// --- Then ---
		want := "m[0]: invalid value: another key sets the value (ECInvValue)"
		xrrtest.AssertFieldsEqual(t, want, err)
		assert.Equal(t, map[key]string{{}: "a"}, dst.M)
	})

	t.Run("negative zero array key updates the entry", func(t *testing.T) {
		// --- Given ---
		neg := [1]float64{math.Copysign(0, -1)}
		dst := &struct {
			M map[[1]float64]string `url:"m"`
		}{M: map[[1]float64]string{neg: "old"}}
		src := url.Values{"m[0]": {"new"}}

		// --- When ---
		err := NewDecoder().Decode(dst, src)

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, map[[1]float64]string{{0}: "new"}, dst.M)
		assert.Len(t, 1, dst.M)
	})

	t.Run("error - map key not comparable", func(t *testing.T) {
		// --- Given ---
		dec := NewDecoder(WithDecFunc(func(s string) (any, error) {
			return []byte(s), nil
		}))
		dst := &struct {
			M map[any]int `url:"m"`
		}{}
		src := url.Values{"m[x]": {"1"}}

		// --- When ---
		err := dec.Decode(dst, src)

		// --- Then ---
		want := "m[x]: invalid value: map key not comparable (ECInvValue)"
		xrrtest.AssertFieldsEqual(t, want, err)
		assert.Nil(t, dst.M)
	})

	t.Run("nil interface map key", func(t *testing.T) {
		// --- Given ---
		dec := NewDecoder(WithDecFunc(func(s string) (any, error) {
			if s == "nil" {
				return nil, nil
			}
			return s, nil
		}))
		dst := &struct {
			M map[any]int `url:"m"`
		}{}
		src := url.Values{"m[nil]": {"1"}, "m[a]": {"2"}}

		// --- When ---
		err := dec.Decode(dst, src)

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, map[any]int{nil: 1, "a": 2}, dst.M)
	})

	t.Run("map keys printing alike", func(t *testing.T) {
		// --- Given ---
		type key struct{ A, B string }
		dec := NewDecoder(WithDecFunc(func(s string) (key, error) {
			a, b, _ := strings.Cut(s, "|")
			return key{a, b}, nil
		}))
		dst := &struct {
			M map[key]int `url:"m"`
		}{}
		src := url.Values{"m[a b|]": {"1"}, "m[a|b ]": {"2"}}

		// --- When ---
		err := dec.Decode(dst, src)

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, map[key]int{{"a b", ""}: 1, {"a", "b "}: 2}, dst.M)
	})

	t.Run("map of plain values takes the rest of a key", func(t *testing.T) {
		// --- Given ---
		dst := &TBox{}
		src := url.Values{"attr.a.b": {"x"}}

		// --- When ---
		err := NewDecoder().Decode(dst, src)

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, map[string]string{"a.b": "x"}, dst.Attr)
	})

	t.Run("slice of structs", func(t *testing.T) {
		// --- Given ---
		dst := &TBox{List: []TItem{{Size: 7}, {Size: 8}, {Size: 9}}}
		src := url.Values{
			"list[1].size": {"2"},
			"list.0.size":  {"1"},
			"list[1].name": {"b"},
		}

		// --- When ---
		err := NewDecoder().Decode(dst, src)

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, []TItem{{Size: 1}, {Size: 2, Name: "b"}}, dst.List)
	})

	t.Run("slice of struct pointers with a gap", func(t *testing.T) {
		// --- Given ---
		dst := &TBox{}
		src := url.Values{"plist[2].size": {"3"}}

		// --- When ---
		err := NewDecoder().Decode(dst, src)

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, []*TItem{nil, nil, {Size: 3}}, dst.PList)
	})

	t.Run("array of structs", func(t *testing.T) {
		// --- Given ---
		dst := &TBox{Arr: [2]TItem{{Size: 7}, {Size: 8}}}
		src := url.Values{"arr[1].size": {"2"}}

		// --- When ---
		err := NewDecoder().Decode(dst, src)

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, [2]TItem{{}, {Size: 2}}, dst.Arr)
	})

	t.Run("nested containers", func(t *testing.T) {
		// --- Given ---
		dst := &TBox{}
		src := url.Values{
			"nest[a][1].size": {"2"},
			"nest[a][0].size": {"1"},
			"grid[1]":         {"3", "4"},
			"grid[0]":         {"1"},
		}

		// --- When ---
		err := NewDecoder().Decode(dst, src)

		// --- Then ---
		assert.NoError(t, err)
		want := map[string][]TItem{"a": {{Size: 1}, {Size: 2}}}
		assert.Equal(t, want, dst.Nest)
		assert.Equal(t, [][]int{{1}, {3, 4}}, dst.Grid)
	})

	t.Run("field name with a dot", func(t *testing.T) {
		// --- Given ---
		dst := &TBox{}
		src := url.Values{"[page.size]": {"10"}}

		// --- When ---
		err := NewDecoder().Decode(dst, src)

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, 10, dst.Dotted)
	})

	t.Run("error - container keys", func(t *testing.T) {
		// --- Given ---
		dst := &TBox{}
		src := url.Values{
			"list[x].size":    {"1"},
			"list[1001].size": {"1"},
			"arr[2].size":     {"1"},
			"items[a]":        {"1"},
			"items[a].zzz":    {"1"},
			"list[0":          {"1"},
			"page.size":       {"1"},
		}

		// --- When ---
		err := NewDecoder().Decode(dst, src)

		// --- Then ---
		want := "" +
			"arr[2].size: invalid value: index 2 past the array end " +
			"(ECInvValue); " +
			"items[a]: invalid field path (ECUnkFieldPath); " +
			"items[a].zzz: invalid field path (ECUnkFieldPath); " +
			"list[0: invalid field path (ECUnkFieldPath); " +
			"list[1001].size: invalid value: index 1001 over the limit " +
			"1000 (ECInvValue); " +
			"list[x].size: invalid field path (ECUnkFieldPath); " +
			"page.size: invalid field path (ECUnkFieldPath)"
		xrrtest.AssertFieldsEqual(t, want, err)
		assert.Nil(t, dst.List)
		assert.Nil(t, dst.Items)
	})

	t.Run("error - decoder error wrapping unknown path", func(t *testing.T) {
		// --- Given ---
		dec := NewDecoder(
			WithDecAllowUnknown(),
			WithDecFunc(func(string) (TPoint, error) {
				return TPoint{}, fmt.Errorf("bad: %w", ErrUnkFieldPath)
			}),
		)
		dst := &struct {
			P TPoint `url:"p"`
		}{}
		src := url.Values{"p": {"1,2"}}

		// --- When ---
		err := dec.Decode(dst, src)

		// --- Then ---
		want := "p: bad: invalid field path (ECGeneric)"
		xrrtest.AssertFieldsEqual(t, want, err)
	})

	t.Run("error - pointer map value keeps its slice", func(t *testing.T) {
		// --- Given ---
		type item struct {
			L []int `url:"l"`
		}
		dst := &struct {
			M map[string]*item `url:"m"`
		}{M: map[string]*item{"k": {L: []int{1, 2}}}}
		src := url.Values{"m[k].l[0]": {"x"}}

		// --- When ---
		err := NewDecoder().Decode(dst, src)

		// --- Then ---
		want := "" +
			"m[k].l[0]: invalid value: cannot convert `x` to int " +
			"(ECInvValue)"
		xrrtest.AssertFieldsEqual(t, want, err)
		assert.Equal(t, []int{1, 2}, dst.M["k"].L)
	})

	t.Run("error - failed indexes keep containers", func(t *testing.T) {
		// --- Given ---
		dst := &struct {
			L []int  `url:"l,required"`
			A [2]int `url:"a"`
		}{L: []int{5}, A: [2]int{7, 8}}
		src := url.Values{"l[0]": {"x"}, "a[0]": {"x"}}

		// --- When ---
		err := NewDecoder().Decode(dst, src)

		// --- Then ---
		want := "" +
			"a[0]: invalid value: cannot convert `x` to int (ECInvValue); " +
			"l[0]: invalid value: cannot convert `x` to int (ECInvValue)"
		xrrtest.AssertFieldsEqual(t, want, err)
		assert.Equal(t, []int{5}, dst.L)
		assert.Equal(t, [2]int{7, 8}, dst.A)
	})

	t.Run("error - failed keys leave nil pointers nil", func(t *testing.T) {
		// --- Given ---
		dst := &struct {
			P   *int       `url:"p"`
			Sub *TRequired `url:"sub"`
		}{}
		src := url.Values{"p": {"x"}, "sub.f_int": {"x"}}

		// --- When ---
		err := NewDecoder().Decode(dst, src)

		// --- Then ---
		want := "" +
			"p: invalid value: cannot convert `x` to int (ECInvValue); " +
			"sub.f_int: invalid value: cannot convert `x` to int " +
			"(ECInvValue)"
		xrrtest.AssertFieldsEqual(t, want, err)
		assert.Nil(t, dst.P)
		assert.Nil(t, dst.Sub)
	})

	t.Run("error - failed unmarshaler keeps the value", func(t *testing.T) {
		// --- Given ---
		dst := &struct {
			L TLevel    `url:"l"`
			F TPromZero `url:"f"`
		}{L: 5}
		src := url.Values{"l": {"Lx"}, "f": {"x"}}

		// --- When ---
		err := NewDecoder().Decode(dst, src)

		// --- Then ---
		xrrtest.AssertFieldCnt(t, 2, err)
		assert.Equal(t, TLevel(5), dst.L)
		assert.Nil(t, dst.F.Time)
	})

	t.Run("error - failed unmarshaler keeps embeds nil", func(t *testing.T) {
		// --- Given ---
		type mid struct{ *TTextPtr }
		type outer struct{ *mid }
		dst := &struct {
			F outer `url:"f"`
		}{F: outer{&mid{}}}
		src := url.Values{"f": {"GG"}}

		// --- When ---
		err := NewDecoder().Decode(dst, src)

		// --- Then ---
		xrrtest.AssertFieldCnt(t, 1, err)
		assert.Nil(t, dst.F.TTextPtr)
	})

	t.Run("error - map entry not written on error", func(t *testing.T) {
		// --- Given ---
		dst := &TBox{Items: map[string]TItem{"a": {Size: 1}}}
		src := url.Values{"items[a].size": {"x"}, "items[b].size": {"x"}}

		// --- When ---
		err := NewDecoder().Decode(dst, src)

		// --- Then ---
		want := "" +
			"items[a].size: invalid value: cannot convert `x` to int " +
			"(ECInvValue); " +
			"items[b].size: invalid value: cannot convert `x` to int " +
			"(ECInvValue)"
		xrrtest.AssertFieldsEqual(t, want, err)
		assert.Equal(t, map[string]TItem{"a": {Size: 1}}, dst.Items)
	})

	t.Run("error - failed index adds no elements", func(t *testing.T) {
		// --- Given ---
		dst := &struct {
			List []TRequired `url:"list"`
		}{}
		src := url.Values{"list[2].f_int": {"x"}}

		// --- When ---
		err := NewDecoder().Decode(dst, src)

		// --- Then ---
		want := "" +
			"list[2].f_int: invalid value: cannot convert `x` to int " +
			"(ECInvValue)"
		xrrtest.AssertFieldsEqual(t, want, err)
		assert.Len(t, 0, dst.List)
		assert.Equal(t, 0, cap(dst.List))
	})

	t.Run("error - alias value error kept over required", func(t *testing.T) {
		// --- Given ---
		dst := &struct {
			Age int `url:"age,required"`
		}{}
		src := url.Values{"[age]": {"x"}}

		// --- When ---
		err := NewDecoder().Decode(dst, src)

		// --- Then ---
		want := "[age]: invalid value: cannot convert `x` to int (ECInvValue)"
		xrrtest.AssertFieldsEqual(t, want, err)
	})

	t.Run("error - alias no decoder kept over required", func(t *testing.T) {
		// --- Given ---
		dst := &struct {
			F chan int `url:"f,required"`
		}{}
		src := url.Values{"[f]": {"1"}}

		// --- When ---
		err := NewDecoder().Decode(dst, src)

		// --- Then ---
		want := "[f]: decoder not found for `chan int` (ECDecNotFound)"
		xrrtest.AssertFieldsEqual(t, want, err)
	})

	t.Run("error - required under a registered map key", func(t *testing.T) {
		// --- Given ---
		dst := &struct {
			M map[TPoint]TRequired `url:"m"`
		}{}
		src := url.Values{"m[1,2].f_int": {"5"}}

		// --- When ---
		err := NewDecoder(WithDecFunc(decPoint)).Decode(dst, src)

		// --- Then ---
		want := "m[1,2].f_str: cannot be blank (ECRequired)"
		xrrtest.AssertFieldsEqual(t, want, err)
	})

	t.Run("error - required under a prefilled map key", func(t *testing.T) {
		// --- Given ---
		dst := &struct {
			M map[TPoint]TRequired `url:"m"`
		}{M: map[TPoint]TRequired{{1, 2}: {}, {3, 4}: {FStr: "x"}}}
		src := url.Values{"m[01,2].f_int": {"5"}}

		// --- When ---
		err := NewDecoder(WithDecFunc(decPoint)).Decode(dst, src)

		// --- Then ---
		want := "m[01,2].f_str: cannot be blank (ECRequired)"
		xrrtest.AssertFieldsEqual(t, want, err)
		assert.Len(t, 2, dst.M)
		assert.Equal(t, TRequired{FInt: 5}, dst.M[TPoint{1, 2}])
	})

	t.Run("error - required under registered time key", func(t *testing.T) {
		// --- Given ---
		dec := NewDecoder(WithDecFunc(func(s string) (time.Time, error) {
			return time.Parse(time.DateOnly, s)
		}))
		dst := &struct {
			M map[time.Time]TRequired `url:"m"`
		}{}
		src := url.Values{"m[2026-01-02].f_int": {"5"}}

		// --- When ---
		err := dec.Decode(dst, src)

		// --- Then ---
		want := "m[2026-01-02].f_str: cannot be blank (ECRequired)"
		xrrtest.AssertFieldsEqual(t, want, err)
	})

	t.Run("error - aliases of one field", func(t *testing.T) {
		// --- Given ---
		dst := &struct {
			Name string            `url:"name"`
			Attr map[string]string `url:"attr"`
		}{}
		src := url.Values{
			"name": {"a"}, "[name]": {"b"},
			"attr.k": {"c"}, "attr[k]": {"d"},
		}

		// --- When ---
		err := NewDecoder().Decode(dst, src)

		// --- Then ---
		want := "" +
			"attr[k]: invalid value: another key sets the value " +
			"(ECInvValue); " +
			"name: invalid value: another key sets the value (ECInvValue)"
		xrrtest.AssertFieldsEqual(t, want, err)
		assert.Equal(t, "b", dst.Name)
		assert.Equal(t, map[string]string{"k": "c"}, dst.Attr)
	})

	t.Run("error - values and indexes of one slice", func(t *testing.T) {
		// --- Given ---
		dst := &struct {
			SS []string `url:"ss"`
		}{}
		src := url.Values{"ss": {"a", "b"}, "ss[1]": {"x"}}

		// --- When ---
		err := NewDecoder().Decode(dst, src)

		// --- Then ---
		want := "" +
			"ss[1]: invalid value: another key sets the value " +
			"(ECInvValue)"
		xrrtest.AssertFieldsEqual(t, want, err)
		assert.Equal(t, []string{"a", "b"}, dst.SS)
	})

	t.Run("error - indexed keys over the element budget", func(t *testing.T) {
		// --- Given ---
		dst := &struct {
			A []struct {
				B []int `url:"b"`
			} `url:"a"`
		}{}
		src := url.Values{"a[0].b[10]": {"1"}, "a[1].b[10]": {"2"}}

		// --- When ---
		err := NewDecoder(WithDecMaxIndex(10)).Decode(dst, src)

		// --- Then ---
		want := "" +
			"a[1].b[10]: invalid value: index 10 adds too many elements " +
			"(ECInvValue)"
		xrrtest.AssertFieldsEqual(t, want, err)
		assert.Len(t, 1, dst.A)
		assert.Len(t, 11, dst.A[0].B)
	})

	t.Run("error - failed key spends its budget", func(t *testing.T) {
		// --- Given ---
		dst := &struct {
			A []int `url:"a"`
			B []int `url:"b"`
		}{}
		src := url.Values{"a[2]": {"x"}, "b[2]": {"1"}}

		// --- When ---
		err := NewDecoder(WithDecMaxIndex(2)).Decode(dst, src)

		// --- Then ---
		want := "" +
			"a[2]: invalid value: cannot convert `x` to int (ECInvValue); " +
			"b[2]: invalid value: index 2 adds too many elements (ECInvValue)"
		xrrtest.AssertFieldsEqual(t, want, err)
		assert.Nil(t, dst.A)
		assert.Nil(t, dst.B)
	})

	t.Run("error - key over the depth limit", func(t *testing.T) {
		// --- Given ---
		type node struct {
			Next *node `url:"next"`
			V    int   `url:"v"`
		}
		dst := &node{}
		src := url.Values{"next.next.v": {"1"}, "next.next.next.v": {"2"}}

		// --- When ---
		err := NewDecoder(WithDecMaxDepth(3)).Decode(dst, src)

		// --- Then ---
		want := "" +
			"next.next.next.v: invalid value: key depth over the limit 3 " +
			"(ECInvValue)"
		xrrtest.AssertFieldsEqual(t, want, err)
		assert.Equal(t, 1, dst.Next.Next.V)
		assert.Nil(t, dst.Next.Next.Next)
	})

	t.Run("error - key over the default depth limit", func(t *testing.T) {
		// --- Given ---
		type node struct {
			Next *node `url:"next"`
			V    int   `url:"v"`
		}
		dst := &node{}
		deep := strings.Repeat("next.", DefaultMaxDepth) + "v"
		src := url.Values{deep[len("next."):]: {"1"}, deep: {"2"}}

		// --- When ---
		err := NewDecoder().Decode(dst, src)

		// --- Then ---
		want := "invalid value: key depth over the limit 32"
		xrrtest.AssertFieldEqual(t, deep, want, err)
		xrrtest.AssertFieldCnt(t, 1, err)
	})

	t.Run("rest of a key for a map of values is one step", func(t *testing.T) {
		// --- Given ---
		dst := &TBox{}
		src := url.Values{"attr.a.b.c.d": {"x"}}

		// --- When ---
		err := NewDecoder(WithDecMaxDepth(2)).Decode(dst, src)

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, map[string]string{"a.b.c.d": "x"}, dst.Attr)
	})

	t.Run("error - failed array resets spend the budget", func(t *testing.T) {
		// --- Given ---
		dst := &struct {
			A [1000]int `url:"a"`
		}{}
		src := url.Values{"a[00]": {"x"}, "a[0]": {"x"}}

		// --- When ---
		err := NewDecoder(WithDecMaxIndex(10)).Decode(dst, src)

		// --- Then ---
		want := "" +
			"a[00]: invalid value: cannot convert `x` to int (ECInvValue); " +
			"a[0]: invalid value: index 0 resets too many elements (ECInvValue)"
		xrrtest.AssertFieldsEqual(t, want, err)
	})

	t.Run("array reset by a successful key is free", func(t *testing.T) {
		// --- Given ---
		dst := &struct {
			A [2000]int `url:"a"`
			L []int     `url:"l"`
		}{}
		src := url.Values{"a[5]": {"1"}, "l[10]": {"2"}}

		// --- When ---
		err := NewDecoder(WithDecMaxIndex(10)).Decode(dst, src)

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, 1, dst.A[5])
		assert.Equal(t, 2, dst.L[10])
	})

	t.Run("error - index limit option", func(t *testing.T) {
		// --- Given ---
		dst := &TBox{}
		src := url.Values{"list[3].size": {"1"}}

		// --- When ---
		err := NewDecoder(WithDecMaxIndex(2)).Decode(dst, src)

		// --- Then ---
		want := "" +
			"list[3].size: invalid value: index 3 over the limit 2 " +
			"(ECInvValue)"
		xrrtest.AssertFieldsEqual(t, want, err)
	})

	t.Run("error - index limit zero", func(t *testing.T) {
		// --- Given ---
		dst := &TBox{}
		src := url.Values{"list[0].size": {"1"}, "list[1].size": {"2"}}

		// --- When ---
		err := NewDecoder(WithDecMaxIndex(0)).Decode(dst, src)

		// --- Then ---
		want := "" +
			"list[1].size: invalid value: index 1 over the limit 0 " +
			"(ECInvValue)"
		xrrtest.AssertFieldsEqual(t, want, err)
		assert.Equal(t, []TItem{{Size: 1}}, dst.List)
	})

	t.Run("error - negative index limit", func(t *testing.T) {
		// --- Given ---
		dst := &TBox{}
		src := url.Values{"list[0].size": {"1"}}

		// --- When ---
		err := NewDecoder(WithDecMaxIndex(-1)).Decode(dst, src)

		// --- Then ---
		want := "" +
			"list[0].size: invalid value: index 0 over the limit -1 " +
			"(ECInvValue)"
		xrrtest.AssertFieldsEqual(t, want, err)
		assert.Nil(t, dst.List)
	})

	t.Run("largest index limit", func(t *testing.T) {
		// --- Given ---
		dst := &TBox{}
		src := url.Values{"list[1500].size": {"1"}}

		// --- When ---
		err := NewDecoder(WithDecMaxIndex(math.MaxInt)).Decode(dst, src)

		// --- Then ---
		assert.NoError(t, err)
		assert.Len(t, 1501, dst.List)
		assert.Equal(t, TItem{Size: 1}, dst.List[1500])
	})

	t.Run("error - failed value keeps the slice", func(t *testing.T) {
		// --- Given ---
		dst := &struct {
			L []int `url:"l"`
		}{L: []int{5}}
		src := url.Values{"l": {"1", "x"}}

		// --- When ---
		err := NewDecoder().Decode(dst, src)

		// --- Then ---
		want := "l: invalid value: cannot convert `x` to int (ECInvValue)"
		xrrtest.AssertFieldsEqual(t, want, err)
		assert.Equal(t, []int{5}, dst.L)
	})

	t.Run("error - required inside elements", func(t *testing.T) {
		// --- Given ---
		dst := &struct {
			M map[string]TRequired  `url:"m"`
			P map[string]*TRequired `url:"p"`
			L []TRequired           `url:"l"`
			D int                   `url:"a.b,required"`
		}{}
		src := url.Values{
			"m[a].f_int": {"1"},
			"m.b.f_str":  {"x"},
			"p[c].f_int": {"1"},
			"l[1].f_int": {"1"},
		}

		// --- When ---
		err := NewDecoder().Decode(dst, src)

		// --- Then ---
		want := "" +
			"[a.b]: cannot be blank (ECRequired); " +
			"l[0].f_str: cannot be blank (ECRequired); " +
			"l[1].f_str: cannot be blank (ECRequired); " +
			"m[a].f_str: cannot be blank (ECRequired); " +
			"p[c].f_str: cannot be blank (ECRequired)"
		xrrtest.AssertFieldsEqual(t, want, err)
	})

	t.Run("error - required without tag name", func(t *testing.T) {
		// --- Given ---
		dst := &struct {
			F string `url:",required"`
		}{}
		src := url.Values{}

		// --- When ---
		err := NewDecoder().Decode(dst, src)

		// --- Then ---
		xrrtest.AssertFieldsEqual(t, "F: cannot be blank (ECRequired)", err)
	})

	t.Run("unexported field does not hide a tagged field", func(t *testing.T) {
		// --- Given ---
		dst := &struct {
			name string
			Name string `url:"name,required"`
		}{}
		src := url.Values{"name": {"x"}}

		// --- When ---
		err := NewDecoder().Decode(dst, src)

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, "", dst.name)
		assert.Equal(t, "x", dst.Name)
	})

	t.Run("embedded struct implementing unmarshaler", func(t *testing.T) {
		// --- Given ---
		dst := &struct {
			TItf `url:",required"`
		}{}
		src := url.Values{"Str": {"x"}}

		// --- When ---
		err := NewDecoder().Decode(dst, src)

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, "x", dst.Str)
	})

	t.Run("invalid keys do not allocate fields", func(t *testing.T) {
		// --- Given ---
		dst := &TString{}
		src := url.Values{
			"f_p_str.x":   {"a"},
			"f_s_str.x":   {"a"},
			"f_m_int_str": {"a"},
		}

		// --- When ---
		err := NewDecoder(WithDecAllowUnknown()).Decode(dst, src)

		// --- Then ---
		assert.NoError(t, err)
		assert.Nil(t, dst.FpStr)
		assert.Nil(t, dst.FsStr)
		assert.Nil(t, dst.FmStr)
	})

	t.Run("recursive struct", func(t *testing.T) {
		// --- Given ---
		dst := &TNode{}
		src := url.Values{
			"val":           {"1"},
			"req":           {"r"},
			"next.val":      {"2"},
			"next.req":      {"r2"},
			"next.next.val": {"3"},
			"next.next.req": {"r3"},
		}

		// --- When ---
		err := NewDecoder().Decode(dst, src)

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, 1, dst.Val)
		assert.Equal(t, "r", dst.Req)
		assert.Equal(t, 2, dst.Next.Val)
		assert.Equal(t, 3, dst.Next.Next.Val)
		assert.Nil(t, dst.Next.Next.Next)
	})

	t.Run("error - required in recursive struct", func(t *testing.T) {
		// --- Given ---
		dst := &TNode{}
		src := url.Values{"req": {"r"}, "next.val": {"2"}}

		// --- When ---
		err := NewDecoder().Decode(dst, src)

		// --- Then ---
		want := "next.req: cannot be blank (ECRequired)"
		xrrtest.AssertFieldsEqual(t, want, err)
		assert.Equal(t, 2, dst.Next.Val)
	})

	t.Run("error - recursive struct is not allocated", func(t *testing.T) {
		// --- Given ---
		dst := &TNode{}
		src := url.Values{"val": {"1"}}

		// --- When ---
		err := NewDecoder().Decode(dst, src)

		// --- Then ---
		xrrtest.AssertFieldsEqual(t, "req: cannot be blank (ECRequired)", err)
		assert.Equal(t, 1, dst.Val)
		assert.Nil(t, dst.Next)
	})

	t.Run("error - embedding cycle", func(t *testing.T) {
		// --- Given ---
		dst := &TCycle{}
		src := url.Values{"TCycle": {"x"}}

		// --- When ---
		err := NewDecoder().Decode(dst, src)

		// --- Then ---
		want := "TCycle: invalid field path (ECUnkFieldPath)"
		xrrtest.AssertFieldsEqual(t, want, err)
		assert.Nil(t, dst.TCycle)
	})

	t.Run("error - required in a map holding itself", func(t *testing.T) {
		// --- Given ---
		type node struct {
			M map[string]node `url:"m"`
			R string          `url:"r,required"`
		}

		m := map[string]node{}
		m["a"] = node{M: m}

		dst := &node{M: m, R: "x"}
		src := url.Values{}

		// --- When ---
		err := NewDecoder().Decode(dst, src)

		// --- Then ---
		want := "m[a].r: cannot be blank (ECRequired)"
		xrrtest.AssertFieldsEqual(t, want, err)
	})

	t.Run("error - required in a slice holding itself", func(t *testing.T) {
		// --- Given ---
		type node struct {
			L []node `url:"l"`
			R string `url:"r,required"`
		}

		s := make([]node, 1)
		s[0].L = s

		dst := &node{L: s, R: "x"}
		src := url.Values{}

		// --- When ---
		err := NewDecoder().Decode(dst, src)

		// --- Then ---
		want := "l[0].r: cannot be blank (ECRequired)"
		xrrtest.AssertFieldsEqual(t, want, err)
	})

	t.Run("error - required behind a set embedded pointer", func(t *testing.T) {
		// --- Given ---
		dst := &struct {
			*TRequired
			N int `url:"n"`
		}{}
		src := url.Values{"f_int": {"1"}}

		// --- When ---
		err := NewDecoder().Decode(dst, src)

		// --- Then ---
		want := "f_str: cannot be blank (ECRequired)"
		xrrtest.AssertFieldsEqual(t, want, err)
		assert.Equal(t, 1, dst.FInt)
	})

	t.Run("required behind an embedded pointer to itself", func(t *testing.T) {
		// --- Given ---
		type node struct {
			*node
			R string `url:"r,required"`
		}
		dst := &node{R: "x"}
		dst.node = dst
		src := url.Values{}

		// --- When ---
		err := NewDecoder().Decode(dst, src)

		// --- Then ---
		assert.NoError(t, err)
	})

	t.Run("unexported embedding cycle", func(t *testing.T) {
		// --- Given ---
		dst := &TSelfEmb{}
		src := url.Values{"x": {"1"}}

		// --- When ---
		err := NewDecoder().Decode(dst, src)

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, 1, dst.X)
		assert.Nil(t, dst.tSelfEmb.tSelfEmb)
	})

	t.Run("error - unexported pointer in recursive struct", func(t *testing.T) {
		// --- Given ---
		dst := &TUnexpNode{tStr: &tStr{}}
		src := url.Values{
			"f_str":      {"a"},
			"next.f_str": {"b"},
			"next.zzz":   {"c"},
		}

		// --- When ---
		err := NewDecoder().Decode(dst, src)

		// --- Then ---
		want := "" +
			"next.f_str: unexported field (ECUnexported); " +
			"next.zzz: invalid field path (ECUnkFieldPath)"
		xrrtest.AssertFieldsEqual(t, want, err)
		assert.Equal(t, "a", dst.FStr)
		assert.Nil(t, dst.Next)
	})

	t.Run("field with pointer to own type", func(t *testing.T) {
		// --- Given ---
		dst := &TBuilder{}
		src := url.Values{"s": {"a"}}

		// --- When ---
		err := NewDecoder().Decode(dst, src)

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, "a", dst.S)
	})
}

func Test_Decoder_Decode_tabular(t *testing.T) {
	tt := []struct {
		testN string

		dst  any
		src  url.Values
		want any
	}{
		{"string", &struct{ F string }{}, url.Values{"F": {"a"}}, "a"},
		{
			"pointer to string",
			&struct{ F *string }{},
			url.Values{"F": {"a"}},
			new("a"),
		},
		{
			"slice of strings",
			&struct{ F []string }{},
			url.Values{"F": {"a", "b"}},
			[]string{"a", "b"},
		},
		{
			"slice of pointers to strings",
			&struct{ F []*string }{},
			url.Values{"F": {"a", "b"}},
			[]*string{new("a"), new("b")},
		},
		{
			"array of strings",
			&struct{ F [2]string }{},
			url.Values{"F": {"a", "b"}},
			[2]string{"a", "b"},
		},
		{
			"array of pointers to strings",
			&struct{ F [2]*string }{},
			url.Values{"F": {"a", "b"}},
			[2]*string{new("a"), new("b")},
		},
		{
			"map of integers to strings",
			&struct{ F map[int]string }{},
			url.Values{"F.0": {"a"}, "F.1": {"b"}},
			map[int]string{0: "a", 1: "b"},
		},
		{
			"int alias",
			&struct{ F IntAlias }{},
			url.Values{"F": {"1"}},
			IntAlias(1),
		},
		{
			"pointer to int alias",
			&struct{ F *IntAlias }{},
			url.Values{"F": {"1"}},
			new(IntAlias(1)),
		},
		{
			"slice of int aliases",
			&struct{ F []IntAlias }{},
			url.Values{"F": {"1", "2"}},
			[]IntAlias{IntAlias(1), IntAlias(2)},
		},
		{
			"array of int aliases",
			&struct{ F [2]IntAlias }{},
			url.Values{"F": {"1", "2"}},
			[2]IntAlias{IntAlias(1), IntAlias(2)},
		},
	}

	for _, tc := range tt {
		t.Run(tc.testN, func(t *testing.T) {
			// --- When ---
			err := NewDecoder().Decode(tc.dst, tc.src)

			// --- Then ---
			assert.NoError(t, err)
			field := reflect.Indirect(reflect.ValueOf(tc.dst)).Field(0)
			assert.Equal(t, tc.want, field.Interface())
		})
	}
}

func Fuzz_Decoder_Decode(f *testing.F) {
	f.Add("s", "abc", "i", "-1", false)
	f.Add("f_str", "x", "m.a.b", "1", false)
	f.Add("mk.L1", "L2", "t", "2022-03-15T11:43:01+02:00", false)
	f.Add("sub.f_m_int_str.1", "x", "a", "1", true)
	f.Add("ss", "", "sub.f_a_p_str", "y", false)
	f.Add("unknown.key", "v", "p", "7", true)
	f.Add("d", "1h2m", "pl", "L3", false)
	f.Add("f", "NaN", "u", "65535", false)
	f.Add("Ign", "x", "b", "on", false)
	f.Add("pt", "1,2", "e", "x", false)
	f.Add("ls[1].size", "1", "ms[a.b].name", "x", false)
	f.Add("next.s", "x", "next.next.p", "1", false)

	f.Fuzz(func(t *testing.T, k0, v0, k1, v1 string, unknown bool) {
		// --- Given ---
		dec := NewDecoder(WithDecFunc(decPoint))
		if unknown {
			dec = NewDecoder(WithDecFunc(decPoint), WithDecAllowUnknown())
		}

		dst := &TFuzzDec{}

		src := url.Values{k0: {v0}}
		src.Add(k1, v1)
		if _, ok := src["f_str"]; !ok {
			src.Set("f_str", "required")
		}

		// --- When ---
		err := dec.Decode(dst, src)

		// --- Then ---
		if err != nil {
			assert.NotEmpty(t, xrr.FieldNames(err))
			return
		}

		// A decoded value is stable: it encodes to keys which decode to the
		// same value.
		enc := NewEncoder(WithEncFunc(encPoint))
		vls := url.Values{}
		assert.NoError(t, enc.Encode(vls, dst))

		again := &TFuzzDec{}
		assert.NoError(t, NewDecoder(WithDecFunc(decPoint)).Decode(again, vls))

		have := url.Values{}
		assert.NoError(t, enc.Encode(have, again))
		assert.Equal(t, vls, have)
	})
}

func Test_Decoder_tagName(t *testing.T) {
	t.Run("empty tag", func(t *testing.T) {
		// --- Given ---
		dec := &Decoder{}

		// --- When ---
		have := dec.tagName()

		// --- Then ---
		assert.Equal(t, "url", have)
	})

	t.Run("custom tag", func(t *testing.T) {
		// --- Given ---
		dec := &Decoder{tag: "abc"}

		// --- When ---
		have := dec.tagName()

		// --- Then ---
		assert.Equal(t, "abc", have)
	})
}

func Test_Decoder_indexLimit(t *testing.T) {
	t.Run("default", func(t *testing.T) {
		// --- When ---
		have := (&Decoder{}).indexLimit()

		// --- Then ---
		assert.Equal(t, DefaultMaxIndex, have)
	})

	t.Run("set", func(t *testing.T) {
		// --- When ---
		have := NewDecoder(WithDecMaxIndex(3)).indexLimit()

		// --- Then ---
		assert.Equal(t, 3, have)
	})

	t.Run("zero", func(t *testing.T) {
		// --- When ---
		have := NewDecoder(WithDecMaxIndex(0)).indexLimit()

		// --- Then ---
		assert.Equal(t, 0, have)
	})
}

func Test_Decoder_depthLimit(t *testing.T) {
	t.Run("default", func(t *testing.T) {
		// --- When ---
		have := (&Decoder{}).depthLimit()

		// --- Then ---
		assert.Equal(t, DefaultMaxDepth, have)
	})

	t.Run("set", func(t *testing.T) {
		// --- When ---
		have := NewDecoder(WithDecMaxDepth(3)).depthLimit()

		// --- Then ---
		assert.Equal(t, 3, have)
	})
}

func Test_fieldErrors(t *testing.T) {
	t.Run("nil", func(t *testing.T) {
		// --- When ---
		err := fieldErrors(nil, false)

		// --- Then ---
		assert.NoError(t, err)
	})

	t.Run("not nested", func(t *testing.T) {
		// --- Given ---
		ers := xrr.NewFieldErrors(map[string]error{"a": errTest})

		// --- When ---
		err := fieldErrors(ers, false)

		// --- Then ---
		assert.Same(t, ers, err)
	})

	t.Run("nested", func(t *testing.T) {
		// --- Given ---
		empty := xrr.NewFieldErrors(map[string]error{})
		ers := xrr.NewFieldErrors(map[string]error{"a": errTest, "b": empty})

		// --- When ---
		err := fieldErrors(ers, true)

		// --- Then ---
		assert.Equal(t, map[string]error{"a": errTest}, xrr.GetFields(err))
	})
}

func Test_Decoder_state(t *testing.T) {
	t.Run("no pool", func(t *testing.T) {
		// --- Given ---
		dec := &Decoder{cache: &typeCache{}}

		// --- When ---
		have := dec.state()

		// --- Then ---
		assert.Same(t, dec.cache, have.tc)
		assert.True(t, have.track)
		assert.Equal(t, math.MaxInt, have.budget)
	})

	t.Run("from the pool", func(t *testing.T) {
		// --- Given ---
		dec := NewDecoder()
		stt := newState()
		dec.pool.Put(stt)

		// --- When ---
		have := dec.state()

		// --- Then ---
		assert.Same(t, dec.cache, have.tc)
	})

	t.Run("state of another cache", func(t *testing.T) {
		// --- Given ---
		dec := &Decoder{cache: &typeCache{}, pool: &sync.Pool{}}
		stt := newState()
		stt.tc = &typeCache{}
		stt.ftyp[0], stt.ltyp = reflect.TypeFor[TBox](), reflect.TypeFor[int]()
		dec.pool.New = func() any { return stt }

		// --- When ---
		have := dec.state()

		// --- Then ---
		assert.Same(t, dec.cache, have.tc)
		assert.Nil(t, have.ftyp[0])
		assert.Nil(t, have.ltyp)
	})
}

func Test_Decoder_release(t *testing.T) {
	t.Run("clears the state", func(t *testing.T) {
		// --- Given ---
		dec := &Decoder{pool: &sync.Pool{}}
		stt := newState()
		stt.mark("a", pathSet)

		// --- When ---
		dec.release(stt)

		// --- Then ---
		assert.Len(t, 0, stt.paths) // Cleared for the pool, which may drop it.
	})

	t.Run("no pool", func(t *testing.T) {
		// --- Given ---
		stt := newState()
		stt.mark("a", pathSet)

		// --- When ---
		(&Decoder{}).release(stt)

		// --- Then ---
		assert.Len(t, 1, stt.paths)
	})

	t.Run("large state", func(t *testing.T) {
		// --- Given ---
		dec := &Decoder{pool: &sync.Pool{}}
		stt := newState()
		for i := range poolLimit + 1 {
			stt.mark(strconv.Itoa(i), pathSet)
		}

		// --- When ---
		dec.release(stt)

		// --- Then ---
		assert.Len(t, poolLimit+1, stt.paths)
		assert.Nil(t, dec.pool.Get())
	})
}

func Test_Decoder_hasChecks_tabular(t *testing.T) {
	type tPlain struct {
		A int
		B []string
	}
	type tReq struct {
		A int `url:"a,required"`
	}
	type tPres struct {
		A int `url:"a,present"`
	}
	type tIgnored struct {
		A int `url:"-"`
	}

	tt := []struct {
		testN string

		typ  reflect.Type
		want bool
	}{
		{"plain fields", reflect.TypeFor[tPlain](), false},
		{"required", reflect.TypeFor[tReq](), true},
		{"present", reflect.TypeFor[tPres](), true},
		{"ignored field", reflect.TypeFor[tIgnored](), false},
		{"nested pointer", reflect.TypeFor[struct{ P *tReq }](), true},
		{"in a map", reflect.TypeFor[struct{ M map[string]tReq }](), true},
		{"in a slice", reflect.TypeFor[struct{ S []*tPres }](), true},
		{"embedded", reflect.TypeFor[struct{ tReq }](), true},
		{"recursive", reflect.TypeFor[TNode](), true},
		{"recursive without checks", reflect.TypeFor[TCycle](), false},
		{"not a struct", reflect.TypeFor[int](), false},
	}

	for _, tc := range tt {
		t.Run(tc.testN, func(t *testing.T) {
			// --- When ---
			have := NewDecoder().hasChecks(tc.typ)

			// --- Then ---
			assert.Equal(t, tc.want, have)
		})
	}
}

func Test_Decoder_checksIn(t *testing.T) {
	// --- Given ---
	type tReq struct {
		A int `url:"a,required"`
	}
	seen := map[reflect.Type]bool{reflect.TypeFor[tReq](): true}

	// --- When ---
	have := NewDecoder().checksIn(reflect.TypeFor[[]tReq](), seen)

	// --- Then ---
	assert.False(t, have)
}

func Test_parseKey_tabular(t *testing.T) {
	tt := []struct {
		testN string

		key  string
		want []segment
	}{
		{"name", "a", []segment{{text: "a", end: 1}}},
		{
			"dotted",
			"a.b",
			[]segment{{text: "a", end: 1}, {text: "b", end: 3}},
		},
		{
			"brackets",
			"a[b.c][0].d",
			[]segment{
				{text: "a", end: 1},
				{text: "b.c", bracket: true, end: 6},
				{text: "0", bracket: true, end: 9},
				{text: "d", end: 11},
			},
		},
		{
			"leading bracket",
			"[a.b]",
			[]segment{{text: "a.b", bracket: true, end: 5}},
		},
		{"empty", "", nil},
		{"leading dot", ".a", nil},
		{"trailing dot", "a.", nil},
		{"double dot", "a..b", nil},
		{"dot before bracket", "a.[b]", nil},
		{"empty brackets", "a[]", nil},
		{"unclosed bracket", "a[b", nil},
		{"bracket in brackets", "a[b[c]]", nil},
		{"stray closing bracket", "a]b", nil},
		{"text after bracket", "a[b]c", nil},
	}

	for _, tc := range tt {
		t.Run(tc.testN, func(t *testing.T) {
			// --- When ---
			have, ok := parseKey(tc.key, nil)

			// --- Then ---
			assert.Equal(t, tc.want, have)
			assert.Equal(t, tc.want != nil, ok)
		})
	}
}

func Test_isKeyMark_tabular(t *testing.T) {
	tt := []struct {
		testN string

		b    byte
		want bool
	}{
		{"dot", '.', true},
		{"opening bracket", '[', true},
		{"closing bracket", ']', true},
		{"letter", 'a', false},
		{"digit", '0', false},
	}

	for _, tc := range tt {
		t.Run(tc.testN, func(t *testing.T) {
			// --- When ---
			have := isKeyMark(tc.b)

			// --- Then ---
			assert.Equal(t, tc.want, have)
		})
	}
}

func Test_step_path_tabular(t *testing.T) {
	tt := []struct {
		testN string

		stp  step
		pth  string
		txt  string
		want string
	}{
		{"field", step{kind: stepField, name: "b"}, "a", "", "a.b"},
		{"first field", step{kind: stepField, name: "b"}, "", "", "b"},
		{
			"field with dot",
			step{kind: stepField, name: "b.c"},
			"a",
			"",
			"a[b.c]",
		},
		{"map entry", step{kind: stepKey, key: "01"}, "m", "1", "m[1]"},
		{"index", step{kind: stepIndex, pos: 3}, "l", "", "l[3]"},
	}

	for _, tc := range tt {
		t.Run(tc.testN, func(t *testing.T) {
			// --- When ---
			have := tc.stp.path(tc.pth, tc.txt)

			// --- Then ---
			assert.Equal(t, tc.want, have)
		})
	}
}

func Test_Decoder_plan(t *testing.T) {
	t.Run("steps", func(t *testing.T) {
		// --- Given ---
		typ := reflect.TypeFor[TBox]()
		segs, _ := parseKey("nest[a][1].size", nil)

		// --- When ---
		have, err := NewDecoder().plan(typ, segs, nil, newState())

		// --- Then ---
		assert.NoError(t, err)
		want := []step{
			{kind: stepField, idx: []int{5}, name: "nest", unm: unmNo, end: 4},
			{kind: stepKey, key: "a", end: 7},
			{kind: stepIndex, pos: 1, end: 10},
			{kind: stepField, idx: []int{0}, name: "size", unm: unmNo, end: 15},
		}
		assert.Equal(t, want, noSetters(have))
		assert.Nil(t, have[0].set)
		assert.NotNil(t, have[3].set)
		assert.Equal(t, reflect.TypeFor[int](), have[3].ftyp)
	})

	t.Run("embedded field", func(t *testing.T) {
		// --- Given ---
		typ := reflect.TypeFor[TFuzzDec]()
		segs, _ := parseKey("f_str", nil)

		// --- When ---
		have, err := NewDecoder().plan(typ, segs, nil, newState())

		// --- Then ---
		assert.NoError(t, err)
		want := []step{
			{
				kind: stepField,
				idx:  []int{0, 0},
				name: "f_str",
				unm:  unmNo,
				end:  5,
			},
		}
		assert.Equal(t, want, noSetters(have))
		assert.NotNil(t, have[0].set)
	})

	t.Run("rest of the key for a map of plain values", func(t *testing.T) {
		// --- Given ---
		typ := reflect.TypeFor[TBox]()
		segs, _ := parseKey("attr.a.b", nil)

		// --- When ---
		have, err := NewDecoder().plan(typ, segs, nil, newState())

		// --- Then ---
		assert.NoError(t, err)
		want := []step{
			{kind: stepField, idx: []int{7}, name: "attr", unm: unmNo, end: 4},
			{kind: stepKey, key: "a.b"},
		}
		assert.Equal(t, want, have)
	})

	t.Run("error - bracket in the rest of a map key", func(t *testing.T) {
		// --- Given ---
		typ := reflect.TypeFor[TBox]()
		segs, _ := parseKey("attr.a[b]", nil)

		// --- When ---
		have, err := NewDecoder().plan(typ, segs, nil, newState())

		// --- Then ---
		assert.ErrorIs(t, ErrUnkFieldPath, err)
		assert.Nil(t, have)
	})

	t.Run("error - ignored field", func(t *testing.T) {
		// --- Given ---
		typ := reflect.TypeFor[TFuzzDec]()
		segs, _ := parseKey("Ign", nil)

		// --- When ---
		have, err := NewDecoder().plan(typ, segs, nil, newState())

		// --- Then ---
		assert.ErrorIs(t, errIgnored, err)
		assert.Nil(t, have)
	})

	t.Run("error - segment after a value", func(t *testing.T) {
		// --- Given ---
		typ := reflect.TypeFor[TBox]()
		segs, _ := parseKey("attr[k].x", nil)

		// --- When ---
		have, err := NewDecoder().plan(typ, segs, nil, newState())

		// --- Then ---
		assert.ErrorIs(t, ErrUnkFieldPath, err)
		assert.Nil(t, have)
	})

	t.Run("error - over the depth limit", func(t *testing.T) {
		// --- Given ---
		typ := reflect.TypeFor[TBox]()
		segs, _ := parseKey("nest[a][1].size", nil)
		dec := NewDecoder(WithDecMaxDepth(3))

		// --- When ---
		have, err := dec.plan(typ, segs, nil, newState())

		// --- Then ---
		want := "invalid value: key depth over the limit 3"
		assert.ErrorEqual(t, want, err)
		assert.ErrorIs(t, ErrInvValue, err)
		assert.Len(t, 4, have)
	})

	t.Run("error - segment into an interface", func(t *testing.T) {
		// --- Given ---
		typ := reflect.TypeFor[struct {
			F any `url:"f"`
		}]()
		segs, _ := parseKey("f.x", nil)

		// --- When ---
		have, err := NewDecoder().plan(typ, segs, nil, newState())

		// --- Then ---
		assert.ErrorIs(t, ErrUnkFieldPath, err)
		assert.Nil(t, have)
	})
}

func Test_Decoder_index(t *testing.T) {
	t.Run("error - not a number", func(t *testing.T) {
		// --- When ---
		have, err := NewDecoder().index(reflect.TypeFor[[]int](), "x")

		// --- Then ---
		assert.ErrorEqual(t, "invalid field path", err)
		assert.Equal(t, 0, have)
	})

	t.Run("error - negative", func(t *testing.T) {
		// --- When ---
		have, err := NewDecoder().index(reflect.TypeFor[[]int](), "-1")

		// --- Then ---
		assert.ErrorEqual(t, "invalid field path", err)
		assert.Equal(t, 0, have)
	})

	t.Run("error - over the limit", func(t *testing.T) {
		// --- When ---
		have, err := NewDecoder().index(reflect.TypeFor[[]int](), "1001")

		// --- Then ---
		want := "invalid value: index 1001 over the limit 1000"
		assert.ErrorEqual(t, want, err)
		assert.Equal(t, 0, have)
	})

	t.Run("error - overflow", func(t *testing.T) {
		// --- Given ---
		txt := "99999999999999999999"

		// --- When ---
		have, err := NewDecoder().index(reflect.TypeFor[[]int](), txt)

		// --- Then ---
		want := "invalid value: index " + txt + " over the limit 1000"
		assert.ErrorEqual(t, want, err)
		assert.Equal(t, 0, have)
	})

	t.Run("error - past array end", func(t *testing.T) {
		// --- When ---
		have, err := NewDecoder().index(reflect.TypeFor[[2]int](), "2")

		// --- Then ---
		assert.ErrorEqual(t, "invalid value: index 2 past the array end", err)
		assert.Equal(t, 0, have)
	})

	t.Run("error - largest int", func(t *testing.T) {
		// --- Given ---
		dec := NewDecoder(WithDecMaxIndex(math.MaxInt))
		txt := strconv.Itoa(math.MaxInt)

		// --- When ---
		have, err := dec.index(reflect.TypeFor[[]int](), txt)

		// --- Then ---
		want := "invalid value: index " + txt + " over the limit " + txt
		assert.ErrorEqual(t, want, err)
		assert.Equal(t, 0, have)
	})
}

func Test_Decoder_index_tabular(t *testing.T) {
	tt := []struct {
		testN string

		typ  reflect.Type
		txt  string
		want int
	}{
		{"zero", reflect.TypeFor[[]int](), "0", 0},
		{"limit", reflect.TypeFor[[]int](), "1000", 1000},
		{"array last", reflect.TypeFor[[2]int](), "1", 1},
	}

	for _, tc := range tt {
		t.Run(tc.testN, func(t *testing.T) {
			// --- When ---
			have, err := NewDecoder().index(tc.typ, tc.txt)

			// --- Then ---
			assert.NoError(t, err)
			assert.Equal(t, tc.want, have)
		})
	}
}

func Test_newState(t *testing.T) {
	// --- When ---
	have := newState()

	// --- Then ---
	assert.NotNil(t, have.paths)
	assert.Len(t, 0, have.paths)
	assert.Nil(t, have.log)
	assert.NotNil(t, have.entries)
	assert.Len(t, 0, have.entries)
	assert.NotNil(t, have.keyed)
	assert.Len(t, 0, have.keyed)
	assert.Equal(t, math.MaxInt, have.budget)
	assert.True(t, have.track)
	assert.Nil(t, have.tc)
	assert.Equal(t, "", have.key)
	assert.False(t, have.off)
}

func Test_state_has_tabular(t *testing.T) {
	tt := []struct {
		testN string

		pth  string
		flag pathFlag
		want bool
	}{
		{"flag", "a", pathSet, true},
		{"one of the flags", "a", pathIndexed | pathReset, true},
		{"other flag", "a", pathIndexed, false},
		{"other path", "b", pathSet, false},
	}

	for _, tc := range tt {
		t.Run(tc.testN, func(t *testing.T) {
			// --- Given ---
			stt := newState()
			stt.paths["a"] = pathSet | pathReset

			// --- When ---
			have := stt.has(tc.pth, tc.flag)

			// --- Then ---
			assert.Equal(t, tc.want, have)
		})
	}
}

func Test_state_mark(t *testing.T) {
	// --- Given ---
	stt := newState()
	stt.mark("a", pathSet)

	// --- When ---
	stt.mark("a", pathFailed)

	// --- Then ---
	assert.Equal(t, map[string]pathFlag{"a": pathSet | pathFailed}, stt.paths)
}

func Test_state_reach(t *testing.T) {
	t.Run("tracked", func(t *testing.T) {
		// --- Given ---
		stt := newState()

		// --- When ---
		stt.reach("a")

		// --- Then ---
		assert.Equal(t, map[string]pathFlag{"a": pathReached}, stt.paths)
	})

	t.Run("not tracked", func(t *testing.T) {
		// --- Given ---
		stt := newState()
		stt.track = false

		// --- When ---
		stt.reach("a")

		// --- Then ---
		assert.Len(t, 0, stt.paths)
	})
}

func Test_state_clear(t *testing.T) {
	// --- Given ---
	stt := newState()
	dst := reflect.ValueOf(&struct{ F int }{F: 1}).Elem().Field(0)
	stt.markReset("a", dst)
	stt.entries[entryID{pth: "a", key: "k"}] = mapEntry{text: "k"}
	stt.keyed["a"] = []entryID{{pth: "a", key: "k"}}
	stt.mark("b", pathSet)
	stt.budget = 3
	stt.track = false
	tc := &typeCache{}
	stt.tc = tc
	stt.src = url.Values{"a": {"1"}}
	stt.key, stt.off, stt.alias = "a", true, true
	stt.keys = append(stt.keys, "a")
	stt.ftyp[0], stt.ltyp = reflect.TypeFor[TBox](), reflect.TypeFor[int]()

	// --- When ---
	stt.clear()

	// --- Then ---
	assert.Len(t, 0, stt.paths)
	assert.Len(t, 0, stt.log)
	assert.Equal(t, resetLog{}, stt.log[:1][0])
	assert.Len(t, 0, stt.entries)
	assert.Len(t, 0, stt.keyed)
	assert.Equal(t, math.MaxInt, stt.budget)
	assert.True(t, stt.track)
	assert.Nil(t, stt.src)
	assert.Equal(t, "", stt.key)
	assert.False(t, stt.off)
	assert.False(t, stt.alias)
	assert.Len(t, 0, stt.keys)
	assert.Equal(t, "", stt.keys[:1][0])

	assert.Same(t, tc, stt.tc)
	assert.Equal(t, reflect.TypeFor[TBox](), stt.ftyp[0])
	assert.Equal(t, reflect.TypeFor[int](), stt.ltyp)
}

func Test_state_fields(t *testing.T) {
	t.Run("first call", func(t *testing.T) {
		// --- Given ---
		stt := newState()
		stt.tc = &typeCache{}
		typ := reflect.TypeFor[TBox]()

		// --- When ---
		have := stt.fields(typ, "url", NewDecoder())

		// --- Then ---
		stored, _ := stt.tc.fields.Load(typ)
		assert.Len(t, len(stored), have)
		assert.Equal(t, typ, stt.ftyp[0])
		assert.Nil(t, stt.ftyp[1])
	})

	t.Run("type of the call before", func(t *testing.T) {
		// --- Given ---
		stt := newState()
		typ := reflect.TypeFor[TBox]()
		kfs := keyFields{"x": {}}
		stt.ftyp[0], stt.fkfs[0] = typ, kfs

		// --- When ---
		have := stt.fields(typ, "url", NewDecoder())

		// --- Then ---
		assert.Equal(t, kfs, have)
	})

	t.Run("type of the call two before", func(t *testing.T) {
		// --- Given ---
		stt := newState()
		typ := reflect.TypeFor[TBox]()
		kfs := keyFields{"x": {}}
		stt.ftyp[1], stt.fkfs[1] = typ, kfs

		// --- When ---
		have := stt.fields(typ, "url", NewDecoder())

		// --- Then ---
		assert.Equal(t, kfs, have)
	})

	t.Run("other type", func(t *testing.T) {
		// --- Given ---
		stt := newState()
		amb := reflect.TypeFor[TAmbig]()
		stt.ftyp[0], stt.fkfs[0] = amb, keyFields{"x": {}}
		typ := reflect.TypeFor[TBox]()

		// --- When ---
		have := stt.fields(typ, "url", NewDecoder())

		// --- Then ---
		_, ok := have["x"]
		assert.False(t, ok)
		assert.Equal(t, typ, stt.ftyp[0])
		assert.Equal(t, amb, stt.ftyp[1])
	})
}

func Test_state_isLeaf(t *testing.T) {
	t.Run("asks the decoder", func(t *testing.T) {
		// --- Given ---
		stt := newState()

		// --- When ---
		have := stt.isLeaf(reflect.TypeFor[int](), NewDecoder())

		// --- Then ---
		assert.True(t, have)
		assert.Equal(t, reflect.TypeFor[int](), stt.ltyp)
	})

	t.Run("type of the call before", func(t *testing.T) {
		// --- Given ---
		stt := newState()
		stt.ltyp, stt.lleaf = reflect.TypeFor[TItem](), true

		// --- When ---
		have := stt.isLeaf(reflect.TypeFor[TItem](), NewDecoder())

		// --- Then ---
		assert.True(t, have)
	})
}

func Test_state_mapScratch(t *testing.T) {
	// --- Given ---
	stt := newState()
	mf := NewDecoder().mapFactsOf(reflect.TypeFor[map[string]int]())
	mk, mv := stt.mapScratch(mf)

	// --- When ---
	hk, hv := stt.mapScratch(mf)

	// --- Then ---
	assert.Equal(t, reflect.TypeFor[string](), hk.Type())
	assert.Equal(t, reflect.TypeFor[int](), hv.Type())
	assert.True(t, hk.CanSet())
	assert.Equal(t, mk.Addr().Pointer(), hk.Addr().Pointer())
	assert.Equal(t, mv.Addr().Pointer(), hv.Addr().Pointer())
}

func Test_state_level(t *testing.T) {
	t.Run("no cache", func(t *testing.T) {
		// --- Given ---
		stt := newState()
		typ := reflect.TypeFor[TBox]()

		// --- When ---
		have := stt.level(typ, "url")

		// --- Then ---
		assert.Equal(t, newLevel(typ, "url"), have)
	})

	t.Run("from the cache", func(t *testing.T) {
		// --- Given ---
		stt := newState()
		stt.tc = &typeCache{}
		typ := reflect.TypeFor[TBox]()

		// --- When ---
		have := stt.level(typ, "url")

		// --- Then ---
		assert.Equal(t, newLevel(typ, "url"), have)
		_, ok := stt.tc.levels.Load(typ)
		assert.True(t, ok)
	})
}

func Test_state_next_tabular(t *testing.T) {
	tt := []struct {
		testN string

		key  string
		off  bool
		stp  step
		pth  string
		txt  string
		want string
		wOff bool
	}{
		{
			"canonical field",
			"page.size",
			false,
			step{kind: stepField, name: "size", end: 9},
			"page",
			"",
			"page.size",
			false,
		},
		{
			"field not canonical",
			"page[size]",
			false,
			step{kind: stepField, name: "size"},
			"page",
			"",
			"page.size",
			false,
		},
		{
			"field after an entry with another text",
			"m[01].size",
			true,
			step{kind: stepField, name: "size", end: 10},
			"m[1]",
			"",
			"m[1].size",
			true,
		},
		{
			"canonical entry",
			"m[a]",
			false,
			step{kind: stepKey, key: "a", end: 4},
			"m",
			"a",
			"m[a]",
			false,
		},
		{
			"entry with another text",
			"m[01]",
			false,
			step{kind: stepKey, key: "01", end: 5},
			"m",
			"1",
			"m[1]",
			true,
		},
		{
			"canonical index",
			"l[2]",
			false,
			step{kind: stepIndex, pos: 2, end: 4},
			"l",
			"",
			"l[2]",
			false,
		},
		{
			"index not canonical",
			"l.2",
			false,
			step{kind: stepIndex, pos: 2},
			"l",
			"",
			"l[2]",
			false,
		},
	}

	for _, tc := range tt {
		t.Run(tc.testN, func(t *testing.T) {
			// --- Given ---
			stt := newState()
			stt.key, stt.off = tc.key, tc.off

			// --- When ---
			have := stt.next(tc.stp, tc.pth, tc.txt)

			// --- Then ---
			assert.Equal(t, tc.want, have)
			assert.Equal(t, tc.wOff, stt.off)
		})
	}
}

func Test_state_entry(t *testing.T) {
	t.Run("equal keys name one entry", func(t *testing.T) {
		// --- Given ---
		stt := newState()
		m := reflect.ValueOf(map[*int]int(nil))
		one := reflect.ValueOf(new(1))
		ent := stt.entry("m", m, one, "01", false)

		// --- When ---
		have := stt.entry("m", m, reflect.ValueOf(new(1)), "1", false)

		// --- Then ---
		assert.Equal(t, "1", ent.text)
		assert.Equal(t, ent.text, have.text)
		assert.Same(t, one.Interface(), have.key.Interface())
	})

	t.Run("key already in the map", func(t *testing.T) {
		// --- Given ---
		stt := newState()
		one := new(1)
		m := reflect.ValueOf(map[*int]int{one: 1})

		// --- When ---
		have := stt.entry("m", m, reflect.ValueOf(new(1)), "01", false)

		// --- Then ---
		assert.Same(t, one, have.key.Interface())
		assert.Equal(t, "1", have.text)
		assert.True(t, have.named)
	})

	t.Run("key already in the map takes the src text", func(t *testing.T) {
		// --- Given ---
		stt := newState()
		m := reflect.ValueOf(map[TPoint]int{{1, 2}: 1})

		// --- When ---
		have := stt.entry("m", m, reflect.ValueOf(TPoint{1, 2}), "1,2", true)

		// --- Then ---
		assert.Equal(t, "1,2", have.text)
		assert.Equal(t, TPoint{1, 2}, have.key.Interface())
	})

	t.Run("key without an encoder takes the src text", func(t *testing.T) {
		// --- Given ---
		stt := newState()
		m := reflect.ValueOf(map[TPoint]int(nil))
		key := reflect.ValueOf(TPoint{1, 2})

		// --- When ---
		have := stt.entry("m", m, key, "1,2", false)

		// --- Then ---
		assert.Equal(t, "1,2", have.text)
		assert.Equal(t, TPoint{1, 2}, have.key.Interface())
	})

	t.Run("registered key type keeps the src text", func(t *testing.T) {
		// --- Given ---
		stt := newState()
		m := reflect.ValueOf(map[TLevel]int(nil))
		key := reflect.ValueOf(TLevel(9))

		// --- When ---
		have := stt.entry("m", m, key, "9", true)

		// --- Then ---
		assert.Equal(t, "9", have.text)
		assert.Equal(t, TLevel(9), have.key.Interface())
	})

	t.Run("maps at other paths", func(t *testing.T) {
		// --- Given ---
		stt := newState()
		m := reflect.ValueOf(map[*int]int(nil))
		one := reflect.ValueOf(new(1))
		ent := stt.entry("a", m, one, "1", false)

		// --- When ---
		have := stt.entry("b", m, reflect.ValueOf(new(1)), "1", false)

		// --- Then ---
		assert.NotSame(t, ent.key.Interface(), have.key.Interface())
	})

	t.Run("negative zero composite key", func(t *testing.T) {
		// --- Given ---
		stt := newState()
		m := reflect.ValueOf(map[[1]float64]int(nil))
		neg := [1]float64{math.Copysign(0, -1)}
		ent := stt.entry("m", m, reflect.ValueOf(neg), "-0", false)

		// --- When ---
		have := stt.entry("m", m, reflect.ValueOf([1]float64{0}), "0", false)

		// --- Then ---
		assert.Equal(t, ent.text, have.text)
		assert.Equal(t, neg, have.key.Interface())
		assert.Equal(t, "-0", have.text)
	})
}

func Test_textKey_tabular(t *testing.T) {
	tt := []struct {
		testN string

		kind reflect.Kind
		want bool
	}{
		{"bool", reflect.Bool, true},
		{"string", reflect.String, true},
		{"int", reflect.Int, true},
		{"uint8", reflect.Uint8, true},
		{"float", reflect.Float64, false},
		{"struct", reflect.Struct, false},
		{"array", reflect.Array, false},
		{"pointer", reflect.Pointer, false},
		{"interface", reflect.Interface, false},
	}

	for _, tc := range tt {
		t.Run(tc.testN, func(t *testing.T) {
			// --- When ---
			have := textKey(tc.kind)

			// --- Then ---
			assert.Equal(t, tc.want, have)
		})
	}
}

func Test_state_markReset(t *testing.T) {
	t.Run("paths", func(t *testing.T) {
		// --- Given ---
		stt := newState()

		// --- When ---
		stt.markReset("a", reflect.Value{})
		stt.markReset("b", reflect.Value{})
		stt.markReset("a", reflect.Value{})

		// --- Then ---
		want := map[string]pathFlag{"a": pathReset, "b": pathReset}
		assert.Equal(t, want, stt.paths)
		assert.Len(t, 2, stt.log)
		assert.Equal(t, "a", stt.log[0].pth)
		assert.Equal(t, "b", stt.log[1].pth)
	})

	t.Run("zeroes the value", func(t *testing.T) {
		// --- Given ---
		stt := newState()
		s := []int{1, 2}

		// --- When ---
		stt.markReset("a", reflect.ValueOf(&s).Elem())

		// --- Then ---
		assert.Nil(t, s)
		assert.Len(t, 1, stt.log)
		assert.Equal(t, []int{1, 2}, stt.log[0].old.Interface())
	})
}

func Test_state_markReached(t *testing.T) {
	// --- Given ---
	stt := newState()
	steps := []step{
		{kind: stepField, idx: []int{5}, name: "nest"},
		{kind: stepKey, key: "a"},
		{kind: stepIndex, pos: 1},
		{kind: stepField, idx: []int{0}, name: "a.b"},
	}

	// --- When ---
	stt.markReached(steps)

	// --- Then ---
	want := map[string]pathFlag{
		"":                pathReached,
		"nest":            pathReached,
		"nest[a]":         pathReached,
		"nest[a][1]":      pathReached,
		"nest[a][1][a.b]": pathReached,
	}
	assert.Equal(t, want, stt.paths)
}

func Test_state_undo(t *testing.T) {
	t.Run("sets the values back", func(t *testing.T) {
		// --- Given ---
		s := []int{1, 2}

		stt := newState()
		stt.markReset("a", reflect.Value{})
		stt.markReset("b", reflect.ValueOf(&s).Elem())
		stt.markReset("c", reflect.Value{})

		// --- When ---
		stt.undo(1)

		// --- Then ---
		assert.Equal(t, map[string]pathFlag{"a": pathReset}, stt.paths)
		assert.Len(t, 1, stt.log)
		assert.Equal(t, "a", stt.log[0].pth)
		assert.Equal(t, []int{1, 2}, s)
	})

	t.Run("charges array resets to the budget", func(t *testing.T) {
		// --- Given ---
		arr := [3]int{1, 2, 3}
		s := []int{1}

		stt := newState()
		stt.budget = 10
		stt.markReset("a", reflect.ValueOf(&arr).Elem())
		stt.markReset("s", reflect.ValueOf(&s).Elem())

		// --- When ---
		stt.undo(0)

		// --- Then ---
		assert.Equal(t, 7, stt.budget)
		assert.Equal(t, [3]int{1, 2, 3}, arr)
		assert.Equal(t, []int{1}, s)
	})

	t.Run("budget does not go below zero", func(t *testing.T) {
		// --- Given ---
		arr := [3]int{1, 2, 3}

		stt := newState()
		stt.budget = 2
		stt.markReset("a", reflect.ValueOf(&arr).Elem())

		// --- When ---
		stt.undo(0)

		// --- Then ---
		assert.Equal(t, 0, stt.budget)
	})
}

func Test_Decoder_apply(t *testing.T) {
	t.Run("error - unexported nil pointer", func(t *testing.T) {
		// --- Given ---
		dst := reflect.ValueOf((*TItem)(nil))
		stp := []step{{kind: stepField, idx: []int{0}, name: "size"}}
		vls := []string{"1"}

		// --- When ---
		err := NewDecoder().apply(dst, stp, vls, newState(), "")

		// --- Then ---
		assert.ErrorIs(t, ErrUnexported, err)
	})

	t.Run("failed key into a map entry undoes its reset", func(t *testing.T) {
		// --- Given ---
		dec := NewDecoder()
		m := map[string][]int{"k": {9, 9, 9}}
		dst := reflect.ValueOf(&m).Elem()
		key := step{kind: stepKey, key: "k"}
		stp := []step{key, {kind: stepIndex, pos: 0}}
		vls := []string{"1"}

		stt := newState()
		bad := []step{key, {kind: stepIndex, pos: 1}}
		_ = dec.apply(dst, bad, []string{"abc"}, stt, "") // Fails on abc.

		// --- When ---
		err := dec.apply(dst, stp, vls, stt, "")

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, map[string][]int{"k": {1}}, m)
	})

	t.Run("error - failed key adds no slice elements", func(t *testing.T) {
		// --- Given ---
		s := []TRequired{{FStr: "a"}}
		dst := reflect.ValueOf(&s).Elem()
		stp := []step{
			{kind: stepIndex, pos: 3},
			{kind: stepField, idx: []int{1}, name: "f_int"},
		}
		vls := []string{"x"}

		stt := newState()
		stt.markReset("", reflect.Value{})

		// --- When ---
		err := NewDecoder().apply(dst, stp, vls, stt, "")

		// --- Then ---
		assert.ErrorEqual(t, "invalid value: cannot convert `x` to int", err)
		assert.Equal(t, []TRequired{{FStr: "a"}}, s)
	})

	t.Run("error - failed key keeps spare capacity", func(t *testing.T) {
		// --- Given ---
		s := make([]TRequired, 1, 8)
		s[0].FStr = "a"
		dst := reflect.ValueOf(&s).Elem()
		stp := []step{
			{kind: stepIndex, pos: 3},
			{kind: stepField, idx: []int{1}, name: "f_int"},
		}
		vls := []string{"x"}

		stt := newState()
		stt.markReset("", reflect.Value{})

		// --- When ---
		err := NewDecoder().apply(dst, stp, vls, stt, "")

		// --- Then ---
		assert.ErrorEqual(t, "invalid value: cannot convert `x` to int", err)
		assert.Equal(t, []TRequired{{FStr: "a"}}, s)
		assert.Equal(t, 8, cap(s))
	})
}

func Test_Decoder_applyLeaf(t *testing.T) {
	t.Run("sets the value", func(t *testing.T) {
		// --- Given ---
		s := &struct{ F *int }{}
		dst := reflectkit.GetValue(t, s, "F")
		stt := newState()

		// --- When ---
		err := NewDecoder().applyLeaf(dst, []string{"3"}, stt, "f", unmUnknown)

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, 3, *s.F)
	})

	t.Run("known unmarshaler", func(t *testing.T) {
		// --- Given ---
		s := &struct{ F TTextPtr }{}
		dst := reflectkit.GetValue(t, s, "F")
		stt := newState()

		// --- When ---
		err := NewDecoder().applyLeaf(dst, []string{"4"}, stt, "f", unmYes)

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, TTextPtr(4), s.F)
		assert.Len(t, 1, stt.log) // The value was saved first.
	})

	t.Run("error - value", func(t *testing.T) {
		// --- Given ---
		s := &struct{ F int }{}
		dst := reflectkit.GetValue(t, s, "F")
		stt := newState()
		stt.key = "x"

		// --- When ---
		err := NewDecoder().applyLeaf(dst, []string{"a"}, stt, "f", unmNo)

		// --- Then ---
		assert.ErrorIs(t, ErrInvValue, err)
		assert.True(t, stt.has("f", pathFailed))
	})

	t.Run("error - no decoder", func(t *testing.T) {
		// --- Given ---
		s := &struct{ F chan int }{}
		dst := reflectkit.GetValue(t, s, "F")
		stt := newState()

		// --- When ---
		err := NewDecoder().applyLeaf(dst, []string{"a"}, stt, "f", unmNo)

		// --- Then ---
		assert.ErrorIs(t, ErrDecNotFound, err)
	})

	t.Run("error - conflict", func(t *testing.T) {
		// --- Given ---
		s := &struct{ F int }{}
		dst := reflectkit.GetValue(t, s, "F")
		stt := newState()
		stt.mark("f", pathSet)

		// --- When ---
		err := NewDecoder().applyLeaf(dst, []string{"1"}, stt, "f", unmNo)

		// --- Then ---
		assert.ErrorIs(t, ErrInvValue, err)
		assert.Equal(t, 0, s.F)
	})
}

func Test_Decoder_setLeafValue(t *testing.T) {
	t.Run("pointer", func(t *testing.T) {
		// --- Given ---
		s := &struct{ F *int }{}
		dst := reflectkit.GetValue(t, s, "F")
		stt := newState()

		// --- When ---
		err := NewDecoder().setLeafValue(dst, []string{"3"}, stt, unmUnknown)

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, 3, *s.F)
		assert.Len(t, 1, stt.log) // The nil pointer, before it was set.
	})

	t.Run("error - no decoder", func(t *testing.T) {
		// --- Given ---
		s := &struct{ F chan int }{}
		dst := reflectkit.GetValue(t, s, "F")

		// --- When ---
		err := NewDecoder().setLeafValue(dst, []string{"x"}, newState(), unmNo)

		// --- Then ---
		assert.ErrorIs(t, ErrDecNotFound, err)
	})
}

func Test_Decoder_setLeaf(t *testing.T) {
	t.Run("sets the value", func(t *testing.T) {
		// --- Given ---
		s := &struct{ F int }{}
		dst := reflectkit.GetValue(t, s, "F")
		typ := reflect.TypeFor[int]()
		stt := newState()
		stt.key = "f"

		// --- When ---
		set := builtinSetter(typ)
		err := NewDecoder().setLeaf(dst, []string{"5"}, stt, "f", typ, set)

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, 5, s.F)
	})

	t.Run("error - value", func(t *testing.T) {
		// --- Given ---
		s := &struct{ F int }{}
		dst := reflectkit.GetValue(t, s, "F")
		typ := reflect.TypeFor[int]()
		stt := newState()
		stt.key = "f"

		// --- When ---
		set := builtinSetter(typ)
		err := NewDecoder().setLeaf(dst, []string{"x"}, stt, "f", typ, set)

		// --- Then ---
		assert.ErrorIs(t, ErrInvValue, err)
		assert.True(t, stt.has("f", pathFailed))
	})

	t.Run("error - conflict", func(t *testing.T) {
		// --- Given ---
		s := &struct{ F int }{}
		dst := reflectkit.GetValue(t, s, "F")
		typ := reflect.TypeFor[int]()
		stt := newState()
		stt.key, stt.alias = "f", true
		stt.mark("f", pathSet)

		// --- When ---
		set := builtinSetter(typ)
		err := NewDecoder().setLeaf(dst, []string{"5"}, stt, "f", typ, set)

		// --- Then ---
		assert.ErrorIs(t, ErrInvValue, err)
		assert.Equal(t, 0, s.F)
	})
}

func Test_state_setAt(t *testing.T) {
	t.Run("canonical key", func(t *testing.T) {
		// --- Given ---
		stt := newState()
		stt.key, stt.track = "a", false

		// --- When ---
		err := stt.setAt("a", reflect.TypeFor[int]())

		// --- Then ---
		assert.NoError(t, err)
		assert.Len(t, 0, stt.paths)
		assert.False(t, stt.alias)
	})

	t.Run("canonical key is reached", func(t *testing.T) {
		// --- Given ---
		stt := newState()
		stt.key = "a"

		// --- When ---
		err := stt.setAt("a", reflect.TypeFor[int]())

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, map[string]pathFlag{"a": pathReached}, stt.paths)
	})

	t.Run("canonical key of a slice", func(t *testing.T) {
		// --- Given ---
		stt := newState()
		stt.key, stt.track = "a", false

		// --- When ---
		err := stt.setAt("a", reflect.TypeFor[*[]int]())

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, map[string]pathFlag{"a": pathSet}, stt.paths)
	})

	t.Run("other key", func(t *testing.T) {
		// --- Given ---
		stt := newState()
		stt.key, stt.track = "[a]", false

		// --- When ---
		err := stt.setAt("a", reflect.TypeFor[int]())

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, map[string]pathFlag{"a": pathSet}, stt.paths)
		assert.True(t, stt.alias)
	})

	t.Run("error - other key after the canonical one", func(t *testing.T) {
		// --- Given ---
		stt := newState()
		stt.src = url.Values{"a": {"1"}, "b[0]": {"2"}}
		stt.key = "b[0]"

		// --- When ---
		err := stt.setAt("a", reflect.TypeFor[int]())

		// --- Then ---
		assert.ErrorIs(t, ErrInvValue, err)
	})

	t.Run("canonical key after another key", func(t *testing.T) {
		// --- Given ---
		stt := newState()
		stt.key, stt.alias = "a", true
		stt.mark("a", pathSet)

		// --- When ---
		err := stt.setAt("a", reflect.TypeFor[int]())

		// --- Then ---
		assert.ErrorIs(t, ErrInvValue, err)
	})

	t.Run("error - indexed slice", func(t *testing.T) {
		// --- Given ---
		stt := newState()
		stt.key = "a"
		stt.mark("a", pathIndexed)

		// --- When ---
		err := stt.setAt("a", reflect.TypeFor[[]int]())

		// --- Then ---
		assert.ErrorIs(t, ErrInvValue, err)
	})
}

func Test_Decoder_isPlainKey_tabular(t *testing.T) {
	reg := WithDecFunc(func(string) (int, error) { return 0, nil })

	tt := []struct {
		testN string

		dec  *Decoder
		typ  reflect.Type
		want bool
	}{
		{"string", NewDecoder(), reflect.TypeFor[string](), true},
		{"int", NewDecoder(), reflect.TypeFor[int](), true},
		{"named int", NewDecoder(), reflect.TypeFor[IntAlias](), true},
		{"bool", NewDecoder(), reflect.TypeFor[bool](), true},
		{"float", NewDecoder(), reflect.TypeFor[float64](), false},
		{"uintptr", NewDecoder(), reflect.TypeFor[uintptr](), false},
		{"duration", NewDecoder(), reflect.TypeFor[time.Duration](), true},
		{"text marshaler", NewDecoder(), reflect.TypeFor[TTextVal](), false},
		{"text unmarshaler", NewDecoder(), reflect.TypeFor[TTextPtr](), false},
		{"pointer", NewDecoder(), reflect.TypeFor[*int](), false},
		{
			"registered",
			NewDecoder(reg),
			reflect.TypeFor[int](),
			false,
		},
		{"zero value", &Decoder{}, reflect.TypeFor[string](), true},
	}

	for _, tc := range tt {
		t.Run(tc.testN, func(t *testing.T) {
			// --- When ---
			have := tc.dec.isPlainKey(tc.typ)

			// --- Then ---
			assert.Equal(t, tc.want, have)
		})
	}
}

func Test_Decoder_isUnmType_tabular(t *testing.T) {
	tt := []struct {
		testN string

		dec  *Decoder
		typ  reflect.Type
		want bool
	}{
		{"pointer method", NewDecoder(), reflect.TypeFor[TTextPtr](), true},
		{"int", NewDecoder(), reflect.TypeFor[int](), false},
		{"time", NewDecoder(), reflect.TypeFor[time.Time](), false},
		{"pointer", NewDecoder(), reflect.TypeFor[*TTextPtr](), false},
		{
			"interface",
			NewDecoder(),
			reflect.TypeFor[encoding.TextUnmarshaler](),
			false,
		},
		{
			"registered",
			NewDecoder(WithDecFunc(func(string) (TTextPtr, error) {
				return 0, nil
			})),
			reflect.TypeFor[TTextPtr](),
			false,
		},
	}

	for _, tc := range tt {
		t.Run(tc.testN, func(t *testing.T) {
			// --- When ---
			have := tc.dec.isUnmType(tc.typ)

			// --- Then ---
			assert.Equal(t, tc.want, have)
		})
	}
}

func Test_keyConflict(t *testing.T) {
	// --- When ---
	err := keyConflict()

	// --- Then ---
	assert.ErrorEqual(t, "invalid value: another key sets the value", err)
	assert.ErrorIs(t, ErrInvValue, err)
}

func Test_Decoder_isLeafType_tabular(t *testing.T) {
	tt := []struct {
		testN string

		typ  reflect.Type
		want bool
	}{
		{"registered", reflect.TypeFor[TPoint](), true},
		{"time", reflect.TypeFor[time.Time](), true},
		{"unmarshaler", reflect.TypeFor[TItf](), true},
		{"unmarshaler map", reflect.TypeFor[TUnmMap](), true},
		{"string", reflect.TypeFor[string](), true},
		{"interface", reflect.TypeFor[any](), true},
		{"struct", reflect.TypeFor[TItem](), false},
		{"map", reflect.TypeFor[map[string]int](), false},
		{"slice", reflect.TypeFor[[]int](), false},
		{"array", reflect.TypeFor[[2]int](), false},
	}

	for _, tc := range tt {
		t.Run(tc.testN, func(t *testing.T) {
			// --- When ---
			have := NewDecoder(WithDecFunc(decPoint)).isLeafType(tc.typ)

			// --- Then ---
			assert.Equal(t, tc.want, have)
		})
	}
}

func Test_Decoder_isLeafValue_tabular(t *testing.T) {
	tt := []struct {
		testN string

		typ  reflect.Type
		want bool
	}{
		{"leaf", reflect.TypeFor[string](), true},
		{"pointer to leaf", reflect.TypeFor[*int](), true},
		{"slice of leaves", reflect.TypeFor[[]*int](), true},
		{"array of leaves", reflect.TypeFor[[2]int](), true},
		{"struct", reflect.TypeFor[TItem](), false},
		{"slice of structs", reflect.TypeFor[[]TItem](), false},
		{"map", reflect.TypeFor[map[string]int](), false},
	}

	for _, tc := range tt {
		t.Run(tc.testN, func(t *testing.T) {
			// --- When ---
			have := NewDecoder().isLeafValue(tc.typ)

			// --- Then ---
			assert.Equal(t, tc.want, have)
		})
	}
}

func Test_Decoder_required_tabular(t *testing.T) {
	cyc := &TNode{}
	cyc.Next = cyc

	type lst struct {
		L []lst  `url:"l"`
		R string `url:"r,required"`
	}
	sub := make([]lst, 2)
	sub[1].L = sub[:1]

	type in struct {
		N int `url:"n,required"`
	}
	type out struct {
		F in  `url:"f"`
		Q *in `url:"q"`
	}
	fst := &out{}
	fst.Q = &fst.F

	tt := []struct {
		testN string

		dst  any
		want []string
	}{
		{"empty", &TRequired{}, []string{"f_str"}},
		{"not empty", &TRequired{FStr: "a"}, nil},
		{"leaf struct", &TComplex{}, []string{"f_itf"}},
		{"leaf struct not empty", &TComplex{FItf: TItf{Str: "a"}}, nil},
		{"mixed struct", &TMixed{}, []string{"f_str"}},
		{"not exported embedded", &TNotExpEmb{}, []string{"f_str"}},
		{"embedded struct", &struct{ TRequired }{}, []string{"f_str"}},
		{
			"nested struct value",
			&struct {
				V TRequired `url:"v"`
			}{},
			[]string{"v.f_str"},
		},
		{
			"nil nested struct pointer",
			&struct {
				P *TRequired `url:"p"`
			}{},
			nil,
		},
		{
			"nested struct pointer",
			&struct {
				P *TRequired `url:"p"`
			}{P: &TRequired{}},
			[]string{"p.f_str"},
		},
		{
			"ignored and unexported fields",
			&struct {
				A string `url:"-,required"`
				b string `url:"b,required"`
			}{},
			nil,
		},
		{"pointer cycle", cyc, []string{"next.req", "req"}},
		{
			"map and slice elements",
			&struct {
				M map[int]TRequired  `url:"m"`
				P []*TRequired       `url:"p"`
				A [1]TRequired       `url:"a"`
				N map[string][]TNode `url:"n"`
			}{
				M: map[int]TRequired{7: {}, 8: {FStr: "a"}},
				P: []*TRequired{nil, {}},
				N: map[string][]TNode{"k": {{Req: "r"}, {}}},
			},
			[]string{"a[0].f_str", "m[7].f_str", "n[k][1].req", "p[1].f_str"},
		},
		{
			"sub-slice of a walked slice",
			&lst{L: sub},
			[]string{"l[0].r", "l[1].l[0].r", "l[1].r", "r"},
		},
		{
			"pointer to the first field",
			&struct {
				P *out `url:"p"`
			}{P: fst},
			[]string{"p.f.n", "p.q.n"},
		},
		{
			"pointer cycle in elements",
			&struct {
				L []*TNode `url:"l"`
			}{L: []*TNode{cyc}},
			[]string{"l[0].req"},
		},
		{
			"dotted field name",
			&struct {
				D string `url:"a.b,required"`
			}{},
			[]string{"[a.b]"},
		},
		{
			"required nil nested struct pointer",
			&struct {
				P *TRequired `url:"p,required"`
			}{},
			[]string{"p"},
		},
		{
			"required zero nested struct pointer",
			&struct {
				P *TRequired `url:"p,required"`
			}{P: &TRequired{}},
			[]string{"p", "p.f_str"},
		},
		{
			"required nested struct pointer",
			&struct {
				P *TRequired `url:"p,required"`
			}{P: &TRequired{FStr: "a"}},
			nil,
		},
		{
			"present field no key reached",
			&struct {
				A int `url:"a,present"`
			}{A: 1},
			[]string{"a"},
		},
		{
			"required embedded struct",
			&struct {
				TRequired `url:",required"`
			}{},
			[]string{"f_str"},
		},
	}

	for _, tc := range tt {
		t.Run(tc.testN, func(t *testing.T) {
			// --- Given ---
			sv := mirror.NewStructValue(tc.dst)
			lvl := newLevel(reflect.TypeOf(tc.dst), "url")
			seen := map[ptrKey]bool{}

			var have []string
			setErr := func(pth string, _ error) { have = append(have, pth) }

			// --- When ---
			NewDecoder().required("", sv, lvl, seen, newState(), setErr)

			// --- Then ---
			slices.Sort(have)
			assert.Equal(t, tc.want, have)
		})
	}
}

func Test_Decoder_presentIn(t *testing.T) {
	type base struct {
		ID int    `url:"id,present"`
		N  string `url:"n"`
	}

	t.Run("present field no key reached", func(t *testing.T) {
		// --- Given ---
		typ := reflect.TypeFor[base]()
		lvl := newLevel(typ, "url")
		types := map[reflect.Type]bool{}

		var have []string
		setErr := func(pth string, _ error) { have = append(have, pth) }

		// --- When ---
		NewDecoder().presentIn("", typ, lvl, types, newState(), setErr)

		// --- Then ---
		assert.Equal(t, []string{"id"}, have)
		assert.Len(t, 0, types)
	})

	t.Run("struct no key reached", func(t *testing.T) {
		// --- Given ---
		typ := reflect.TypeFor[base]()
		lvl := newLevel(typ, "url")
		types := map[reflect.Type]bool{}

		var have []string
		setErr := func(pth string, _ error) { have = append(have, pth) }

		// --- When ---
		NewDecoder().presentIn("p", typ, lvl, types, newState(), setErr)

		// --- Then ---
		assert.Nil(t, have)
	})
}

func Test_Decoder_requiredIn(t *testing.T) {
	t.Run("pointer to slice", func(t *testing.T) {
		// --- Given ---
		s := []TRequired{{FStr: "a"}, {}}
		val := reflect.ValueOf(&s)
		seen := map[ptrKey]bool{}

		var have []string
		setErr := func(pth string, _ error) { have = append(have, pth) }

		// --- When ---
		NewDecoder().requiredIn("l", val, seen, newState(), setErr)

		// --- Then ---
		assert.Equal(t, []string{"l[1].f_str"}, have)
	})

	t.Run("nil pointer", func(t *testing.T) {
		// --- Given ---
		val := reflect.ValueOf((*[]TRequired)(nil))
		seen := map[ptrKey]bool{}

		var have []string
		setErr := func(pth string, _ error) { have = append(have, pth) }

		// --- When ---
		NewDecoder().requiredIn("l", val, seen, newState(), setErr)

		// --- Then ---
		assert.Nil(t, have)
	})

	t.Run("leaf elements", func(t *testing.T) {
		// --- Given ---
		val := reflect.ValueOf([]TReqUnm{{}})
		seen := map[ptrKey]bool{}

		var have []string
		setErr := func(pth string, _ error) { have = append(have, pth) }

		// --- When ---
		NewDecoder().requiredIn("l", val, seen, newState(), setErr)

		// --- Then ---
		assert.Nil(t, have)
	})

	t.Run("map key text from the src key", func(t *testing.T) {
		// --- Given ---
		val := reflect.ValueOf(map[TPoint]TRequired{{1, 2}: {}})
		seen := map[ptrKey]bool{}

		stt := newState()
		m := reflect.ValueOf(map[TPoint]TRequired(nil))
		_ = stt.entry("m", m, reflect.ValueOf(TPoint{1, 2}), "1,2", false)

		var have []string
		setErr := func(pth string, _ error) { have = append(have, pth) }

		// --- When ---
		NewDecoder().requiredIn("m", val, seen, stt, setErr)

		// --- Then ---
		assert.Equal(t, []string{"m[1,2].f_str"}, have)
	})

	t.Run("map key text of a key no src key reached", func(t *testing.T) {
		// --- Given ---
		val := reflect.ValueOf(map[TPoint]TRequired{{1, 2}: {}})
		seen := map[ptrKey]bool{}

		var have []string
		setErr := func(pth string, _ error) { have = append(have, pth) }

		// --- When ---
		NewDecoder().requiredIn("m", val, seen, newState(), setErr)

		// --- Then ---
		assert.Equal(t, []string{"m[{1 2}].f_str"}, have)
	})
}

func Test_isUnmarshal_tabular(t *testing.T) {
	type S struct{ F string }

	tt := []struct {
		testN string

		s    any
		want bool
	}{
		{
			"field with type implementing with pointer receiver",
			&struct{ F TTextPtr }{},
			true,
		},
		{
			"nil pointer to type implementing with pointer receiver",
			&struct{ F *TTextPtr }{},
			false,
		},
		{"int field", &struct{ F int }{}, false},
		{"field with type not implementing", &struct{ F S }{}, false},
	}

	for _, tc := range tt {
		t.Run(tc.testN, func(t *testing.T) {
			// --- When ---
			have := isUnmarshal(reflectkit.GetValue(t, tc.s, "F"), nil)

			// --- Then ---
			assert.Equal(t, tc.want, have)
		})
	}
}

func Test_unmarshalText(t *testing.T) {
	t.Run("value field with pointer receiver method", func(t *testing.T) {
		// --- Given ---
		s := &struct{ F TTextPtr }{}

		// --- When ---
		err := unmarshalText(reflectkit.GetValue(t, s, "F"), nil, []string{"1"})

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, TTextPtr(1), s.F)
	})

	t.Run("pointer field with pointer receiver method", func(t *testing.T) {
		// --- Given ---
		s := &struct{ F *TTextPtr }{F: new(TTextPtr(0))}
		dst := reflectkit.GetValue(t, s, "F")
		// Must call reflect.Value.Elem before calling unmarshalText.
		dst = dst.Elem()

		// --- When ---
		err := unmarshalText(dst, nil, []string{"1"})

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, new(TTextPtr(1)), s.F)
	})

	t.Run("nil pointer field with pointer receiver method", func(t *testing.T) {
		// --- Given ---
		s := &struct{ F *TTextPtr }{}
		dst := reflectkit.GetValue(t, s, "F")
		// Must instantiate new instance of the dst type and
		// call reflect.Value.Elem before calling unmarshalText.
		dst.Set(reflect.New(dst.Type().Elem()))
		dst = dst.Elem()

		// --- When ---
		err := unmarshalText(dst, nil, []string{"1"})

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, new(TTextPtr(1)), s.F)
	})

	t.Run("error - many values", func(t *testing.T) {
		// --- Given ---
		s := &struct{ F TTextPtr }{}
		dst := reflectkit.GetValue(t, s, "F")

		// --- When ---
		err := unmarshalText(dst, nil, []string{"1", "2"})

		// --- Then ---
		assert.ErrorEqual(t, "require single value: received 2", err)
		assert.Equal(t, TTextPtr(0), s.F)
	})

	t.Run("method promoted from nil embedded pointer", func(t *testing.T) {
		// --- Given ---
		s := &struct{ F TPromZero }{}
		dst := reflectkit.GetValue(t, s, "F")

		// --- When ---
		err := unmarshalText(dst, nil, []string{"2000-01-02T03:04:05Z"})

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, time.Date(2000, 1, 2, 3, 4, 5, 0, time.UTC), *s.F.Time)
	})

	t.Run("error - unexported nil embedded pointer", func(t *testing.T) {
		// --- Given ---
		type tTime struct{ time.Time }
		s := &struct{ F struct{ *tTime } }{}
		dst := reflectkit.GetValue(t, s, "F")

		// --- When ---
		err := unmarshalText(dst, nil, []string{"2000-01-02T03:04:05Z"})

		// --- Then ---
		assert.ErrorIs(t, ErrUnexported, err)
		assert.Nil(t, s.F.tTime)
	})
}

func Test_allocPath(t *testing.T) {
	t.Run("nil pointers on the path", func(t *testing.T) {
		// --- Given ---
		s := &struct{ *TPromZero }{}
		dst := reflect.ValueOf(s).Elem()

		// --- When ---
		err := allocPath(dst, "UnmarshalText", nil)

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, time.Time{}, *s.Time)
	})

	t.Run("nil pointer behind embedded struct", func(t *testing.T) {
		// --- Given ---
		s := &struct{ TPromZero }{}
		dst := reflect.ValueOf(s).Elem()

		// --- When ---
		err := allocPath(dst, "UnmarshalText", nil)

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, time.Time{}, *s.Time)
	})

	t.Run("pointers off the path stay nil", func(t *testing.T) {
		// --- Given ---
		type wrap struct{ *TTextPtr }
		s := &struct {
			wrap
			*TTextPtr
			TOwnUnm
		}{}
		dst := reflect.ValueOf(s).Elem()

		// --- When ---
		err := allocPath(dst, "MarshalText", nil)

		// --- Then ---
		assert.NoError(t, err)
		assert.NotNil(t, s.TTextPtr)
		assert.Nil(t, s.wrap.TTextPtr)
		assert.Nil(t, s.TItem)
	})

	t.Run("no method", func(t *testing.T) {
		// --- Given ---
		s := &struct{ *TItem }{}
		dst := reflect.ValueOf(s).Elem()

		// --- When ---
		err := allocPath(dst, "UnmarshalText", nil)

		// --- Then ---
		assert.NoError(t, err)
		assert.Nil(t, s.TItem)
	})

	t.Run("error - unexported nil pointer on the path", func(t *testing.T) {
		// --- Given ---
		s := &struct{ *tUnmInner }{}
		dst := reflect.ValueOf(s).Elem()

		// --- When ---
		err := allocPath(dst, "UnmarshalText", nil)

		// --- Then ---
		assert.ErrorIs(t, ErrUnexported, err)
		assert.Nil(t, s.tUnmInner)
	})

	t.Run("nil pointers behind an embedded interface", func(t *testing.T) {
		// --- Given ---
		in := &struct{ *TItf }{}
		s := &struct{ encoding.TextUnmarshaler }{in}
		dst := reflect.ValueOf(s).Elem()

		// --- When ---
		err := allocPath(dst, "UnmarshalText", nil)

		// --- Then ---
		assert.NoError(t, err)
		assert.NotNil(t, in.TItf)
	})

	t.Run("held value with a nil embed off the path", func(t *testing.T) {
		// --- Given ---
		in := &TItf{}
		held := struct {
			*TItf
			*TItem
		}{TItf: in}
		s := &struct{ encoding.TextUnmarshaler }{held}
		dst := reflect.ValueOf(s).Elem()

		// --- When ---
		err := allocPath(dst, "UnmarshalText", nil)

		// --- Then ---
		assert.NoError(t, err)
	})

	t.Run("error - nil pointer held by an interface", func(t *testing.T) {
		// --- Given ---
		s := &struct{ encoding.TextUnmarshaler }{(*TItf)(nil)}
		dst := reflect.ValueOf(s).Elem()

		// --- When ---
		err := allocPath(dst, "UnmarshalText", nil)

		// --- Then ---
		assert.ErrorIs(t, ErrDecNotFound, err)
	})

	t.Run("error - nil embed in a held value", func(t *testing.T) {
		// --- Given ---
		s := &struct{ encoding.TextUnmarshaler }{struct{ *TItf }{}}
		dst := reflect.ValueOf(s).Elem()

		// --- When ---
		err := allocPath(dst, "UnmarshalText", nil)

		// --- Then ---
		assert.ErrorIs(t, ErrDecNotFound, err)
	})

	t.Run("error - nil interface on the path", func(t *testing.T) {
		// --- Given ---
		s := &struct{ encoding.TextUnmarshaler }{}
		dst := reflect.ValueOf(s).Elem()

		// --- When ---
		err := allocPath(dst, "UnmarshalText", nil)

		// --- Then ---
		assert.ErrorIs(t, ErrDecNotFound, err)
	})
}

func Test_nilOnPath_tabular(t *testing.T) {
	type wrap struct{ *TItf }

	tt := []struct {
		testN string

		val  any
		want bool
	}{
		{"declared", &TItf{}, false},
		{"nil pointer", (*wrap)(nil), true},
		{"nil embed on the path", wrap{}, true},
		{"embed on the path", wrap{&TItf{}}, false},
		{
			"nil embed off the path",
			struct {
				wrap
				*TItem
			}{wrap: wrap{&TItf{}}},
			false,
		},
		{
			"nil embed behind an interface",
			struct{ encoding.TextUnmarshaler }{wrap{}},
			true,
		},
		{
			"nil interface",
			struct{ encoding.TextUnmarshaler }{},
			true,
		},
	}

	for _, tc := range tt {
		t.Run(tc.testN, func(t *testing.T) {
			// --- When ---
			have := nilOnPath(reflect.ValueOf(tc.val), "UnmarshalText")

			// --- Then ---
			assert.Equal(t, tc.want, have)
		})
	}
}

func Test_setDst(t *testing.T) {
	t.Run("set using decoder", func(t *testing.T) {
		// --- Given ---
		s := &struct{ F int }{}

		// --- When ---
		have, err := setDst(reflectkit.GetValue(t, s, "F"), nil, nil, nil, "1")

		// --- Then ---
		assert.NoError(t, err)
		assert.True(t, have)
		assert.Equal(t, 1, s.F)
	})

	t.Run("error - decoder", func(t *testing.T) {
		// --- Given ---
		s := &struct{ F byte }{}
		dst := reflectkit.GetValue(t, s, "F")

		// --- When ---
		have, err := setDst(dst, nil, nil, nil, "256")

		// --- Then ---
		assert.ErrorIs(t, ErrInvValue, err)
		xrrtest.AssertCode(t, ECInvValue, err)
		want := "invalid value: cannot convert `256` to uint8"
		assert.ErrorEqual(t, want, err)
		assert.True(t, have)
		assert.Equal(t, byte(0), s.F)
	})

	t.Run("set using Unmarshaler", func(t *testing.T) {
		// --- Given ---
		s := &struct{ F TTextPtr }{}

		// --- When ---
		have, err := setDst(reflectkit.GetValue(t, s, "F"), nil, nil, nil, "7F")

		// --- Then ---
		assert.NoError(t, err)
		assert.True(t, have)
		assert.Equal(t, TTextPtr(127), s.F)
	})

	t.Run("error - Unmarshaler", func(t *testing.T) {
		// --- Given ---
		s := &struct{ F TTextPtr }{}

		// --- When ---
		have, err := setDst(reflectkit.GetValue(t, s, "F"), nil, nil, nil, "GG")

		// --- Then ---
		assert.ErrorIs(t, ErrInvValue, err)
		xrrtest.AssertCode(t, ECInvValue, err)
		assert.ErrorEqual(t, "invalid value: cannot convert `GG` to byte", err)
		assert.True(t, have)
		assert.Equal(t, TTextPtr(0), s.F)
	})

	t.Run("unsupported type", func(t *testing.T) {
		// --- Given ---
		s := &struct{ F map[int]int }{}

		// --- When ---
		have, err := setDst(reflectkit.GetValue(t, s, "F"), nil, nil, nil, "1")

		// --- Then ---
		assert.NoError(t, err)
		assert.False(t, have)
		assert.Equal(t, map[int]int(nil), s.F)
	})

	t.Run("set type alias", func(t *testing.T) {
		// --- Given ---
		type Alias int
		s := &struct{ F Alias }{}

		// --- When ---
		have, err := setDst(reflectkit.GetValue(t, s, "F"), nil, nil, nil, "1")

		// --- Then ---
		assert.NoError(t, err)
		assert.True(t, have)
		assert.Equal(t, Alias(1), s.F)
	})

	t.Run("custom decoder used", func(t *testing.T) {
		// --- Given ---
		s := &struct{ F string }{}
		reg := decReg{
			reflect.TypeOf(""): func(vls ...string) (any, error) {
				return strings.Join(vls, "-"), nil
			},
		}

		dst := reflectkit.GetValue(t, s, "F")

		// --- When ---
		have, err := setDst(dst, reg, nil, nil, "a", "b")

		// --- Then ---
		assert.NoError(t, err)
		assert.True(t, have)
		assert.Equal(t, "a-b", s.F)
	})

	t.Run("error - custom decoder returns wrong type", func(t *testing.T) {
		// --- Given ---
		s := &struct{ F int }{}
		reg := decReg{
			reflect.TypeOf(1): func(vls ...string) (any, error) {
				return "abc", nil
			},
		}

		// --- When ---
		have, err := setDst(reflectkit.GetValue(t, s, "F"), reg, nil, nil, "1")

		// --- Then ---
		want := "invalid type: expected decoder to return `int` got `string`"
		assert.ErrorEqual(t, want, err)
		assert.ErrorIs(t, ErrType, err)
		xrrtest.AssertCode(t, ECInvType, err)
		assert.True(t, have)
		assert.Equal(t, 0, s.F)
	})
}

func Test_setValue(t *testing.T) {
	t.Run("unmarshaler", func(t *testing.T) {
		// --- Given ---
		s := &struct{ F TTextPtr }{}
		dst := reflectkit.GetValue(t, s, "F")

		// --- When ---
		have, err := setValue(dst, nil, nil, true, nil, []string{"7"})

		// --- Then ---
		assert.NoError(t, err)
		assert.True(t, have)
		assert.Equal(t, TTextPtr(7), s.F)
	})

	t.Run("built-in", func(t *testing.T) {
		// --- Given ---
		s := &struct{ F int }{}
		dst := reflectkit.GetValue(t, s, "F")

		// --- When ---
		have, err := setValue(dst, nil, nil, false, nil, []string{"7"})

		// --- Then ---
		assert.NoError(t, err)
		assert.True(t, have)
		assert.Equal(t, 7, s.F)
	})

	t.Run("registered over built-in", func(t *testing.T) {
		// --- Given ---
		s := &struct{ F int }{}
		dst := reflectkit.GetValue(t, s, "F")
		reg := decReg{
			reflect.TypeFor[int](): func(vls ...string) (any, error) {
				return len(vls[0]), nil
			},
		}

		// --- When ---
		have, err := setValue(dst, reg, nil, false, nil, []string{"abc"})

		// --- Then ---
		assert.NoError(t, err)
		assert.True(t, have)
		assert.Equal(t, 3, s.F)
	})

	t.Run("no decoder", func(t *testing.T) {
		// --- Given ---
		s := &struct{ F chan int }{}
		dst := reflectkit.GetValue(t, s, "F")

		// --- When ---
		have, err := setValue(dst, nil, nil, false, nil, []string{"x"})

		// --- Then ---
		assert.NoError(t, err)
		assert.False(t, have)
	})
}

func Test_setSliceOrArray(t *testing.T) {
	t.Run("slice", func(t *testing.T) {
		// --- Given ---
		s := &struct{ F []string }{F: make([]string, 0)}
		dst := mirror.NewStructValue(s).FieldByName("F").Value()

		// --- When ---
		err := setSliceOrArray(dst, nil, nil, "a", "b")

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, []string{"a", "b"}, s.F)
	})

	t.Run("replaces slice", func(t *testing.T) {
		// --- Given ---
		old := []string{"o1", "o2"}
		s := &struct{ F []string }{F: old}
		dst := mirror.NewStructValue(s).FieldByName("F").Value()

		// --- When ---
		err := setSliceOrArray(dst, nil, nil, "a")

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, []string{"a"}, s.F)
		assert.Equal(t, []string{"o1", "o2"}, old)
	})

	t.Run("zeroes array elements past values", func(t *testing.T) {
		// --- Given ---
		s := &struct{ F [3]int }{F: [3]int{7, 8, 9}}
		dst := mirror.NewStructValue(s).FieldByName("F").Value()

		// --- When ---
		err := setSliceOrArray(dst, nil, nil, "1")

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, [3]int{1, 0, 0}, s.F)
	})

	t.Run("nil slice", func(t *testing.T) {
		// --- Given ---
		s := &struct{ F []string }{}
		dst := mirror.NewStructValue(s).FieldByName("F").Value()

		// --- When ---
		err := setSliceOrArray(dst, nil, nil, "a", "b")

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, []string{"a", "b"}, s.F)
	})

	t.Run("slice of pointers", func(t *testing.T) {
		// --- Given ---
		s := &struct{ F []*string }{F: make([]*string, 0)}
		dst := mirror.NewStructValue(s).FieldByName("F").Value()

		// --- When ---
		err := setSliceOrArray(dst, nil, nil, "a", "b")

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, []*string{new("a"), new("b")}, s.F)
	})

	t.Run("nil slice of pointers", func(t *testing.T) {
		// --- Given ---
		s := &struct{ F []*string }{}
		dst := mirror.NewStructValue(s).FieldByName("F").Value()

		// --- When ---
		err := setSliceOrArray(dst, nil, nil, "a", "b")

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, []*string{new("a"), new("b")}, s.F)
	})

	t.Run("custom decoder used", func(t *testing.T) {
		// --- Given ---
		s := &struct{ F []string }{}
		dst := mirror.NewStructValue(s).FieldByName("F").Value()
		reg := decReg{
			reflect.TypeOf(""): func(vls ...string) (any, error) {
				return strings.ToUpper(vls[0]), nil
			},
		}

		// --- When ---
		err := setSliceOrArray(dst, reg, nil, "a", "b")

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, []string{"A", "B"}, s.F)
	})

	t.Run("error - no decoder", func(t *testing.T) {
		// --- Given ---
		type ct struct{}
		s := &struct{ F []ct }{}
		dst := mirror.NewStructValue(s).FieldByName("F").Value()

		// --- When ---
		err := setSliceOrArray(dst, nil, nil, "a")

		// --- Then ---
		assert.ErrorEqual(t, "decoder not found for `xbind.ct`", err)
		xrrtest.AssertCode(t, ECDecNotFound, err)
	})

	t.Run("error - decoder", func(t *testing.T) {
		// --- Given ---
		s := &struct{ F []TTextPtr }{}
		dst := mirror.NewStructValue(s).FieldByName("F").Value()

		// --- When ---
		err := setSliceOrArray(dst, nil, nil, "GG")

		// --- Then ---
		assert.ErrorEqual(t, "invalid value: cannot convert `GG` to byte", err)
		xrrtest.AssertCode(t, ECInvValue, err)
	})

	t.Run("array", func(t *testing.T) {
		// --- Given ---
		s := &struct{ F [2]string }{}
		dst := mirror.NewStructValue(s).FieldByName("F").Value()

		// --- When ---
		err := setSliceOrArray(dst, nil, nil, "a", "b")

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, [2]string{"a", "b"}, s.F)
	})

	t.Run("array fewer values than elements", func(t *testing.T) {
		// --- Given ---
		s := &struct{ F [2]string }{}
		dst := mirror.NewStructValue(s).FieldByName("F").Value()

		// --- When ---
		err := setSliceOrArray(dst, nil, nil, "a")

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, [2]string{"a", ""}, s.F)
	})

	t.Run("error - array more values than elements", func(t *testing.T) {
		// --- Given ---
		s := &struct{ F [1]string }{}
		dst := mirror.NewStructValue(s).FieldByName("F").Value()

		// --- When ---
		err := setSliceOrArray(dst, nil, nil, "a", "b")

		// --- Then ---
		assert.ErrorIs(t, ErrInvValue, err)
		assert.ErrorEqual(t, "invalid value: too many values for an array", err)
		xrrtest.AssertCode(t, ECInvValue, err)
	})

	t.Run("error - slice keeps its value", func(t *testing.T) {
		// --- Given ---
		s := &struct{ F []int }{F: []int{5}}
		dst := mirror.NewStructValue(s).FieldByName("F").Value()

		// --- When ---
		err := setSliceOrArray(dst, nil, nil, "1", "x")

		// --- Then ---
		assert.ErrorEqual(t, "invalid value: cannot convert `x` to int", err)
		assert.Equal(t, []int{5}, s.F)
	})

	t.Run("error - array keeps its value", func(t *testing.T) {
		// --- Given ---
		s := &struct{ F [3]int }{F: [3]int{7, 8, 9}}
		dst := mirror.NewStructValue(s).FieldByName("F").Value()

		// --- When ---
		err := setSliceOrArray(dst, nil, nil, "1", "x")

		// --- Then ---
		assert.ErrorEqual(t, "invalid value: cannot convert `x` to int", err)
		assert.Equal(t, [3]int{7, 8, 9}, s.F)
	})
}

func Test_decKey(t *testing.T) {
	t.Run("value key", func(t *testing.T) {
		// --- When ---
		have, err := decKey(reflect.TypeFor[int](), nil, nil, "42")

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, 42, have.Interface())
	})

	t.Run("pointer key", func(t *testing.T) {
		// --- When ---
		have, err := decKey(reflect.TypeFor[*int](), nil, nil, "42")

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, 42, *have.Interface().(*int))
	})

	t.Run("negative zero key", func(t *testing.T) {
		// --- When ---
		have, err := decKey(reflect.TypeFor[float64](), nil, nil, "-0")

		// --- Then ---
		assert.NoError(t, err)
		assert.False(t, math.Signbit(have.Float()))
	})

	t.Run("error - key", func(t *testing.T) {
		// --- When ---
		have, err := decKey(reflect.TypeFor[int](), nil, nil, "x")

		// --- Then ---
		assert.ErrorIs(t, ErrInvValue, err)
		assert.False(t, have.IsValid())
	})

	t.Run("error - no decoder", func(t *testing.T) {
		// --- When ---
		have, err := decKey(reflect.TypeFor[chan int](), nil, nil, "x")

		// --- Then ---
		assert.ErrorIs(t, ErrDecNotFound, err)
		assert.False(t, have.IsValid())
	})
}

func Test_deref(t *testing.T) {
	t.Run("allocates nil pointers", func(t *testing.T) {
		// --- Given ---
		var pp **int
		val := reflect.ValueOf(&pp).Elem()

		// --- When ---
		have, err := deref(val, nil)

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, reflect.Int, have.Kind())
		assert.NotNil(t, pp)
		assert.NotNil(t, *pp)
	})

	t.Run("saves the nil pointers it allocates", func(t *testing.T) {
		// --- Given ---
		var pp **int
		val := reflect.ValueOf(&pp).Elem()

		var saved []string
		save := func(v reflect.Value) {
			saved = append(saved, fmt.Sprintf("%s %v", v.Type(), v.IsNil()))
		}

		// --- When ---
		_, err := deref(val, save)

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, []string{"**int true", "*int true"}, saved)
	})

	t.Run("error - unexported nil pointer", func(t *testing.T) {
		// --- Given ---
		val := reflect.ValueOf(TUnexpPtr{}).Field(1)

		// --- When ---
		have, err := deref(val, nil)

		// --- Then ---
		assert.ErrorIs(t, ErrUnexported, err)
		assert.Equal(t, reflect.Pointer, have.Kind())
	})
}

func Test_decNotFound(t *testing.T) {
	// --- When ---
	err := decNotFound(reflect.TypeFor[chan int]())

	// --- Then ---
	assert.ErrorEqual(t, "decoder not found for `chan int`", err)
	assert.ErrorIs(t, ErrDecNotFound, err)
	xrrtest.AssertCode(t, ECDecNotFound, err)
}
