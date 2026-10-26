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
	"net/url"
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
		// The text of the cause the time error wraps depends on Go version.
		want := `{"code":"ECInvTimeFormat","error":"invalid RFC3339 time; `
		assert.Contain(t, want, string(jsonkit.To(t, err)))
	})

	t.Run("error - array for nested UnmarshalJSON", func(t *testing.T) {
		// --- When ---
		err := UnmarshalJSON([]byte(`[]`), &TJSONTime{})

		// --- Then ---
		assert.ErrorIs(t, xrr.ErrInvJSON, err)
		want := `
		{
			"code": "ECInvJSON",
			"error": "invalid JSON",
			"errors": [
				{
					"code": "ECInvJSON",
					"error": "unmarshal type mismatch; expected object"
				}
			]
		}`
		assert.JSON(t, want, string(jsonkit.To(t, err)))
	})

	t.Run("error - wrong type in embedded struct", func(t *testing.T) {
		// --- Given ---
		dst := &struct {
			*TJSONInner
			X int `json:"x"`
		}{}

		// --- When ---
		err := UnmarshalJSON([]byte(`{"name": 1}`), dst)

		// --- Then ---
		assert.ErrorIs(t, ErrBind, err)
		want := `
		{
			"code": "ECBinding",
			"error": "binding error",
			"fields": {
				"name": {"code": "ECInvType", "error": "invalid type"}
			}
		}`
		assert.JSON(t, want, string(jsonkit.To(t, err)))
	})

	t.Run("error - invalid time in nested UnmarshalJSON", func(t *testing.T) {
		// --- Given ---
		dst := &struct {
			Doc TJSONTime `json:"doc"`
		}{}

		// --- When ---
		err := UnmarshalJSON([]byte(`{"doc": {"f_tim": "x"}}`), dst)

		// --- Then ---
		assert.ErrorIs(t, ErrTimeFormat, err)
		assert.ErrorIs(t, ErrBind, err)
		want := `{"code":"ECBinding","error":"binding error","errors":[` +
			`{"code":"ECInvTimeFormat","error":"invalid RFC3339 time; ` +
			`parsing time \"x\" as \"2006-01-02T15:04:05Z07:00\": ` +
			`cannot parse \"x\" as \"2006\""}]}`
		assert.Equal(t, want, string(jsonkit.To(t, err)))
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

func Test_UnmarshalJSONReader_tabular(t *testing.T) {
	tt := []struct {
		testN string

		data string
	}{
		{"error - garbage", `{"f_data": 1} x`},
		{"error - incomplete string", `{"f_data": 1} "x`},
		{"error - incomplete literal", `{"f_data": 1} tru`},
		{"error - number out of range", `{"f_data": 1} 1e999`},
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

		dst := &data{}

		// --- When ---
		err := UnmarshalJSONBody(req, dst)

		// --- Then ---
		assert.Same(t, ErrBind, err)
	})

	t.Run("error - nil request", func(t *testing.T) {
		// --- Given ---
		dst := &data{}

		// --- When ---
		err := UnmarshalJSONBody(nil, dst)

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

	t.Run("translated error", func(t *testing.T) {
		// --- Given ---
		var out struct{ A int }
		e := HUE(json.Unmarshal([]byte("[]"), &out))

		// --- When ---
		err := HUE(e)

		// --- Then ---
		assert.Equal(t, e, err)
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

func Test_translated(t *testing.T) {
	inv := xrr.Envelop(errors.New("e"), xrr.ErrInvJSON)
	bnd := xrr.Envelop(errors.New("e"), ErrBind)
	other := xrr.Envelop(errors.New("e"), errors.New("lead"))

	tt := []struct {
		testN string
		err   error
		want  bool
	}{
		{"invalid JSON lead", inv, true},
		{"binding lead", bnd, true},
		{"other lead", other, false},
		{"not an envelope", errors.New("e"), false},
	}

	for _, tc := range tt {
		t.Run(tc.testN, func(t *testing.T) {
			// --- When ---
			have := translated(tc.err)

			// --- Then ---
			assert.Equal(t, tc.want, have)
		})
	}
}

func Test_HUEFor(t *testing.T) {
	t.Run("nil error", func(t *testing.T) {
		// --- When ---
		err := HUEFor(&TJSONInner{}, nil)

		// --- Then ---
		assert.Nil(t, err)
	})

	t.Run("wrong type in embedded struct", func(t *testing.T) {
		// --- Given ---
		dst := &struct{ *TJSONInner }{}
		ute := json.Unmarshal([]byte(`{"name": 1}`), dst)

		// --- When ---
		err := HUEFor(dst, ute)

		// --- Then ---
		assert.ErrorIs(t, ErrBind, err)
		want := `
		{
			"code": "ECBinding",
			"error": "binding error",
			"fields": {
				"name": {"code": "ECInvType", "error": "invalid type"}
			}
		}`
		assert.JSON(t, want, string(jsonkit.To(t, err)))
	})

	t.Run("error not about a field", func(t *testing.T) {
		// --- Given ---
		se := json.Unmarshal([]byte("{!!!}"), &TJSONInner{})

		// --- When ---
		err := HUEFor(&TJSONInner{}, se)

		// --- Then ---
		assert.Equal(t, HUE(se), err)
	})
}

func Test_jsonFieldError(t *testing.T) {
	t.Run("rewrites a copy", func(t *testing.T) {
		// --- Given ---
		ute := &json.UnmarshalTypeError{Field: "TJSONInner.name"}
		dst := &struct{ *TJSONInner }{}

		// --- When ---
		err := jsonFieldError(dst, ute)

		// --- Then ---
		have, ok := errors.AsType[*json.UnmarshalTypeError](err)
		assert.True(t, ok)
		assert.Equal(t, "name", have.Field)
		assert.Equal(t, "TJSONInner.name", ute.Field)
	})

	t.Run("not a type error", func(t *testing.T) {
		// --- Given ---
		e := errors.New("e")

		// --- When ---
		err := jsonFieldError(&TJSONInner{}, e)

		// --- Then ---
		assert.Same(t, e, err)
	})

	t.Run("type error without field", func(t *testing.T) {
		// --- Given ---
		ute := &json.UnmarshalTypeError{}

		// --- When ---
		err := jsonFieldError(&TJSONInner{}, ute)

		// --- Then ---
		assert.Same(t, ute, err)
	})

	t.Run("nil dst", func(t *testing.T) {
		// --- Given ---
		ute := &json.UnmarshalTypeError{Field: "TJSONInner.name"}

		// --- When ---
		err := jsonFieldError(nil, ute)

		// --- Then ---
		assert.Same(t, ute, err)
	})
}

func Test_jsonPath_tabular(t *testing.T) {
	type embPtr struct {
		*TJSONInner
		X int `json:"x"`
	}
	type embVal struct{ TJSONInner }
	type named struct {
		In TJSONInner `json:"in"`
	}
	type tagged struct {
		TJSONInner `json:"inner"`
	}
	type list struct {
		L []embPtr `json:"l"`
	}
	type unm struct {
		T TJSONTime `json:"t"`
	}
	type hidden struct {
		Skip TJSONInner `json:"-"`
	}

	tt := []struct {
		testN string
		typ   reflect.Type
		path  string
		want  string
	}{
		{
			"embedded pointer",
			reflect.TypeFor[embPtr](),
			"TJSONInner.name",
			"name",
		},
		{"embedded value", reflect.TypeFor[embVal](), "TJSONInner.Num", "Num"},
		{"own field", reflect.TypeFor[embPtr](), "x", "x"},
		{
			"pointer to pointer",
			reflect.TypeFor[**embPtr](),
			"TJSONInner.name",
			"name",
		},
		{"named field", reflect.TypeFor[named](), "in.name", "in.name"},
		{
			"tagged embedded",
			reflect.TypeFor[tagged](),
			"inner.name",
			"inner.name",
		},
		{
			"slice element",
			reflect.TypeFor[list](),
			"l.TJSONInner.name",
			"l.name",
		},
		{
			"unknown segment",
			reflect.TypeFor[embPtr](),
			"zzz.TJSONInner",
			"zzz.TJSONInner",
		},
		{"json unmarshaler", reflect.TypeFor[unm](), "t.T.f_tim", "t.T.f_tim"},
		{"ignored field", reflect.TypeFor[hidden](), "Skip.name", "Skip.name"},
		{"not a struct", reflect.TypeFor[int](), "a.b", "a.b"},
	}

	for _, tc := range tt {
		t.Run(tc.testN, func(t *testing.T) {
			// --- When ---
			have := jsonPath(tc.typ, tc.path)

			// --- Then ---
			assert.Equal(t, tc.want, have)
		})
	}
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
		assert.ErrorIs(t, ute, err)
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
		assert.ErrorIs(t, e, err)
	})

	t.Run("time ParseError", func(t *testing.T) {
		// --- Given ---
		_, pe := time.Parse(time.RFC3339, "2022")

		// --- When ---
		have, err := MaybeHUE(pe)

		// --- Then ---
		assert.ErrorIs(t, ErrTimeFormat, err)
		assert.ErrorIs(t, ErrBind, err)
		assert.ErrorIs(t, pe, err)
		want := "" +
			`{"code":"ECInvTimeFormat","error":"invalid RFC3339 time; ` +
			`parsing time \"2022\" as `
		assert.Contain(t, want, string(jsonkit.To(t, err)))
		assert.True(t, have)
	})

	t.Run("time error already translated", func(t *testing.T) {
		// --- Given ---
		_, pe := time.Parse(time.RFC3339, "2022")
		_, e := MaybeHUE(pe)

		// --- When ---
		have, err := MaybeHUE(e)

		// --- Then ---
		assert.True(t, have)
		assert.Equal(t, e, err)
	})

	t.Run("query field time error", func(t *testing.T) {
		// --- Given ---
		dst := &TWithTime{}
		e := NewDecoder().Decode(dst, url.Values{"f_time": {"x"}})

		// --- When ---
		have, err := MaybeHUE(e)

		// --- Then ---
		assert.False(t, have)
		assert.Equal(t, e, err)
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
		var e xrr.Envelope
		assert.ErrorAs(t, &e, err)
		assert.ErrorIs(t, xrr.ErrInvJSON, e.Lead())
		assert.ErrorEqual(t, "unmarshal type mismatch", e.Unwrap())
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
