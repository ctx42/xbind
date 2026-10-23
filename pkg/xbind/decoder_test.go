// SPDX-FileCopyrightText: (c) 2026 Rafal Zajac
// SPDX-License-Identifier: MIT

package xbind

import (
	"encoding"
	"errors"
	"io"
	"net/url"
	"reflect"
	"slices"
	"strconv"
	"strings"
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

	t.Run("field names overlap and tag name overlap", func(t *testing.T) {
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
		assert.Equal(t, "abc", dst.TRequired.FStr)
		assert.Equal(t, 123, dst.TRequired.FInt)
		assert.Equal(t, "", dst.FStr)
		assert.Exact(t, "2000-01-02T03:04:05Z", dst.FTime)
		assert.Equal(t, 0, dst.FInt)
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
		}{S: []string{"default"}, A: [2]int{7, 8}}
		src := url.Values{"s": {"x"}, "a": {"1"}}

		// --- When ---
		err := NewDecoder().Decode(dst, src)

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, []string{"default", "x"}, dst.S)
		assert.Equal(t, [2]int{1, 8}, dst.A)
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
	f.Add("next.s", "x", "next.next.p", "1", false)

	f.Fuzz(func(t *testing.T, k0, v0, k1, v1 string, unknown bool) {
		// --- Given ---
		dec := NewDecoder()
		if unknown {
			dec = NewDecoder(WithDecAllowUnknown())
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
		enc := url.Values{}
		assert.NoError(t, NewEncoder().Encode(enc, dst))

		again := &TFuzzDec{}
		assert.NoError(t, NewDecoder().Decode(again, enc))

		have := url.Values{}
		assert.NoError(t, NewEncoder().Encode(have, again))
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

func Test_Decoder_resolve(t *testing.T) {
	t.Run("field", func(t *testing.T) {
		// --- Given ---
		pth := []string{"f_str"}
		s := &TString{}

		// --- When ---
		have := NewDecoder().resolve("", pth, mirror.NewStructValue(s), nil)

		// --- Then ---
		assert.False(t, have.isZero())
		assert.Equal(t, "f_str", have.pth)

		have.dst.NewIfNil().Value().Set(reflect.ValueOf("test"))
		assert.Equal(t, "test", s.FStr)
	})

	t.Run("pointer field", func(t *testing.T) {
		// --- Given ---
		pth := []string{"f_p_str"}
		s := &TString{}

		// --- When ---
		have := NewDecoder().resolve("", pth, mirror.NewStructValue(s), nil)

		// --- Then ---
		assert.False(t, have.isZero())
		tag := must.Value(have.dst.Tag("url"))
		assert.Equal(t, "f_p_str", tag.NameOrField())
		assert.Equal(t, "f_p_str", have.pth)

		_ = must.Value(setDst(have.dst.NewIfNil().Value(), nil, "test"))
		assert.Equal(t, "test", *s.FpStr)
	})

	t.Run("slice field", func(t *testing.T) {
		// --- Given ---
		pth := []string{"f_s_str"}
		s := &TString{}

		// --- When ---
		have := NewDecoder().resolve("", pth, mirror.NewStructValue(s), nil)

		// --- Then ---
		assert.False(t, have.isZero())
		tag := must.Value(have.dst.Tag("url"))
		assert.Equal(t, "f_s_str", tag.NameOrField())
		assert.Equal(t, "f_s_str", have.pth)

		var items []reflect.Value
		items = append(items, reflect.ValueOf("a"), reflect.ValueOf("b"))
		fld := have.dst.NewIfNil().Value()
		fld.Set(reflect.Append(fld, items...))
		assert.Equal(t, []string{"a", "b"}, s.FsStr)
	})

	t.Run("slice of pointers field", func(t *testing.T) {
		// --- Given ---
		pth := []string{"f_s_p_str"}
		s := &TString{}

		// --- When ---
		have := NewDecoder().resolve("", pth, mirror.NewStructValue(s), nil)

		// --- Then ---
		assert.False(t, have.isZero())
		tag := must.Value(have.dst.Tag("url"))
		assert.Equal(t, "f_s_p_str", tag.NameOrField())
		assert.Equal(t, "f_s_p_str", have.pth)

		var items []reflect.Value
		idx0, idx1 := "a", "b"
		items = append(items, reflect.ValueOf(&idx0), reflect.ValueOf(&idx1))
		fld := have.dst.NewIfNil().Value()
		fld.Set(reflect.Append(fld, items...))
		assert.Equal(t, []*string{&idx0, &idx1}, s.FspStr)
	})

	t.Run("map field", func(t *testing.T) {
		// --- Given ---
		pth := []string{"f_m_int_str", "4", "val"}
		s := &TString{}

		// --- When ---
		have := NewDecoder().resolve("", pth, mirror.NewStructValue(s), nil)

		// --- Then ---
		assert.False(t, have.isZero())
		tag := must.Value(have.dst.Tag("url"))
		assert.Equal(t, "f_m_int_str", tag.NameOrField())
		assert.Equal(t, "f_m_int_str", have.pth)

		fld := have.dst.NewIfNil().Value()
		fld.SetMapIndex(reflect.ValueOf(4), reflect.ValueOf("tst"))
		assert.HasKey(t, 4, s.FmStr)
		assert.Equal(t, "tst", s.FmStr[4])
	})

	t.Run("array field", func(t *testing.T) {
		// --- Given ---
		pth := []string{"f_a_str"}
		s := &TString{}

		// --- When ---
		have := NewDecoder().resolve("", pth, mirror.NewStructValue(s), nil)

		// --- Then ---
		assert.False(t, have.isZero())
		tag := must.Value(have.dst.Tag("url"))
		assert.Equal(t, "f_a_str", tag.NameOrField())
		assert.Equal(t, "f_a_str", have.pth)

		have.dst.NewIfNil().Value().Index(0).Set(reflect.ValueOf("a"))
		have.dst.NewIfNil().Value().Index(1).Set(reflect.ValueOf("b"))
		assert.Equal(t, [...]string{"a", "b"}, s.FaStr)
	})

	t.Run("array of pointers field", func(t *testing.T) {
		// --- Given ---
		pth := []string{"f_a_p_str"}
		s := &TString{}

		// --- When ---
		have := NewDecoder().resolve("", pth, mirror.NewStructValue(s), nil)

		// --- Then ---
		assert.False(t, have.isZero())
		tag := must.Value(have.dst.Tag("url"))
		assert.Equal(t, "f_a_p_str", tag.NameOrField())
		assert.Equal(t, "f_a_p_str", have.pth)

		idx0, idx1 := "a", "b"
		have.dst.NewIfNil().Value().Index(0).Set(reflect.ValueOf(&idx0))
		have.dst.NewIfNil().Value().Index(1).Set(reflect.ValueOf(&idx1))
		assert.Equal(t, [...]*string{&idx0, &idx1}, s.FapStr)
	})

	t.Run("map sub-struct field", func(t *testing.T) {
		// --- Given ---
		pth := []string{"sub0", "f_m_int_str", "4", "val"}
		s := &struct {
			Sub0 TString  `url:"sub0"`
			Sub1 *TString `url:"sub1"`
		}{}

		// --- When ---
		have := NewDecoder().resolve("", pth, mirror.NewStructValue(s), nil)

		// --- Then ---
		assert.False(t, have.isZero())
		tag := must.Value(have.dst.Tag("url"))
		assert.Equal(t, "f_m_int_str", tag.NameOrField())
		assert.Equal(t, "sub0.f_m_int_str", have.pth)

		fld := have.dst.NewIfNil().Value()
		fld.SetMapIndex(reflect.ValueOf(4), reflect.ValueOf("tst"))
		assert.HasKey(t, 4, s.Sub0.FmStr)
		assert.Equal(t, "tst", s.Sub0.FmStr[4])
	})

	t.Run("nested struct value", func(t *testing.T) {
		// --- Given ---
		pth := []string{"sub0", "f_str"}
		s := &struct {
			Sub0 TString  `url:"sub0"`
			Sub1 *TString `url:"sub1"`
		}{}

		// --- When ---
		have := NewDecoder().resolve("", pth, mirror.NewStructValue(s), nil)

		// --- Then ---
		assert.False(t, have.isZero())
		assert.Equal(t, "f_str", must.Value(have.dst.Tag("url")).NameOrField())
		assert.Equal(t, "sub0.f_str", have.pth)

		have.dst.NewIfNil().Value().Set(reflect.ValueOf("test"))
		assert.Equal(t, "test", s.Sub0.FStr)
	})

	t.Run("nested struct pointer", func(t *testing.T) {
		// --- Given ---
		pth := []string{"sub1", "f_str"}
		s := &struct {
			Sub0 TString  `url:"sub0"`
			Sub1 *TString `url:"sub1"`
		}{}

		// --- When ---
		have := NewDecoder().resolve("", pth, mirror.NewStructValue(s), nil)

		// --- Then ---
		assert.False(t, have.isZero())
		assert.Equal(t, "f_str", must.Value(have.dst.Tag("url")).NameOrField())
		assert.Equal(t, "sub1.f_str", have.pth)

		have.dst.NewIfNil().Value().Set(reflect.ValueOf("test"))
		assert.Equal(t, "test", s.Sub1.FStr)
	})

	t.Run("slice of structs", func(t *testing.T) {
		// --- Given ---
		pth := []string{"sub0", "f_str"}
		s := &struct {
			FsS []TString `url:"sub0"`
		}{}

		// --- When ---
		have := NewDecoder().resolve("", pth, mirror.NewStructValue(s), nil)

		// --- Then ---
		assert.False(t, have.isZero())
		assert.Equal(t, "sub0", must.Value(have.dst.Tag("url")).NameOrField())
		assert.Equal(t, "sub0", have.pth)
		assert.Equal(t, []string{"f_str"}, have.remainder)
		assert.Nil(t, s.FsS)
	})

	t.Run("slice of struct pointers", func(t *testing.T) {
		// --- Given ---
		pth := []string{"sub0", "f_str"}
		s := &struct {
			Sub0 []*TString `url:"sub0"`
		}{}

		// --- When ---
		have := NewDecoder().resolve("", pth, mirror.NewStructValue(s), nil)

		// --- Then ---
		assert.False(t, have.isZero())
		assert.Equal(t, "sub0", must.Value(have.dst.Tag("url")).NameOrField())
		assert.Equal(t, "sub0", have.pth)
		assert.Equal(t, []string{"f_str"}, have.remainder)
		assert.Nil(t, s.Sub0)
	})

	t.Run("array of structs", func(t *testing.T) {
		// --- Given ---
		pth := []string{"sub0", "f_str"}
		s := &struct {
			Sub0 [2]TString `url:"sub0"`
		}{}

		// --- When ---
		have := NewDecoder().resolve("", pth, mirror.NewStructValue(s), nil)

		// --- Then ---
		assert.False(t, have.isZero())
		assert.Equal(t, "sub0", must.Value(have.dst.Tag("url")).NameOrField())
		assert.Equal(t, "sub0", have.pth)
		assert.Equal(t, []string{"f_str"}, have.remainder)
	})

	t.Run("array of struct pointers", func(t *testing.T) {
		// --- Given ---
		pth := []string{"sub0", "f_str"}
		s := &struct {
			Sub0 [2]*TString `url:"sub0"`
		}{}

		// --- When ---
		have := NewDecoder().resolve("", pth, mirror.NewStructValue(s), nil)

		// --- Then ---
		assert.False(t, have.isZero())
		assert.Equal(t, "sub0", must.Value(have.dst.Tag("url")).NameOrField())
		assert.Equal(t, "sub0", have.pth)
		assert.Equal(t, []string{"f_str"}, have.remainder)
		assert.Len(t, 2, s.Sub0)
		assert.Nil(t, s.Sub0[0])
		assert.Nil(t, s.Sub0[1])
	})

	t.Run("unexported nil embedded pointer", func(t *testing.T) {
		// --- Given ---
		pth := []string{"f_str"}
		s := &TUnexpNode{}

		// --- When ---
		have := NewDecoder().resolve("", pth, mirror.NewStructValue(s), nil)

		// --- Then ---
		assert.True(t, have.unexported)
		assert.Equal(t, "f_str", have.pth)
		assert.Nil(t, have.dst)
		assert.Nil(t, s.tStr)
	})

	t.Run("unknown path with unexported pointer", func(t *testing.T) {
		// --- Given ---
		pth := []string{"zzz"}
		s := &TUnexpNode{}

		// --- When ---
		have := NewDecoder().resolve("", pth, mirror.NewStructValue(s), nil)

		// --- Then ---
		assert.True(t, have.isZero())
		assert.False(t, have.unexported)
	})
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
				map[ptrKey]bool{},
				setErr,
			)

			// --- Then ---
			slices.Sort(have)
			assert.Equal(t, tc.want, have)
		})
	}
}

func Test_Decoder_isLeaf_tabular(t *testing.T) {
	tt := []struct {
		testN string

		val  reflect.Value
		want bool
	}{
		{"registered", reflect.ValueOf(TPoint{}), true},
		{"pointer to registered", reflect.ValueOf(&TPoint{}), true},
		{"unmarshaler", reflect.ValueOf(&TItf{}), true},
		{"addressable unmarshaler", reflect.ValueOf(&TItf{}).Elem(), true},
		{"not addressable unmarshaler", reflect.ValueOf(TItf{}), false},
		{"struct", reflect.ValueOf(TString{}), false},
	}

	for _, tc := range tt {
		t.Run(tc.testN, func(t *testing.T) {
			// --- When ---
			have := NewDecoder(WithDecFunc(decPoint)).isLeaf(tc.val)

			// --- Then ---
			assert.Equal(t, tc.want, have)
		})
	}
}

func Test_result_isZero(t *testing.T) {
	t.Run("zero", func(t *testing.T) {
		// --- Given ---
		res := result{}

		// --- When ---
		have := res.isZero()

		// --- Then ---
		assert.True(t, have)
	})

	t.Run("resolved", func(t *testing.T) {
		// --- Given ---
		fld := mirror.NewStructValue(&TString{}).FieldByName("FStr")
		res := result{dst: fld, pth: "f_str"}

		// --- When ---
		have := res.isZero()

		// --- Then ---
		assert.False(t, have)
	})

	t.Run("unexported", func(t *testing.T) {
		// --- Given ---
		res := result{pth: "f_str", unexported: true}

		// --- When ---
		have := res.isZero()

		// --- Then ---
		assert.False(t, have)
	})
}

func Test_result_isComplete_tabular(t *testing.T) {
	sv := mirror.NewStructValue(&TString{})
	mapFld := sv.FieldByName("FmStr")
	strFld := sv.FieldByName("FStr")

	tt := []struct {
		testN string

		res  result
		want bool
	}{
		{"zero result", result{}, false},
		{"unexported", result{pth: "f_str", unexported: true}, false},
		{
			"map field with remainder",
			result{dst: mapFld, remainder: []string{"1"}},
			true,
		},
		{"map field without remainder", result{dst: mapFld}, false},
		{
			"map field with empty key",
			result{dst: mapFld, remainder: []string{""}},
			false,
		},
		{
			"non-map field with remainder",
			result{dst: strFld, remainder: []string{"junk"}},
			false,
		},
		{"non-map field without remainder", result{dst: strFld}, true},
	}

	for _, tc := range tt {
		t.Run(tc.testN, func(t *testing.T) {
			// --- When ---
			have := tc.res.isComplete()

			// --- Then ---
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

func Test_setMap(t *testing.T) {
	t.Run("existing map", func(t *testing.T) {
		// --- Given ---
		s := &struct{ F map[int]string }{F: make(map[int]string)}
		dst := mirror.NewStructValue(s).FieldByName("F").Value()

		// --- When ---
		err := setMap(dst, nil, "1", "a")

		// --- Then ---
		assert.NoError(t, err)
		assert.HasKeyValue(t, 1, "a", s.F)
	})

	t.Run("allocated nil map", func(t *testing.T) {
		// --- Given ---
		s := &struct{ F map[int]string }{}
		dst := mirror.NewStructValue(s).FieldByName("F").NewIfNil().Value()

		// --- When ---
		err := setMap(dst, nil, "1", "a")

		// --- Then ---
		assert.NoError(t, err)
		assert.HasKeyValue(t, 1, "a", s.F)
	})

	t.Run("pointer map key", func(t *testing.T) {
		// --- Given ---
		s := &struct{ F map[*int]string }{}
		dst := mirror.NewStructValue(s).FieldByName("F").NewIfNil().Value()

		// --- When ---
		err := setMap(dst, nil, "1", "a")

		// --- Then ---
		assert.NoError(t, err)
		var k *int
		var v string
		for k, v = range s.F {
		}
		assert.Equal(t, 1, *k)
		assert.Equal(t, "a", v)
	})

	t.Run("pointer map value", func(t *testing.T) {
		// --- Given ---
		s := &struct{ F map[int]*string }{}
		dst := mirror.NewStructValue(s).FieldByName("F").NewIfNil().Value()

		// --- When ---
		err := setMap(dst, nil, "1", "a")

		// --- Then ---
		assert.NoError(t, err)
		var k int
		var v *string
		for k, v = range s.F {
		}
		assert.Equal(t, 1, k)
		assert.Equal(t, "a", *v)
	})

	t.Run("custom decoder used for map key", func(t *testing.T) {
		// --- Given ---
		s := &struct{ F map[int]string }{}
		dst := mirror.NewStructValue(s).FieldByName("F").NewIfNil().Value()
		reg := decReg{
			reflect.TypeOf(1): func(vls ...string) (any, error) {
				return 123, nil
			},
		}

		// --- When ---
		err := setMap(dst, reg, "1", "a")

		// --- Then ---
		assert.NoError(t, err)
		assert.HasKeyValue(t, 123, "a", s.F)
	})

	t.Run("custom decoder used for map value", func(t *testing.T) {
		// --- Given ---
		s := &struct{ F map[int]string }{}
		dst := mirror.NewStructValue(s).FieldByName("F").NewIfNil().Value()
		reg := decReg{
			reflect.TypeOf(""): func(vls ...string) (any, error) {
				return strings.ToUpper(vls[0]), nil
			},
		}

		// --- When ---
		err := setMap(dst, reg, "1", "a")

		// --- Then ---
		assert.NoError(t, err)
		assert.HasKeyValue(t, 1, "A", s.F)
	})

	t.Run("error - no decoder for map key", func(t *testing.T) {
		// --- Given ---
		type ct struct{}
		s := &struct{ F map[*ct]string }{}
		dst := mirror.NewStructValue(s).FieldByName("F").Value()

		// --- When ---
		err := setMap(dst, nil, "1", "a")

		// --- Then ---
		assert.ErrorEqual(t, "decoder not found for `*xbind.ct`", err)
		xrrtest.AssertCode(t, ECDecNotFound, err)
	})

	t.Run("error - no decoder for map value", func(t *testing.T) {
		// --- Given ---
		type ct struct{}
		s := &struct{ F map[string]ct }{}
		dst := mirror.NewStructValue(s).FieldByName("F").Value()

		// --- When ---
		err := setMap(dst, nil, "1", "a")

		// --- Then ---
		assert.ErrorEqual(t, "decoder not found for `xbind.ct`", err)
		xrrtest.AssertCode(t, ECDecNotFound, err)
	})

	t.Run("error - map key", func(t *testing.T) {
		// --- Given ---
		s := &struct{ F map[TTextPtr]string }{}
		dst := mirror.NewStructValue(s).FieldByName("F").Value()

		// --- When ---
		err := setMap(dst, nil, "GG", "a")

		// --- Then ---
		assert.ErrorEqual(t, "invalid value: cannot convert `GG` to byte", err)
		xrrtest.AssertCode(t, ECInvValue, err)
	})

	t.Run("error - map value", func(t *testing.T) {
		// --- Given ---
		s := &struct{ F map[string]TTextPtr }{}
		dst := mirror.NewStructValue(s).FieldByName("F").Value()

		// --- When ---
		err := setMap(dst, nil, "a", "GG")

		// --- Then ---
		assert.ErrorEqual(t, "invalid value: cannot convert `GG` to byte", err)
		xrrtest.AssertCode(t, ECInvValue, err)
	})
}

func Test_setField(t *testing.T) {
	t.Run("set slice", func(t *testing.T) {
		// --- Given ---
		s := &struct{ F []string }{}
		dst := mirror.NewStructValue(s).FieldByName("F").Value()

		// --- When ---
		err := setField(dst, nil, "", "a", "b")

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, []string{"a", "b"}, s.F)
	})

	t.Run("set array", func(t *testing.T) {
		// --- Given ---
		s := &struct{ F [3]string }{}
		dst := mirror.NewStructValue(s).FieldByName("F").Value()

		// --- When ---
		err := setField(dst, nil, "", "a", "b")

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, [3]string{"a", "b", ""}, s.F)
	})

	t.Run("set map", func(t *testing.T) {
		// --- Given ---
		s := &struct{ F map[int]string }{}
		dst := mirror.NewStructValue(s).FieldByName("F").NewIfNil().Value()

		// --- When ---
		err := setField(dst, nil, "1", "abc")

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, map[int]string{1: "abc"}, s.F)
	})

	t.Run("set field", func(t *testing.T) {
		// --- Given ---
		s := &struct{ F string }{}
		dst := mirror.NewStructValue(s).FieldByName("F").Value()

		// --- When ---
		err := setField(dst, nil, "", "abc")

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, "abc", s.F)
	})

	t.Run("custom decoder used", func(t *testing.T) {
		// --- Given ---
		s := &struct{ F string }{}
		dst := mirror.NewStructValue(s).FieldByName("F").Value()
		reg := decReg{
			reflect.TypeOf(""): func(vls ...string) (any, error) {
				return strings.ToUpper(vls[0]), nil
			},
		}

		// --- When ---
		err := setField(dst, reg, "", "abc")

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, "ABC", s.F)
	})

	t.Run("error - custom decoder", func(t *testing.T) {
		// --- Given ---
		s := &struct{ F string }{}
		dst := mirror.NewStructValue(s).FieldByName("F").Value()
		reg := decReg{
			reflect.TypeOf(""): func(vls ...string) (any, error) {
				return "", errors.New("test msg")
			},
		}

		// --- When ---
		err := setField(dst, reg, "", "abc")

		// --- Then ---
		assert.ErrorEqual(t, "test msg", err)
		assert.Equal(t, "", s.F)
	})

	t.Run("error - no decoder", func(t *testing.T) {
		// --- Given ---
		type ct struct{}
		s := &struct{ F ct }{}
		dst := mirror.NewStructValue(s).FieldByName("F").Value()

		// --- When ---
		err := setField(dst, nil, "", "a")

		// --- Then ---
		assert.ErrorEqual(t, "decoder not found for `xbind.ct`", err)
		xrrtest.AssertCode(t, ECDecNotFound, err)
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
