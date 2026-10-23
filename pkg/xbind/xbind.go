// SPDX-FileCopyrightText: (c) 2026 Rafal Zajac
// SPDX-License-Identifier: MIT

// Package xbind binds HTTP request data to Go structs and back.
//
// A [Decoder] sets struct fields from [url.Values] (query strings, form
// bodies) and an [Encoder] turns a struct into [url.Values]. Both read the
// "url" struct tag for the field name and options:
//
//	type Query struct {
//		Name  string            `url:"name,required"`  // Must be present.
//		Tags  []string          `url:"tag"`            // Repeated key.
//		Attrs map[string]string `url:"attr"`           // Keys attr.<key>.
//		Page  *Page             `url:"page"`           // Keys page.<field>.
//		Note  string            `url:"note,omitempty"` // Skipped when empty.
//		Skip  int               `url:"-"`              // Never bound.
//	}
//
// A key is the dotted path to a field: the fields of a nested struct are
// prefixed by the key of the struct field, map entries by the key of the map
// field, and the fields of embedded structs belong to the outer struct.
// Slices and arrays take one key with many values. Supported are bool,
// string, the int, uint (except uintptr) and float kinds, their named
// variants, [time.Time] (RFC 3339), [time.Duration], and types implementing
// [encoding.TextUnmarshaler] / [encoding.TextMarshaler] or [Marshaler].
//
// [UnmarshalJSON], [UnmarshalJSONReader] and [UnmarshalJSONBody] decode JSON
// bodies strictly (unknown fields and trailing data are errors) and translate
// [encoding/json] errors with [HUE].
//
// Errors are [xrr] errors with codes; errors returned by marshalers and
// unmarshalers pass through. Decoding problems of individual values come
// back as [xrr.FieldErrors] keyed by the source key; use [EncloseError] to
// put them under the [ErrBind] lead error.
package xbind

import (
	"net/url"

	"github.com/ctx42/xrr/pkg/xrr"
)

// defaultTag is the struct tag name read by [Encoder] and [Decoder].
const defaultTag = "url"

// Marshaler is the interface implemented by types that can marshal themselves
// into [url.Values]. The [Encoder] does not call it on a nil pointer, a nil
// Marshaler interface, or a Marshaler interface holding a nil pointer; such a
// value adds nothing. A method promoted from a nil embedded pointer is still
// called, with a nil receiver.
type Marshaler interface {
	// MarshalValues adds the values of the type to dst under the prefix key,
	// or under keys starting with the prefix. The prefix is empty for the
	// struct passed to [Encoder.Encode] and for the structs it embeds.
	MarshalValues(dst url.Values, prefix string) error
}

// Error codes.
const (
	ECBinding = "ECBinding"

	ECReqStructPtr = "ECReqStructPtr"

	ECUnkFieldPath = "ECUnkFieldPath"

	// ECDecNotFound represents the missing type decoder error code.
	ECDecNotFound = "ECDecoderNotFound"

	// ECEncNotFound represents the missing type encoder error code.
	ECEncNotFound = "ECEncoderNotFound"

	// ECRequired represents the error code for a required field that was not
	// set. Only [Decoder] sets it.
	ECRequired = "ECRequired"

	// ECInvTimeFormat represents the error code for field values with an
	// invalid RFC 3339 time format.
	ECInvTimeFormat = "ECInvTimeFormat"

	// ECInvDurFormat represents the error code for field values with an
	// invalid [time.Duration] string representation.
	ECInvDurFormat = "ECInvDurFormat"

	ECInvType = "ECInvType"

	// ECInvValue represents the error code for values that cannot be decoded
	// from a string to a type.
	ECInvValue = "ECInvValue"

	// ECReqSingle represents the error code used when a single value is
	// expected but none or more than one were given.
	ECReqSingle = "ECReqSingle"

	// ECUnexported represents the error code for an embedded struct the
	// decoder cannot reach because it is an unexported nil pointer.
	ECUnexported = "ECUnexported"

	// ECCycle represents the error code for a pointer to a struct already
	// being encoded. Only [Encoder] sets it.
	ECCycle = "ECCycle"
)

// Binding errors.
var (
	// ErrBind represents a binding error either from the request body or from
	// the query parameters.
	ErrBind = xrr.New("binding error", ECBinding)

	// ErrReqStructPtr represents the struct pointer required error.
	ErrReqStructPtr = xrr.New("struct pointer required", ECReqStructPtr)

	// ErrType represents a mismatched field type during binding.
	ErrType = xrr.New("invalid type", ECInvType)

	// ErrTimeFormat represents an invalid time format when binding.
	ErrTimeFormat = xrr.New("invalid RFC3339 time", ECInvTimeFormat)

	// ErrDurFormat represents an invalid [time.Duration] string representation
	// format when binding.
	ErrDurFormat = xrr.New("invalid duration string", ECInvDurFormat)

	// ErrRequired represents an error for missing required fields.
	ErrRequired = xrr.New("cannot be blank", ECRequired)

	// ErrInvValue represents an error for an invalid field value.
	ErrInvValue = xrr.New("invalid value", ECInvValue)

	// ErrReqSingleValue represents an error when none or many values are
	// provided for a field taking a single value.
	ErrReqSingleValue = xrr.New("require single value", ECReqSingle)

	// ErrUnexported represents a nil pointer to an embedded unexported
	// struct, which the decoder can neither allocate nor descend into.
	ErrUnexported = xrr.New("unexported field", ECUnexported)

	// ErrCycle represents a pointer cycle: a pointer to a struct the encoder
	// is already encoding.
	ErrCycle = xrr.New("pointer cycle", ECCycle)

	// ErrUnkFieldPath represents a source key that does not match a field.
	ErrUnkFieldPath = xrr.New("invalid field path", ECUnkFieldPath)

	// ErrDecNotFound represents a type the [Decoder] cannot decode.
	ErrDecNotFound = xrr.New("decoder not found", ECDecNotFound)

	// ErrEncNotFound represents a type the [Encoder] cannot encode.
	ErrEncNotFound = xrr.New("encoder not found", ECEncNotFound)
)

// EncloseError envelops err with [xrr.Envelop] using [ErrBind] as the lead
// error. It returns nil if err is nil.
func EncloseError(err error) error {
	if err == nil {
		return nil
	}
	return xrr.Envelop(err, ErrBind)
}
