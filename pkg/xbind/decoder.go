// SPDX-FileCopyrightText: (c) 2026 Rafal Zajac
// SPDX-License-Identifier: MIT

package xbind

import (
	"encoding"
	"fmt"
	"net/url"
	"reflect"
	"slices"
	"strings"

	"github.com/ctx42/xrr/pkg/xrr"

	"github.com/ctx42/mirror/pkg/mirror"
)

// iUnmarshaler is used to check if a type implements the
// [encoding.TextUnmarshaler] interface.
var iUnmarshaler = reflect.TypeFor[encoding.TextUnmarshaler]()

// DecoderOption configures a [Decoder] built by [NewDecoder].
type DecoderOption func(*Decoder)

// WithDecTag is a [NewDecoder] option setting the struct tag name used for
// field name aliases and options.
func WithDecTag(tag string) DecoderOption {
	return func(dec *Decoder) { dec.tag = tag }
}

// WithDecAllowUnknown is a [NewDecoder] option allowing unknown fields in the
// source map.
func WithDecAllowUnknown() DecoderOption {
	return func(dec *Decoder) { dec.allowUnknown = true }
}

// WithDecFunc is a [NewDecoder] option decoding every value of type T with
// fn: fields of type T or *T, and slice, array and map elements and map keys
// of those types. A value of type T takes a single src value, and an error
// returned by fn is returned as is. The function takes precedence over the
// [encoding.TextUnmarshaler] method of T and the decoders the package
// provides, [time.Time] and [time.Duration] included. T must not be a pointer
// type.
func WithDecFunc[T any](fn func(string) (T, error)) DecoderOption {
	typ := reflect.TypeFor[T]()
	return func(dec *Decoder) {
		if dec.reg == nil {
			dec.reg = make(decReg)
		}
		dec.reg[typ] = func(vls ...string) (any, error) {
			val, err := single(vls)
			if err != nil {
				return nil, err
			}
			return fn(val)
		}
	}
}

// Decoder decodes [url.Values] to a struct. The zero value is ready to use,
// reads the "url" struct tag and rejects unknown keys.
type Decoder struct {
	tag          string // Struct field tag to use for aliasing.
	allowUnknown bool   // Allow unknown fields.
	reg          decReg // Decoders registered with WithDecFunc.
}

// NewDecoder returns a new instance of [Decoder] reading the "url" struct tag
// and rejecting unknown keys unless the options say otherwise.
func NewDecoder(opts ...DecoderOption) *Decoder {
	dec := &Decoder{tag: defaultTag}
	for _, opt := range opts {
		opt(dec)
	}
	return dec
}

// Decode sets the fields of the dst struct from src. The dst argument must be
// a pointer to a struct. The src keys are paths to the dst fields in dotted
// notation; the path of a map field ends with the map key. A tag name with a
// dot never matches a key. Errors of the src values are returned as
// [xrr.FieldErrors] keyed by the src keys; the other values are still set.
// Decoded slice elements are appended to the elements dst already has; array
// elements past the decoded values keep their values. A nil pointer to a
// nested struct is allocated only when a src key leads into it. A field with
// the "required" option is an [ErrRequired] error when it is empty after
// decoding: a zero number, an empty string, slice or map, a nil pointer, or a
// zero struct; a bool is never empty. The required fields of a nil struct
// pointer, an absent struct, are not checked. A src
// key leading into an embedded nil pointer to a struct, which the decoder
// cannot allocate because the field is unexported, is an [ErrUnexported]
// field error. A struct tag of dst that does not parse is an [ErrInvTag]
// error. Pointers to pointers to structs are not supported.
func (dec *Decoder) Decode(dst any, src url.Values) error {
	sv := mirror.NewStructValue(dst)
	if sv == nil {
		return ErrReqStructPtr
	}
	seen := map[reflect.Type]bool{}
	if err := checkTags("", sv.Type(), dec.tagName(), seen); err != nil {
		return err
	}
	return dec.decode(sv, src).Filter()
}

// tagName returns the name of the struct tag the decoder reads.
func (dec *Decoder) tagName() string {
	if dec.tag == "" {
		return defaultTag
	}
	return dec.tag
}

// decode decodes src to the dst struct and returns the errors of the src
// values and of the required fields left empty.
func (dec *Decoder) decode(
	dst *mirror.StructValue,
	src url.Values,
) *xrr.FieldErrors {

	var ers *xrr.FieldErrors
	setErr := func(pth string, err error) {
		if ers == nil {
			ers = &xrr.FieldErrors{}
		}
		ers.Set(pth, err)
	}

	// Go over src keys and match them with struct fields.
	for pth, vls := range src {
		res := dec.resolve("", strings.Split(pth, "."), dst, nil)
		if res.ignored {
			continue
		}
		if res.unexported {
			setErr(pth, ErrUnexported)
			continue
		}
		if !res.isComplete() {
			if !dec.allowUnknown {
				setErr(pth, ErrUnkFieldPath)
			}
			continue
		}
		key := strings.Join(res.remainder, ".")
		dstV := res.dst.NewIfNil().Value()
		if err := setField(dstV, dec.reg, key, vls...); err != nil {
			setErr(pth, err)
		}
	}

	// A field with a value error keeps that error.
	dec.required("", dst, map[ptrKey]bool{}, func(pth string, err error) {
		if ers == nil || ers.Get(pth) == nil {
			setErr(pth, err)
		}
	})
	return ers
}

// resolve takes a path to a field and returns its value and its path. The
// outer types are the structs embedding dst at the current path segment; an
// embedded struct of one of them is skipped, so an embedding cycle ends.
func (dec *Decoder) resolve(
	pth string,
	remainder []string,
	dst *mirror.StructValue,
	outer []reflect.Type,
) result {

	if len(remainder) == 0 {
		return result{}
	}
	outer = append(outer, dst.Type())
	var ignored bool // The name matched an ignored field.
	for i, fld := range dst.Fields() {
		tag, _ := fld.Tag(dec.tagName()) // Decode checked the tags.
		if fld.IsAnonymous() && indirect(fld.Type()).Kind() == reflect.Struct {
			if tag.IsIgnored() || slices.Contains(outer, indirect(fld.Type())) {
				continue
			}
			fv := dst.FieldByIndex(i)
			sv := fv.StructValue().NewIfNil()
			val := fv.Value()
			unexported := val.Kind() == reflect.Pointer && val.IsNil()
			if unexported {
				// An unexported pointer, which cannot be allocated. A
				// throwaway value tells whether the path leads into it.
				tmp := reflect.New(val.Type().Elem()).Interface()
				sv = mirror.NewStructValue(tmp)
			}
			res := dec.resolve(pth, remainder, sv, outer)
			if !res.isZero() {
				if unexported {
					return result{pth: res.pth, unexported: true}
				}
				return res
			}
			ignored = ignored || res.ignored
			continue
		}

		name := tag.NameOrField()
		if name != remainder[0] {
			continue
		}
		if tag.IsIgnored() {
			ignored = true
			continue
		}
		if !fld.IsExported() {
			continue
		}
		fldPath := buildPath(pth, name)
		fv := dst.FieldByIndex(i)
		ret := fv.Value()

		if ret.Type().Implements(iUnmarshaler) ||
			ret.CanAddr() && ret.Addr().Type().Implements(iUnmarshaler) {

			return result{
				dst:       fv,
				pth:       fldPath,
				unm:       true,
				remainder: remainder[1:],
			}
		}

		if typ := indirect(ret.Type()); typ.Kind() == reflect.Struct &&
			typ != typTime && dec.reg[typ] == nil {

			sv := fv.NewIfNil().StructValue()
			return dec.resolve(fldPath, remainder[1:], sv, nil)
		}
		return result{dst: fv, pth: fldPath, remainder: remainder[1:]}
	}
	return result{ignored: ignored}
}

// required sets an [ErrRequired] error, using setErr, for each required field
// of dst whose value is empty (see isEmpty). A nil struct pointer is an
// absent struct, so its fields are not checked. Ignored fields and unexported
// fields, other than embedded structs, are skipped; an embedded struct has no
// path, so its required option is ignored. The seen map holds the struct
// pointers on the path to dst, so a pointer cycle ends.
func (dec *Decoder) required(
	prefix string,
	dst *mirror.StructValue,
	seen map[ptrKey]bool,
	setErr func(string, error),
) {

	for i := 0; i < dst.NumField(); i++ {
		fld := dst.FieldByIndex(i)
		tag, _ := fld.Tag(dec.tagName()) // Decode checked the tags.
		if tag.IsIgnored() {
			continue
		}
		anon := fld.IsAnonymous()

		isStruct := indirect(fld.Type()).Kind() == reflect.Struct
		if anon {
			// Skip embeddings that are interfaces which we have no way to
			// set, and unexported embeddings of other types.
			if fld.Kind() == reflect.Interface {
				continue
			}
			if !fld.IsExported() && !isStruct {
				continue
			}
		} else if !fld.IsExported() {
			continue
		}

		fldPath := buildPath(prefix, tag.NameOrField())
		if isStruct && (anon || !dec.isLeaf(fld.Value())) {
			// The path to a field that comes from an embedded struct does
			// not include the name of the embedded struct.
			if anon {
				fldPath = prefix
			} else if tag.Contains("required") && isEmpty(fld.Value()) {
				setErr(fldPath, ErrRequired)
			}
			val := fld.Value()
			if val.Kind() != reflect.Pointer {
				dec.required(fldPath, fld.StructValue(), seen, setErr)
				continue
			}
			key := ptrKey{ptr: val.Pointer(), typ: val.Type()}
			if val.IsNil() || seen[key] {
				continue // An absent struct, or a pointer cycle.
			}
			seen[key] = true
			dec.required(fldPath, fld.StructValue(), seen, setErr)
			delete(seen, key)
			continue
		}
		if tag.Contains("required") && isEmpty(fld.Value()) {
			setErr(fldPath, ErrRequired)
		}
	}
}

// isLeaf returns true if the struct, or pointer to a struct, val is set as a
// whole: by a registered decoder or by its [encoding.TextUnmarshaler].
func (dec *Decoder) isLeaf(val reflect.Value) bool {
	if dec.reg[indirect(val.Type())] != nil ||
		val.Type().Implements(iUnmarshaler) {

		return true
	}
	return val.CanAddr() && val.Addr().Type().Implements(iUnmarshaler)
}

// result represents the result of resolving a field path.
type result struct {
	dst     *mirror.FieldValue // Value to use to set a field.
	pth     string             // Absolute path to a field.
	ignored bool               // The path leads to an ignored field.
	unm     bool               // The field implements TextUnmarshaler.

	// The path leads into a nil embedded struct pointer the decoder cannot
	// allocate because the embedded type is unexported; dst is nil.
	unexported bool

	// Unused path suffix (the key in the case of maps).
	remainder []string
}

// isZero returns true if the path did not resolve to a field, or to an
// unexported field.
func (res result) isZero() bool { return res.dst == nil && !res.unexported }

// isComplete returns true if the path resolved to a field and its remainder
// fits the field: a non-empty map key for a map field, or a pointer to a map,
// nothing for other fields and for fields implementing TextUnmarshaler.
func (res result) isComplete() bool {
	if res.dst == nil {
		return false
	}
	if !res.unm && indirect(res.dst.Type()).Kind() == reflect.Map {
		return strings.Join(res.remainder, ".") != ""
	}
	return len(res.remainder) == 0
}

// isUnmarshal returns true if dst is addressable and its address implements
// the [encoding.TextUnmarshaler] interface. It returns false for [time.Time],
// which is decoded by a custom decoder, and for pointers and interfaces.
func isUnmarshal(dst reflect.Value) bool {
	if !dst.CanAddr() || dst.Type() == typTime {
		return false
	}
	switch dst.Kind() { //nolint:exhaustive
	case reflect.Pointer, reflect.Interface:
		return false

	default:
		return dst.Addr().Type().Implements(iUnmarshaler)
	}
}

// useUnmarshal sets dst using its [encoding.TextUnmarshaler] implementation,
// when it has one (see isUnmarshal), and returns true. Such a dst takes
// exactly one value. The dst must not be a pointer.
func useUnmarshal(dst reflect.Value, vls ...string) (bool, error) {
	if !isUnmarshal(dst) {
		return false, nil
	}
	val, err := single(vls)
	if err != nil {
		return true, err
	}
	unm, _ := dst.Addr().Interface().(encoding.TextUnmarshaler)
	return true, unm.UnmarshalText([]byte(val))
}

// setDst sets dst with the given values. It checks first if dst implements
// the [encoding.TextUnmarshaler] interface, then it uses decFor with the reg
// registry, then it decodes a slice or an array element by element. If one
// of them was used, it returns true and the error returned by it. If none can
// decode the value, it returns false with a nil error. Nil pointers are
// allocated before dst is set.
func setDst(dst reflect.Value, reg decReg, vls ...string) (bool, error) {
	for dst.Kind() == reflect.Pointer {
		if dst.IsNil() {
			dst.Set(reflect.New(dst.Type().Elem()))
		}
		dst = dst.Elem()
	}
	if reg[dst.Type()] == nil {
		if ok, err := useUnmarshal(dst, vls...); ok {
			return true, err
		}
	}

	typ := dst.Type()
	if dfn := decFor(typ, reg); dfn != nil {
		val, err := dfn(vls...)
		if err != nil {
			return true, err
		}
		vv := reflect.ValueOf(val)
		if !vv.IsValid() || !vv.CanConvert(typ) {
			// This may happen only if decoder returns type which is in
			// fact not the one it should be decoding.
			const format = "expected decoder to return %#q got `%T`"
			return true, fmt.Errorf(format, typ.String(), val)
		}
		// The destination may be an alias of the type returned by decoder.
		dst.Set(vv.Convert(typ))
		return true, nil
	}

	switch dst.Kind() { //nolint:exhaustive
	case reflect.Slice, reflect.Array:
		return true, setSliceOrArray(dst, reg, vls...)

	default:
		return false, nil
	}
}

// setSliceOrArray expects dst to be a slice or an array. Each of the elements
// in vls will be decoded based on the array or slice type and set. In the case
// of a slice, the new values are appended. In the case of an array, the values
// are set on indexes in the same order as in vls; the elements past the
// values keep their values.
func setSliceOrArray(dst reflect.Value, reg decReg, vls ...string) error {
	et := dst.Type().Elem() // Element type.
	if dst.Kind() == reflect.Array && dst.Len() < len(vls) {
		const format = "%w: too many values for an array"
		return xrr.Newf(format, ErrInvValue, xrr.WithCode(ECInvValue))
	}

	for i, val := range vls {
		ne := reflect.New(indirect(et))
		ok, err := setDst(ne.Elem(), reg, val)
		if err != nil {
			return err
		}
		if !ok {
			return decNotFound(et)
		}
		if et.Kind() != reflect.Pointer {
			ne = ne.Elem() // Slice element is not a pointer type.
		}
		if dst.Kind() == reflect.Slice {
			dst.Set(reflect.Append(dst, ne))
		} else {
			dst.Index(i).Set(ne)
		}
	}
	return nil
}

// setMap expects dst to be a non-nil map. It sets the map key to the values.
func setMap(dst reflect.Value, reg decReg, key string, vls ...string) error {
	kt := dst.Type().Key()
	nk := reflect.New(indirect(kt))
	ok, err := setDst(nk.Elem(), reg, key)
	if !ok {
		return decNotFound(kt)
	}
	if err != nil {
		return err
	}
	if kt.Kind() != reflect.Pointer {
		nk = nk.Elem() // Map key is not a pointer type.
	}

	vt := dst.Type().Elem()
	nv := reflect.New(indirect(vt))
	if ok, err = setDst(nv.Elem(), reg, vls...); !ok {
		return decNotFound(vt)
	}
	if err != nil {
		return err
	}
	if vt.Kind() != reflect.Pointer {
		nv = nv.Elem() // Map value is not a pointer type.
	}

	dst.SetMapIndex(nk, nv)
	return nil
}

// setField sets a struct field, or the field a non-nil pointer points to,
// using the values. If the field is a map, the key must be set to the string
// representation of the map key.
func setField(dst reflect.Value, reg decReg, key string, vls ...string) error {
	val := reflect.Indirect(dst)
	if val.IsValid() && reg[val.Type()] != nil {
		_, err := setDst(dst, reg, vls...)
		return err
	}
	if ok, err := useUnmarshal(val, vls...); ok {
		return err
	}

	switch val.Kind() { //nolint:exhaustive
	case reflect.Slice, reflect.Array:
		return setSliceOrArray(val, reg, vls...)

	case reflect.Map:
		if val.IsNil() {
			val.Set(reflect.MakeMap(val.Type()))
		}
		return setMap(val, reg, key, vls...)

	default:
		if ok, err := setDst(dst, reg, vls...); ok {
			return err
		}
		return decNotFound(dst.Type())
	}
}

// decNotFound returns an error wrapping [ErrDecNotFound] naming the typ type.
func decNotFound(typ reflect.Type) error {
	code := xrr.WithCode(ECDecNotFound)
	return xrr.Newf("%w for %#q", ErrDecNotFound, typ.String(), code)
}
