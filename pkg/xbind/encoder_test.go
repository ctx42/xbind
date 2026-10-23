// SPDX-FileCopyrightText: (c) 2026 Rafal Zajac
// SPDX-License-Identifier: MIT

package xbind

import (
	"encoding"
	"errors"
	"fmt"
	"math"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ctx42/testing/pkg/assert"
	"github.com/ctx42/testkit/pkg/reflectkit"
	"github.com/ctx42/xrr/pkg/xrr/xrrtest"
)

func Test_WithEncTag(t *testing.T) {
	// --- Given ---
	enc := &Encoder{}

	// --- When ---
	WithEncTag("abc")(enc)

	// --- Then ---
	assert.Equal(t, "abc", enc.tag)
}

func Test_WithEncFunc(t *testing.T) {
	// --- Given ---
	enc := &Encoder{}

	// --- When ---
	WithEncFunc(encPoint)(enc)

	// --- Then ---
	fn := enc.reg[reflect.TypeFor[TPoint]()]
	have, err := fn(reflect.ValueOf(TPoint{X: 1, Y: 2}))
	assert.NoError(t, err)
	assert.Equal(t, "1,2", have)
}

func Test_NewEncoder(t *testing.T) {
	t.Run("defaults", func(t *testing.T) {
		// --- When ---
		have := NewEncoder()

		// --- Then ---
		assert.Equal(t, "url", have.tag)
		assert.Nil(t, have.reg)
	})

	t.Run("options called", func(t *testing.T) {
		// --- When ---
		have := NewEncoder(WithEncTag("abc"), WithEncFunc(encPoint))

		// --- Then ---
		assert.Equal(t, "abc", have.tag)
		assert.HasKey(t, reflect.TypeFor[TPoint](), have.reg)
	})
}

func Test_Encoder_Encode(t *testing.T) {
	t.Run("error - nil dst", func(t *testing.T) {
		// --- When ---
		err := NewEncoder().Encode(nil, &TString{})

		// --- Then ---
		assert.Same(t, ErrNilDst, err)
	})

	t.Run("struct", func(t *testing.T) {
		// --- Given ---
		dst := url.Values{}
		s := &TString{
			FStr:   "FStr",
			FsStr:  []string{"FsStr0", "FsStr1"},
			FpStr:  new("FpStr"),
			FspStr: []*string{new("FspStr0"), new("FspStr1")},
			FmStr:  map[int]string{1: "FmStr1", 3: "FmStr3"},
			FaStr:  [2]string{"FaStr0", "FaStr1"},
			FapStr: [2]*string{new("FapStr0"), new("FapStr1")},
		}

		// --- When ---
		err := NewEncoder().Encode(dst, s)

		// --- Then ---
		assert.NoError(t, err)
		want := url.Values{
			"f_a_p_str":      {"FapStr0", "FapStr1"},
			"f_a_str":        {"FaStr0", "FaStr1"},
			"f_m_int_str[1]": {"FmStr1"},
			"f_m_int_str[3]": {"FmStr3"},
			"f_p_str":        {"FpStr"},
			"f_s_p_str":      {"FspStr0", "FspStr1"},
			"f_s_str":        {"FsStr0", "FsStr1"},
			"f_str":          {"FStr"},
		}
		assert.Equal(t, want, dst)
	})

	t.Run("struct with anonymous field", func(t *testing.T) {
		// --- Given ---
		dst := url.Values{}
		s := &Anonymous{
			TWithTime: TWithTime{
				FTime: time.Date(2022, 3, 4, 5, 6, 7, 0, time.UTC),
			},
		}

		// --- When ---
		err := NewEncoder().Encode(dst, s)

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, "f_time=2022-03-04T05%3A06%3A07Z", dst.Encode())
	})

	t.Run("struct with anonymous pointer field", func(t *testing.T) {
		// --- Given ---
		dst := url.Values{}
		s := &AnonymousPtr{
			TWithTime: &TWithTime{
				FTime: time.Date(2022, 3, 4, 5, 6, 7, 0, time.UTC),
			},
		}

		// --- When ---
		err := NewEncoder().Encode(dst, s)

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, "f_time=2022-03-04T05%3A06%3A07Z", dst.Encode())
	})

	t.Run("struct with anonymous nil pointer field", func(t *testing.T) {
		// --- Given ---
		dst := url.Values{}
		s := &AnonymousPtr{}

		// --- When ---
		err := NewEncoder().Encode(dst, s)

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, "", dst.Encode())
	})

	t.Run("struct zero values", func(t *testing.T) {
		// --- Given ---
		dst := url.Values{}
		s := &TString{}

		// --- When ---
		err := NewEncoder().Encode(dst, s)

		// --- Then ---
		assert.NoError(t, err)
		want := "f_a_str=&f_a_str=&f_str="
		assert.Equal(t, want, dst.Encode())
	})

	t.Run("struct with type aliases", func(t *testing.T) {
		// --- Given ---
		dst := url.Values{}
		s := &TAliases{
			FIntAlias:  1,
			FsIntAlias: []IntAlias{2, 3},
			FFltAlias:  4.1,
			FsFltAlias: []FltAlias{5.1, 5.2},
			FStrAlias:  "FStrAlias",
			FsStrAlias: []StrAlias{"FsStrAlias0", "FsStrAlias1"},
		}

		// --- When ---
		err := NewEncoder().Encode(dst, s)

		// --- Then ---
		assert.NoError(t, err)
		want := "" +
			"f_flt_alias=4.1&" +
			"f_int_alias=1&" +
			"f_s_flt_alias=5.1&" +
			"f_s_flt_alias=5.2&" +
			"f_s_int_alias=2&" +
			"f_s_int_alias=3&" +
			"f_s_str_alias=FsStrAlias0&" +
			"f_s_str_alias=FsStrAlias1&" +
			"f_str_alias=FStrAlias"
		assert.Equal(t, want, dst.Encode())
	})

	t.Run("struct with type aliases zero values", func(t *testing.T) {
		// --- Given ---
		dst := url.Values{}
		s := &TAliases{}

		// --- When ---
		err := NewEncoder().Encode(dst, s)

		// --- Then ---
		assert.NoError(t, err)
		want := "" +
			"f_flt_alias=0&" +
			"f_int_alias=0&" +
			"f_str_alias="
		assert.Equal(t, want, dst.Encode())
	})

	t.Run("struct no field aliases", func(t *testing.T) {
		// --- Given ---
		dst := url.Values{}
		s := &struct {
			FStr  string
			FsStr []string
		}{
			FStr:  "FStr",
			FsStr: []string{"abc"},
		}

		// --- When ---
		err := NewEncoder().Encode(dst, s)

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, "FStr=FStr&FsStr=abc", dst.Encode())
	})

	t.Run("skipped fields", func(t *testing.T) {
		// --- Given ---
		dst := url.Values{}
		s := &struct {
			FStr1 string
			FStr2 string `url:"-"`
		}{
			FStr1: "FStr1",
			FStr2: "FStr2",
		}

		// --- When ---
		err := NewEncoder().Encode(dst, s)

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, "FStr1=FStr1", dst.Encode())
	})

	t.Run("not exported fields", func(t *testing.T) {
		// --- Given ---
		dst := url.Values{}
		s := &struct {
			FStr1 string
			fStr2 string
			fStr3 string `url:"-"`
		}{
			FStr1: "FStr1",
			fStr2: "FStr2",
			fStr3: "FStr3",
		}

		// --- When ---
		err := NewEncoder().Encode(dst, s)

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, "FStr1=FStr1", dst.Encode())
	})

	t.Run("custom tag", func(t *testing.T) {
		// --- Given ---
		dst := url.Values{}
		s := &struct {
			FStr string `custom:"f_str"`
		}{
			FStr: "FStr",
		}

		// --- When ---
		err := NewEncoder(WithEncTag("custom")).Encode(dst, s)

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, "f_str=FStr", dst.Encode())
	})

	t.Run("custom marshaler pointer receiver", func(t *testing.T) {
		// --- Given ---
		dst := url.Values{}
		s := &TStructMar{
			fStr:  "fStr",
			fInt:  10,
			fsInt: []int{0, 1},
		}

		// --- When ---
		err := NewEncoder().Encode(dst, s)

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, "f_int=10&f_s_int=0&f_s_int=1&f_str=fStr", dst.Encode())
	})

	t.Run("embedded custom marshaler pointer receiver", func(t *testing.T) {
		// --- Given ---
		dst := url.Values{}
		s := &struct {
			*TStructMar
			FStr string
		}{
			TStructMar: &TStructMar{
				fStr:  "abc",
				fInt:  10,
				fsInt: []int{1, 2},
			},
			FStr: "xyz",
		}

		// --- When ---
		err := NewEncoder().Encode(dst, s)

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, "f_int=10&f_s_int=1&f_s_int=2&f_str=abc", dst.Encode())
	})

	t.Run("embedded custom marshaler", func(t *testing.T) {
		// --- Given ---
		dst := url.Values{}
		s := &struct {
			TMarValVal
			FStr string
		}{
			TMarValVal: TMarValVal{"abc"},
			FStr:       "xyz",
		}

		// --- When ---
		err := NewEncoder().Encode(dst, s)

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, "f_str=mar_abc", dst.Encode())
	})

	t.Run("embedded multi structs", func(t *testing.T) {
		// --- Given ---
		dst := url.Values{}
		s := &TMultiEmbedded{
			TString: TString{
				FStr:  "ab",
				FsStr: []string{"a", "b"},
			},
			TAliases: TAliases{
				FIntAlias:  1,
				FsIntAlias: []IntAlias{1, 0},
			},
		}

		// --- When ---
		err := NewEncoder().Encode(dst, s)

		// --- Then ---
		assert.NoError(t, err)
		want := "" +
			"f_a_str=&" +
			"f_a_str=&" +
			"f_flt_alias=0&" +
			"f_int_alias=1&" +
			"f_s_int_alias=1&" +
			"f_s_int_alias=0&" +
			"f_s_str=a&" +
			"f_s_str=b&" +
			"f_str=ab&" +
			"f_str_alias="
		assert.Equal(t, want, dst.Encode())
	})

	t.Run("struct field with marshaler", func(t *testing.T) {
		// --- Given ---
		dst := url.Values{}
		s := &struct {
			FMar TAliasMar `url:"f_mar"`
			FStr string
		}{
			FMar: TAliasMar("abc"),
			FStr: "xyz",
		}

		// --- When ---
		err := NewEncoder().Encode(dst, s)

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, "FStr=xyz&f_mar=mar_abc", dst.Encode())
	})

	t.Run("slice with elements implementing marshaler", func(t *testing.T) {
		// --- Given ---
		dst := url.Values{}
		s := &struct {
			FMar []*TMarValPtr `url:"f_mar"`
			FStr string
		}{
			FMar: []*TMarValPtr{{"a"}, {"b"}},
			FStr: "xyz",
		}

		// --- When ---
		err := NewEncoder().Encode(dst, s)

		// --- Then ---
		assert.NoError(t, err)
		want := url.Values{
			"FStr":           {"xyz"},
			"f_mar[0].f_str": {"mar_a"},
			"f_mar[1].f_str": {"mar_b"},
		}
		assert.Equal(t, want, dst)
	})

	t.Run("map with elements implementing marshaler", func(t *testing.T) {
		// --- Given ---
		dst := url.Values{}
		s := &struct {
			FMar map[string]*TMarValPtr `url:"f_mar"`
			FStr string
		}{
			FMar: map[string]*TMarValPtr{"A": {"a"}, "B": {"b"}},
			FStr: "xyz",
		}

		// --- When ---
		err := NewEncoder().Encode(dst, s)

		// --- Then ---
		assert.NoError(t, err)
		want := url.Values{
			"FStr":           {"xyz"},
			"f_mar[A].f_str": {"mar_a"},
			"f_mar[B].f_str": {"mar_b"},
		}
		assert.Equal(t, want, dst)
	})

	t.Run("omitempty", func(t *testing.T) {
		// --- Given ---
		dst := url.Values{}
		s := &struct {
			FStr1 string `url:",omitempty"`
			FStr2 string `url:",omitempty"`
		}{
			FStr1: "",
			FStr2: "abc",
		}

		// --- When ---
		err := NewEncoder().Encode(dst, s)

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, "FStr2=abc", dst.Encode())
	})

	t.Run("omitempty nil field with marshaler", func(t *testing.T) {
		// --- Given ---
		dst := url.Values{}
		s := &struct {
			FMar *TStructMar `url:",omitempty"`
			FStr string
		}{
			FStr: "abc",
		}

		// --- When ---
		err := NewEncoder().Encode(dst, s)

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, "FStr=abc", dst.Encode())
	})

	t.Run("nil field with marshaler", func(t *testing.T) {
		// --- Given ---
		dst := url.Values{}
		s := &struct {
			FMar *TStructMar
			FStr string
		}{
			FStr: "abc",
		}

		// --- When ---
		err := NewEncoder().Encode(dst, s)

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, "FStr=abc", dst.Encode())
	})

	t.Run("struct with time field", func(t *testing.T) {
		// --- Given ---
		dst := url.Values{}
		s := &TWithTime{
			FTime: time.Date(2022, 3, 4, 5, 6, 7, 0, time.UTC),
		}

		// --- When ---
		err := NewEncoder().Encode(dst, s)

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, "f_time=2022-03-04T05%3A06%3A07Z", dst.Encode())
	})

	t.Run("struct with Marshaler field", func(t *testing.T) {
		// --- Given ---
		dst := url.Values{}
		s := &TWithMarField{
			FStr: TMarValVal{fStr: "a"},
			FMar: &TMarValPtr{fStr: "b"},
		}

		// --- When ---
		err := NewEncoder().Encode(dst, s)

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, "f_mar.f_str=mar_a&f_p_mar.f_str=mar_b", dst.Encode())
	})

	t.Run("struct with slice of Marshaler", func(t *testing.T) {
		// --- Given ---
		dst := url.Values{}
		s := &TWithMarSlice{
			FsMar: []TMarValVal{
				{fStr: "a"},
				{fStr: "b"},
			},
			FspMar: []*TMarValPtr{
				{fStr: "c"},
				{fStr: "d"},
			},
		}

		// --- When ---
		err := NewEncoder().Encode(dst, s)

		// --- Then ---
		assert.NoError(t, err)
		want := url.Values{
			"f_s_mar[0].f_str":   {"mar_a"},
			"f_s_mar[1].f_str":   {"mar_b"},
			"f_s_p_mar[0].f_str": {"mar_c"},
			"f_s_p_mar[1].f_str": {"mar_d"},
		}
		assert.Equal(t, want, dst)
	})

	t.Run("struct with map of Marshaler", func(t *testing.T) {
		// --- Given ---
		dst := url.Values{}
		s := &TWithMarMap{
			FsMar: map[int]TMarValVal{
				1: {fStr: "a"},
				2: {fStr: "b"},
			},
			FspMar: map[int]*TMarValPtr{
				3: {fStr: "c"},
				4: {fStr: "d"},
			},
		}

		// --- When ---
		err := NewEncoder().Encode(dst, s)

		// --- Then ---
		assert.NoError(t, err)
		want := url.Values{
			"f_m_mar[1].f_str":   {"mar_a"},
			"f_m_mar[2].f_str":   {"mar_b"},
			"f_m_p_mar[3].f_str": {"mar_c"},
			"f_m_p_mar[4].f_str": {"mar_d"},
		}
		assert.Equal(t, want, dst)
	})

	t.Run("struct with TextMarshaler field", func(t *testing.T) {
		// --- Given ---
		dst := url.Values{}
		s := &struct{ F TTextPtr }{F: '*'}

		// --- When ---
		err := NewEncoder().Encode(dst, s)

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, url.Values{"F": {"*"}}, dst)
	})

	t.Run("array implementing MarshalValues", func(t *testing.T) {
		// --- Given ---
		dst := url.Values{}
		s := &struct{ F ArrImp }{F: [3]byte{'A', 'B', 'C'}}

		// --- When ---
		err := NewEncoder().Encode(dst, s)

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, "F=abc", dst.Encode())
	})

	t.Run("map implementing MarshalValues", func(t *testing.T) {
		// --- Given ---
		dst := url.Values{}
		s := &struct{ F MapImp }{F: map[string]string{"A": "a"}}

		// --- When ---
		err := NewEncoder().Encode(dst, s)

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, "F.AA=aa", dst.Encode())
	})

	t.Run("struct with not supported skipped field", func(t *testing.T) {
		// --- Given ---
		dst := url.Values{}
		s := &struct {
			F  func() `url:"-"`
			FF int
		}{
			F:  func() {},
			FF: 42,
		}

		// --- When ---
		err := NewEncoder().Encode(dst, s)

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, "FF=42", dst.Encode())
	})

	t.Run("struct value with pointer receiver marshaler", func(t *testing.T) {
		// --- Given ---
		dst := url.Values{}

		// --- When ---
		err := NewEncoder().Encode(dst, struct{ F TTextPtr }{F: '*'})

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, url.Values{"F": {"*"}}, dst)
	})

	t.Run("struct value with slice array and map fields", func(t *testing.T) {
		// --- Given ---
		dst := url.Values{}
		s := struct {
			S []string
			A [1]int
			M map[string]string
		}{
			S: []string{"a"},
			A: [1]int{1},
			M: map[string]string{"k": "v"},
		}

		// --- When ---
		err := NewEncoder().Encode(dst, s)

		// --- Then ---
		assert.NoError(t, err)
		want := url.Values{"A": {"1"}, "M[k]": {"v"}, "S": {"a"}}
		assert.Equal(t, want, dst)
	})

	t.Run("pointers to slice array and map fields", func(t *testing.T) {
		// --- Given ---
		dst := url.Values{}
		s := &struct {
			S  *[]string
			A  *[1]int
			M  *map[string]string
			NS *[]string
			NA *[1]int
			NM *map[string]string
		}{
			S: &[]string{"a"},
			A: &[1]int{1},
			M: &map[string]string{"k": "v"},
		}

		// --- When ---
		err := NewEncoder().Encode(dst, s)

		// --- Then ---
		assert.NoError(t, err)
		want := url.Values{"A": {"1"}, "M[k]": {"v"}, "S": {"a"}}
		assert.Equal(t, want, dst)
	})

	t.Run("omitempty struct with unexported fields", func(t *testing.T) {
		// --- Given ---
		type unexp struct{ val int }
		dst := url.Values{}
		s := &struct {
			F unexp `url:"f,omitempty"`
			I int   `url:"i"`
		}{F: unexp{val: 0}, I: 1}

		// --- When ---
		err := NewEncoder().Encode(dst, s)

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, "i=1", dst.Encode())
	})

	t.Run("nil interface fields implementing marshalers", func(t *testing.T) {
		// --- Given ---
		dst := url.Values{}
		s := &struct {
			M Marshaler
			T encoding.TextMarshaler
		}{}

		// --- When ---
		err := NewEncoder().Encode(dst, s)

		// --- Then ---
		assert.NoError(t, err)
		assert.Empty(t, dst)
	})

	t.Run("embedded struct skipped", func(t *testing.T) {
		// --- Given ---
		dst := url.Values{}
		s := &struct {
			TWithTime `url:"-"`
			FInt      int `url:"f_int"`
		}{
			TWithTime: TWithTime{FTime: time.Now()},
			FInt:      1,
		}

		// --- When ---
		err := NewEncoder().Encode(dst, s)

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, "f_int=1", dst.Encode())
	})

	t.Run("embedded struct omitempty", func(t *testing.T) {
		// --- Given ---
		dst := url.Values{}
		s := &struct {
			TWithTime `url:",omitempty"`
			FInt      int `url:"f_int"`
		}{FInt: 1}

		// --- When ---
		err := NewEncoder().Encode(dst, s)

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, "f_int=1", dst.Encode())
	})

	t.Run("embedded non-struct type", func(t *testing.T) {
		// --- Given ---
		dst := url.Values{}
		s := &struct{ IntAlias }{IntAlias: 5}

		// --- When ---
		err := NewEncoder().Encode(dst, s)

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, "IntAlias=5", dst.Encode())
	})

	t.Run("map with TextMarshaler keys", func(t *testing.T) {
		// --- Given ---
		dst := url.Values{}
		s := &struct{ M map[TTextVal]string }{M: map[TTextVal]string{'A': "x"}}

		// --- When ---
		err := NewEncoder().Encode(dst, s)

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, url.Values{"M[A]": {"x"}}, dst)
	})

	t.Run("map with slice values", func(t *testing.T) {
		// --- Given ---
		dst := url.Values{}
		s := &struct{ M map[string][]string }{
			M: map[string][]string{"a": {"x", "y"}},
		}

		// --- When ---
		err := NewEncoder().Encode(dst, s)

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, url.Values{"M[a]": {"x", "y"}}, dst)
	})

	t.Run("zero value encoder", func(t *testing.T) {
		// --- Given ---
		enc := &Encoder{}
		dst := url.Values{}
		s := &struct {
			A int `url:"a"`
			B int `url:"-"`
		}{A: 1, B: 2}

		// --- When ---
		err := enc.Encode(dst, s)

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, "a=1", dst.Encode())
	})

	t.Run("error - source not struct", func(t *testing.T) {
		// --- Given ---
		dst := url.Values{}

		// --- When ---
		err := NewEncoder().Encode(dst, make(map[int]int))

		// --- Then ---
		assert.Same(t, ErrReqStruct, err)
	})

	t.Run("error - top level marshaler", func(t *testing.T) {
		// --- Given ---
		dst := url.Values{}
		s := TMarValVal{fStr: "abcd"}

		// --- When ---
		err := NewEncoder().Encode(dst, s)

		// --- Then ---
		assert.ErrorEqual(t, "string too long", err)
	})

	t.Run("error - map key without encoder", func(t *testing.T) {
		// --- Given ---
		type ct struct{}
		dst := url.Values{}
		s := &struct {
			FMap map[ct]int
			FStr string
		}{
			FMap: map[ct]int{ct{}: 1},
			FStr: "abc",
		}

		// --- When ---
		err := NewEncoder().Encode(dst, s)

		// --- Then ---
		want := "FMap: encoder not found for `xbind.ct` (ECEncNotFound)"
		xrrtest.AssertFieldsEqual(t, want, err)
		assert.Equal(t, "", dst.Encode())
	})

	t.Run("error - not supported slice element", func(t *testing.T) {
		// --- Given ---
		dst := url.Values{}
		s := &struct {
			FMap []func()
			FStr string
		}{
			FMap: []func(){func() {}},
			FStr: "abc",
		}

		// --- When ---
		err := NewEncoder().Encode(dst, s)

		// --- Then ---
		want := "FMap[0]: encoder not found for `func()` (ECEncNotFound)"
		xrrtest.AssertEqual(t, want, err)
		assert.Equal(t, "", dst.Encode())
	})

	t.Run("error - not supported field", func(t *testing.T) {
		// --- Given ---
		dst := url.Values{}
		s := &struct {
			F  func()
			FF int
		}{
			F:  func() {},
			FF: 10,
		}

		// --- When ---
		err := NewEncoder().Encode(dst, s)

		// --- Then ---
		want := "F: encoder not found for `func()` (ECEncNotFound)"
		xrrtest.AssertEqual(t, want, err)
		assert.Equal(t, "", dst.Encode())
	})

	t.Run("error - pointer to not supported type", func(t *testing.T) {
		// --- Given ---
		dst := url.Values{}
		s := &struct{ F *chan int }{}

		// --- When ---
		err := NewEncoder().Encode(dst, s)

		// --- Then ---
		want := "F: encoder not found for `*chan int` (ECEncNotFound)"
		xrrtest.AssertEqual(t, want, err)
		assert.Equal(t, "", dst.Encode())
	})

	t.Run("error - embedded struct field", func(t *testing.T) {
		// --- Given ---
		type emb struct{ F func() }
		dst := url.Values{}
		s := &struct{ emb }{emb: emb{F: func() {}}}

		// --- When ---
		err := NewEncoder().Encode(dst, s)

		// --- Then ---
		want := "F: encoder not found for `func()` (ECEncNotFound)"
		xrrtest.AssertEqual(t, want, err)
		assert.Equal(t, "", dst.Encode())
	})

	t.Run("error - field path for missing encoder", func(t *testing.T) {
		// --- Given ---
		dst := url.Values{}
		s := &struct {
			S []chan int `url:"s"`
		}{S: []chan int{make(chan int)}}

		// --- When ---
		err := NewEncoder().Encode(dst, s)

		// --- Then ---
		want := "encoder not found for `chan int`"
		xrrtest.AssertFieldEqual(t, "s[0]", want, err)
	})

	t.Run("map with pointer receiver TextMarshalers", func(t *testing.T) {
		// --- Given ---
		dst := url.Values{}
		s := &struct {
			V map[string]TTextPtr
			K map[TTextPtr]int
		}{
			V: map[string]TTextPtr{"a": 'A'},
			K: map[TTextPtr]int{'B': 1},
		}

		// --- When ---
		err := NewEncoder().Encode(dst, s)

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, url.Values{"V[a]": {"A"}, "K[B]": {"1"}}, dst)
	})

	t.Run("embedded colliding unexported marshalers", func(t *testing.T) {
		// --- Given ---
		dst := url.Values{}
		s := &struct {
			tTextA
			tTextB
		}{tTextA{A: "a"}, tTextB{B: "b"}}

		// --- When ---
		err := NewEncoder().Encode(dst, s)

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, url.Values{"A": {"a"}, "B": {"b"}}, dst)
	})

	t.Run("nil pointers in marshaler interfaces", func(t *testing.T) {
		// --- Given ---
		dst := url.Values{}
		s := &struct {
			M  Marshaler
			T  encoding.TextMarshaler
			MS []Marshaler
		}{
			M:  (*TMarValPtr)(nil),
			T:  (*TTextPtr)(nil),
			MS: []Marshaler{(*TMarValPtr)(nil)},
		}

		// --- When ---
		err := NewEncoder().Encode(dst, s)

		// --- Then ---
		assert.NoError(t, err)
		assert.Empty(t, dst)
	})

	t.Run("nil embedded colliding marshaler pointers", func(t *testing.T) {
		// --- Given ---
		dst := url.Values{}
		s := &struct {
			*TMarValPtr
			*TStructMar
			FStr string `url:"f_str"`
		}{FStr: "x"}

		// --- When ---
		err := NewEncoder().Encode(dst, s)

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, url.Values{"f_str": {"x"}}, dst)
	})

	t.Run("error - empty map key", func(t *testing.T) {
		// --- Given ---
		dst := url.Values{}
		s := &struct{ M map[string]string }{M: map[string]string{"": "x"}}

		// --- When ---
		err := NewEncoder().Encode(dst, s)

		// --- Then ---
		assert.ErrorIs(t, ErrInvValue, err)
		xrrtest.AssertFieldEqual(t, "M", "invalid value: empty map key", err)
	})

	t.Run("omitempty with pointer receiver IsZero", func(t *testing.T) {
		// --- Given ---
		dst := url.Values{}
		s := &struct {
			F TPtrZero `url:"f,omitempty"`
		}{F: TPtrZero{N: -1}}

		// --- When ---
		err := NewEncoder().Encode(dst, s)

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, url.Values{}, dst)
	})

	t.Run("map type implementing TextMarshaler", func(t *testing.T) {
		// --- Given ---
		dst := url.Values{}
		s := &struct{ M TUnmMap }{M: TUnmMap{"a": "b"}}

		// --- When ---
		err := NewEncoder().Encode(dst, s)

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, url.Values{"M": {"a=b"}}, dst)
	})

	t.Run("error - pointer receiver TextMarshaler", func(t *testing.T) {
		// --- Given ---
		dst := url.Values{}
		s := &struct{ F TTextPtr }{F: 0}

		// --- When ---
		err := NewEncoder().Encode(dst, s)

		// --- Then ---
		xrrtest.AssertFieldEqual(t, "F", "non-printable ascii character", err)
	})

	t.Run("omitempty with IsZero promoted from nil", func(t *testing.T) {
		// --- Given ---
		dst := url.Values{}
		s := &struct {
			F TPromZero `url:"f,omitempty"`
			I int       `url:"i"`
		}{I: 1}

		// --- When ---
		err := NewEncoder().Encode(dst, s)

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, url.Values{"i": {"1"}}, dst)
	})

	t.Run("nil pointer in interface map key", func(t *testing.T) {
		// --- Given ---
		dst := url.Values{}
		s := &struct {
			M map[encoding.TextMarshaler]string
		}{M: map[encoding.TextMarshaler]string{(*TTextPtr)(nil): "x"}}

		// --- When ---
		err := NewEncoder().Encode(dst, s)

		// --- Then ---
		assert.NoError(t, err)
		assert.Empty(t, dst)
	})

	t.Run("pointer to pointer to TextMarshaler", func(t *testing.T) {
		// --- Given ---
		dst := url.Values{}
		lvl := new(TLevel(7))
		s := &struct {
			PP **TLevel `url:"pp"`
			NP **TLevel `url:"np"`
		}{PP: &lvl}

		// --- When ---
		err := NewEncoder().Encode(dst, s)

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, url.Values{"pp": {"L7"}}, dst)
	})

	t.Run("omitempty with IsZero promoted from deep nil", func(t *testing.T) {
		// --- Given ---
		dst := url.Values{}
		s := &struct {
			F struct{ TPromZero } `url:"f,omitempty"`
			I int                 `url:"i"`
		}{I: 1}

		// --- When ---
		err := NewEncoder().Encode(dst, s)

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, url.Values{"i": {"1"}}, dst)
	})

	t.Run("map with NaN key", func(t *testing.T) {
		// --- Given ---
		dst := url.Values{}
		s := &struct {
			M map[float64]string
		}{M: map[float64]string{math.NaN(): "x"}}

		// --- When ---
		err := NewEncoder().Encode(dst, s)

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, url.Values{"M[NaN]": {"x"}}, dst)
	})

	t.Run("registered encoder", func(t *testing.T) {
		// --- Given ---
		dst := url.Values{}
		tim := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
		s := &struct {
			T  time.Time            `url:"t"`
			TP *time.Time           `url:"tp"`
			TS []time.Time          `url:"ts"`
			TK map[time.Time]string `url:"tk"`
			P  TPoint               `url:"p"`
			PP *TPoint              `url:"pp"`
			L  TLevel               `url:"l"`
		}{
			T:  tim,
			TP: &tim,
			TS: []time.Time{tim},
			TK: map[time.Time]string{tim: "k"},
			P:  TPoint{X: 1, Y: 2},
			PP: &TPoint{X: 3, Y: 4},
			L:  TLevel(9),
		}
		enc := NewEncoder(
			WithEncFunc(func(tim time.Time) (string, error) {
				return tim.Format(time.DateOnly), nil
			}),
			WithEncFunc(encPoint),
			WithEncFunc(func(lvl TLevel) (string, error) {
				return strconv.Itoa(int(lvl)), nil
			}),
		)

		// --- When ---
		err := enc.Encode(dst, s)

		// --- Then ---
		assert.NoError(t, err)
		want := url.Values{
			"t":              {"2026-01-02"},
			"tp":             {"2026-01-02"},
			"ts":             {"2026-01-02"},
			"tk[2026-01-02]": {"k"},
			"p":              {"1,2"},
			"pp":             {"3,4"},
			"l":              {"9"},
		}
		assert.Equal(t, want, dst)
	})

	t.Run("containers of structs", func(t *testing.T) {
		// --- Given ---
		dst := url.Values{}
		src := &TBox{
			Items: map[string]TItem{
				"a":   {Size: 1, Name: "x"},
				"c.d": {Size: 3},
			},
			PItems: map[string]*TItem{"a": {Size: 2}},
			List:   []TItem{{Size: 1}, {Size: 2, Name: "b"}},
			PList:  []*TItem{nil, {Size: 3}},
			Arr:    [2]TItem{{}, {Size: 2}},
			Nest:   map[string][]TItem{"a": {{Size: 1}}},
			Grid:   [][]int{{1}, {3, 4}},
			Attr:   map[string]string{"a.b": "y", "k": "v"},
			Dotted: 10,
		}

		// --- When ---
		err := NewEncoder().Encode(dst, src)

		// --- Then ---
		assert.NoError(t, err)
		want := url.Values{
			"items[a].size":   {"1"},
			"items[a].name":   {"x"},
			"items[c.d].size": {"3"},
			"items[c.d].name": {""},
			"pitems[a].size":  {"2"},
			"pitems[a].name":  {""},
			"list[0].size":    {"1"},
			"list[0].name":    {""},
			"list[1].size":    {"2"},
			"list[1].name":    {"b"},
			"plist[1].size":   {"3"},
			"plist[1].name":   {""},
			"arr[0].size":     {"0"},
			"arr[0].name":     {""},
			"arr[1].size":     {"2"},
			"arr[1].name":     {""},
			"nest[a][0].size": {"1"},
			"nest[a][0].name": {""},
			"grid[0]":         {"1"},
			"grid[1]":         {"3", "4"},
			"attr[a.b]":       {"y"},
			"attr[k]":         {"v"},
			"[page.size]":     {"10"},
		}
		assert.Equal(t, want, dst)

		have := &TBox{}
		assert.NoError(t, NewDecoder().Decode(have, dst))
		assert.Equal(t, src, have)
	})

	t.Run("error - map key with a bracket", func(t *testing.T) {
		// --- Given ---
		dst := url.Values{}
		src := &TBox{Attr: map[string]string{"a]": "x"}}

		// --- When ---
		err := NewEncoder().Encode(dst, src)

		// --- Then ---
		want := "attr: invalid value: map key with a bracket (ECInvValue)"
		xrrtest.AssertFieldsEqual(t, want, err)
	})

	t.Run("error - tag name with a bracket", func(t *testing.T) {
		// --- Given ---
		src := &struct {
			F int `url:"a[b]"`
		}{}

		// --- When ---
		err := NewEncoder().Encode(url.Values{}, src)

		// --- Then ---
		assert.ErrorEqual(t, "invalid struct tag: F: bracket in name", err)
		assert.ErrorIs(t, ErrInvTag, err)
	})

	t.Run("embedded TextMarshaler does not hide fields", func(t *testing.T) {
		// --- Given ---
		dst := url.Values{}
		s := struct {
			time.Time
			Name string `url:"name"`
		}{Time: time.Unix(1, 0), Name: "a"}

		// --- When ---
		err := NewEncoder().Encode(dst, s)

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, url.Values{"name": {"a"}}, dst)
	})

	t.Run("registered struct type at the top level", func(t *testing.T) {
		// --- Given ---
		dst := url.Values{}

		// --- When ---
		err := NewEncoder(WithEncFunc(encPoint)).Encode(dst, TPoint{1, 2})

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, url.Values{"X": {"1"}, "Y": {"2"}}, dst)
	})

	t.Run("outer field shadows embedded field", func(t *testing.T) {
		// --- Given ---
		dst := url.Values{}
		s := &struct {
			TRequired
			FStr string `url:"f_str"`
		}{TRequired: TRequired{FStr: "in", FInt: 2}, FStr: "out"}

		// --- When ---
		err := NewEncoder().Encode(dst, s)

		// --- Then ---
		assert.NoError(t, err)
		want := url.Values{"f_str": {"out"}, "f_int": {"2"}}
		assert.Equal(t, want, dst)
	})

	t.Run("ambiguous fields are not encoded", func(t *testing.T) {
		// --- Given ---
		dst := url.Values{}
		s := &TAmbig{TAmbA: TAmbA{X: "1", A: "2"}, TAmbB: TAmbB{X: "3"}}

		// --- When ---
		err := NewEncoder().Encode(dst, s)

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, url.Values{"a": {"2"}}, dst)
	})

	t.Run("error - malformed struct tag", func(t *testing.T) {
		// --- When ---
		err := NewEncoder().Encode(url.Values{}, badTag(`url:n`, false))

		// --- Then ---
		assert.ErrorEqual(t, "invalid struct tag: N", err)
		assert.ErrorIs(t, ErrInvTag, err)
		xrrtest.AssertCode(t, ECInvTag, err)
	})

	t.Run("nil pointers", func(t *testing.T) {
		// --- Given ---
		dst := url.Values{}
		s := &struct {
			P  *int           `url:"p"`
			PL *TLevel        `url:"pl"`
			S  []*int         `url:"s"`
			M  map[string]int `url:"m"`
		}{S: []*int{new(1), nil, new(2)}}

		// --- When ---
		err := NewEncoder().Encode(dst, s)

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, url.Values{"s": {"1", "2"}}, dst)
	})

	t.Run("embedded interface is skipped", func(t *testing.T) {
		// --- Given ---
		dst := url.Values{}
		s := &struct {
			fmt.Stringer
			A int `url:"a"`
		}{A: 1}

		// --- When ---
		err := NewEncoder().Encode(dst, s)

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, url.Values{"a": {"1"}}, dst)
	})

	t.Run("recursive struct", func(t *testing.T) {
		// --- Given ---
		dst := url.Values{}
		s := &TNode{Val: 1, Next: &TNode{Val: 2, Req: "r"}}

		// --- When ---
		err := NewEncoder().Encode(dst, s)

		// --- Then ---
		assert.NoError(t, err)
		want := url.Values{
			"val":      {"1"},
			"req":      {""},
			"next.val": {"2"},
			"next.req": {"r"},
		}
		assert.Equal(t, want, dst)
	})

	t.Run("shared pointer is not a cycle", func(t *testing.T) {
		// --- Given ---
		dst := url.Values{}
		shared := &TNode{Val: 2}
		s := &struct {
			A *TNode `url:"a"`
			B *TNode `url:"b"`
		}{A: shared, B: shared}

		// --- When ---
		err := NewEncoder().Encode(dst, s)

		// --- Then ---
		assert.NoError(t, err)
		want := url.Values{
			"a.val": {"2"},
			"a.req": {""},
			"b.val": {"2"},
			"b.req": {""},
		}
		assert.Equal(t, want, dst)
	})

	t.Run("error - pointer cycle", func(t *testing.T) {
		// --- Given ---
		dst := url.Values{}
		s := &TNode{Val: 1}
		s.Next = &TNode{Val: 2, Next: s}

		// --- When ---
		err := NewEncoder().Encode(dst, s)

		// --- Then ---
		assert.ErrorIs(t, ErrCycle, err)
		want := "next.next: pointer cycle (ECCycle)"
		xrrtest.AssertFieldsEqual(t, want, err)
	})

	t.Run("error - embedding cycle", func(t *testing.T) {
		// --- Given ---
		dst := url.Values{}
		s := &TCycle{}
		s.TCycle = s

		// --- When ---
		err := NewEncoder().Encode(dst, s)

		// --- Then ---
		assert.Same(t, ErrCycle, err)
	})

	t.Run("field with pointer to own type", func(t *testing.T) {
		// --- Given ---
		dst := url.Values{}
		s := &TBuilder{S: "a"}

		// --- When ---
		err := NewEncoder().Encode(dst, s)

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, url.Values{"s": {"a"}}, dst)
	})
}

func Fuzz_Encoder_Encode(f *testing.F) {
	f.Add(
		"abc",
		int64(-1),
		uint64(1),
		1.5,
		true,
		int64(3e9),
		int64(0),
		int64(0),
		0,
		7,
		"a.b",
		"x",
	)
	f.Add(
		"",
		int64(math.MinInt64),
		uint64(math.MaxUint64),
		math.Inf(-1),
		false,
		int64(math.MinInt64),
		int64(-1),
		int64(999999999),
		-60,
		-3,
		"k",
		"",
	)
	f.Add(
		"\x00 ü&=",
		int64(math.MaxInt64),
		uint64(0),
		math.NaN(),
		true,
		int64(math.MaxInt64),
		int64(253402300799),
		int64(1),
		1439,
		0,
		"",
		"v",
	)

	f.Fuzz(func(
		t *testing.T,
		str string,
		i64 int64,
		u64 uint64,
		f64 float64,
		bln bool,
		dur int64,
		sec int64,
		nsec int64,
		off int,
		lvl int,
		mk string,
		mv string,
	) {

		// --- Given ---
		// Keep the time within the years RFC 3339 can represent.
		const lo, hi = -62135596800 + 86400, 253402300799 - 86400
		sec = lo + (sec%(hi-lo)+(hi-lo))%(hi-lo)
		nsec = (nsec%1e9 + 1e9) % 1e9
		zone := time.FixedZone("", off%1440*60)
		tim := time.Unix(sec, nsec).In(zone)

		var p *int64
		var ps *string
		if bln {
			p, ps = &i64, &str
		}

		dst := url.Values{}
		src := &TFuzzEnc{
			TFuzzEmb: TFuzzEmb{S: mv, E: mv},

			S:  str,
			I:  i64,
			U:  u64,
			F:  f64,
			B:  bln,
			D:  time.Duration(dur),
			T:  tim,
			L:  TLevel(lvl),
			SS: []string{str, mv},
			M:  map[string]string{mk: mv},
			MK: map[TLevel]string{TLevel(lvl): mv},
			P:  p,
			PS: ps,
			PT: TPoint{X: int(i64), Y: int(u64)},
		}

		// --- When ---
		err := NewEncoder(WithEncFunc(encPoint)).Encode(dst, src)

		// --- Then ---
		switch {
		case mk == "":
			want := "invalid value: empty map key"
			xrrtest.AssertFieldEqual(t, "m", want, err)
			return

		case strings.ContainsAny(mk, "[]"):
			want := "invalid value: map key with a bracket"
			xrrtest.AssertFieldEqual(t, "m", want, err)
			return
		}
		assert.NoError(t, err)
		assert.Equal(t, []string{str}, dst["s"])

		// Decoding replaces the slice already there.
		have := &TFuzzEnc{SS: []string{"junk"}}
		dec := NewDecoder(WithDecFunc(decPoint))
		assert.NoError(t, dec.Decode(have, dst))
		assert.True(t, have.T.Equal(src.T))
		_, wOff := src.T.Zone()
		_, hOff := have.T.Zone()
		assert.Equal(t, wOff, hOff)
		bothNaN := math.IsNaN(have.F) && math.IsNaN(src.F)
		assert.True(t, have.F == src.F || bothNaN)
		have.T, have.F, src.F = src.T, 0, 0
		src.TFuzzEmb.S = "" // A shadowed field is not encoded.
		assert.Equal(t, src, have)
	})
}

func Test_Encoder_tagName(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		// --- Given ---
		enc := &Encoder{}

		// --- When ---
		have := enc.tagName()

		// --- Then ---
		assert.Equal(t, "url", have)
	})

	t.Run("custom", func(t *testing.T) {
		// --- Given ---
		enc := &Encoder{tag: "custom"}

		// --- When ---
		have := enc.tagName()

		// --- Then ---
		assert.Equal(t, "custom", have)
	})
}

func Test_encSrc(t *testing.T) {
	t.Run("encoder not found", func(t *testing.T) {
		// --- Given ---
		type ct struct{ a string } //nolint:unused
		dst := url.Values{}
		s := &struct{ F ct }{}
		src := reflectkit.GetValue(t, s, "F")

		// --- When ---
		have, err := encSrc(dst, "", src, nil)

		// --- Then ---
		assert.NoError(t, err)
		assert.False(t, have)
		assert.Equal(t, "", dst.Encode())
	})

	t.Run("custom encoder", func(t *testing.T) {
		// --- Given ---
		type ct struct{ a string }
		dst := url.Values{}
		s := &struct{ F ct }{F: ct{a: "abc"}}
		src := reflectkit.GetValue(t, s, "F")
		enc := func(value reflect.Value) (string, error) {
			return "enc_" + value.Interface().(ct).a, nil
		}
		reg := encReg{reflect.TypeOf(ct{}): enc}

		// --- When ---
		have, err := encSrc(dst, "F", src, reg)

		// --- Then ---
		assert.NoError(t, err)
		assert.True(t, have)
		assert.Equal(t, "F=enc_abc", dst.Encode())
	})

	t.Run("error - MarshalValues", func(t *testing.T) {
		// --- Given ---
		dst := url.Values{}
		s := &struct{ F TMarValVal }{F: TMarValVal{fStr: "abc_"}}
		src := reflectkit.GetValue(t, s, "F")

		// --- When ---
		have, err := encSrc(dst, "", src, nil)

		// --- Then ---
		assert.ErrorEqual(t, "string too long", err)
		assert.True(t, have)
		assert.Equal(t, "", dst.Encode())
	})

	t.Run("error - MarshalText", func(t *testing.T) {
		// --- Given ---
		dst := url.Values{}
		s := &struct{ F *TTextVal }{F: new(TTextVal(0))}
		src := reflectkit.GetValue(t, s, "F")

		// --- When ---
		have, err := encSrc(dst, "F", src, nil)

		// --- Then ---
		assert.ErrorEqual(t, "non-printable ascii character", err)
		assert.True(t, have)
		assert.Equal(t, "", dst.Encode())
	})

	t.Run("text encoders not used at the empty path", func(t *testing.T) {
		// --- Given ---
		dst := url.Values{}
		enc := func(reflect.Value) (string, error) { return "point", nil }
		reg := encReg{reflect.TypeFor[TPoint](): enc}

		// --- When ---
		hTxt, eTxt := encSrc(dst, "", reflect.ValueOf(TTextVal('A')), nil)
		hReg, eReg := encSrc(dst, "", reflect.ValueOf(TPoint{}), reg)

		// --- Then ---
		assert.NoError(t, eTxt)
		assert.False(t, hTxt)
		assert.NoError(t, eReg)
		assert.False(t, hReg)
		assert.Empty(t, dst)
	})

	t.Run("error - custom encoder", func(t *testing.T) {
		// --- Given ---
		type ct struct{ a string }
		dst := url.Values{}
		s := &struct{ F ct }{F: ct{a: "abc"}}
		src := reflectkit.GetValue(t, s, "F")
		enc := func(reflect.Value) (string, error) { return "", errTest }
		reg := encReg{reflect.TypeOf(ct{}): enc}

		// --- When ---
		have, err := encSrc(dst, "F", src, reg)

		// --- Then ---
		assert.ErrorIs(t, errTest, err)
		assert.True(t, have)
		assert.Equal(t, "", dst.Encode())
	})
}

func Test_encSrc_tabular(t *testing.T) {
	tt := []struct {
		testN string

		s      any
		prefix string
		want   string
	}{
		{"string", &struct{ F string }{F: "abc"}, "F", "F=abc"},
		{
			"pointer to string",
			&struct{ F *string }{F: new("abc")},
			"F",
			"F=abc",
		},
		{"int alias", &struct{ F IntAlias }{F: 1}, "F", "F=1"},
		{
			"pointer to int alias",
			&struct{ F *IntAlias }{F: new(IntAlias(1))},
			"F",
			"F=1",
		},
		{
			"TextMarshaler pointer receiver",
			&struct{ F *TTextPtr }{F: new(TTextPtr(65))},
			"F",
			"F=A",
		},
		{
			"TextMarshaler value receiver",
			&struct{ F TTextVal }{F: TTextVal(65)},
			"F",
			"F=A",
		},
		{
			"Marshaler pointer receiver",
			&struct{ F *TMarValPtr }{F: &TMarValPtr{fStr: "abc"}},
			"F",
			"F.f_str=mar_abc",
		},
		{
			"Marshaler value receiver",
			&struct{ F TMarValVal }{F: TMarValVal{fStr: "abc"}},
			"F",
			"F.f_str=mar_abc",
		},
		{"nil TextMarshaler", &struct{ F *TTextVal }{}, "F", ""},
		{"nil Marshaler", &struct{ F *TMarValVal }{}, "F", ""},
	}

	for _, tc := range tt {
		t.Run(tc.testN, func(t *testing.T) {
			// --- Given ---
			dst := url.Values{}
			src := reflectkit.GetValue(t, tc.s, "F")

			// --- When ---
			have, err := encSrc(dst, tc.prefix, src, nil)

			// --- Then ---
			assert.NoError(t, err)
			assert.True(t, have)
			assert.Equal(t, tc.want, dst.Encode())
		})
	}
}

func Test_isMarshaler_tabular(t *testing.T) {
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
			have := isMarshaler(tc.typ)

			// --- Then ---
			assert.Equal(t, tc.want, have)
		})
	}
}

func Test_Encoder_encodeSlice(t *testing.T) {
	t.Run("custom encoder", func(t *testing.T) {
		// --- Given ---
		type ct struct{ a string }
		dst := url.Values{}
		s := &struct{ F []ct }{F: []ct{{a: "abc"}, {a: "xyz"}}}
		src := reflectkit.GetValue(t, s, "F")
		enc := func(value reflect.Value) (string, error) {
			return "enc_" + value.Interface().(ct).a, nil
		}
		reg := encReg{reflect.TypeOf(ct{}): enc}

		// --- When ---
		err := (&Encoder{reg: reg}).encodeSlice(
			dst,
			"F",
			src,
			map[ptrKey]bool{},
		)

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, "F=enc_abc&F=enc_xyz", dst.Encode())
	})

	t.Run("error - encoder not found", func(t *testing.T) {
		// --- Given ---
		dst := url.Values{}
		s := &struct {
			F []chan int `url:"-"`
		}{F: []chan int{make(chan int)}}
		src := reflectkit.GetValue(t, s, "F")

		// --- When ---
		err := (&Encoder{}).encodeSlice(dst, "F", src, map[ptrKey]bool{})

		// --- Then ---
		want := "F[0]: encoder not found for `chan int` (ECEncNotFound)"
		xrrtest.AssertEqual(t, want, err)
		assert.Equal(t, "", dst.Encode())
	})

	t.Run("error - element encoder", func(t *testing.T) {
		// --- Given ---
		type ct struct{ a string }
		dst := url.Values{}
		s := &struct{ F []ct }{F: []ct{{a: "abc"}, {a: "xyz"}}}
		src := reflectkit.GetValue(t, s, "F")
		enc := func(value reflect.Value) (string, error) {
			val := "enc_" + value.Interface().(ct).a
			if val == "enc_xyz" {
				return "", errors.New("msg")
			}
			return val, nil
		}
		reg := encReg{reflect.TypeOf(ct{}): enc}

		// --- When ---
		err := (&Encoder{reg: reg}).encodeSlice(
			dst,
			"F",
			src,
			map[ptrKey]bool{},
		)

		// --- Then ---
		assert.ErrorEqual(t, "F[1]: msg", err)
		assert.Equal(t, "F=enc_abc", dst.Encode())
	})
}

func Test_Encoder_encodeSlice_tabular(t *testing.T) {
	tt := []struct {
		testN string

		s      any
		prefix string
		want   string
	}{
		{
			"slice of strings",
			&struct{ F []string }{F: []string{"abc", "def"}},
			"F",
			"F=abc&F=def",
		},
		{
			"slice of pointers to strings",
			&struct{ F []*string }{F: []*string{new("abc"), new("def")}},
			"F",
			"F=abc&F=def",
		},
		{
			"array of strings",
			&struct{ F [2]string }{F: [2]string{"abc", "def"}},
			"F",
			"F=abc&F=def",
		},
		{
			"array of pointers to strings",
			&struct{ F [2]*string }{F: [2]*string{new("abc"), new("def")}},
			"F",
			"F=abc&F=def",
		},
		{
			"slice of aliases",
			&struct{ F []IntAlias }{F: []IntAlias{IntAlias(1), IntAlias(2)}},
			"F",
			"F=1&F=2",
		},
		{
			"slice of pointers to aliases",
			&struct {
				F []*IntAlias
			}{
				F: []*IntAlias{
					new(IntAlias(1)),
					new(IntAlias(2)),
				},
			},
			"F",
			"F=1&F=2",
		},
	}

	for _, tc := range tt {
		t.Run(tc.testN, func(t *testing.T) {
			// --- Given ---
			dst := url.Values{}
			src := reflectkit.GetValue(t, tc.s, "F")

			// --- When ---
			err := (&Encoder{}).encodeSlice(
				dst,
				tc.prefix,
				src,
				map[ptrKey]bool{},
			)

			// --- Then ---
			assert.NoError(t, err)
			assert.Equal(t, tc.want, dst.Encode())
		})
	}
}

func Test_Encoder_encodeMap(t *testing.T) {
	t.Run("nil key and nil value", func(t *testing.T) {
		// --- Given ---
		dst := url.Values{}
		lvl := TLevel(1)
		src := reflect.ValueOf(map[*TLevel]*int{nil: new(1), &lvl: nil})

		// --- When ---
		err := (&Encoder{}).encodeMap(dst, "M", src, map[ptrKey]bool{})

		// --- Then ---
		assert.NoError(t, err)
		assert.Empty(t, dst)
	})

	t.Run("nil pointer in interface key", func(t *testing.T) {
		// --- Given ---
		dst := url.Values{}
		key := encoding.TextMarshaler((*TTextPtr)(nil))
		src := reflect.ValueOf(map[encoding.TextMarshaler]int{key: 1})

		// --- When ---
		err := (&Encoder{}).encodeMap(dst, "M", src, map[ptrKey]bool{})

		// --- Then ---
		assert.NoError(t, err)
		assert.Empty(t, dst)
	})

	t.Run("custom key encoder", func(t *testing.T) {
		// --- Given ---
		type ct struct{ a string }
		dst := url.Values{}
		s := &struct {
			F map[ct]int
		}{
			map[ct]int{
				ct{a: "abc"}: 1,
				ct{a: "xyz"}: 2,
			},
		}
		src := reflectkit.GetValue(t, s, "F")
		enc := func(value reflect.Value) (string, error) {
			return "enc_" + value.Interface().(ct).a, nil
		}
		reg := encReg{reflect.TypeOf(ct{}): enc}

		// --- When ---
		err := (&Encoder{reg: reg}).encodeMap(dst, "F", src, map[ptrKey]bool{})

		// --- Then ---
		assert.NoError(t, err)
		want := url.Values{"F[enc_abc]": {"1"}, "F[enc_xyz]": {"2"}}
		assert.Equal(t, want, dst)
	})

	t.Run("custom value encoder", func(t *testing.T) {
		// --- Given ---
		type ct struct{ a string }
		dst := url.Values{}
		s := &struct {
			F map[int]ct
		}{
			map[int]ct{
				1: {a: "abc"},
				2: {a: "xyz"},
			},
		}
		src := reflectkit.GetValue(t, s, "F")
		enc := func(value reflect.Value) (string, error) {
			return "enc_" + value.Interface().(ct).a, nil
		}
		reg := encReg{reflect.TypeOf(ct{}): enc}

		// --- When ---
		err := (&Encoder{reg: reg}).encodeMap(dst, "F", src, map[ptrKey]bool{})

		// --- Then ---
		assert.NoError(t, err)
		want := url.Values{"F[1]": {"enc_abc"}, "F[2]": {"enc_xyz"}}
		assert.Equal(t, want, dst)
	})

	t.Run("error - key encoder not found", func(t *testing.T) {
		// --- Given ---
		type ct struct{ a string }
		dst := url.Values{}
		s := &struct {
			F map[ct]int
		}{
			map[ct]int{
				ct{a: "abc"}: 1,
			},
		}
		src := reflectkit.GetValue(t, s, "F")

		// --- When ---
		err := (&Encoder{}).encodeMap(dst, "F", src, map[ptrKey]bool{})

		// --- Then ---
		want := "F: encoder not found for `xbind.ct` (ECEncNotFound)"
		xrrtest.AssertEqual(t, want, err)
		assert.Equal(t, "", dst.Encode())
	})

	t.Run("error - value encoder not found", func(t *testing.T) {
		// --- Given ---
		dst := url.Values{}
		s := &struct {
			F map[int]chan int
		}{
			map[int]chan int{1: make(chan int)},
		}
		src := reflectkit.GetValue(t, s, "F")

		// --- When ---
		err := (&Encoder{}).encodeMap(dst, "F", src, map[ptrKey]bool{})

		// --- Then ---
		want := "F[1]: encoder not found for `chan int` (ECEncNotFound)"
		xrrtest.AssertEqual(t, want, err)
		assert.Equal(t, "", dst.Encode())
	})

	t.Run("error - slice value encoder not found", func(t *testing.T) {
		// --- Given ---
		dst := url.Values{}
		s := &struct {
			F map[string][]chan int
		}{
			map[string][]chan int{
				"a": {make(chan int)},
			},
		}
		src := reflectkit.GetValue(t, s, "F")

		// --- When ---
		err := (&Encoder{}).encodeMap(dst, "F", src, map[ptrKey]bool{})

		// --- Then ---
		want := "F[a][0]: encoder not found for `chan int` (ECEncNotFound)"
		xrrtest.AssertEqual(t, want, err)
		assert.Equal(t, "", dst.Encode())
	})

	t.Run("error - key encoder", func(t *testing.T) {
		// --- Given ---
		type ct struct{ a string }
		dst := url.Values{}
		s := &struct {
			F map[ct]int
		}{
			map[ct]int{
				ct{a: "abc"}: 1,
				ct{a: "xyz"}: 2,
			},
		}
		src := reflectkit.GetValue(t, s, "F")
		enc := func(value reflect.Value) (string, error) {
			val := "enc_" + value.Interface().(ct).a
			if val == "enc_xyz" {
				return "", errors.New("msg")
			}
			return val, nil
		}
		reg := encReg{reflect.TypeOf(ct{}): enc}

		// --- When ---
		err := (&Encoder{reg: reg}).encodeMap(dst, "F", src, map[ptrKey]bool{})

		// --- Then ---
		xrrtest.AssertEqual(t, "F: msg (ECGeneric)", err)
	})

	t.Run("error - value encoder", func(t *testing.T) {
		// --- Given ---
		type ct struct{ a string }
		dst := url.Values{}
		s := &struct {
			F map[int]ct
		}{
			map[int]ct{
				1: {a: "abc"},
				2: {a: "xyz"},
			},
		}
		src := reflectkit.GetValue(t, s, "F")
		enc := func(value reflect.Value) (string, error) {
			val := "enc_" + value.Interface().(ct).a
			if val == "enc_xyz" {
				return "", errors.New("msg")
			}
			return val, nil
		}
		reg := encReg{reflect.TypeOf(ct{}): enc}

		// --- When ---
		err := (&Encoder{reg: reg}).encodeMap(dst, "F", src, map[ptrKey]bool{})

		// --- Then ---
		xrrtest.AssertEqual(t, "F[2]: msg (ECGeneric)", err)
	})
}

func Test_Encoder_encodeMap_tabular(t *testing.T) {
	tt := []struct {
		testN string

		s      any
		prefix string
		want   url.Values
	}{
		{
			"map integers to strings",
			&struct{ F map[int]string }{F: map[int]string{1: "abc", 2: "def"}},
			"F",
			url.Values{"F[1]": {"abc"}, "F[2]": {"def"}},
		},
		{
			"map integers to pointers to strings",
			&struct {
				F map[int]*string
			}{
				F: map[int]*string{
					1: new("abc"),
					2: new("def"),
				},
			},
			"F",
			url.Values{"F[1]": {"abc"}, "F[2]": {"def"}},
		},
		{
			"map pointers to integers to pointers to strings",
			&struct {
				F map[*int]*string
			}{
				F: map[*int]*string{
					new(1): new("abc"),
					new(2): new("def"),
				},
			},
			"F",
			url.Values{"F[1]": {"abc"}, "F[2]": {"def"}},
		},
	}

	for _, tc := range tt {
		t.Run(tc.testN, func(t *testing.T) {
			// --- Given ---
			dst := url.Values{}
			src := reflectkit.GetValue(t, tc.s, "F")

			// --- When ---
			err := (&Encoder{}).encodeMap(
				dst,
				tc.prefix,
				src,
				map[ptrKey]bool{},
			)

			// --- Then ---
			assert.NoError(t, err)
			assert.Equal(t, tc.want, dst)
		})
	}
}

func Test_Encoder_isValueType_tabular(t *testing.T) {
	tt := []struct {
		testN string

		typ  reflect.Type
		want bool
	}{
		{"registered", reflect.TypeFor[TPoint](), true},
		{"pointer to registered", reflect.TypeFor[*TPoint](), true},
		{"marshaler", reflect.TypeFor[TMarValVal](), false},
		{"pointer receiver marshaler", reflect.TypeFor[TMarValPtr](), false},
		{"text marshaler", reflect.TypeFor[TTextVal](), true},
		{"pointer receiver text marshaler", reflect.TypeFor[TTextPtr](), true},
		{"base type", reflect.TypeFor[int](), true},
		{"struct", reflect.TypeFor[TItem](), false},
		{"function", reflect.TypeFor[func()](), false},
	}

	for _, tc := range tt {
		t.Run(tc.testN, func(t *testing.T) {
			// --- When ---
			have := NewEncoder(WithEncFunc(encPoint)).isValueType(tc.typ)

			// --- Then ---
			assert.Equal(t, tc.want, have)
		})
	}
}

func Test_encMapKey(t *testing.T) {
	t.Run("base type", func(t *testing.T) {
		// --- When ---
		have, err := encMapKey(reflect.ValueOf(42), nil)

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, "42", have)
	})

	t.Run("TextMarshaler", func(t *testing.T) {
		// --- When ---
		have, err := encMapKey(reflect.ValueOf(TTextVal('A')), nil)

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, "A", have)
	})

	t.Run("TextMarshaler in interface", func(t *testing.T) {
		// --- Given ---
		m := map[encoding.TextMarshaler]int{TTextVal('A'): 1}
		key := reflect.ValueOf(m).MapKeys()[0]

		// --- When ---
		have, err := encMapKey(key, nil)

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, "A", have)
	})

	t.Run("error - MarshalText", func(t *testing.T) {
		// --- When ---
		have, err := encMapKey(reflect.ValueOf(TTextVal(0)), nil)

		// --- Then ---
		assert.ErrorEqual(t, "non-printable ascii character", err)
		assert.Equal(t, "", have)
	})

	t.Run("error - encoder not found", func(t *testing.T) {
		// --- Given ---
		type ct struct{}

		// --- When ---
		have, err := encMapKey(reflect.ValueOf(ct{}), nil)

		// --- Then ---
		assert.ErrorEqual(t, "encoder not found for `xbind.ct`", err)
		assert.ErrorIs(t, ErrEncNotFound, err)
		assert.Equal(t, "", have)
	})
}

func Test_fieldError(t *testing.T) {
	t.Run("nil error", func(t *testing.T) {
		// --- When ---
		have := fieldError("f", nil)

		// --- Then ---
		assert.Nil(t, have)
	})

	t.Run("empty field", func(t *testing.T) {
		// --- When ---
		have := fieldError("", errTest)

		// --- Then ---
		assert.Same(t, errTest, have)
	})

	t.Run("field", func(t *testing.T) {
		// --- When ---
		have := fieldError("f", errTest)

		// --- Then ---
		xrrtest.AssertFieldIs(t, "f", errTest, have)
	})
}

func Test_encNotFound(t *testing.T) {
	// --- When ---
	err := encNotFound(reflect.TypeFor[chan int]())

	// --- Then ---
	assert.ErrorEqual(t, "encoder not found for `chan int`", err)
	assert.ErrorIs(t, ErrEncNotFound, err)
	xrrtest.AssertCode(t, ECEncNotFound, err)
}
