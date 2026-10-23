// SPDX-FileCopyrightText: (c) 2026 Rafal Zajac
// SPDX-License-Identifier: MIT

package xbind

import (
	"encoding"
	"io"
	"math"
	"net/url"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ctx42/testing/pkg/assert"
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

func Test_WithDecFunc(t *testing.T) {
	// --- Given ---
	dec := &Decoder{}

	// --- When ---
	WithDecFunc(decPoint)(dec)

	// --- Then ---
	have, err := dec.reg[reflect.TypeFor[TPoint]()]("1,2")
	assert.NoError(t, err)
	assert.Equal(t, TPoint{X: 1, Y: 2}, have)

	_, err = dec.reg[reflect.TypeFor[TPoint]()]("1,2", "3,4")
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

	t.Run("embedded interface not ignored", func(t *testing.T) {
		// --- Given ---
		dst := &struct {
			io.Reader
			FStr string `url:"f_str"`
		}{}
		src := url.Values{"f_str": {"abc"}}

		// --- When ---
		err := NewDecoder().Decode(dst, src)

		// --- Then ---
		assert.NoError(t, err)
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

	t.Run("error - not exported struct fields ignored", func(t *testing.T) {
		// --- Given ---
		dst := &struct {
			F1 time.Time `url:"f_1"`
			f2 int
		}{}
		src := url.Values{"f_1": {"2000-01-02T03:04:05"}}

		// --- When ---
		err := NewDecoder().Decode(dst, src)

		// --- Then ---
		want := "" +
			"f_1: invalid RFC3339 time: `2000-01-02T03:04:05` " +
			"(ECInvTimeFormat)"
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
		assert.ErrorIs(t, ErrUnexported, err)
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

		// --- When ---
		err := NewDecoder().Decode(dst, url.Values{})

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

		// --- When ---
		err := NewDecoder().Decode(dst, url.Values{})

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

	t.Run("registered decoder", func(t *testing.T) {
		// --- Given ---
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
		dec := NewDecoder(
			WithDecFunc(func(s string) (time.Time, error) {
				return time.Parse(time.DateOnly, s)
			}),
			WithDecFunc(decPoint),
			WithDecFunc(func(s string) (TLevel, error) {
				n, err := strconv.Atoi(s)
				return TLevel(n), err
			}),
		)

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
		assert.Equal(t, TLevel(9), dst.L)
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

	t.Run("error - malformed struct tag", func(t *testing.T) {
		// --- When ---
		err := NewDecoder().Decode(badTag(`url:n`, false), url.Values{})

		// --- Then ---
		assert.ErrorEqual(t, "invalid struct tag: N", err)
		assert.ErrorIs(t, ErrInvTag, err)
		xrrtest.AssertCode(t, ECInvTag, err)
	})

	t.Run("error - malformed tag in nil nested struct", func(t *testing.T) {
		// --- When ---
		err := NewDecoder().Decode(badTag(`url:n`, true), url.Values{})

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
		want := map[int][]TItem{1: {{Size: 1}, {Size: 2}}}
		assert.Equal(t, want, dst.M)
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

	t.Run("error - map entry not written on error", func(t *testing.T) {
		// --- Given ---
		dst := &TBox{Items: map[string]TItem{"a": {Size: 1}}}
		src := url.Values{"items[a].size": {"x"}, "items[b].size": {"x"}}

		// --- When ---
		err := NewDecoder().Decode(dst, src)

		// --- Then ---
		xrrtest.AssertFieldCnt(t, 2, err)
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
		want := "list[2].f_int: invalid value: cannot convert `x` to int " +
			"(ECInvValue)"
		xrrtest.AssertFieldsEqual(t, want, err)
		assert.Len(t, 0, dst.List)
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

	t.Run("index limit option", func(t *testing.T) {
		// --- Given ---
		dst := &TBox{}
		src := url.Values{"list[3].size": {"1"}}

		// --- When ---
		err := NewDecoder(WithDecMaxIndex(2)).Decode(dst, src)

		// --- Then ---
		want := "list[3].size: invalid value: index 3 over the limit 2 " +
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
		want := "list[1].size: invalid value: index 1 over the limit 0 " +
			"(ECInvValue)"
		xrrtest.AssertFieldsEqual(t, want, err)
		assert.Equal(t, []TItem{{Size: 1}}, dst.List)
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

		// --- When ---
		err := NewDecoder().Decode(dst, url.Values{})

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

	t.Run("recursive struct is not allocated", func(t *testing.T) {
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

	t.Run("embedding cycle", func(t *testing.T) {
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

	t.Run("unexported nil pointer in recursive struct", func(t *testing.T) {
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
		assert.NotNil(t, dst.Next)
		assert.Nil(t, dst.Next.tStr)
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
		pnt := NewEncoder(WithEncFunc(encPoint))
		enc := url.Values{}
		assert.NoError(t, pnt.Encode(enc, dst))

		again := &TFuzzDec{}
		assert.NoError(t, NewDecoder(WithDecFunc(decPoint)).Decode(again, enc))

		have := url.Values{}
		assert.NoError(t, pnt.Encode(have, again))
		assert.Equal(t, enc, have)
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

func Test_parseKey_tabular(t *testing.T) {
	tt := []struct {
		testN string

		key  string
		want []segment
	}{
		{"name", "a", []segment{{text: "a"}}},
		{"dotted", "a.b", []segment{{text: "a"}, {text: "b"}}},
		{
			"brackets",
			"a[b.c][0].d",
			[]segment{
				{text: "a"},
				{text: "b.c", bracket: true},
				{text: "0", bracket: true},
				{text: "d"},
			},
		},
		{"leading bracket", "[a.b]", []segment{{text: "a.b", bracket: true}}},
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
			have, ok := parseKey(tc.key)

			// --- Then ---
			assert.Equal(t, tc.want, have)
			assert.Equal(t, tc.want != nil, ok)
		})
	}
}

func Test_Decoder_plan(t *testing.T) {
	t.Run("steps", func(t *testing.T) {
		// --- Given ---
		segs, _ := parseKey("nest[a][1].size")

		// --- When ---
		have, err := NewDecoder().plan(reflect.TypeFor[TBox](), segs)

		// --- Then ---
		assert.NoError(t, err)
		want := []step{
			{kind: stepField, idx: []int{5}, name: "nest"},
			{kind: stepKey, key: "a", name: "[a]"},
			{kind: stepIndex, pos: 1, name: "[1]"},
			{kind: stepField, idx: []int{0}, name: "size"},
		}
		assert.Equal(t, want, have)
	})

	t.Run("embedded field", func(t *testing.T) {
		// --- Given ---
		segs, _ := parseKey("f_str")

		// --- When ---
		have, err := NewDecoder().plan(reflect.TypeFor[TFuzzDec](), segs)

		// --- Then ---
		assert.NoError(t, err)
		want := []step{{kind: stepField, idx: []int{0, 0}, name: "f_str"}}
		assert.Equal(t, want, have)
	})

	t.Run("rest of the key for a map of plain values", func(t *testing.T) {
		// --- Given ---
		segs, _ := parseKey("attr.a.b")

		// --- When ---
		have, err := NewDecoder().plan(reflect.TypeFor[TBox](), segs)

		// --- Then ---
		assert.NoError(t, err)
		want := []step{
			{kind: stepField, idx: []int{7}, name: "attr"},
			{kind: stepKey, key: "a.b", name: "[a.b]"},
		}
		assert.Equal(t, want, have)
	})

	t.Run("error - bracket in the rest of a map key", func(t *testing.T) {
		// --- Given ---
		segs, _ := parseKey("attr.a[b]")

		// --- When ---
		have, err := NewDecoder().plan(reflect.TypeFor[TBox](), segs)

		// --- Then ---
		assert.ErrorIs(t, ErrUnkFieldPath, err)
		assert.Nil(t, have)
	})

	t.Run("error - ignored field", func(t *testing.T) {
		// --- Given ---
		segs, _ := parseKey("Ign")

		// --- When ---
		have, err := NewDecoder().plan(reflect.TypeFor[TFuzzDec](), segs)

		// --- Then ---
		assert.ErrorIs(t, errIgnored, err)
		assert.Nil(t, have)
	})

	t.Run("error - segment after a value", func(t *testing.T) {
		// --- Given ---
		segs, _ := parseKey("page[size].x")

		// --- When ---
		have, err := NewDecoder().plan(reflect.TypeFor[TBox](), segs)

		// --- Then ---
		assert.ErrorIs(t, ErrUnkFieldPath, err)
		assert.Nil(t, have)
	})

	t.Run("error - segment into an interface", func(t *testing.T) {
		// --- Given ---
		segs, _ := parseKey("f.x")
		typ := reflect.TypeFor[struct {
			F any `url:"f"`
		}]()

		// --- When ---
		have, err := NewDecoder().plan(typ, segs)

		// --- Then ---
		assert.ErrorIs(t, ErrUnkFieldPath, err)
		assert.Nil(t, have)
	})
}

func Test_Decoder_index(t *testing.T) {
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

func Test_Decoder_index_errors_tabular(t *testing.T) {
	tt := []struct {
		testN string

		typ  reflect.Type
		txt  string
		want string
	}{
		{"not a number", reflect.TypeFor[[]int](), "x", "invalid field path"},
		{"negative", reflect.TypeFor[[]int](), "-1", "invalid field path"},
		{
			"over the limit",
			reflect.TypeFor[[]int](),
			"1001",
			"invalid value: index 1001 over the limit 1000",
		},
		{
			"overflow",
			reflect.TypeFor[[]int](),
			"99999999999999999999",
			"invalid value: index 99999999999999999999 over the limit 1000",
		},
		{
			"past array end",
			reflect.TypeFor[[2]int](),
			"2",
			"invalid value: index 2 past the array end",
		},
	}

	for _, tc := range tt {
		t.Run(tc.testN, func(t *testing.T) {
			// --- When ---
			have, err := NewDecoder().index(tc.typ, tc.txt)

			// --- Then ---
			assert.ErrorEqual(t, tc.want, err)
			assert.Equal(t, 0, have)
		})
	}
}

func Test_newState(t *testing.T) {
	// --- When ---
	have := newState()

	// --- Then ---
	assert.NotNil(t, have.reset)
	assert.Len(t, 0, have.reset)
	assert.Nil(t, have.log)
	assert.NotNil(t, have.entries)
	assert.Len(t, 0, have.entries)
	assert.NotNil(t, have.failed)
	assert.Len(t, 0, have.failed)
}

func Test_state_markReset(t *testing.T) {
	// --- Given ---
	stt := newState()

	// --- When ---
	stt.markReset("a")
	stt.markReset("b")
	stt.markReset("a")

	// --- Then ---
	assert.Equal(t, map[string]bool{"a": true, "b": true}, stt.reset)
	assert.Equal(t, []string{"a", "b"}, stt.log)
}

func Test_state_entry(t *testing.T) {
	t.Run("equal keys name one entry", func(t *testing.T) {
		// --- Given ---
		stt := newState()
		one := reflect.ValueOf(new(1))
		ent := stt.entry("m", one, "01")

		// --- When ---
		have := stt.entry("m", reflect.ValueOf(new(1)), "1")

		// --- Then ---
		assert.Equal(t, "1", ent.text)
		assert.Equal(t, ent.text, have.text)
		assert.Same(t, one.Interface(), have.key.Interface())
	})

	t.Run("key without an encoder takes the src text", func(t *testing.T) {
		// --- Given ---
		stt := newState()
		key := reflect.ValueOf(TPoint{1, 2})

		// --- When ---
		have := stt.entry("m", key, "1,2")

		// --- Then ---
		assert.Equal(t, "1,2", have.text)
		assert.Equal(t, TPoint{1, 2}, have.key.Interface())
	})

	t.Run("maps at other paths", func(t *testing.T) {
		// --- Given ---
		stt := newState()
		one := reflect.ValueOf(new(1))
		ent := stt.entry("a", one, "1")

		// --- When ---
		have := stt.entry("b", reflect.ValueOf(new(1)), "1")

		// --- Then ---
		assert.NotSame(t, ent.key.Interface(), have.key.Interface())
	})
}

func Test_state_undo(t *testing.T) {
	// --- Given ---
	stt := newState()
	stt.markReset("a")
	stt.markReset("b")
	stt.markReset("c")

	// --- When ---
	stt.undo(1)

	// --- Then ---
	assert.Equal(t, map[string]bool{"a": true}, stt.reset)
	assert.Equal(t, []string{"a"}, stt.log)
}

func Test_Decoder_apply(t *testing.T) {
	t.Run("error - unexported nil pointer", func(t *testing.T) {
		// --- Given ---
		dst := reflect.ValueOf((*TItem)(nil))
		stp := []step{{kind: stepField, idx: []int{0}, name: "size"}}

		// --- When ---
		err := NewDecoder().apply(dst, stp, []string{"1"}, newState(), "")

		// --- Then ---
		assert.ErrorIs(t, ErrUnexported, err)
	})

	t.Run("failed key into a map entry undoes its reset", func(t *testing.T) {
		// --- Given ---
		dec := NewDecoder()
		m := map[string][]int{"k": {9, 9, 9}}
		dst := reflect.ValueOf(&m).Elem()
		stt := newState()
		key := step{kind: stepKey, key: "k", name: "[k]"}
		bad := []step{key, {kind: stepIndex, pos: 1, name: "[1]"}}
		_ = dec.apply(dst, bad, []string{"abc"}, stt, "") // Fails on abc.
		stp := []step{key, {kind: stepIndex, pos: 0, name: "[0]"}}

		// --- When ---
		err := dec.apply(dst, stp, []string{"1"}, stt, "")

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, map[string][]int{"k": {1}}, m)
	})

	t.Run("error - failed key adds no slice elements", func(t *testing.T) {
		// --- Given ---
		s := []TRequired{{FStr: "a"}}
		dst := reflect.ValueOf(&s).Elem()
		stt := newState()
		stt.markReset("")
		stp := []step{
			{kind: stepIndex, pos: 3, name: "[3]"},
			{kind: stepField, idx: []int{1}, name: "f_int"},
		}

		// --- When ---
		err := NewDecoder().apply(dst, stp, []string{"x"}, stt, "")

		// --- Then ---
		assert.ErrorIs(t, ErrInvValue, err)
		assert.Equal(t, []TRequired{{FStr: "a"}}, s)
	})
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
			"required embedded struct",
			&struct {
				TRequired `url:",required"`
			}{TRequired{FStr: "a"}},
			nil,
		},
	}

	for _, tc := range tt {
		t.Run(tc.testN, func(t *testing.T) {
			// --- Given ---
			var have []string
			setErr := func(pth string, _ error) { have = append(have, pth) }

			// --- When ---
			NewDecoder().required(
				"",
				mirror.NewStructValue(tc.dst),
				newLevel(reflect.TypeOf(tc.dst), "url"),
				map[ptrKey]bool{},
				newState(),
				setErr,
			)

			// --- Then ---
			slices.Sort(have)
			assert.Equal(t, tc.want, have)
		})
	}
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
			have := isUnmarshal(reflectkit.GetValue(t, tc.s, "F"))

			// --- Then ---
			assert.Equal(t, tc.want, have)
		})
	}
}

func Test_useUnmarshal(t *testing.T) {
	t.Run("value field with pointer receiver method", func(t *testing.T) {
		// --- Given ---
		s := &struct{ F TTextPtr }{}

		// --- When ---
		have, err := useUnmarshal(reflectkit.GetValue(t, s, "F"), "1")

		// --- Then ---
		assert.NoError(t, err)
		assert.True(t, have)
		assert.Equal(t, TTextPtr(1), s.F)
	})

	t.Run("pointer field with pointer receiver method", func(t *testing.T) {
		// --- Given ---
		s := &struct{ F *TTextPtr }{F: new(TTextPtr(0))}
		dst := reflectkit.GetValue(t, s, "F")
		// Must call reflect.Value.Elem before calling useUnmarshal.
		dst = dst.Elem()

		// --- When ---
		have, err := useUnmarshal(dst, "1")

		// --- Then ---
		assert.NoError(t, err)
		assert.True(t, have)
		assert.Equal(t, new(TTextPtr(1)), s.F)
	})

	t.Run("nil pointer field with pointer receiver method", func(t *testing.T) {
		// --- Given ---
		s := &struct{ F *TTextPtr }{}
		dst := reflectkit.GetValue(t, s, "F")
		// Must instantiate new instance of the dst type and
		// call reflect.Value.Elem before calling useUnmarshal.
		dst.Set(reflect.New(dst.Type().Elem()))
		dst = dst.Elem()

		// --- When ---
		have, err := useUnmarshal(dst, "1")

		// --- Then ---
		assert.NoError(t, err)
		assert.True(t, have)
		assert.Equal(t, new(TTextPtr(1)), s.F)
	})

	t.Run("unmarshal time", func(t *testing.T) {
		// --- Given ---
		s := &struct{ F time.Time }{}
		dst := reflectkit.GetValue(t, s, "F")

		// --- When ---
		have, err := useUnmarshal(dst, "2000-01-02T03:04:05+01:00")

		// --- Then ---
		assert.NoError(t, err)
		assert.False(t, have)
		assert.Zero(t, s.F)
	})

	t.Run("error - many values", func(t *testing.T) {
		// --- Given ---
		s := &struct{ F TTextPtr }{}

		// --- When ---
		have, err := useUnmarshal(reflectkit.GetValue(t, s, "F"), "1", "2")

		// --- Then ---
		assert.ErrorEqual(t, "require single value: received 2", err)
		assert.True(t, have)
		assert.Equal(t, TTextPtr(0), s.F)
	})
}

func Test_setDst(t *testing.T) {
	t.Run("set using decoder", func(t *testing.T) {
		// --- Given ---
		s := &struct{ F int }{}

		// --- When ---
		have, err := setDst(reflectkit.GetValue(t, s, "F"), nil, "1")

		// --- Then ---
		assert.NoError(t, err)
		assert.True(t, have)
		assert.Equal(t, 1, s.F)
	})

	t.Run("error - decoder", func(t *testing.T) {
		// --- Given ---
		s := &struct{ F byte }{}

		// --- When ---
		have, err := setDst(reflectkit.GetValue(t, s, "F"), nil, "256")

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
		have, err := setDst(reflectkit.GetValue(t, s, "F"), nil, "7F")

		// --- Then ---
		assert.NoError(t, err)
		assert.True(t, have)
		assert.Equal(t, TTextPtr(127), s.F)
	})

	t.Run("error - Unmarshaler", func(t *testing.T) {
		// --- Given ---
		s := &struct{ F TTextPtr }{}

		// --- When ---
		have, err := setDst(reflectkit.GetValue(t, s, "F"), nil, "GG")

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
		have, err := setDst(reflectkit.GetValue(t, s, "F"), nil, "1")

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
		have, err := setDst(reflectkit.GetValue(t, s, "F"), nil, "1")

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

		// --- When ---
		have, err := setDst(reflectkit.GetValue(t, s, "F"), reg, "a", "b")

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
		have, err := setDst(reflectkit.GetValue(t, s, "F"), reg, "1")

		// --- Then ---
		want := "expected decoder to return `int` got `string`"
		assert.ErrorEqual(t, want, err)
		assert.True(t, have)
		assert.Equal(t, 0, s.F)
	})
}

func Test_setSliceOrArray(t *testing.T) {
	t.Run("slice", func(t *testing.T) {
		// --- Given ---
		s := &struct{ F []string }{F: make([]string, 0)}
		dst := mirror.NewStructValue(s).FieldByName("F").Value()

		// --- When ---
		err := setSliceOrArray(dst, nil, "a", "b")

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
		err := setSliceOrArray(dst, nil, "a")

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
		err := setSliceOrArray(dst, nil, "1")

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, [3]int{1, 0, 0}, s.F)
	})

	t.Run("nil slice", func(t *testing.T) {
		// --- Given ---
		s := &struct{ F []string }{}
		dst := mirror.NewStructValue(s).FieldByName("F").Value()

		// --- When ---
		err := setSliceOrArray(dst, nil, "a", "b")

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, []string{"a", "b"}, s.F)
	})

	t.Run("slice of pointers", func(t *testing.T) {
		// --- Given ---
		s := &struct{ F []*string }{F: make([]*string, 0)}
		dst := mirror.NewStructValue(s).FieldByName("F").Value()

		// --- When ---
		err := setSliceOrArray(dst, nil, "a", "b")

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, []*string{new("a"), new("b")}, s.F)
	})

	t.Run("nil slice of pointers", func(t *testing.T) {
		// --- Given ---
		s := &struct{ F []*string }{}
		dst := mirror.NewStructValue(s).FieldByName("F").Value()

		// --- When ---
		err := setSliceOrArray(dst, nil, "a", "b")

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
		err := setSliceOrArray(dst, reg, "a", "b")

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
		err := setSliceOrArray(dst, nil, "a")

		// --- Then ---
		assert.ErrorEqual(t, "decoder not found for `xbind.ct`", err)
		xrrtest.AssertCode(t, ECDecNotFound, err)
	})

	t.Run("error - decoder", func(t *testing.T) {
		// --- Given ---
		s := &struct{ F []TTextPtr }{}
		dst := mirror.NewStructValue(s).FieldByName("F").Value()

		// --- When ---
		err := setSliceOrArray(dst, nil, "GG")

		// --- Then ---
		assert.ErrorEqual(t, "invalid value: cannot convert `GG` to byte", err)
		xrrtest.AssertCode(t, ECInvValue, err)
	})

	t.Run("array", func(t *testing.T) {
		// --- Given ---
		s := &struct{ F [2]string }{}
		dst := mirror.NewStructValue(s).FieldByName("F").Value()

		// --- When ---
		err := setSliceOrArray(dst, nil, "a", "b")

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, [2]string{"a", "b"}, s.F)
	})

	t.Run("array fewer values than elements", func(t *testing.T) {
		// --- Given ---
		s := &struct{ F [2]string }{}
		dst := mirror.NewStructValue(s).FieldByName("F").Value()

		// --- When ---
		err := setSliceOrArray(dst, nil, "a")

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, [2]string{"a", ""}, s.F)
	})

	t.Run("error - array more values than elements", func(t *testing.T) {
		// --- Given ---
		s := &struct{ F [1]string }{}
		dst := mirror.NewStructValue(s).FieldByName("F").Value()

		// --- When ---
		err := setSliceOrArray(dst, nil, "a", "b")

		// --- Then ---
		assert.ErrorIs(t, ErrInvValue, err)
		assert.ErrorEqual(t, "invalid value: too many values for an array", err)
		xrrtest.AssertCode(t, ECInvValue, err)
	})
}

func Test_decKey(t *testing.T) {
	t.Run("value key", func(t *testing.T) {
		// --- When ---
		have, err := decKey(reflect.TypeFor[int](), nil, "42")

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, 42, have.Interface())
	})

	t.Run("pointer key", func(t *testing.T) {
		// --- When ---
		have, err := decKey(reflect.TypeFor[*int](), nil, "42")

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, 42, *have.Interface().(*int))
	})

	t.Run("error - key", func(t *testing.T) {
		// --- When ---
		have, err := decKey(reflect.TypeFor[int](), nil, "x")

		// --- Then ---
		assert.ErrorIs(t, ErrInvValue, err)
		assert.False(t, have.IsValid())
	})

	t.Run("error - no decoder", func(t *testing.T) {
		// --- When ---
		have, err := decKey(reflect.TypeFor[chan int](), nil, "x")

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
		have, err := deref(val)

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, reflect.Int, have.Kind())
		assert.NotNil(t, pp)
		assert.NotNil(t, *pp)
	})

	t.Run("error - unexported nil pointer", func(t *testing.T) {
		// --- Given ---
		val := reflect.ValueOf(TUnexpPtr{}).Field(1)

		// --- When ---
		have, err := deref(val)

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
