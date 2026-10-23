// SPDX-FileCopyrightText: (c) 2026 Rafal Zajac
// SPDX-License-Identifier: MIT

package xbind

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"testing/iotest"
	"time"

	"github.com/ctx42/testing/pkg/assert"
	"github.com/ctx42/testkit/pkg/jsonkit"
	"github.com/ctx42/xrr/pkg/xrr"
	"github.com/ctx42/xrr/pkg/xrr/xrrtest"
)

func Test_UnmarshalJSON(t *testing.T) {
	t.Run("error - dst not a pointer", func(t *testing.T) {
		// --- Given ---
		dst := struct{}{}

		// --- When ---
		err := UnmarshalJSON([]byte(`{}`), dst)

		// --- Then ---
		_, ok := errors.AsType[*json.InvalidUnmarshalError](err)
		assert.True(t, ok)
		assert.ErrorIsNot(t, ErrBind, err)
	})

	t.Run("ok", func(t *testing.T) {
		// --- Given ---
		dst := &struct {
			Data int `json:"f_data"`
		}{}

		// --- When ---
		err := UnmarshalJSON([]byte(`{"f_data": 1}`), dst)

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, 1, dst.Data)
	})

	t.Run("error - string for duration", func(t *testing.T) {
		// --- Given ---
		dst := &struct {
			Dur time.Duration `json:"f_dur"`
		}{}

		// --- When ---
		err := UnmarshalJSON([]byte(`{"f_dur": "100"}`), dst)

		// --- Then ---
		assert.ErrorIs(t, ErrBind, err)
		want := `
		{
			"error": "binding error",
			"code": "ECBinding",
			"fields": {
				"f_dur": {"code": "ECInvType", "error": "invalid type"}
			}
		}`
		assert.JSON(t, want, string(jsonkit.To(t, err)))
	})

	t.Run("error - number for time", func(t *testing.T) {
		// --- Given ---
		dst := &struct {
			Tim time.Time `json:"f_tim"`
		}{}

		// --- When ---
		err := UnmarshalJSON([]byte(`{"f_tim": 100}`), dst)

		// --- Then ---
		assert.ErrorIs(t, ErrTimeFormat, err)
		assert.ErrorIs(t, ErrBind, err)
		want := `
		{
			"error": "binding error",
			"code": "ECBinding",
			"errors": [
				{"code": "ECInvTimeFormat", "error": "invalid RFC3339 time"}
			]
		}`
		assert.JSON(t, want, string(jsonkit.To(t, err)))
	})

	t.Run("error - string for int", func(t *testing.T) {
		// --- Given ---
		dst := &struct {
			Int int `json:"f_int"`
		}{}

		// --- When ---
		err := UnmarshalJSON([]byte(`{"f_int": "100"}`), dst)

		// --- Then ---
		assert.ErrorIs(t, ErrBind, err)
		want := `
		{
			"error": "binding error",
			"code": "ECBinding",
			"fields": {
				"f_int": {"code": "ECInvType", "error": "invalid type"}
			}
		}`
		assert.JSON(t, want, string(jsonkit.To(t, err)))
	})

	t.Run("error - float for int", func(t *testing.T) {
		// --- Given ---
		dst := &struct {
			Int int `json:"f_int"`
		}{}

		// --- When ---
		err := UnmarshalJSON([]byte(`{"f_int": 1.23}`), dst)

		// --- Then ---
		assert.ErrorIs(t, ErrBind, err)
		want := `
		{
			"error": "binding error",
			"code": "ECBinding",
			"fields": {
				"f_int": {"code": "ECInvType", "error": "invalid type"}
			}
		}`
		assert.JSON(t, want, string(jsonkit.To(t, err)))
	})

	t.Run("error - string for uint", func(t *testing.T) {
		// --- Given ---
		dst := &struct {
			UInt uint `json:"f_uint"`
		}{}

		// --- When ---
		err := UnmarshalJSON([]byte(`{"f_uint": "100"}`), dst)

		// --- Then ---
		assert.ErrorIs(t, ErrBind, err)
		want := `
		{
			"error": "binding error",
			"code": "ECBinding",
			"fields": {
				"f_uint": {"code": "ECInvType", "error": "invalid type"}
			}
		}`
		assert.JSON(t, want, string(jsonkit.To(t, err)))
	})

	t.Run("error - float for uint", func(t *testing.T) {
		// --- Given ---
		dst := &struct {
			UInt uint `json:"f_uint"`
		}{}

		// --- When ---
		err := UnmarshalJSON([]byte(`{"f_uint": 1.23}`), dst)

		// --- Then ---
		assert.ErrorIs(t, ErrBind, err)
		want := `
		{
			"error": "binding error",
			"code": "ECBinding",
			"fields": {
				"f_uint": {"code": "ECInvType", "error": "invalid type"}
			}
		}`
		assert.JSON(t, want, string(jsonkit.To(t, err)))
	})

	t.Run("error - negative for uint", func(t *testing.T) {
		// --- Given ---
		dst := &struct {
			UInt uint `json:"f_uint"`
		}{}

		// --- When ---
		err := UnmarshalJSON([]byte(`{"f_uint": -123}`), dst)

		// --- Then ---
		assert.ErrorIs(t, ErrBind, err)
		want := `
		{
			"error": "binding error",
			"code": "ECBinding",
			"fields": {
				"f_uint": {"code": "ECInvType", "error": "invalid type"}
			}
		}`
		assert.JSON(t, want, string(jsonkit.To(t, err)))
	})

	t.Run("error - invalid JSON", func(t *testing.T) {
		// --- Given ---
		dst := &struct {
			Data string `json:"f_data"`
		}{}

		// --- When ---
		err := UnmarshalJSON([]byte(`{!!!}`), dst)

		// --- Then ---
		var e xrr.Envelope
		assert.ErrorAs(t, &e, err)
		assert.ErrorContain(t, "invalid character", e.Unwrap())
		xrrtest.AssertCode(t, "ECInvJSON", e.Unwrap())
		assert.ErrorIs(t, xrr.ErrInvJSON, e.Lead())
	})

	t.Run("error - trailing data", func(t *testing.T) {
		// --- Given ---
		dst := &struct {
			Data int `json:"f_data"`
		}{}

		// --- When ---
		err := UnmarshalJSON([]byte(`{"f_data": 1} {"f_data": 2}`), dst)

		// --- Then ---
		var e xrr.Envelope
		assert.ErrorAs(t, &e, err)
		assert.ErrorIs(t, xrr.ErrInvJSON, e.Lead())
		xrrtest.AssertCode(t, "ECInvJSON", e.Unwrap())
		assert.ErrorEqual(t, "trailing data after JSON value", e.Unwrap())
	})

	t.Run("error - empty body", func(t *testing.T) {
		// --- Given ---
		dst := &struct {
			Data int `json:"f_data"`
		}{}

		// --- When ---
		err := UnmarshalJSON([]byte(``), dst)

		// --- Then ---
		var e xrr.Envelope
		assert.ErrorAs(t, &e, err)
		assert.ErrorIs(t, ErrBind, e.Lead())
		xrrtest.AssertCode(t, "ECInvJSON", e.Unwrap())
		assert.ErrorEqual(t, "unexpected end of JSON", e.Unwrap())
	})

	t.Run("error - truncated JSON", func(t *testing.T) {
		// --- Given ---
		dst := &struct {
			Data int `json:"f_data"`
		}{}

		// --- When ---
		err := UnmarshalJSON([]byte(`{"f_data":`), dst)

		// --- Then ---
		var e xrr.Envelope
		assert.ErrorAs(t, &e, err)
		assert.ErrorIs(t, ErrBind, e.Lead())
		xrrtest.AssertCode(t, "ECInvJSON", e.Unwrap())
		assert.ErrorEqual(t, "unexpected end of JSON", e.Unwrap())
	})

	t.Run("error - unknown field", func(t *testing.T) {
		// --- Given ---
		dst := &struct {
			Data string `json:"f_data"`
		}{}

		// --- When ---
		err := UnmarshalJSON([]byte(`{"f_data": "abc", "_extra_": 1}`), dst)

		// --- Then ---
		assert.ErrorIs(t, ErrBind, err)
		want := `
		{
			"code":"ECBinding",
			"error":"binding error",
			"errors":[
				{
					"code":"ECGeneric",
					"error":"json: unknown field \"_extra_\""
				}
			]
		}`
		assert.JSON(t, want, string(jsonkit.To(t, err)))
	})
}

func Test_UnmarshalJSONReader(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		// --- Given ---
		dst := &struct {
			Data int `json:"f_data"`
		}{}

		// --- When ---
		err := UnmarshalJSONReader(strings.NewReader(`{"f_data": 1}`), dst)

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, 1, dst.Data)
	})

	t.Run("error - nil reader", func(t *testing.T) {
		// --- Given ---
		dst := &struct {
			Data int `json:"f_data"`
		}{}

		// --- When ---
		err := UnmarshalJSONReader(nil, dst)

		// --- Then ---
		assert.ErrorIs(t, ErrBind, err)
		want := `{"error":"binding error","code":"ECBinding"}`
		assert.JSON(t, want, string(jsonkit.To(t, err)))
	})

	t.Run("error - read error after value", func(t *testing.T) {
		// --- Given ---
		r := io.MultiReader(
			strings.NewReader(`{"f_data": 1}`),
			iotest.ErrReader(errTest),
		)
		dst := &struct {
			Data int `json:"f_data"`
		}{}

		// --- When ---
		err := UnmarshalJSONReader(r, dst)

		// --- Then ---
		assert.ErrorIs(t, errTest, err)
		assert.ErrorIs(t, ErrBind, err)
	})
}

func Test_UnmarshalJSONr_tabular(t *testing.T) {
	tt := []struct {
		testN string

		data string
	}{
		{"error - garbage", `{"f_data": 1} x`},
		{"error - incomplete string", `{"f_data": 1} "x`},
		{"error - incomplete literal", `{"f_data": 1} tru`},
	}

	for _, tc := range tt {
		t.Run(tc.testN, func(t *testing.T) {
			// --- Given ---
			dst := &struct {
				Data int `json:"f_data"`
			}{}

			// --- When ---
			err := UnmarshalJSONReader(strings.NewReader(tc.data), dst)

			// --- Then ---
			var e xrr.Envelope
			assert.ErrorAs(t, &e, err)
			assert.ErrorIs(t, xrr.ErrInvJSON, e.Lead())
			assert.ErrorEqual(t, "trailing data after JSON value", e.Unwrap())
		})
	}
}

func Fuzz_UnmarshalJSONReader(f *testing.F) {
	f.Add([]byte(`{"f_data": 1, "s": "x", "a": [1, 2], "m": {"k": 1.5}}`))
	f.Add([]byte(`{"t": "2022-03-15T11:43:01+02:00"}`))
	f.Add([]byte(`{"f_data": "1"}`))
	f.Add([]byte(`{"unknown": 1}`))
	f.Add([]byte(`{"f_data": 1} {"f_data": 2}`))
	f.Add([]byte(`{"f_data":`))
	f.Add([]byte(`[1]`))
	f.Add([]byte(``))
	f.Add([]byte(`null`))

	type body struct {
		Data int                `json:"f_data"`
		Str  string             `json:"s"`
		Tim  time.Time          `json:"t"`
		Arr  []int              `json:"a"`
		Map  map[string]float64 `json:"m"`
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		// --- Given ---
		// The reference: strict encoding/json decoding of a single value.
		want := &body{}
		ref := json.NewDecoder(bytes.NewReader(data))
		ref.DisallowUnknownFields()
		wErr := ref.Decode(want)
		if wErr == nil {
			if _, tErr := ref.Token(); !errors.Is(tErr, io.EOF) {
				wErr = errors.New("trailing data")
			}
		}

		have := &body{}

		// --- When ---
		err := UnmarshalJSONReader(bytes.NewReader(data), have)

		// --- Then ---
		if wErr != nil {
			isBind := errors.Is(err, ErrBind)
			assert.True(t, isBind || errors.Is(err, xrr.ErrInvJSON))
			return
		}
		assert.NoError(t, err)
		assert.Equal(t, want, have)
	})
}

func Test_UnmarshalJSONBody(t *testing.T) {
	type data struct {
		Data int `json:"data"`
	}

	t.Run("ok", func(t *testing.T) {
		// --- Given ---
		body := &TRC{}
		req := httptest.NewRequest(http.MethodPost, "/api", body)
		dst := &data{}

		// --- When ---
		err := UnmarshalJSONBody(req, dst)

		// --- Then ---
		assert.NoError(t, err)
		assert.Equal(t, 123, dst.Data)
		assert.True(t, body.Closed)
	})

	t.Run("error - nil body", func(t *testing.T) {
		// --- Given ---
		req := httptest.NewRequest(http.MethodPost, "/api", nil)
		req.Body = nil

		// --- When ---
		err := UnmarshalJSONBody(req, &data{})

		// --- Then ---
		assert.Same(t, ErrBind, err)
	})

	t.Run("error - nil request", func(t *testing.T) {
		// --- When ---
		err := UnmarshalJSONBody(nil, &data{})

		// --- Then ---
		assert.ErrorIs(t, ErrBind, err)
		want := `{"code":"ECBinding","error":"binding error"}`
		assert.JSON(t, want, string(jsonkit.To(t, err)))
	})
}

func Test_HUE(t *testing.T) {
	t.Run("nil error", func(t *testing.T) {
		// --- When ---
		err := HUE(nil)

		// --- Then ---
		assert.Nil(t, err)
	})

	t.Run("error type handled by MaybeHUE", func(t *testing.T) {
		// --- Given ---
		var out string
		se := json.Unmarshal([]byte("{!!!}"), &out)

		// --- When ---
		err := HUE(se)

		// --- Then ---
		var e xrr.Envelope
		assert.ErrorAs(t, &e, err)
		assert.ErrorIs(t, xrr.ErrInvJSON, e.Lead())
	})

	t.Run("invalid unmarshal error", func(t *testing.T) {
		// --- Given ---
		iue := &json.InvalidUnmarshalError{Type: reflect.TypeFor[int]()}

		// --- When ---
		err := HUE(iue)

		// --- Then ---
		assert.Same(t, iue, err)
	})

	t.Run("other error", func(t *testing.T) {
		// --- When ---
		err := HUE(errTest)

		// --- Then ---
		var e xrr.Envelope
		assert.ErrorAs(t, &e, err)
		assert.Same(t, ErrBind, e.Lead())
		assert.Same(t, errTest, e.Unwrap())
	})
}

func Test_MaybeHUE(t *testing.T) {
	t.Run("nil error", func(t *testing.T) {
		// --- When ---
		have, err := MaybeHUE(nil)

		// --- Then ---
		assert.Nil(t, err)
		assert.False(t, have)
	})

	t.Run("SyntaxError", func(t *testing.T) {
		// --- Given ---
		var out string
		se := json.Unmarshal([]byte("{!!!}"), &out)

		// --- When ---
		have, err := MaybeHUE(se)

		// --- Then ---
		var e xrr.Envelope
		assert.ErrorAs(t, &e, err)
		assert.ErrorContain(t, "invalid character", e.Unwrap())
		xrrtest.AssertCode(t, "ECInvJSON", e.Unwrap())
		assert.ErrorIs(t, xrr.ErrInvJSON, e.Lead())
		assert.True(t, have)
	})

	t.Run("other error", func(t *testing.T) {
		// --- When ---
		have, err := MaybeHUE(errTest)

		// --- Then ---
		assert.Same(t, errTest, err)
		assert.False(t, have)
	})

	t.Run("UnmarshalTypeError for time", func(t *testing.T) {
		// --- Given ---
		ute := &json.UnmarshalTypeError{
			Value: "number",
			Type:  reflect.TypeFor[time.Time](),
			Field: "f_tim",
		}

		// --- When ---
		have, err := MaybeHUE(ute)

		// --- Then ---
		assert.ErrorIs(t, ErrTimeFormat, err)
		assert.ErrorIs(t, ErrBind, err)
		assert.True(t, have)
	})

	t.Run("time UnmarshalJSON error", func(t *testing.T) {
		// --- Given ---
		e := errors.New("Time.UnmarshalJSON: input is not a JSON string")

		// --- When ---
		have, err := MaybeHUE(e)

		// --- Then ---
		assert.True(t, have)
		assert.ErrorIs(t, ErrTimeFormat, err)
		assert.ErrorIs(t, ErrBind, err)
	})

	t.Run("time ParseError", func(t *testing.T) {
		// --- Given ---
		_, pe := time.Parse(time.RFC3339, "2022")

		// --- When ---
		have, err := MaybeHUE(pe)

		// --- Then ---
		assert.ErrorIs(t, ErrTimeFormat, err)
		assert.ErrorIs(t, ErrBind, err)
		want := `
		{
			"code":"ECBinding",
			"error":"binding error",
			"errors":[
				{"code":"ECInvTimeFormat","error":"invalid RFC3339 time"}
			]
		}`
		assert.JSON(t, want, string(jsonkit.To(t, err)))
		assert.True(t, have)
	})

	t.Run("unknown field", func(t *testing.T) {
		// --- When ---
		have, err := MaybeHUE(errors.New("json: unknown field \"_extra_\""))

		// --- Then ---
		assert.ErrorIs(t, ErrBind, err)
		want := `
		{
			"code":"ECBinding",
			"error":"binding error",
			"errors":[
				{
					"code":"ECGeneric",
					"error":"json: unknown field \"_extra_\""
				}
			]
		}`
		assert.JSON(t, want, string(jsonkit.To(t, err)))
		assert.True(t, have)
	})

	t.Run("unknown field named like a time error", func(t *testing.T) {
		// --- Given ---
		e := errors.New(`json: unknown field "Time.UnmarshalJSON"`)

		// --- When ---
		have, err := MaybeHUE(e)

		// --- Then ---
		assert.True(t, have)
		assert.ErrorIs(t, ErrBind, err)
		assert.False(t, errors.Is(err, ErrTimeFormat))
		assert.ErrorEqual(t, `json: unknown field "Time.UnmarshalJSON"`, err)
	})

	t.Run("UnmarshalTypeError without type", func(t *testing.T) {
		// --- Given ---
		ute := &json.UnmarshalTypeError{Value: "x"}

		// --- When ---
		have, err := MaybeHUE(ute)

		// --- Then ---
		assert.True(t, have)
		assert.Same(t, xrr.ErrInvJSON, err)
	})
}

func Test_MaybeHUE_tabular(t *testing.T) {
	tt := []struct {
		testN string

		typ  reflect.Type
		want string
	}{
		{"array", reflect.TypeFor[[2]int](), "expected array"},
		{"slice", reflect.TypeFor[[]int](), "expected array"},
		{"map", reflect.TypeFor[map[string]int](), "expected object"},
		{"struct", reflect.TypeFor[struct{}](), "expected object"},
		{"string", reflect.TypeFor[string](), "expected string"},
		{"int", reflect.TypeFor[int](), "expected number"},
		{"float", reflect.TypeFor[float64](), "expected number"},
		{"bool", reflect.TypeFor[bool](), "expected boolean"},
	}

	for _, tc := range tt {
		t.Run(tc.testN, func(t *testing.T) {
			// --- Given ---
			ute := &json.UnmarshalTypeError{Value: "x", Type: tc.typ}

			// --- When ---
			have, err := MaybeHUE(ute)

			// --- Then ---
			assert.True(t, have)
			var e xrr.Envelope
			assert.ErrorAs(t, &e, err)
			assert.ErrorIs(t, xrr.ErrInvJSON, e.Lead())
			assert.ErrorContain(t, tc.want, e.Unwrap())
		})
	}
}

func Test_jsonType_tabular(t *testing.T) {
	tt := []struct {
		testN string

		typ  reflect.Type
		want string
	}{
		{"array", reflect.TypeFor[[2]int](), "array"},
		{"slice", reflect.TypeFor[[]int](), "array"},
		{"map", reflect.TypeFor[map[string]int](), "object"},
		{"struct", reflect.TypeFor[struct{}](), "object"},
		{"pointer to struct", reflect.TypeFor[*struct{}](), "object"},
		{"string", reflect.TypeFor[string](), "string"},
		{"bool", reflect.TypeFor[bool](), "boolean"},
		{"text unmarshaler", reflect.TypeFor[TLevel](), "string"},
		{"bytes", reflect.TypeFor[[]byte](), "string"},
		{"text unmarshaler pointer", reflect.TypeFor[*TLevel](), "string"},
		{"int", reflect.TypeFor[int](), "number"},
		{"int8", reflect.TypeFor[int8](), "number"},
		{"int16", reflect.TypeFor[int16](), "number"},
		{"int32", reflect.TypeFor[int32](), "number"},
		{"int64", reflect.TypeFor[int64](), "number"},
		{"uint", reflect.TypeFor[uint](), "number"},
		{"uint8", reflect.TypeFor[uint8](), "number"},
		{"uint16", reflect.TypeFor[uint16](), "number"},
		{"uint32", reflect.TypeFor[uint32](), "number"},
		{"uint64", reflect.TypeFor[uint64](), "number"},
		{"uintptr", reflect.TypeFor[uintptr](), "number"},
		{"float32", reflect.TypeFor[float32](), "number"},
		{"float64", reflect.TypeFor[float64](), "number"},
		{"chan", reflect.TypeFor[chan int](), "value"},
		{"interface", reflect.TypeFor[any](), "value"},
	}

	for _, tc := range tt {
		t.Run(tc.testN, func(t *testing.T) {
			// --- When ---
			have := jsonType(tc.typ)

			// --- Then ---
			assert.Equal(t, tc.want, have)
		})
	}
}
