// SPDX-FileCopyrightText: (c) 2026 Rafal Zajac
// SPDX-License-Identifier: MIT

package xbind

import (
	"encoding"
	"errors"
	"fmt"
	"net/url"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"time"

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

// WithEncFunc is a [NewEncoder] option encoding every value of type T with fn:
// fields of type T or *T, and slice, array and map elements and map keys of
// those types. The function takes precedence over the [Marshaler] and
// [encoding.TextMarshaler] methods of T and the encoders the package provides,
// [time.Duration] included. For a pointer T, fn encodes the type T points to,
// as [WithDecFunc] decodes it: fn gets the address of the value, or of a copy
// when the value has none, and never a nil pointer. When functions are
// registered for both T and *T, the later one encodes both.
func WithEncFunc[T any](fn func(T) (string, error)) EncoderOption {
	typ := reflect.TypeFor[T]()
	isPtr := typ.Kind() == reflect.Pointer
	if isPtr {
		typ = typ.Elem() // The encoder dereferences pointers first.
	}
	return func(enc *Encoder) {
		if enc.reg == nil {
			enc.reg = make(encReg)
		}
		enc.reg[typ] = func(v reflect.Value) (string, error) {
			if isPtr {
				if !v.CanAddr() {
					v = addressable(v) // A copy has an address.
				}
				v = v.Addr()
			}
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
// [ErrNilDst] error and any other src, a nil struct pointer included, the
// [ErrReqStruct] error. Among the fields of a struct and the structs it embeds,
// a key names the field Go promotion selects, as in [encoding/json]. Each
// exported field is added under its path, written as [Decoder.Decode] reads it:
// struct field names after a dot, or in brackets when they contain one; map
// keys, and the indexes of slice and array elements other than plain values,
// such as structs, maps, slices and [Marshaler] values, in brackets ("attr[k]",
// "list[0].name"); plain values, encoded as one text each, as values of one
// key. A map key that is empty, holds a bracket, or is or holds a NaN is an
// [ErrInvValue] error, and so are two keys of one map encoded to one text; the
// entries of a map are encoded in the order of their key texts. A [Marshaler]
// implemented by src, or by a struct embedded in it, encodes the whole struct;
// [encoding.TextMarshaler] and functions registered with [WithEncFunc] apply
// only to values with a key, so src and the structs embedded in it are encoded
// field by field. [encoding.TextMarshaler] is used when the type, or a pointer
// to it, also implements [encoding.TextUnmarshaler]; otherwise the built-in
// encoder or the fields are used, so the text decodes back to the same value.
// A field, slice or array element, or map value of a type with
// no encoder, such as a func, a chan, an interface other than [Marshaler] and
// [encoding.TextMarshaler] or a pointer to a pointer to a struct, is an
// [ErrEncNotFound] error, even when nil. Any other nil pointer or interface
// adds nothing: a nil field has no key, a map entry with a nil key or value is
// left out, and a nil plain value in a slice or array has no value, so the
// elements after it move up one place when decoded. A value whose [Marshaler]
// or [encoding.TextMarshaler] method the call would reach through a nil
// embedded field counts as nil too (see [Marshaler]). An indexed element or a
// map value adding no keys, such as a nil pointer, an empty slice or a struct
// whose fields are all empty and omitempty, is not written either, so such a
// map entry, or such an element at the end of a slice, does not decode back. A
// pointer to a struct, or a map or slice, already being encoded, a cycle, is an
// [ErrCycle] error keyed by the field path of the repeated value, and a struct
// tag of src that does not parse, or that names a field with a bracket
// ("ids[]") or an embedded struct, is an [ErrInvTag] error; the tags inside a
// type encoded as one value, as by its MarshalText method, are not checked. A
// [time.Time] whose zone offset has seconds, which RFC 3339 cannot write, is
// encoded in UTC, so it decodes to the same instant, when its year in UTC is
// one RFC 3339 can write; a struct embedding a [time.Time] keeps its own
// encoding. On error, dst keeps the values encoded before the error, which may
// include some values of the failing field.
func (enc *Encoder) Encode(dst url.Values, src any) error {
	if dst == nil {
		return ErrNilDst
	}
	val := reflect.ValueOf(src)
	if reflect.Indirect(val).Kind() != reflect.Struct {
		return ErrReqStruct
	}
	seen := map[reflect.Type]bool{}
	tag := enc.tagName()
	if err := checkTags("", val.Type(), tag, enc.isLeafType, seen); err != nil {
		return err
	}
	if val.Kind() != reflect.Pointer {
		// Copy src to an addressable value, so the methods with pointer
		// receivers are found.
		ptr := reflect.New(val.Type())
		ptr.Elem().Set(val)
		val = ptr
	}
	return enc.encode(dst, "", val, newEncState(), level{})
}

// tagName returns the name of the struct tag the encoder reads.
func (enc *Encoder) tagName() string {
	if enc.tag == "" {
		return defaultTag
	}
	return enc.tag
}

// encode encodes src to dst under the field path. The est holds the
// pointers, maps and slices on the path to src; meeting one of them again is
// a cycle. The lvl is the level position of an embedded struct src, or the
// level of a struct src, or of the struct it points to, built beforehand; for
// any other src it is the zero level.
func (enc *Encoder) encode(
	dst url.Values,
	field string,
	src reflect.Value,
	est *encState,
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
			if est.seen[key] {
				return fieldError(field, ErrCycle)
			}
			est.seen[key] = true
			defer delete(est.seen, key)
			src = src.Elem()
		}
	}
	if kind := src.Kind(); kind == reflect.Slice || kind == reflect.Map {
		// A map or slice holding itself is a cycle without a pointer.
		if src.Len() > 0 {
			key := ptrKey{ptr: src.Pointer(), typ: src.Type(), len: src.Len()}
			if est.seen[key] {
				return fieldError(field, ErrCycle)
			}
			est.seen[key] = true
			defer delete(est.seen, key)
		}
	}

	switch src.Kind() { //nolint:exhaustive
	case reflect.Slice, reflect.Array:
		return enc.encodeSlice(dst, field, src, est)

	case reflect.Map:
		return enc.encodeMap(dst, field, src, est)

	case reflect.Struct:
		return enc.encodeFields(dst, field, src, est, lvl)

	default:
		return fieldError(field, encNotFound(src.Type()))
	}
}

// encodeFields encodes the fields of the src struct to dst under the field
// path. The est is passed to the encode method. The lvl is the level
// position of src, the zero level for a struct starting a level; a field its
// key does not name there is skipped.
func (enc *Encoder) encodeFields(
	dst url.Values,
	field string,
	src reflect.Value,
	est *encState,
	lvl level,
) error {

	if lvl.vis == nil {
		lvl = est.level(src.Type(), enc.tagName())
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
			err := enc.encode(dst, field, val, est, lvl.embedded(i))
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
		if err := enc.encode(dst, fldPath, val, est, level{}); err != nil {
			return err
		}
	}
	return nil
}

// encState is the state of one [Encoder.Encode] call.
type encState struct {
	seen   map[ptrKey]bool        // Pointers, maps and slices on the path.
	levels map[reflect.Type]level // Levels of the struct types met.
}

// newEncState returns the state of a new [Encoder.Encode] call.
func newEncState() *encState {
	return &encState{
		seen:   make(map[ptrKey]bool),
		levels: make(map[reflect.Type]level),
	}
}

// level returns the level of the typ struct for the key struct tag, built on
// the first call for the type.
func (est *encState) level(typ reflect.Type, key string) level {
	lvl, ok := est.levels[typ]
	if !ok {
		lvl = newLevel(typ, key)
		est.levels[typ] = lvl
	}
	return lvl
}

// ptrKey identifies a pointer, map or slice on the encoding path. The type
// tells apart a pointer to a struct and a pointer to its first field, and the
// length a slice and the shorter slices of its array.
type ptrKey struct {
	ptr uintptr
	typ reflect.Type
	len int
}

// encSrc encodes src to dst. It returns false if the encoder for the src value
// could not be found, true otherwise. When it returns true, check the error;
// when it returns false, the error is always nil.
//
// It first checks for an encoder registered for the src type, then if src, or
// its address, implements [Marshaler], then [encoding.TextMarshaler] when the
// type also implements [encoding.TextUnmarshaler]; lastly, it calls encFor to
// find an encoder. For an inline src, the struct passed to
// [Encoder.Encode] or a struct embedded in a struct, only [Marshaler] is
// used. A nil pointer or interface with an encoder, or a value whose
// marshaler method would be reached through a nil embedded field (see
// nilPromoted), adds nothing to dst and returns true.
func encSrc(
	dst url.Values,
	prefix string,
	src reflect.Value,
	reg encReg,
	inline bool,
) (bool, error) {

	// Look into an interface implementing a marshaler for the pointer it
	// may hold, unless a function is registered for the interface.
	isItf := src.Kind() == reflect.Interface
	if isItf && !src.IsNil() && isMarshaler(src.Type()) &&
		reg.lookup(src.Type()) == nil {

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
		if nilPromoted(src, "MarshalValues") {
			return true, nil
		}
		m := src.Interface().(Marshaler) //nolint:forcetypeassert
		return true, m.MarshalValues(dst, prefix)
	}
	efn := reg.lookup(typ)
	// The methods of a value read from an unexported field cannot be called.
	// MarshalText is used only when Decode can read that text back. A nil
	// marshaler still adds nothing.
	paired := typ.Implements(iMarshaler) || textUnmarshaler(typ)
	if efn == nil && isMarshaler(typ) && src.CanInterface() &&
		(isNil(src) || paired) {

		name := "MarshalText"
		if typ.Implements(iMarshaler) {
			name = "MarshalValues"
		}
		if isNil(src) || nilPromoted(src, name) {
			return true, nil
		}
		if m, ok := src.Interface().(Marshaler); ok {
			return true, m.MarshalValues(dst, prefix)
		}
		m := src.Interface().(encoding.TextMarshaler) //nolint:forcetypeassert
		text, err := marshalText(m)
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

// textUnmarshaler reports whether typ, or a pointer to the type it is or
// points to, implements [encoding.TextUnmarshaler].
func textUnmarshaler(typ reflect.Type) bool {
	if typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	return typ.Implements(iUnmarshaler) ||
		reflect.PointerTo(typ).Implements(iUnmarshaler)
}

// encodeSlice encodes the src slice or array to dst. Elements encoded as one
// value (see isValueType) are added as values of the prefix key; any other
// element, a struct, a map or a [Marshaler], is encoded under the prefix key
// and its index ("list[0]"). An element of an interface type with no
// registered function is told by the value it holds. The est is passed to the
// encode method.
func (enc *Encoder) encodeSlice(
	dst url.Values,
	prefix string,
	src reflect.Value,
	est *encState,
) error {

	et := src.Type().Elem()
	value := enc.isValueType(et)
	// A function registered for the interface encodes every value it holds.
	itf := et.Kind() == reflect.Interface && enc.reg.lookup(et) == nil
	lvl := enc.elemLevel(src.Type(), est)
	for i := 0; i < src.Len(); i++ {
		val := src.Index(i)
		isVal := value
		if value && itf && !val.IsNil() {
			// The interface may hold a Marshaler, which is no value.
			isVal = enc.isValueType(val.Elem().Type())
		}
		if isVal {
			if ok, err := encSrc(dst, prefix, val, enc.reg, false); ok {
				if err != nil {
					ipt := prefix + "[" + strconv.Itoa(i) + "]"
					return xrr.NewFieldError(ipt, err)
				}
				continue
			}
		}
		ipt := prefix + "[" + strconv.Itoa(i) + "]" // Only indexed values.
		if err := enc.encode(dst, ipt, val, est, lvl); err != nil {
			return err
		}
	}
	return nil
}

// encodeMap encodes the src map to dst, each value under the prefix key and its
// map key ("attr[k]"). The keys are encoded with [encoding.TextMarshaler] or
// the base type encoders; an empty key, one with a bracket, one that is or
// holds a NaN, and two keys encoded to one text are errors. The entries are
// encoded in the order of their key texts, and of the keys failing to encode,
// the error of the first one in Go syntax order is returned. The est is passed
// to the encode method.
func (enc *Encoder) encodeMap(
	dst url.Values,
	prefix string,
	src reflect.Value,
	est *encState,
) error {

	type pair struct {
		key string        // Encoded map key.
		mk  reflect.Value // Map key.
		val reflect.Value // Map value.
	}
	var prs []pair
	var kerr error    // Error of the failing key first in Go syntax order.
	var kerrGo string // The Go syntax of that key.
	lvl := enc.elemLevel(src.Type(), est)
	for it := src.MapRange(); it.Next(); {
		mk := it.Key()
		// A function registered for the interface takes the key as is.
		itf := mk.Kind() == reflect.Interface
		if itf && !mk.IsNil() && enc.reg.lookup(mk.Type()) == nil {
			mk = mk.Elem()
		}
		// Reduce pointers to pointers to a single pointer.
		for mk.Kind() == reflect.Pointer && !mk.IsNil() &&
			mk.Type().Elem().Kind() == reflect.Pointer {

			mk = mk.Elem()
		}
		if isNil(mk) {
			continue // A nil key adds nothing.
		}
		key, err := encMapKey(mk, enc.reg)
		if errors.Is(err, errNilKey) {
			continue // A key counting as nil adds nothing.
		}
		if err != nil {
			txt := fmt.Sprintf("%#v", mk.Interface())
			if kerr == nil || txt < kerrGo {
				kerr, kerrGo = err, txt
			}
			continue
		}
		prs = append(prs, pair{key: key, mk: mk, val: it.Value()})
	}
	if kerr != nil {
		return fieldError(prefix, kerr)
	}
	// In key order, the output and the first error are the same every time.
	slices.SortFunc(prs, func(a, b pair) int {
		return strings.Compare(a.key, b.key)
	})
	for i, pr := range prs {
		var format string
		switch {
		case isNaNKey(pr.mk):
			format = "%w: NaN map key" // Decode rejects it.

		case pr.key == "":
			format = "%w: empty map key"

		case strings.ContainsAny(pr.key, "[]"):
			format = "%w: map key with a bracket"

		case i > 0 && prs[i-1].key == pr.key:
			format = "%w: duplicate map key" // Decode takes them as one.
		}
		if format != "" {
			err := xrr.Newf(format, ErrInvValue, xrr.WithCode(ECInvValue))
			return fieldError(prefix, err)
		}
		pth := prefix + "[" + pr.key + "]"
		val := addressable(pr.val)
		if err := enc.encode(dst, pth, val, est, lvl); err != nil {
			return err
		}
	}
	return nil
}

// elemLevel returns the level of the elements of the typ container, taken
// from est, when they are structs or pointers to structs; else it returns
// the zero level.
func (enc *Encoder) elemLevel(typ reflect.Type, est *encState) level {
	if et := indirect(typ.Elem()); et.Kind() == reflect.Struct {
		return est.level(et, enc.tagName())
	}
	return level{}
}

// isValueType returns true if a value of the typ type, or a pointer to one,
// is encoded as one text value: by a registered encoder, by its
// [encoding.TextMarshaler] when it also implements [encoding.TextUnmarshaler],
// or by a built-in encoder. A [Marshaler] is not. An interface implementing
// [encoding.TextMarshaler] is one; the value it holds decides.
func (enc *Encoder) isValueType(typ reflect.Type) bool {
	if enc.reg.lookup(typ) != nil {
		return true // Registered for typ or for the type it points to.
	}
	typ = indirect(typ)
	ptr := reflect.PointerTo(typ)
	if typ.Implements(iMarshaler) || ptr.Implements(iMarshaler) {
		return false
	}
	if typ.Implements(iTextMarshaler) || ptr.Implements(iTextMarshaler) {
		if typ.Kind() == reflect.Interface || textUnmarshaler(typ) {
			return true
		}
	}
	return encFor(typ, enc.reg) != nil
}

// isLeafType returns true if a value of the typ type, which is not a pointer,
// is encoded as a whole: by a registered encoder, by its [Marshaler] or
// [encoding.TextMarshaler] when it also implements [encoding.TextUnmarshaler],
// or by a built-in encoder.
func (enc *Encoder) isLeafType(typ reflect.Type) bool {
	if typ.Implements(iMarshaler) ||
		reflect.PointerTo(typ).Implements(iMarshaler) {

		return true
	}
	return enc.isValueType(typ)
}

// errNilKey reports a map key whose MarshalText method would be reached
// through a nil embedded field (see nilPromoted); such a key counts as nil.
var errNilKey = errors.New("nil map key")

// encMapKey encodes the map key to a string. The key must not be nil or an
// interface holding a nil pointer. A key counting as nil returns errNilKey. A
// float key of -0, or a pointer to one, encodes as "0".
func encMapKey(key reflect.Value, reg encReg) (string, error) {
	efn := reg.lookup(key.Type())
	if efn == nil && key.Kind() == reflect.Interface {
		key = key.Elem()
		efn = reg.lookup(key.Type())
	}
	if efn != nil {
		return efn(key)
	}
	if key.Kind() != reflect.Pointer &&
		reflect.PointerTo(key.Type()).Implements(iTextMarshaler) {

		key = addressable(key).Addr()
	}
	typ := key.Type()
	if typ.Implements(iTextMarshaler) {
		if nilPromoted(key, "MarshalText") {
			return "", errNilKey
		}
		m := key.Interface().(encoding.TextMarshaler) //nolint:forcetypeassert
		text, err := marshalText(m)
		return string(text), err
	}
	if val := reflect.Indirect(key); val.CanFloat() && val.Float() == 0 {
		return "0", nil // Decode takes a -0 key for the 0 one.
	}
	if efn = encFor(typ, reg); efn == nil {
		return "", encNotFound(typ)
	}
	return efn(key)
}

// marshalText returns the text m, which must not be a nil pointer, marshals
// to. A [time.Time] whose zone offset has seconds, which RFC 3339 cannot
// write, is marshaled in UTC, so it decodes back to the same instant, unless
// its year in UTC is one RFC 3339 cannot write either. A struct embedding a
// [time.Time] is marshaled by the method it gets from it.
func marshalText(m encoding.TextMarshaler) ([]byte, error) {
	var tim time.Time
	switch val := m.(type) {
	case time.Time:
		tim = val

	case *time.Time:
		tim = *val

	default:
		return m.MarshalText()
	}
	if _, off := tim.Zone(); off%60 != 0 {
		if utc := tim.UTC(); utc.Year() >= 0 && utc.Year() <= 9999 {
			return utc.MarshalText()
		}
	}
	return m.MarshalText()
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
