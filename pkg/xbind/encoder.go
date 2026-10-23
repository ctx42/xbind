// SPDX-FileCopyrightText: (c) 2026 Rafal Zajac
// SPDX-License-Identifier: MIT

package xbind

import (
	"encoding"
	"net/url"
	"reflect"
	"strconv"
	"strings"

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
	reg encReg // Encoders registered with WithEncFunc.
}

// EncoderOption configures an [Encoder] built by [NewEncoder].
type EncoderOption func(*Encoder)

// WithEncTag is a [NewEncoder] option setting the struct tag name used for
// field name aliases and options.
func WithEncTag(tag string) EncoderOption {
	return func(enc *Encoder) { enc.tag = tag }
}

// WithEncFunc is a [NewEncoder] option encoding every value of type T with
// fn: fields of type T or *T, and slice, array and map elements and map keys
// of those types. The function takes precedence over the [Marshaler] and
// [encoding.TextMarshaler] methods of T and the encoders the package
// provides, [time.Duration] included.
func WithEncFunc[T any](fn func(T) (string, error)) EncoderOption {
	typ := reflect.TypeFor[T]()
	return func(enc *Encoder) {
		if enc.reg == nil {
			enc.reg = make(encReg)
		}
		enc.reg[typ] = func(v reflect.Value) (string, error) {
			val, _ := reflect.TypeAssert[T](v)
			return fn(val)
		}
	}
}

// NewEncoder returns a new instance of [Encoder] reading the "url" struct tag
// unless the [WithEncTag] option says otherwise.
func NewEncoder(opts ...EncoderOption) *Encoder {
	enc := &Encoder{tag: defaultTag}
	for _, opt := range opts {
		opt(enc)
	}
	return enc
}

// Encode encodes the src struct, or a pointer to it, into dst; a nil dst is the
// [ErrNilDst] error and any other src the [ErrReqStruct] error. Among the
// fields of a struct and the structs it embeds, a key names the field Go
// promotion selects, as in [encoding/json]. Each exported field is added under
// its path, written as [Decoder.Decode] reads it: struct field names after a
// dot, or in brackets when they contain one; map keys, and the indexes of slice
// and array elements that are structs, maps or [Marshaler] values, in brackets
// ("attr[k]", "list[0].name"); other slice and array elements as values of one
// key. A map key that is empty or holds a bracket is an [ErrInvValue] error. A
// [Marshaler] implemented by src, or by a struct embedded in it, encodes the
// whole struct; [encoding.TextMarshaler] and functions registered with
// [WithEncFunc] apply only to values with a key, so src and the structs
// embedded in it are encoded field by field. A nil pointer or interface adds
// nothing: a nil field has no key, a map entry with a nil key or value is left
// out, and a nil slice or array element has no value, so the elements after it
// move up one place when decoded. A pointer to a struct already being encoded,
// a pointer cycle, is an [ErrCycle] error keyed by the field path of the
// repeated pointer, and a struct tag of src that does not parse is an
// [ErrInvTag] error.
// On error, dst keeps the values encoded before the error, which may include
// some values of the failing field.
func (enc *Encoder) Encode(dst url.Values, src any) error {
	if dst == nil {
		return ErrNilDst
	}
	val := reflect.ValueOf(src)
	if reflect.Indirect(val).Kind() != reflect.Struct {
		return ErrReqStruct
	}
	seen := map[reflect.Type]bool{}
	if err := checkTags("", val.Type(), enc.tagName(), seen); err != nil {
		return err
	}
	if val.Kind() != reflect.Pointer {
		// Copy src to an addressable value, so the methods with pointer
		// receivers are found.
		ptr := reflect.New(val.Type())
		ptr.Elem().Set(val)
		val = ptr
	}
	return enc.encode(dst, "", val, map[ptrKey]bool{}, level{})
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
// The lvl is the level position of an embedded struct src; for any other src
// it is the zero level.
func (enc *Encoder) encode(
	dst url.Values,
	field string,
	src reflect.Value,
	seen map[ptrKey]bool,
	lvl level,
) error {

	// The src struct and the structs embedded in it are encoded inline.
	inline := field == "" || lvl.idx != nil
	if ok, err := encSrc(dst, field, src, enc.reg, inline); ok {
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
		return enc.encodeSlice(dst, field, src, seen)

	case reflect.Map:
		return enc.encodeMap(dst, field, src, seen)

	case reflect.Struct:
		return enc.encodeFields(dst, field, src, seen, lvl)

	default:
		return fieldError(field, encNotFound(src.Type()))
	}
}

// encodeFields encodes the fields of the src struct to dst under the field
// path. The seen map is passed to the encode method. The lvl is the level
// position of src, the zero level for a struct starting a level; a field its
// key does not name there is skipped.
func (enc *Encoder) encodeFields(
	dst url.Values,
	field string,
	src reflect.Value,
	seen map[ptrKey]bool,
	lvl level,
) error {

	if lvl.vis == nil {
		lvl = newLevel(src.Type(), enc.tagName())
	}
	md := mirror.NewTypeMetadata(src.Type())
	for i := 0; i < src.NumField(); i++ {
		fld := md.FieldByIndex(i)
		tag, _ := fld.Tag(enc.tagName()) // Encode checked the tags.
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
			err := enc.encode(dst, field, val, seen, lvl.embedded(i))
			if err != nil {
				return err
			}
			continue
		}
		name := tag.NameOrField()
		if !fld.IsExported() || !lvl.names(name, i) {
			continue
		}

		fldPath := fieldPath(field, name)
		if err := enc.encode(dst, fldPath, val, seen, level{}); err != nil {
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
// It first checks for an encoder registered for the src type, then if src, or
// its address, implements [Marshaler], then [encoding.TextMarshaler]; lastly,
// it calls encFor to find an encoder. For an inline src, the struct passed to
// [Encoder.Encode] or a struct embedded in a struct, only [Marshaler] is
// used. A nil pointer or interface with an encoder adds nothing to dst and
// returns true.
func encSrc(
	dst url.Values,
	prefix string,
	src reflect.Value,
	reg encReg,
	inline bool,
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
	if inline {
		// An inline value is never nil: only a Marshaler, which adds keys
		// under the prefix, encodes it.
		if !typ.Implements(iMarshaler) || !src.CanInterface() {
			return false, nil
		}
		m := src.Interface().(Marshaler) //nolint:forcetypeassert
		return true, m.MarshalValues(dst, prefix)
	}
	efn := reg.lookup(typ)
	// The methods of a value read from an unexported field cannot be called.
	if efn == nil && isMarshaler(typ) && src.CanInterface() {
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

	if efn == nil {
		efn = encFor(typ, reg)
	}
	if efn != nil {
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

// encodeSlice encodes the src slice or array to dst. Elements encoded as one
// value (see isValueType) are added as values of the prefix key; any other
// element, a struct, a map or a [Marshaler], is encoded under the prefix key
// and its index ("list[0]"). The seen map is passed to the encode method.
func (enc *Encoder) encodeSlice(
	dst url.Values,
	prefix string,
	src reflect.Value,
	seen map[ptrKey]bool,
) error {

	value := enc.isValueType(src.Type().Elem())
	for i := 0; i < src.Len(); i++ {
		val := src.Index(i)
		ipt := prefix + "[" + strconv.Itoa(i) + "]"
		if value {
			if ok, err := encSrc(dst, prefix, val, enc.reg, false); ok {
				if err != nil {
					return xrr.NewFieldError(ipt, err)
				}
				continue
			}
		}
		if err := enc.encode(dst, ipt, val, seen, level{}); err != nil {
			return err
		}
	}
	return nil
}

// encodeMap encodes the src map to dst, each value under the prefix key and
// its map key ("attr[k]"). The keys are encoded with
// [encoding.TextMarshaler] or the base type encoders; an empty key, or one
// with a bracket, is an error. The seen map is passed to the encode method.
func (enc *Encoder) encodeMap(
	dst url.Values,
	prefix string,
	src reflect.Value,
	seen map[ptrKey]bool,
) error {

	for it := src.MapRange(); it.Next(); {
		mk := it.Key()
		if isNil(mk) || mk.Kind() == reflect.Interface && isNil(mk.Elem()) {
			continue // A nil key adds nothing.
		}
		key, err := encMapKey(mk, enc.reg)
		if err != nil {
			return fieldError(prefix, err)
		}
		var format string
		switch {
		case key == "":
			format = "%w: empty map key"

		case strings.ContainsAny(key, "[]"):
			format = "%w: map key with a bracket"
		}
		if format != "" {
			err = xrr.Newf(format, ErrInvValue, xrr.WithCode(ECInvValue))
			return fieldError(prefix, err)
		}
		pth := prefix + "[" + key + "]"
		val := addressable(it.Value())
		if err = enc.encode(dst, pth, val, seen, level{}); err != nil {
			return err
		}
	}
	return nil
}

// isValueType returns true if a value of the typ type, or a pointer to one,
// is encoded as one text value: by a registered encoder, by its
// [encoding.TextMarshaler], or by a built-in encoder. A [Marshaler] is not.
func (enc *Encoder) isValueType(typ reflect.Type) bool {
	typ = indirect(typ)
	if enc.reg.lookup(typ) != nil {
		return true
	}
	ptr := reflect.PointerTo(typ)
	if typ.Implements(iMarshaler) || ptr.Implements(iMarshaler) {
		return false
	}
	if typ.Implements(iTextMarshaler) || ptr.Implements(iTextMarshaler) {
		return true
	}
	return encFor(typ, enc.reg) != nil
}

// encMapKey encodes the map key to a string. The key must not be nil or an
// interface holding a nil pointer.
func encMapKey(key reflect.Value, reg encReg) (string, error) {
	if key.Kind() == reflect.Interface {
		key = key.Elem()
	}
	if efn := reg.lookup(key.Type()); efn != nil {
		return efn(key)
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
		return "", encNotFound(typ)
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

// encNotFound returns an error wrapping [ErrEncNotFound] naming the typ type.
func encNotFound(typ reflect.Type) error {
	code := xrr.WithCode(ECEncNotFound)
	return xrr.Newf("%w for %#q", ErrEncNotFound, typ.String(), code)
}
