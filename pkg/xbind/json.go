// SPDX-FileCopyrightText: (c) 2026 Rafal Zajac
// SPDX-License-Identifier: MIT

package xbind

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"reflect"
	"strings"
	"time"

	"github.com/ctx42/xrr/pkg/xrr"
)

// UnmarshalJSON unmarshals JSON data to dst; see [UnmarshalJSONReader].
func UnmarshalJSON(data []byte, dst any) error {
	return UnmarshalJSONReader(bytes.NewReader(data), dst)
}

// UnmarshalJSONReader unmarshals the single JSON value read from r to dst.
// Unknown object fields and data after the value are errors. Errors are
// translated with [HUE]; a nil r returns [ErrBind].
func UnmarshalJSONReader(r io.Reader, dst any) error {
	if r == nil {
		return ErrBind
	}
	dec := json.NewDecoder(r)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return HUE(err)
	}
	_, err := dec.Token()
	if errors.Is(err, io.EOF) {
		return nil
	}
	_, syn := errors.AsType[*json.SyntaxError](err)
	// A number too large for a float64 is a type error.
	_, ute := errors.AsType[*json.UnmarshalTypeError](err)
	if err != nil && !syn && !ute && !errors.Is(err, io.ErrUnexpectedEOF) {
		return HUE(err) // Reading after the value failed.
	}
	err = xrr.New("trailing data after JSON value", xrr.ECInvJSON)
	return xrr.Envelop(err, xrr.ErrInvJSON)
}

// UnmarshalJSONBody unmarshals the JSON request body to dst, see
// [UnmarshalJSONReader], and closes the body. A nil request or body returns
// [ErrBind].
func UnmarshalJSONBody(req *http.Request, dst any) error {
	if req == nil || req.Body == nil {
		return ErrBind
	}
	defer func() { _ = req.Body.Close() }()
	return UnmarshalJSONReader(req.Body, dst)
}

// HUE handles low-level JSON unmarshal errors. It returns the error [MaybeHUE]
// translates err to, and a [json.InvalidUnmarshalError] (a nil or non-pointer
// destination, a programming error) as it is; any other error is enveloped with
// [ErrBind], which replaces the lead of an error that is already an
// [xrr.Envelope], such as one an UnmarshalJSON method returns. It returns nil
// when err is nil.
func HUE(err error) error {
	if err == nil {
		return nil
	}
	if ok, e := MaybeHUE(err); ok {
		return e
	}
	if _, ok := errors.AsType[*json.InvalidUnmarshalError](err); ok {
		return err
	}
	return xrr.Envelop(err, ErrBind)
}

// MaybeHUE handles low-level JSON unmarshal errors. It returns true and the
// handled error if the function managed the passed error in any way;
// otherwise, it returns false and the original passed error.
//
// For a nil error, it returns false and nil.
func MaybeHUE(err error) (bool, error) {
	if err == nil {
		return false, nil
	}
	if ute, ok := errors.AsType[*json.UnmarshalTypeError](err); ok {
		// Since Go 1.27 a JSON value of the wrong type for a time.Time field
		// is reported as a type error instead of the error returned by
		// time.Time.UnmarshalJSON.
		if ute.Type != nil && indirect(ute.Type) == typTime {
			return true, xrr.Envelop(ErrTimeFormat, ErrBind)
		}
		if ute.Field != "" {
			ue := xrr.NewFieldError(ute.Field, ErrType)
			return true, xrr.Envelop(ue, ErrBind)
		}
		if ute.Type == nil {
			return true, xrr.ErrInvJSON
		}
		const format = "unmarshal type mismatch; expected %s"
		typ := jsonType(ute.Type)
		err = xrr.Newf(format, typ, xrr.WithCode(xrr.ECInvJSON))
		return true, xrr.Envelop(err, xrr.ErrInvJSON)
	}

	if se, ok := errors.AsType[*json.SyntaxError](err); ok {
		err = xrr.New(se.Error(), xrr.ECInvJSON)
		return true, xrr.Envelop(err, xrr.ErrInvJSON)
	}

	if _, ok := errors.AsType[*time.ParseError](err); ok {
		return true, xrr.Envelop(ErrTimeFormat, ErrBind)
	}

	// Before the time check: the field name may hold its text.
	if strings.HasPrefix(err.Error(), "json: unknown field ") {
		return true, xrr.Envelop(err, ErrBind)
	}

	// Since Go 1.20 time.Time.UnmarshalJSON returns errors other than
	// *time.ParseError for some invalid inputs.
	if strings.Contains(err.Error(), "Time.UnmarshalJSON") {
		return true, xrr.Envelop(ErrTimeFormat, ErrBind)
	}

	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		err = xrr.New("unexpected end of JSON", xrr.ECInvJSON)
		return true, xrr.Envelop(err, ErrBind)
	}

	// Do not wrap unrecognized errors - they may be caused by reasons other
	// than binding or invalid JSON.
	return false, err
}

// jsonType returns the name of the JSON type a value of type typ decodes from.
func jsonType(typ reflect.Type) string {
	typ = indirect(typ)
	if reflect.PointerTo(typ).Implements(iUnmarshaler) {
		return "string"
	}
	if typ.Kind() == reflect.Slice && typ.Elem().Kind() == reflect.Uint8 {
		return "string" // Base64 encoded bytes.
	}
	switch typ.Kind() { //nolint:exhaustive
	case reflect.Array, reflect.Slice:
		return "array"

	case reflect.Map, reflect.Struct:
		return "object"

	case reflect.String:
		return "string"

	case reflect.Bool:
		return "boolean"

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32,
		reflect.Uint64, reflect.Uintptr, reflect.Float32, reflect.Float64:
		return "number"

	default:
		return "value"
	}
}
