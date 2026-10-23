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

// WithDecMaxIndex is a [NewDecoder] option setting the largest slice or
// array index a src key may use; a larger one is an [ErrInvValue] field
// error. A zero n allows only index 0, and a negative n rejects every index.
// The default is [DefaultMaxIndex].
func WithDecMaxIndex(n int) DecoderOption {
	return func(dec *Decoder) { dec.maxIndex = &n }
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
	maxIndex     *int   // Largest index; nil means DefaultMaxIndex.
	reg          decReg // Decoders registered with WithDecFunc.
}

// DefaultMaxIndex is the largest slice or array index a src key may use
// unless [WithDecMaxIndex] sets another limit.
const DefaultMaxIndex = 1000

// NewDecoder returns a new instance of [Decoder] reading the "url" struct tag
// and rejecting unknown keys unless the options say otherwise.
func NewDecoder(opts ...DecoderOption) *Decoder {
	dec := &Decoder{tag: defaultTag}
	for _, opt := range opts {
		opt(dec)
	}
	return dec
}

// Decode sets the fields of the dst struct from src. The dst argument must be a
// pointer to a struct. The src keys are paths to the dst fields: struct field
// names follow a dot, and map keys and slice or array indexes go in brackets
// ("page.size", "attr[k]", "list[0].name"); a bracketed segment may contain
// dots, so "[page.size]" names a field tagged page.size. A map of values also
// takes the rest of a key after a dot as its key ("attr.a.b"), and an index may
// follow a dot ("list.0"). An index over [DefaultMaxIndex], or the limit
// [WithDecMaxIndex] sets, is an [ErrInvValue] field error. Among the fields of
// a struct and the structs it embeds, a key names the field Go promotion
// selects, as in [encoding/json]. Errors of the src values are returned as
// [xrr.FieldErrors] keyed by the src keys; the other values are still set, and
// a key with a value error adds no slice element or map entry. The first key
// reaching a slice or an array replaces it: the values of one key replace the
// elements, indexed keys fill a new slice whose gaps are zero values, and array
// elements no key sets are zeroed; a field with no key keeps its value. Keys
// are added to an existing map, and the values of a map are updated in place;
// keys decoding to equal values name one entry, and a NaN key is an
// [ErrInvValue] field error. A nil pointer to a nested struct is allocated only
// when a src key leads into it. A field with the "required" option is an
// [ErrRequired] error when it is empty after decoding: a zero number, an empty
// string, slice or map, a nil pointer, or a zero struct; a bool is never empty.
// The required fields of a nil struct pointer, an absent struct, are not
// checked; those of the structs in maps, slices and arrays are, under paths
// like "items[k].name". A src key leading into an embedded nil pointer to a
// struct, which the decoder cannot allocate because the field is unexported, is
// an [ErrUnexported] field error. A struct tag of dst that does not parse is an
// [ErrInvTag] error. Pointers to pointers to structs are not supported.
func (dec *Decoder) Decode(dst any, src url.Values) error {
	sv := mirror.NewStructValue(dst)
	if sv == nil {
		return ErrReqStructPtr
	}
	seen := map[reflect.Type]bool{}
	if err := checkTags("", sv.Type(), dec.tagName(), seen); err != nil {
		return err
	}
	return dec.decode(sv, reflect.ValueOf(dst).Elem(), src).Filter()
}

// tagName returns the name of the struct tag the decoder reads.
func (dec *Decoder) tagName() string {
	if dec.tag == "" {
		return defaultTag
	}
	return dec.tag
}

// indexLimit returns the largest index a src key may use.
func (dec *Decoder) indexLimit() int {
	if dec.maxIndex == nil {
		return DefaultMaxIndex
	}
	return *dec.maxIndex
}

// decode decodes src to the dst struct, sv and val being two views of it, and
// returns the errors of the src values and of the required fields left
// empty.
func (dec *Decoder) decode(
	sv *mirror.StructValue,
	val reflect.Value,
	src url.Values,
) *xrr.FieldErrors {

	var ers *xrr.FieldErrors
	setErr := func(pth string, err error) {
		if ers == nil {
			ers = &xrr.FieldErrors{}
		}
		ers.Set(pth, err)
	}

	stt := newState()
	for key, vls := range src {
		segs, ok := parseKey(key)
		var steps []step
		var err error
		if !ok {
			err = ErrUnkFieldPath
		} else {
			steps, err = dec.plan(val.Type(), segs)
		}
		if err == nil {
			err = dec.apply(val, steps, vls, stt, "")
		}
		switch {
		case err == nil, errors.Is(err, errIgnored):
		case errors.Is(err, ErrUnkFieldPath):
			if !dec.allowUnknown {
				setErr(key, ErrUnkFieldPath)
			}
		default:
			setErr(key, err)
		}
	}

	// A field with a value error keeps that error.
	lvl := newLevel(sv.Type(), dec.tagName())
	dec.required("", sv, lvl, map[ptrKey]bool{}, func(pth string, err error) {
		if ers == nil || ers.Get(pth) == nil {
			setErr(pth, err)
		}
	})
	return ers
}

// segment is one part of a src key: a name, or a literal written in
// brackets, which may contain dots.
type segment struct {
	text    string // Segment text.
	bracket bool   // The segment was written in brackets.
}

// parseKey splits the src key into segments: names separated by dots, and
// literals in brackets ("a.b[c.d][0]"). It returns false for an empty key or
// segment, a bracket that does not close, a bracket in brackets, and text or
// a dot that does not follow the rules.
func parseKey(key string) ([]segment, bool) {
	var segs []segment
	for i := 0; i < len(key); {
		if key[i] == '[' {
			end := strings.IndexAny(key[i+1:], "[]")
			if end <= 0 || key[i+1+end] != ']' {
				return nil, false
			}
			txt := key[i+1 : i+1+end]
			segs = append(segs, segment{text: txt, bracket: true})
			i += end + 2
		} else {
			end := strings.IndexAny(key[i:], ".[]")
			if end < 0 {
				end = len(key) - i
			}
			if end == 0 {
				return nil, false
			}
			segs = append(segs, segment{text: key[i : i+end]})
			i += end
		}
		if i == len(key) || key[i] == '[' {
			continue
		}
		// Only a dot followed by a name may come next.
		if key[i] != '.' || i+1 == len(key) ||
			strings.IndexByte(".[]", key[i+1]) >= 0 {

			return nil, false
		}
		i++
	}
	return segs, len(segs) > 0
}

// stepKind tells what a step of a key path selects.
type stepKind uint8

// Kinds of steps.
const (
	stepField stepKind = iota // A struct field.
	stepKey                   // A map entry.
	stepIndex                 // A slice or array element.
)

// step is one move along a key path, from a value to a part of it.
type step struct {
	kind stepKind
	idx  []int  // Field index path, through embedded structs.
	key  string // Map key text.
	pos  int    // Slice or array index.
	name string // Canonical path segment: a field name, [key] or [n].
}

// errIgnored reports a src key naming an ignored field; the key is skipped.
var errIgnored = errors.New("ignored field")

// plan returns the steps a src key, split into segs, takes from a value of
// type typ. A struct segment names a field, a map segment is the key, and a
// slice or array segment is an index; a map of plain values given a key that
// is not bracketed takes the rest of the key as its key. The typ struct
// itself is always walked field by field, like the encoder does at the empty
// path, even when it has a promoted UnmarshalText method. A key naming
// nothing returns [ErrUnkFieldPath], one naming an ignored field returns
// errIgnored, and an index over the limit or past an array end returns an
// error wrapping [ErrInvValue].
func (dec *Decoder) plan(typ reflect.Type, segs []segment) ([]step, error) {
	var steps []step
	for {
		typ = indirect(typ)
		leaf := len(steps) > 0 && dec.isLeafType(typ)
		if len(segs) == 0 {
			// Structs and maps take more of the key.
			kind := typ.Kind()
			if !leaf && (kind == reflect.Struct || kind == reflect.Map) {
				return nil, ErrUnkFieldPath
			}
			return steps, nil
		}
		seg := segs[0]
		switch {
		case leaf:
			return nil, ErrUnkFieldPath

		case typ.Kind() == reflect.Struct:
			lvl := newLevel(typ, dec.tagName())
			idx, ok := lvl.vis[seg.text]
			if !ok {
				if lvl.ign[seg.text] {
					return nil, errIgnored
				}
				return nil, ErrUnkFieldPath
			}
			stp := step{kind: stepField, idx: idx, name: seg.text}
			steps = append(steps, stp)
			typ = typ.FieldByIndex(idx).Type
			segs = segs[1:]

		case typ.Kind() == reflect.Map:
			key := seg.text
			segs = segs[1:]
			if !seg.bracket && dec.isLeafValue(typ.Elem()) {
				txt := []string{key}
				for _, nxt := range segs {
					if nxt.bracket {
						return nil, ErrUnkFieldPath
					}
					txt = append(txt, nxt.text)
				}
				key = strings.Join(txt, ".")
				segs = nil
			}
			stp := step{kind: stepKey, key: key, name: "[" + key + "]"}
			steps = append(steps, stp)
			typ = typ.Elem()

		default: // A slice or an array, the other types that are not leaves.
			pos, err := dec.index(typ, seg.text)
			if err != nil {
				return nil, err
			}
			name := "[" + strconv.Itoa(pos) + "]"
			steps = append(steps, step{kind: stepIndex, pos: pos, name: name})
			typ = typ.Elem()
			segs = segs[1:]
		}
	}
}

// index returns the index txt names in a slice or array of type typ. Text
// that is not a decimal number returns [ErrUnkFieldPath]; an index over the
// limit, or past the end of an array, returns an error wrapping
// [ErrInvValue].
func (dec *Decoder) index(typ reflect.Type, txt string) (int, error) {
	if strings.TrimLeft(txt, "0123456789") != "" {
		return 0, ErrUnkFieldPath
	}
	pos, err := strconv.Atoi(txt)
	// No slice holds the largest int index, and the length past it overflows.
	if err != nil || pos > dec.indexLimit() || pos == math.MaxInt {
		const format = "%w: index %s over the limit %d"
		code := xrr.WithCode(ECInvValue)
		return 0, xrr.Newf(format, ErrInvValue, txt, dec.indexLimit(), code)
	}
	if typ.Kind() == reflect.Array && pos >= typ.Len() {
		const format = "%w: index %d past the array end"
		return 0, xrr.Newf(format, ErrInvValue, pos, xrr.WithCode(ECInvValue))
	}
	return pos, nil
}

// state is the state of one Decode call.
type state struct {
	reset   map[string]bool     // Canonical paths of slices and arrays reset.
	log     []string            // The paths in reset, in the order set.
	entries map[string]mapEntry // Map entries reached, by entry identity.
}

// mapEntry is a map entry src keys reach.
type mapEntry struct {
	key  reflect.Value // The map key, decoded from the first src key.
	text string        // The key text in the canonical path of the entry.
}

// newState returns the state of a new Decode call.
func newState() *state {
	return &state{
		reset:   make(map[string]bool),
		entries: make(map[string]mapEntry),
	}
}

// entry returns the entry of the map at the canonical path pth that the
// decoded key names; txt is the src text of the key. Keys decoding to equal
// values, by their encoded text, name one entry: its key is the first one
// decoded, so pointer keys and keys holding pointers find it again, and its
// text is the encoded text or, when the key type has no encoder, the src
// text of the first key.
func (stt *state) entry(pth string, key reflect.Value, txt string) mapEntry {
	id := pth + "[" + keyText(key) + "]"
	if ent, ok := stt.entries[id]; ok {
		return ent
	}
	ent := mapEntry{key: key, text: txt}
	if enc, err := encMapKey(key, nil); err == nil {
		ent.text = enc
	}
	stt.entries[id] = ent
	return ent
}

// markReset records the canonical path pth as reset.
func (stt *state) markReset(pth string) {
	if !stt.reset[pth] {
		stt.reset[pth] = true
		stt.log = append(stt.log, pth)
	}
}

// undo forgets the paths reset since the log held n of them.
func (stt *state) undo(n int) {
	for _, pth := range stt.log[n:] {
		delete(stt.reset, pth)
	}
	stt.log = stt.log[:n]
}

// apply moves from dst along the steps and sets the value reached to vls.
// Nil pointers on the way are allocated, and map entries are written back
// after the value in them is set. The first step into a slice or array in a
// Decode call resets it; stt holds the canonical paths reset so far, and pth
// is the canonical path of dst. A key whose value fails writes no map entry
// and adds no slice elements, and the resets it made in a map entry are
// forgotten.
func (dec *Decoder) apply(
	dst reflect.Value,
	steps []step,
	vls []string,
	stt *state,
	pth string,
) error {

	if len(steps) == 0 {
		stt.markReset(pth)
		ok, err := setDst(dst, dec.reg, vls...)
		if !ok {
			return decNotFound(dst.Type())
		}
		return err
	}
	dst, err := deref(dst)
	if err != nil {
		return err
	}
	stp := steps[0]
	switch stp.kind {
	case stepField:
		for n, i := range stp.idx {
			if n > 0 {
				if dst, err = deref(dst); err != nil {
					return err
				}
			}
			dst = dst.Field(i)
		}
		return dec.apply(dst, steps[1:], vls, stt, fieldPath(pth, stp.name))

	case stepKey:
		if dst.IsNil() {
			dst.Set(reflect.MakeMap(dst.Type()))
		}
		key, err := decKey(dst.Type().Key(), dec.reg, stp.key)
		if err != nil {
			return err
		}
		if isNaN(key) {
			// No lookup finds a NaN key, so its entry cannot be read back.
			const format = "%w: NaN map key"
			return xrr.Newf(format, ErrInvValue, xrr.WithCode(ECInvValue))
		}
		ent := stt.entry(pth, key, stp.key)
		key = ent.key
		kpt := pth + "[" + ent.text + "]"
		val := reflect.New(dst.Type().Elem()).Elem()
		// A value the key sets as a whole is new, so a pointer in the map
		// keeps its pointee.
		if cur := dst.MapIndex(key); cur.IsValid() && len(steps) > 1 {
			val.Set(cur)
		}
		mark := len(stt.log)
		err = dec.apply(val, steps[1:], vls, stt, kpt)
		if err != nil {
			stt.undo(mark) // The value with the resets is dropped.
			return err
		}
		dst.SetMapIndex(key, val)
		return nil

	default:
		if !stt.reset[pth] {
			stt.markReset(pth)
			dst.SetZero()
		}
		size := dst.Len()
		if dst.Kind() == reflect.Slice && size <= stp.pos {
			dst.Grow(stp.pos + 1 - size)
			dst.SetLen(stp.pos + 1)
		}
		elem := dst.Index(stp.pos)
		err := dec.apply(elem, steps[1:], vls, stt, pth+stp.name)
		if err != nil && dst.Len() > size {
			for i := size; i < dst.Len(); i++ {
				dst.Index(i).SetZero()
			}
			dst.SetLen(size)
		}
		return err
	}
}

// isLeafType returns true if a value of the typ type, which is not a
// pointer, is set from src values as a whole: by a registered decoder, by its
// [encoding.TextUnmarshaler], or by a built-in decoder. Structs, maps,
// slices and arrays without one take more of the key.
func (dec *Decoder) isLeafType(typ reflect.Type) bool {
	if dec.reg[typ] != nil || typ == typTime {
		return true
	}
	if typ.Implements(iUnmarshaler) ||
		reflect.PointerTo(typ).Implements(iUnmarshaler) {

		return true
	}
	switch typ.Kind() { //nolint:exhaustive
	case reflect.Struct, reflect.Map, reflect.Slice, reflect.Array:
		return false

	default:
		return true
	}
}

// isLeafValue returns true if a map value of the typ type is set from src
// values without more of the key: a leaf type, or a slice or array of one.
func (dec *Decoder) isLeafValue(typ reflect.Type) bool {
	typ = indirect(typ)
	if dec.isLeafType(typ) {
		return true
	}
	kind := typ.Kind()
	if kind == reflect.Slice || kind == reflect.Array {
		return dec.isLeafType(indirect(typ.Elem()))
	}
	return false
}

// required sets an [ErrRequired] error, using setErr, for each required field
// of dst whose value is empty (see isEmpty). A nil struct pointer is an
// absent struct, so its fields are not checked. Ignored fields and unexported
// fields, other than embedded structs, are skipped; an embedded struct has no
// path, so its required option is ignored. The lvl is the level position of
// dst; a field its key does not name there is skipped. The seen map holds
// the struct pointers on the path to dst, so a pointer cycle ends.
func (dec *Decoder) required(
	prefix string,
	dst *mirror.StructValue,
	lvl level,
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

		name := tag.NameOrField()
		if (!anon || !isStruct) && !lvl.names(name, i) {
			continue
		}
		fldPath := fieldPath(prefix, name)
		if isStruct && (anon || !dec.isLeafType(indirect(fld.Type()))) {
			// The path to a field that comes from an embedded struct does
			// not include the name of the embedded struct.
			if anon {
				fldPath = prefix
			} else if tag.Contains("required") && isEmpty(fld.Value()) {
				setErr(fldPath, ErrRequired)
			}
			nxt := lvl.embedded(i)
			if !anon {
				nxt = newLevel(fld.Type(), dec.tagName())
			}
			val := fld.Value()
			if val.Kind() != reflect.Pointer {
				dec.required(fldPath, fld.StructValue(), nxt, seen, setErr)
				continue
			}
			key := ptrKey{ptr: val.Pointer(), typ: val.Type()}
			if val.IsNil() || seen[key] {
				continue // An absent struct, or a pointer cycle.
			}
			seen[key] = true
			dec.required(fldPath, fld.StructValue(), nxt, seen, setErr)
			delete(seen, key)
			continue
		}
		if tag.Contains("required") && isEmpty(fld.Value()) {
			setErr(fldPath, ErrRequired)
		}
		dec.requiredIn(fldPath, fld.Value(), seen, setErr)
	}
}

// requiredIn checks the required fields of the structs held in val, a slice,
// array or map, or a pointer to one, and in the containers nested in it; pth
// is the canonical path of val, and seen is as for required.
func (dec *Decoder) requiredIn(
	pth string,
	val reflect.Value,
	seen map[ptrKey]bool,
	setErr func(string, error),
) {

	if val.Kind() == reflect.Pointer {
		key := ptrKey{ptr: val.Pointer(), typ: val.Type()}
		if val.IsNil() || seen[key] {
			return
		}
		seen[key] = true
		defer delete(seen, key)
		val = val.Elem()
	}
	if dec.isLeafType(val.Type()) {
		return
	}
	switch val.Kind() { //nolint:exhaustive
	case reflect.Struct:
		if !val.CanAddr() {
			val = addressable(val)
		}
		sv := mirror.NewStructValue(val.Addr().Interface())
		lvl := newLevel(val.Type(), dec.tagName())
		dec.required(pth, sv, lvl, seen, setErr)

	case reflect.Slice, reflect.Array:
		for i := 0; i < val.Len(); i++ {
			ipt := pth + "[" + strconv.Itoa(i) + "]"
			dec.requiredIn(ipt, val.Index(i), seen, setErr)
		}

	case reflect.Map:
		for it := val.MapRange(); it.Next(); {
			kpt := pth + "[" + keyText(it.Key()) + "]"
			dec.requiredIn(kpt, it.Value(), seen, setErr)
		}
	}
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
// in vls will be decoded based on the array or slice type and set. A slice is
// replaced by a new one holding the values. An array gets the values on
// indexes in the same order as in vls; the elements past the values are
// zeroed.
func setSliceOrArray(dst reflect.Value, reg decReg, vls ...string) error {
	et := dst.Type().Elem() // Element type.
	if dst.Kind() == reflect.Array {
		if dst.Len() < len(vls) {
			const format = "%w: too many values for an array"
			return xrr.Newf(format, ErrInvValue, xrr.WithCode(ECInvValue))
		}
		for i := len(vls); i < dst.Len(); i++ {
			dst.Index(i).SetZero()
		}
	} else {
		dst.Set(reflect.MakeSlice(dst.Type(), 0, len(vls)))
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

// decKey returns a map key of the kt type decoded from key.
func decKey(kt reflect.Type, reg decReg, key string) (reflect.Value, error) {
	nk := reflect.New(indirect(kt))
	ok, err := setDst(nk.Elem(), reg, key)
	if !ok {
		return reflect.Value{}, decNotFound(kt)
	}
	if err != nil {
		return reflect.Value{}, err
	}
	if kt.Kind() != reflect.Pointer {
		nk = nk.Elem() // Map key is not a pointer type.
	}
	return nk, nil
}

// deref returns the value val points to, through any number of pointers,
// allocating the nil ones. A nil pointer that cannot be set, an embedded
// pointer to an unexported struct type, returns [ErrUnexported].
func deref(val reflect.Value) (reflect.Value, error) {
	for val.Kind() == reflect.Pointer {
		if val.IsNil() {
			if !val.CanSet() {
				return val, ErrUnexported
			}
			val.Set(reflect.New(val.Type().Elem()))
		}
		val = val.Elem()
	}
	return val, nil
}

// decNotFound returns an error wrapping [ErrDecNotFound] naming the typ type.
func decNotFound(typ reflect.Type) error {
	code := xrr.WithCode(ECDecNotFound)
	return xrr.Newf("%w for %#q", ErrDecNotFound, typ.String(), code)
}
