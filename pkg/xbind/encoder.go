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
	tag   string     // Struct field tag to use for aliasing.
	reg   encReg     // Encoders registered with WithEncFunc.
	cache *typeCache // Facts about the types met; nil builds them per call.
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
// unless the [WithEncTag] option says otherwise. The encoder caches what it
// learns about each type it encodes, so later calls for the type do less
// work; a zero-value [Encoder] caches nothing.
func NewEncoder(opts ...EncoderOption) *Encoder {
	enc := &Encoder{tag: defaultTag, cache: &typeCache{}}
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
	err := enc.cache.checkTags(val.Type(), enc.tagName(), enc.isLeafType)
	if err != nil {
		return err
	}
	if val.Kind() != reflect.Pointer && enc.needsAddr(val.Type()) {
		// Copy src to an addressable value, so the methods with pointer
		// receivers are found.
		ptr := reflect.New(val.Type())
		ptr.Elem().Set(val)
		val = ptr
	}
	// A value whose type graph cannot loop holds no cycle to look for.
	est := encState{noLoop: !enc.canLoop(val.Type())}
	return enc.encode(dst, "", val, &est, level{})
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

	if !enc.isComposite(src.Type()) {
		// The src struct and the structs embedded in it are encoded inline.
		inline := field == "" || lvl.idx != nil
		if ok, err := enc.encSrc(dst, field, src, inline); ok {
			return fieldError(field, err)
		}
	}

	if src.Kind() == reflect.Pointer {
		switch src.Type().Elem().Kind() { //nolint:exhaustive
		case reflect.Slice, reflect.Array, reflect.Map, reflect.Struct:
			if src.IsNil() {
				return nil
			}
			key := ptrKey{ptr: src.Pointer(), typ: src.Type()}
			if !est.enter(key) {
				return fieldError(field, ErrCycle)
			}
			defer est.leave(key)
			src = src.Elem()
		}
	}
	if kind := src.Kind(); kind == reflect.Slice || kind == reflect.Map {
		// A map or slice holding itself is a cycle without a pointer; one of
		// plain values holds nothing.
		if src.Len() > 0 && enc.plain(src.Type().Elem()) == nil {
			key := ptrKey{ptr: src.Pointer(), typ: src.Type(), len: src.Len()}
			if !est.enter(key) {
				return fieldError(field, ErrCycle)
			}
			defer est.leave(key)
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

	if lvl.nms == nil {
		lvl = enc.cache.level(src.Type(), enc.tagName())
	}
	fls := est.fields(enc, src.Type())
	return enc.encodeWith(dst, field, src, est, lvl, fls)
}

// encodeWith is encodeFields given the plans fls of the fields of src.
func (enc *Encoder) encodeWith(
	dst url.Values,
	field string,
	src reflect.Value,
	est *encState,
	lvl level,
	fls []encField,
) error {

	for i, fld := range fls {
		if fld.skip {
			continue
		}
		val := src.Field(i)
		if fld.omit && isZero(val) {
			continue
		}
		if fld.emb {
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
		// The plan knows the names of the level of the struct itself.
		named := fld.named
		if lvl.idx != nil {
			named = lvl.names(fld.name, i)
		}
		if !named {
			continue
		}

		fldPath := fieldPath(field, fld.name)
		if fld.plain != nil {
			txt, err := fld.plain(val)
			if err != nil {
				return fieldError(fldPath, err)
			}
			dst[fldPath] = append(dst[fldPath], txt)
			continue
		}
		if fld.elems != nil {
			// As encode and encodeSlice encode it.
			if err := encodeValues(dst, fldPath, val, fld.elems); err != nil {
				return err
			}
			continue
		}
		if fld.mp != nil {
			// As encode and encodeMap encode it: no value holds a cycle.
			err := enc.encodeMapSure(dst, fldPath, val, est, fld.mp)
			if err != nil {
				return err
			}
			continue
		}
		if fld.sub != nil {
			// As encode and encodeFields encode it.
			err := enc.encodeWith(dst, fldPath, val, est, fld.slvl, fld.sub)
			if err != nil {
				return err
			}
			continue
		}
		if err := enc.encode(dst, fldPath, val, est, level{}); err != nil {
			return err
		}
	}
	return nil
}

// encField is the plan for a field of a struct the encoder encodes.
type encField struct {
	name  string     // Key name of the field.
	omit  bool       // The field has the omitempty option.
	skip  bool       // The field adds nothing: ignored, unexported, interface.
	emb   bool       // The field is an embedded struct, or a pointer to one.
	named bool       // The level of the struct names the field.
	plain encFn      // The encoder of a field of a plain value type, or nil.
	elems encFn      // The encoder of the plain values of a slice or array.
	sub   []encField // The field plans of a struct field encoded by parts.
	slvl  level      // The level of that struct.
	sure  bool       // Encoding the field never fails (see encodesSure).
	mp    *sureMap   // How a map field encoded by parts, never failing, is.
}

// encodesSure returns true if encoding a struct with the field plans fls never
// fails: each field is skipped, or a plain value other than a [time.Time],
// whose text may fail, or a slice or array of such values, or a struct of such
// fields; an embedded struct, or any other field, may fail.
func encodesSure(fls []encField) bool {
	for _, fld := range fls {
		if !fld.skip && !fld.sure {
			return false
		}
	}
	return true
}

// fields returns the plans of the fields of the typ struct, in their order.
func (enc *Encoder) fields(typ reflect.Type) []encField {
	if enc.cache != nil {
		if fls, ok := enc.cache.plans.Load(typ); ok {
			return fls
		}
	}
	md := mirror.ReflectType(typ)
	lvl := enc.cache.level(typ, enc.tagName())
	fls := make([]encField, md.NumField())
	for i := range fls {
		fld := md.FieldByIndex(i)
		tag, _ := fld.Tag(enc.tagName()) // Encode checked the tags.
		fls[i] = encField{
			name: tag.NameOrField(),
			omit: tag.Contains("omitempty"),
		}
		switch {
		case tag.IsIgnored():
			fls[i].skip = true

		case fld.IsAnonymous() && fld.Kind() == reflect.Interface:
			fls[i].skip = true // Like the decoder, skip embedded interfaces.

		case fld.IsAnonymous() && indirect(fld.Type()).Kind() == reflect.Struct:
			fls[i].emb = true

		case !fld.IsExported():
			fls[i].skip = true

		default:
			fls[i].named = lvl.names(fls[i].name, i)
			fls[i].plain = enc.plain(fld.Type())
			kind := fld.Type().Kind()
			if (kind == reflect.Slice || kind == reflect.Array) &&
				enc.isComposite(fld.Type()) {

				fls[i].elems = enc.plain(fld.Type().Elem())
			}
			if kind == reflect.Struct && enc.isComposite(fld.Type()) {
				// A struct held by value cannot hold itself, so this ends.
				fls[i].sub = enc.fields(fld.Type())
				fls[i].slvl = enc.cache.level(fld.Type(), enc.tagName())
			}
			ft := fld.Type()
			// A map of structs may hold this struct, so its plan is left
			// for when it is encoded, this plan being stored.
			if kind == reflect.Map && enc.isComposite(ft) &&
				enc.plain(ft.Elem()) != nil {

				fls[i].mp = enc.mapPlan(ft)
			}
			switch {
			case fls[i].plain != nil:
				fls[i].sure = ft != typTime

			case fls[i].elems != nil:
				fls[i].sure = ft.Elem() != typTime

			case fls[i].sub != nil:
				fls[i].sure = encodesSure(fls[i].sub)
			}
		}
	}
	if enc.cache != nil {
		enc.cache.plans.Store(typ, fls)
	}
	return fls
}

// needsAddr returns true if encoding a value of the typ type held by value,
// not addressable, could differ from encoding an addressable one: typ, or a
// type typ holds by value in struct fields and array elements, has a
// [Marshaler], [encoding.TextMarshaler] or IsZero method only on its pointer.
func (enc *Encoder) needsAddr(typ reflect.Type) bool {
	return enc.cache.addr(typ, func(typ reflect.Type) bool {
		return enc.addrIn(typ, map[reflect.Type]bool{})
	})
}

// addrIn is needsAddr for the typ type; the seen map holds the types walked.
func (enc *Encoder) addrIn(typ reflect.Type, seen map[reflect.Type]bool) bool {
	if seen[typ] {
		return false
	}
	seen[typ] = true
	inf := enc.cache.info(typ)
	if inf.pmar && !inf.mar || inf.ptext && !inf.text {
		return true
	}
	switch typ.Kind() { //nolint:exhaustive
	case reflect.Array:
		return enc.addrIn(typ.Elem(), seen)

	case reflect.Struct:
		ptr := reflect.PointerTo(typ)
		if ptr.Implements(iZeroer) && !typ.Implements(iZeroer) {
			return true
		}
		for i := range typ.NumField() {
			if enc.addrIn(typ.Field(i).Type, seen) {
				return true
			}
		}
	}
	return false
}

// canLoop returns true if a value of the typ type can hold a pointer, map or
// slice holding it again: the type graph of typ has a cycle, or an interface,
// which may hold anything.
func (enc *Encoder) canLoop(typ reflect.Type) bool {
	return enc.cache.loop(typ, func(typ reflect.Type) bool {
		return loopsIn(typ, map[reflect.Type]bool{}, map[reflect.Type]bool{})
	})
}

// loopsIn is canLoop for the typ type; path holds the types on the path to
// it, and done the types known not to loop.
func loopsIn(typ reflect.Type, path, done map[reflect.Type]bool) bool {
	if path[typ] {
		return true
	}
	if done[typ] {
		return false
	}
	path[typ] = true
	defer delete(path, typ)
	switch typ.Kind() { //nolint:exhaustive
	case reflect.Interface:
		return true

	case reflect.Pointer, reflect.Slice, reflect.Array, reflect.Map:
		if loopsIn(typ.Elem(), path, done) {
			return true
		}

	case reflect.Struct:
		for i := range typ.NumField() {
			if loopsIn(typ.Field(i).Type, path, done) {
				return true
			}
		}
	}
	done[typ] = true
	return false
}

// isComposite returns true if typ is a struct, slice, array or map type that
// encSrc has no encoder for, so its values are encoded part by part: no
// function is registered for the type, and it, or a pointer to it,
// implements neither [Marshaler] nor [encoding.TextMarshaler].
func (enc *Encoder) isComposite(typ reflect.Type) bool {
	return enc.cache.composite(typ, enc.compositeType)
}

// compositeType is isComposite without the cache.
func (enc *Encoder) compositeType(typ reflect.Type) bool {
	switch typ.Kind() { //nolint:exhaustive
	case reflect.Struct, reflect.Slice, reflect.Array, reflect.Map:

	default:
		return false
	}
	if enc.reg.lookup(typ) != nil {
		return false
	}
	inf := enc.cache.info(typ)
	return !inf.mar && !inf.text && !inf.pmar && !inf.ptext
}

// plain returns the encoder of the values of a plain value type typ: a bool,
// number or string type no function is registered for and which, by value or
// by pointer, implements neither [Marshaler] nor [encoding.TextMarshaler]; such
// a value is encoded by the built-in encoder of its type, as encSrc does. It
// returns nil for any other type.
func (enc *Encoder) plain(typ reflect.Type) encFn {
	if enc.cache != nil {
		if efn, ok := enc.cache.plains.Load(typ); ok {
			return efn
		}
	}
	var efn encFn
	if typ == typTime && enc.reg.lookup(typ) == nil {
		efn = encTime // As encSrc encodes it, by its MarshalText method.
	}
	switch typ.Kind() { //nolint:exhaustive
	case reflect.Bool, reflect.String,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32,
		reflect.Uint64, reflect.Float32, reflect.Float64:

		inf := enc.cache.info(typ)
		if enc.reg.lookup(typ) == nil &&
			!inf.mar && !inf.text && !inf.pmar && !inf.ptext {

			efn = encFor(typ, enc.reg)
		}
	}
	if enc.cache != nil {
		enc.cache.plains.Store(typ, efn)
	}
	return efn
}

// pathScan is the number of pointers, maps and slices on the encoding path an
// [Encoder.Encode] call looks through one by one; a longer path goes to a map.
const pathScan = 8

// encState is the state of one [Encoder.Encode] call.
type encState struct {
	path [pathScan]ptrKey // The pointers, maps and slices on the path.
	n    int              // The number of them in path.
	seen map[ptrKey]bool  // The ones past pathScan, made lazily.
	ftyp reflect.Type     // The struct type of the last fields call.
	fls  []encField       // Its plans.

	noLoop bool // The values encoded hold no cycle.
}

// fields returns the plans of the fields of the typ struct (see
// Encoder.fields); a call for the struct type of the call before takes them
// from the state.
func (est *encState) fields(enc *Encoder, typ reflect.Type) []encField {
	if typ != est.ftyp || est.fls == nil {
		est.ftyp, est.fls = typ, enc.fields(typ)
	}
	return est.fls
}

// newEncState returns the state of a new [Encoder.Encode] call.
func newEncState() *encState { return &encState{} }

// enter records the key of a pointer, map or slice the encoding path enters,
// and returns false when the path holds it already: a cycle. The keys must
// leave in the reverse order.
func (est *encState) enter(key ptrKey) bool {
	if est.noLoop {
		return true
	}
	if est.seen[key] {
		return false
	}
	for _, cur := range est.path[:est.n] {
		if cur == key {
			return false
		}
	}
	if est.n < pathScan {
		est.path[est.n] = key
		est.n++
		return true
	}
	if est.seen == nil {
		est.seen = make(map[ptrKey]bool)
	}
	est.seen[key] = true
	return true
}

// leave forgets the key of a pointer, map or slice the encoding path leaves,
// the last one that entered.
func (est *encState) leave(key ptrKey) {
	if est.noLoop {
		return
	}
	if est.seen[key] {
		delete(est.seen, key)
		return
	}
	est.n--
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
func (enc *Encoder) encSrc(
	dst url.Values,
	prefix string,
	src reflect.Value,
	inline bool,
) (bool, error) {

	reg := enc.reg
	// Look into an interface implementing a marshaler for the pointer it
	// may hold, unless a function is registered for the interface.
	isItf := src.Kind() == reflect.Interface
	if isItf && !src.IsNil() && enc.cache.isMarshaler(src.Type()) &&
		reg.lookup(src.Type()) == nil {

		src = src.Elem()
	}
	// Reduce pointers to pointers to a single pointer.
	for src.Kind() == reflect.Pointer && !src.IsNil() &&
		src.Type().Elem().Kind() == reflect.Pointer {

		src = src.Elem()
	}
	if src.Kind() != reflect.Pointer && src.CanAddr() {
		if ptr := src.Addr(); enc.cache.isMarshaler(ptr.Type()) {
			src = ptr
		}
	}

	typ := src.Type()
	inf := enc.cache.info(typ)
	if inline {
		// An inline value is never nil: only a Marshaler, which adds keys
		// under the prefix, encodes it.
		if !inf.mar || !src.CanInterface() {
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
	paired := inf.mar || enc.cache.textUnmarshaler(typ)
	if efn == nil && (inf.mar || inf.text) && src.CanInterface() &&
		(isNil(src) || paired) {

		name := "MarshalText"
		if inf.mar {
			name = "MarshalValues"
		}
		if isNil(src) || nilPromoted(src, name) {
			return true, nil
		}
		if m, ok := src.Interface().(Marshaler); ok {
			return true, m.MarshalValues(dst, prefix)
		}
		m := src.Interface().(encoding.TextMarshaler) //nolint:forcetypeassert
		text, err := marshalString(m)
		if err != nil {
			return true, err
		}
		dst[prefix] = append(dst[prefix], text)
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

	sp := enc.slicePlan(src.Type())
	if sp.plain != nil {
		return encodeValues(dst, prefix, src, sp.plain)
	}
	if sp.flat {
		// The key of each field written to the stack, then copied once.
		var buf [64]byte
		pre := append(buf[:0], prefix...)
		for i := range src.Len() {
			ipt := strconv.AppendInt(append(pre, '['), int64(i), 10)
			ipt = append(ipt, ']')
			if err := encodeFlat(dst, ipt, src.Index(i), sp.fls); err != nil {
				return err
			}
		}
		return nil
	}
	value, itf, lvl, byFields := sp.value, sp.itf, sp.lvl, sp.byFields
	for i := 0; i < src.Len(); i++ {
		val := src.Index(i)
		isVal := value
		if value && itf && !val.IsNil() {
			// The interface may hold a Marshaler, which is no value.
			isVal = enc.isValueType(val.Elem().Type())
		}
		if isVal {
			if ok, err := enc.encSrc(dst, prefix, val, false); ok {
				if err != nil {
					ipt := prefix + "[" + strconv.Itoa(i) + "]"
					return xrr.NewFieldError(ipt, err)
				}
				continue
			}
		}
		ipt := prefix + "[" + strconv.Itoa(i) + "]" // Only indexed values.
		var err error
		if byFields {
			err = enc.encodeFields(dst, ipt, val, est, lvl)
		} else {
			err = enc.encode(dst, ipt, val, est, lvl)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// slicePlan is how the encoder encodes the elements of a slice or array type
// (see encodeSlice).
type slicePlan struct {
	plain    encFn      // The encoder of plain value elements, or nil.
	value    bool       // The elements are values (see isValueType).
	itf      bool       // They are interfaces with no function registered.
	lvl      level      // The level of struct elements.
	byFields bool       // They are structs encoded by parts.
	fls      []encField // Their field plans.
	flat     bool       // Their fields are plain values (see isFlat).
}

// slicePlan returns the slicePlan of the typ slice or array type.
func (enc *Encoder) slicePlan(typ reflect.Type) *slicePlan {
	if enc.cache != nil {
		if sp, ok := enc.cache.slices.Load(typ); ok {
			return sp
		}
	}
	et := typ.Elem()
	sp := &slicePlan{plain: enc.plain(et)}
	if sp.plain == nil {
		sp.value = enc.isValueType(et)
		// A function registered for the interface encodes every value.
		sp.itf = et.Kind() == reflect.Interface && enc.reg.lookup(et) == nil
		sp.lvl = enc.elemLevel(typ)
		// For a struct encoded part by part, encode does no more than this.
		sp.byFields = et.Kind() == reflect.Struct && enc.isComposite(et)
		if sp.byFields {
			sp.fls = enc.fields(et)
			sp.flat = isFlat(sp.fls)
		}
	}
	if enc.cache != nil {
		enc.cache.slices.Store(typ, sp)
	}
	return sp
}

// isFlat returns true if a struct with the field plans fls is encoded, at its
// own level, by encoding plain values and slices of them: each field is
// skipped or has a plain or elems encoder.
func isFlat(fls []encField) bool {
	for _, fld := range fls {
		if !fld.skip && fld.plain == nil && fld.elems == nil {
			return false
		}
	}
	return true
}

// encodeFlat is encodeWith for the src struct with the flat field plans fls
// (see isFlat), at the level of its own type, under the path in pre.
func encodeFlat(
	dst url.Values,
	pre []byte,
	src reflect.Value,
	fls []encField,
) error {

	n := len(pre)
	for i, fld := range fls {
		if fld.skip {
			continue
		}
		val := src.Field(i)
		if fld.omit && isZero(val) || !fld.named {
			continue
		}
		pre = appendField(pre[:n], fld.name)
		key := string(pre)
		if fld.plain != nil {
			txt, err := fld.plain(val)
			if err != nil {
				return fieldError(key, err)
			}
			dst[key] = append(dst[key], txt)
			continue
		}
		if err := encodeValues(dst, key, val, fld.elems); err != nil {
			return err
		}
	}
	return nil
}

// appendField appends the field name to the path in pre as fieldPath does.
func appendField(pre []byte, name string) []byte {
	if strings.Contains(name, ".") {
		pre = append(pre, '[')
		pre = append(pre, name...)
		return append(pre, ']')
	}
	if len(pre) > 0 {
		pre = append(pre, '.')
	}
	return append(pre, name...)
}

// encodeValues adds the elements of the src slice or array, each encoded by
// plain, as values of the prefix key of dst.
func encodeValues(
	dst url.Values,
	prefix string,
	src reflect.Value,
	plain encFn,
) error {

	vls := dst[prefix]
	for i := range src.Len() {
		txt, err := plain(src.Index(i))
		if err != nil {
			ipt := prefix + "[" + strconv.Itoa(i) + "]"
			return xrr.NewFieldError(ipt, err)
		}
		vls = append(vls, txt)
	}
	if src.Len() > 0 {
		dst[prefix] = vls
	}
	return nil
}

// encodeMap encodes the src map to dst, each value under the prefix key and its
// map key ("attr[k]"). The keys are encoded with [encoding.TextMarshaler] or
// the base type encoders; an empty key, one with a bracket, one that is or
// holds a NaN, and two keys encoded to one text are errors. The entries are
// encoded in the order of their key texts, and of the keys failing to encode,
// the error of the first one in Go syntax order is returned; a map whose
// encoding cannot fail (see sureMap) is encoded in map order, which gives the
// same dst. The est is passed to the encode method.
func (enc *Encoder) encodeMap(
	dst url.Values,
	prefix string,
	src reflect.Value,
	est *encState,
) error {

	if mp := enc.mapPlan(src.Type()); mp != nil {
		return enc.encodeMapSure(dst, prefix, src, est, mp)
	}
	return enc.encodeMapSorted(dst, prefix, src, est)
}

// encodeMapSorted is encodeMap encoding the entries in key order.
func (enc *Encoder) encodeMapSorted(
	dst url.Values,
	prefix string,
	src reflect.Value,
	est *encState,
) error {

	type pair struct {
		key string        // Encoded map key.
		nan bool          // The map key is or holds a NaN.
		val reflect.Value // Addressable copy of the map value, unless plain.
		txt string        // Encoded plain map value.
		err error         // Error encoding the plain map value.
	}
	var kerr error    // Error of the failing key first in Go syntax order.
	var kerrGo string // The Go syntax of that key.
	kt, vt := src.Type().Key(), src.Type().Elem()
	lvl := enc.elemLevel(src.Type())
	plain := enc.plain(vt)
	// For a struct encoded part by part, encode does no more than this.
	byFields := vt.Kind() == reflect.Struct && enc.isComposite(vt)
	// A key that is no pointer or interface is copied to one value in turn,
	// and so is a plain value, which is encoded right away.
	var key1, val1 reflect.Value
	if kind := kt.Kind(); kind != reflect.Pointer && kind != reflect.Interface {
		key1 = reflect.New(kt).Elem()
	}
	if plain != nil {
		val1 = reflect.New(vt).Elem()
	}
	var prsBuf [8]pair // On the stack, for most maps.
	prs := prsBuf[:0]
	if src.Len() > len(prsBuf) {
		prs = make([]pair, 0, src.Len())
	}
	for it := src.MapRange(); it.Next(); {
		var mk reflect.Value
		if key1.IsValid() {
			key1.SetIterKey(it)
			mk = key1
		} else {
			mk = it.Key()
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
		}
		if isNil(mk) {
			continue // A nil key adds nothing.
		}
		key, err := encMapKey(mk, enc.reg, enc.cache)
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
		pr := pair{key: key, nan: isNaNKey(mk)}
		if plain != nil {
			val1.SetIterValue(it)
			pr.txt, pr.err = plain(val1)
		} else {
			pr.val = reflect.New(vt).Elem()
			pr.val.SetIterValue(it)
		}
		prs = append(prs, pr)
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
		case pr.nan:
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
		if plain != nil {
			if pr.err != nil {
				return fieldError(pth, pr.err)
			}
			dst[pth] = append(dst[pth], pr.txt)
			continue
		}
		var err error
		if byFields {
			err = enc.encodeFields(dst, pth, pr.val, est, lvl)
		} else {
			err = enc.encode(dst, pth, pr.val, est, lvl)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// sureMap is how the encoder encodes a map whose encoding never fails: its
// keys are of a bool, integer or string kind with no function registered and
// no text methods, encoded by the built-in encoder of their kind, so two keys
// never have one text; its values are plain values other than [time.Time], or
// structs encoded by parts whose encoding never fails (see encodesSure).
type sureMap struct {
	kt  reflect.Type // Key type.
	vt  reflect.Type // Value type.
	key encFn        // Key encoder.
	str bool         // The keys are strings, maybe empty or holding a bracket.
	val encFn        // Encoder of a plain value, or nil.
	fls []encField   // Plans of the fields of a struct value.
	lvl level        // Level of a struct value.
	flt bool         // The struct value is flat (see isFlat).
}

// mapPlan returns the sureMap of the typ map type, or nil when its encoding
// may fail.
func (enc *Encoder) mapPlan(typ reflect.Type) *sureMap {
	if enc.cache != nil {
		if mp, ok := enc.cache.maps.Load(typ); ok {
			return mp
		}
	}
	mp := enc.newMapPlan(typ)
	if enc.cache != nil {
		enc.cache.maps.Store(typ, mp)
	}
	return mp
}

// newMapPlan is mapPlan without the cache.
func (enc *Encoder) newMapPlan(typ reflect.Type) *sureMap {
	kt, vt := typ.Key(), typ.Elem()
	if !textKey(kt.Kind()) || !isBuiltin(kt) || enc.reg.lookup(kt) != nil {
		return nil
	}
	if inf := enc.cache.info(kt); inf.text || inf.ptext {
		return nil
	}
	mp := &sureMap{kt: kt, vt: vt, key: encFor(kt, enc.reg)}
	mp.str = kt.Kind() == reflect.String
	switch {
	case vt != typTime && enc.plain(vt) != nil:
		mp.val = enc.plain(vt)

	case vt.Kind() == reflect.Struct && enc.isComposite(vt):
		mp.fls = enc.fields(vt)
		if !encodesSure(mp.fls) {
			return nil
		}
		mp.lvl = enc.cache.level(vt, enc.tagName())
		mp.flt = isFlat(mp.fls)

	default:
		return nil
	}
	return mp
}

// encodeMapSure is encodeMap for a src map encoded as mp says: no key or value
// fails, and each entry adds keys of its own, so the entries are encoded in
// the order of the map. A string key that is empty or holds a bracket, an
// error, has the map encoded in key order as encodeMap does.
func (enc *Encoder) encodeMapSure(
	dst url.Values,
	prefix string,
	src reflect.Value,
	est *encState,
	mp *sureMap,
) error {

	if src.Len() == 0 {
		return nil
	}
	key := reflect.New(mp.kt).Elem()
	if mp.val != nil && src.Len() <= sureBuf {
		return enc.encodeFewSure(dst, prefix, src, est, mp, key)
	}
	if mp.str {
		for it := src.MapRange(); it.Next(); {
			key.SetIterKey(it)
			txt := key.String()
			if txt == "" || strings.ContainsAny(txt, "[]") {
				return enc.encodeMapSorted(dst, prefix, src, est)
			}
		}
	}
	val := reflect.New(mp.vt).Elem()
	var buf [64]byte
	pre := append(buf[:0], prefix...)
	for it := src.MapRange(); it.Next(); {
		key.SetIterKey(it)
		val.SetIterValue(it)
		txt, _ := mp.key(key) // Encoding a sure key never fails.
		if mp.flt {
			// The key of each field written to the stack, then copied once.
			kpt := append(append(append(pre, '['), txt...), ']')
			if err := encodeFlat(dst, kpt, val, mp.fls); err != nil {
				return err
			}
			continue
		}
		pth := prefix + "[" + txt + "]"
		if mp.val != nil {
			txt, _ = mp.val(val) // Encoding a plain value never fails.
			dst[pth] = append(dst[pth], txt)
			continue
		}
		err := enc.encodeWith(dst, pth, val, est, mp.lvl, mp.fls)
		if err != nil {
			return err
		}
	}
	return nil
}

// sureBuf is the number of map entries encodeFewSure encodes in one pass.
const sureBuf = 8

// encodeFewSure is encodeMapSure for a map of at most sureBuf plain values,
// key the value its keys are copied to: one pass encodes the entries to the
// stack, and they are added to dst once no key is an error.
func (enc *Encoder) encodeFewSure(
	dst url.Values,
	prefix string,
	src reflect.Value,
	est *encState,
	mp *sureMap,
	key reflect.Value,
) error {

	type entry struct{ key, val string }
	var ents [sureBuf]entry
	val := reflect.New(mp.vt).Elem()
	n := 0
	for it := src.MapRange(); it.Next(); {
		key.SetIterKey(it)
		txt, _ := mp.key(key) // Encoding a sure key never fails.
		if mp.str && (txt == "" || strings.ContainsAny(txt, "[]")) {
			return enc.encodeMapSorted(dst, prefix, src, est)
		}
		val.SetIterValue(it)
		vtx, _ := mp.val(val) // Encoding a plain value never fails.
		ents[n] = entry{key: txt, val: vtx}
		n++
	}
	for _, ent := range ents[:n] {
		pth := prefix + "[" + ent.key + "]"
		dst[pth] = append(dst[pth], ent.val)
	}
	return nil
}

// elemLevel returns the level of the elements of the typ container when they
// are structs or pointers to structs; else it returns the zero level.
func (enc *Encoder) elemLevel(typ reflect.Type) level {
	if et := indirect(typ.Elem()); et.Kind() == reflect.Struct {
		return enc.cache.level(et, enc.tagName())
	}
	return level{}
}

// isValueType returns true if a value of the typ type, or a pointer to one,
// is encoded as one text value: by a registered encoder, by its
// [encoding.TextMarshaler] when it also implements [encoding.TextUnmarshaler],
// or by a built-in encoder. A [Marshaler] is not. An interface implementing
// [encoding.TextMarshaler] is one; the value it holds decides.
func (enc *Encoder) isValueType(typ reflect.Type) bool {
	return enc.cache.value(typ, enc.valueType)
}

// valueType is isValueType without the cache.
func (enc *Encoder) valueType(typ reflect.Type) bool {
	if enc.reg.lookup(typ) != nil {
		return true // Registered for typ or for the type it points to.
	}
	typ = indirect(typ)
	inf := enc.cache.info(typ)
	if inf.mar || inf.pmar {
		return false
	}
	if inf.text || inf.ptext {
		if typ.Kind() == reflect.Interface || enc.cache.textUnmarshaler(typ) {
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
	return enc.cache.leaf(typ, enc.leafType)
}

// leafType is isLeafType without the cache.
func (enc *Encoder) leafType(typ reflect.Type) bool {
	if inf := enc.cache.info(typ); inf.mar || inf.pmar {
		return true
	}
	return enc.isValueType(typ)
}

// errNilKey reports a map key whose MarshalText method would be reached
// through a nil embedded field (see nilPromoted); such a key counts as nil.
var errNilKey = errors.New("nil map key")

// encMapKey encodes the map key to a string, with the reg registry, getting
// the facts about types from tc. The key must not be nil or an interface
// holding a nil pointer. A key counting as nil returns errNilKey. A
// float key of -0, or a pointer to one, encodes as "0".
func encMapKey(key reflect.Value, reg encReg, tc *typeCache) (string, error) {
	efn := reg.lookup(key.Type())
	if efn == nil && key.Kind() == reflect.Interface {
		key = key.Elem()
		efn = reg.lookup(key.Type())
	}
	if efn != nil {
		return efn(key)
	}
	inf := tc.info(key.Type())
	if key.Kind() != reflect.Pointer && inf.ptext {
		key = addressable(key).Addr()
		inf = tc.info(key.Type())
	}
	typ := key.Type()
	if inf.text {
		if nilPromoted(key, "MarshalText") {
			return "", errNilKey
		}
		m := key.Interface().(encoding.TextMarshaler) //nolint:forcetypeassert
		return marshalString(m)
	}
	if val := reflect.Indirect(key); val.CanFloat() && val.Float() == 0 {
		return "0", nil // Decode takes a -0 key for the 0 one.
	}
	if efn = encFor(typ, reg); efn == nil {
		return "", encNotFound(typ)
	}
	return efn(key)
}

// marshalString returns the text m, which must not be a nil pointer,
// marshals to. A [time.Time] whose zone offset has seconds, which RFC 3339
// cannot write, is marshaled in UTC, so it decodes back to the same instant,
// unless its year in UTC is one RFC 3339 cannot write either. A struct
// embedding a [time.Time] is marshaled by the method it gets from it.
func marshalString(m encoding.TextMarshaler) (string, error) {
	var tim time.Time
	switch val := m.(type) {
	case time.Time:
		tim = val

	case *time.Time:
		tim = *val

	default:
		text, err := m.MarshalText()
		return string(text), err
	}
	return timeString(tim)
}

// timeString returns the text marshalString returns for tim.
func timeString(tim time.Time) (string, error) {
	if _, off := tim.Zone(); off%60 != 0 {
		if utc := tim.UTC(); utc.Year() >= 0 && utc.Year() <= 9999 {
			tim = utc
		}
	}
	// The text [time.Time.MarshalText] returns, written to the stack first.
	var buf [len(time.RFC3339Nano) + 8]byte
	text, err := tim.AppendText(buf[:0])
	if err != nil {
		_, err = tim.MarshalText() // The error names MarshalText.
		return "", err
	}
	return string(text), nil
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
