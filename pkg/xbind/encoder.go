// SPDX-FileCopyrightText: (c) 2026 Rafal Zajac
// SPDX-License-Identifier: MIT

package xbind

import (
	"encoding"
	"net/url"
	"reflect"
	"strconv"

	"github.com/ctx42/xrr/pkg/xrr"

	"github.com/ctx42/mirror/pkg/mirror"
)

// iMarshaler is used to check if a type implements the [Marshaler] interface.
var iMarshaler = reflect.TypeFor[Marshaler]()

// iTextMarshaler is used to check if a type implements the
// [encoding.TextMarshaler] interface.
var iTextMarshaler = reflect.TypeFor[encoding.TextMarshaler]()

// Encoder encodes values from a struct into [url.Values]. The zero value is
// ready to use and reads the "url" struct tag.
type Encoder struct {
	tag string // Struct field tag to use for aliasing.
	reg encReg // Custom encoders for types.
}

// WithEncTag is a [NewEncoder] option setting the struct tag name used for
// field name aliases and options.
func WithEncTag(tag string) func(*Encoder) {
	return func(enc *Encoder) { enc.tag = tag }
}

// NewEncoder returns a new instance of [Encoder] reading the "url" struct tag
// unless the [WithEncTag] option says otherwise.
func NewEncoder(opts ...func(*Encoder)) *Encoder {
	enc := &Encoder{
		tag: defaultTag,
		reg: make(encReg),
	}
	for _, opt := range opts {
		opt(enc)
	}
	return enc
}

// Encode encodes the src struct, or a pointer to it, into dst, which must not
// be nil. Each exported field is added under its path; the path of a nested
// struct field is the dotted list of names leading to it. A [Marshaler]
// implemented by src, or by a struct embedded in it, encodes the whole struct;
// an [encoding.TextMarshaler] implemented by src adds its text under the
// empty key and the fields are not encoded. A nil pointer or interface adds
// nothing: a nil field has no key, a map entry with a nil key or value is
// left out, and a nil slice or array element has no value, so the elements
// after it move up one place when decoded. A pointer to a struct already
// being encoded, a pointer cycle, is an [ErrCycle] error keyed by the field
// path of the repeated pointer.
// On error, dst keeps the values encoded before the error, which may include
// some values of the failing field.
func (enc *Encoder) Encode(dst url.Values, src any) error {
	val := reflect.ValueOf(src)
	if reflect.Indirect(val).Kind() != reflect.Struct {
		return ErrReqStructPtr
	}
	if val.Kind() != reflect.Pointer {
		// Copy src to an addressable value, so the methods with pointer
		// receivers are found.
		ptr := reflect.New(val.Type())
		ptr.Elem().Set(val)
		val = ptr
	}
	return enc.encode(dst, "", val, map[ptrKey]bool{})
}

// tagName returns the name of the struct tag the encoder reads.
func (enc *Encoder) tagName() string {
	if enc.tag == "" {
		return defaultTag
	}
	return enc.tag
}

// encode encodes src to dst under the field path. The seen map holds the
// pointers on the path to src; meeting one of them again is a pointer cycle.
func (enc *Encoder) encode(
	dst url.Values,
	field string,
	src reflect.Value,
	seen map[ptrKey]bool,
) error {

	if ok, err := encSrc(dst, field, src, enc.reg); ok {
		return fieldError(field, err)
	}

	if src.Kind() == reflect.Pointer {
		switch src.Type().Elem().Kind() { //nolint:exhaustive
		case reflect.Slice, reflect.Array, reflect.Map, reflect.Struct:
			if src.IsNil() {
				return nil
			}
			key := ptrKey{ptr: src.Pointer(), typ: src.Type()}
			if seen[key] {
				return fieldError(field, ErrCycle)
			}
			seen[key] = true
			defer delete(seen, key)
			src = src.Elem()
		}
	}

	switch src.Kind() { //nolint:exhaustive
	case reflect.Slice, reflect.Array:
		return encSliceOrArray(dst, field, src, enc.reg)

	case reflect.Map:
		return encMap(dst, field, src, enc.reg)

	case reflect.Struct:
		return enc.encodeFields(dst, field, src, seen)

	default:
		return fieldError(field, ErrEncNotFound(src.Type().String()))
	}
}

// encodeFields encodes the fields of the src struct to dst under the field
// path. The seen map is passed to the encode method.
func (enc *Encoder) encodeFields(
	dst url.Values,
	field string,
	src reflect.Value,
	seen map[ptrKey]bool,
) error {

	md := mirror.NewTypeMetadata(src.Type())
	for i := 0; i < src.NumField(); i++ {
		fld := md.FieldByIndex(i)
		tag, _ := fld.Tag(enc.tagName()) // A malformed tag reads as no tag.
		if tag.IsIgnored() {
			continue
		}

		val := src.Field(i)
		if tag.Contains("omitempty") && isZero(val) {
			continue
		}

		if fld.IsAnonymous() && fld.Kind() == reflect.Interface {
			continue // Like the decoder, skip embedded interfaces.
		}
		if fld.IsAnonymous() && indirect(fld.Type()).Kind() == reflect.Struct {
			if isNil(val) {
				continue
			}
			// The fields of an embedded struct belong to the outer struct.
			if err := enc.encode(dst, field, val, seen); err != nil {
				return err
			}
			continue
		}
		if !fld.IsExported() {
			continue
		}

		fldPath := buildPath(field, tag.NameOrField())
		if err := enc.encode(dst, fldPath, val, seen); err != nil {
			return err
		}
	}
	return nil
}

// ptrKey identifies a pointer on the encoding path. The type tells apart a
// pointer to a struct and a pointer to its first field.
type ptrKey struct {
	ptr uintptr
	typ reflect.Type
}

// encSrc encodes src to dst. It returns false if the encoder for the src value
// could not be found, true otherwise. When it returns true, check the error;
// when it returns false, the error is always nil.
//
// It first checks if src, or its address, implements [Marshaler], then
// [encoding.TextMarshaler]; lastly, it calls encFor to find an encoder. A nil
// pointer or interface with an encoder adds nothing to dst and returns true.
func encSrc(
	dst url.Values,
	prefix string,
	src reflect.Value,
	reg encReg,
) (bool, error) {

	// Look into an interface implementing a marshaler for the pointer it
	// may hold.
	isItf := src.Kind() == reflect.Interface
	if isItf && !src.IsNil() && isMarshaler(src.Type()) {
		src = src.Elem()
	}
	// Reduce pointers to pointers to a single pointer.
	for src.Kind() == reflect.Pointer && !src.IsNil() &&
		src.Type().Elem().Kind() == reflect.Pointer {

		src = src.Elem()
	}
	if src.Kind() != reflect.Pointer && src.CanAddr() {
		if ptr := src.Addr(); isMarshaler(ptr.Type()) {
			src = ptr
		}
	}

	typ := src.Type()
	// The methods of a value read from an unexported field cannot be called.
	if isMarshaler(typ) && src.CanInterface() {
		if isNil(src) {
			return true, nil
		}
		if m, ok := src.Interface().(Marshaler); ok {
			return true, m.MarshalValues(dst, prefix)
		}
		m := src.Interface().(encoding.TextMarshaler) //nolint:forcetypeassert
		text, err := m.MarshalText()
		if err != nil {
			return true, err
		}
		dst[prefix] = append(dst[prefix], string(text))
		return true, nil
	}

	if efn := encFor(typ, reg); efn != nil {
		if isNil(src) {
			return true, nil
		}
		v, err := efn(src)
		if err != nil {
			return true, err
		}
		dst[prefix] = append(dst[prefix], v)
		return true, nil
	}
	return false, nil
}

// isMarshaler returns true if typ implements [Marshaler] or
// [encoding.TextMarshaler].
func isMarshaler(typ reflect.Type) bool {
	return typ.Implements(iMarshaler) || typ.Implements(iTextMarshaler)
}

// encSliceOrArray encodes the src slice or array to dst.
func encSliceOrArray(
	dst url.Values,
	prefix string,
	src reflect.Value,
	reg encReg,
) error {

	for i := 0; i < src.Len(); i++ {
		val := src.Index(i)
		ok, err := encSrc(dst, prefix, val, reg)
		if !ok {
			return fieldError(prefix, ErrEncNotFound(val.Type().String()))
		}
		if err != nil {
			return xrr.NewFieldError(buildPath(prefix, strconv.Itoa(i)), err)
		}
	}
	return nil
}

// encMap encodes the src map to dst. The keys are encoded with
// [encoding.TextMarshaler] or the base type encoders, the values like slice
// elements, or as slices.
func encMap(
	dst url.Values,
	prefix string,
	src reflect.Value,
	reg encReg,
) error {

	for it := src.MapRange(); it.Next(); {
		mk := it.Key()
		if isNil(mk) || mk.Kind() == reflect.Interface && isNil(mk.Elem()) {
			continue // A nil key adds nothing.
		}
		key, err := encMapKey(mk, reg)
		if err != nil {
			return fieldError(prefix, err)
		}
		if key == "" {
			const format = "%w: empty map key"
			err = xrr.Newf(format, ErrInvValue, xrr.WithCode(ECInvValue))
			return fieldError(prefix, err)
		}

		mvv := addressable(it.Value())
		pth := buildPath(prefix, key)
		if ok, err := encSrc(dst, pth, mvv, reg); ok {
			if err != nil {
				return xrr.NewFieldError(pth, err)
			}
			continue
		}

		if knd := mvv.Kind(); knd == reflect.Slice || knd == reflect.Array {
			if err = encSliceOrArray(dst, pth, mvv, reg); err != nil {
				return err
			}
			continue
		}
		return xrr.NewFieldError(pth, ErrEncNotFound(mvv.Type().String()))
	}
	return nil
}

// encMapKey encodes the map key to a string. The key must not be nil or an
// interface holding a nil pointer.
func encMapKey(key reflect.Value, reg encReg) (string, error) {
	if key.Kind() == reflect.Interface {
		key = key.Elem()
	}
	if key.Kind() != reflect.Pointer {
		ptr := addressable(key).Addr()
		if ptr.Type().Implements(iTextMarshaler) {
			key = ptr
		}
	}
	typ := key.Type()
	if typ.Implements(iTextMarshaler) {
		m := key.Interface().(encoding.TextMarshaler) //nolint:forcetypeassert
		text, err := m.MarshalText()
		return string(text), err
	}
	efn := encFor(typ, reg)
	if efn == nil {
		return "", ErrEncNotFound(typ.String())
	}
	return efn(key)
}

// fieldError returns err as a field error for the field path. It returns err
// as is when it is nil or the path is empty.
func fieldError(field string, err error) error {
	if err == nil || field == "" {
		return err
	}
	return xrr.NewFieldError(field, err)
}
