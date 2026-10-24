// SPDX-FileCopyrightText: (c) 2026 Rafal Zajac
// SPDX-License-Identifier: MIT

package xbind

import (
	"encoding"
	"errors"
	"maps"
	"math"
	"net/url"
	"reflect"
	"slices"
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

// WithDecMaxIndex is a [NewDecoder] option setting the largest slice or array
// index a src key may use; a larger one is an [ErrInvValue] field error. A zero
// n allows only index 0, and a negative n rejects every index. The limit also
// bounds the slice elements one src may allocate (see [Decoder.Decode]), so a
// large n lets a small src allocate a large slice. The default is
// [DefaultMaxIndex].
func WithDecMaxIndex(n int) DecoderOption {
	return func(dec *Decoder) { dec.maxIndex = &n }
}

// WithDecFunc is a [NewDecoder] option decoding every value of type T with
// fn: fields of type T or *T, and slice, array and map elements and map keys
// of those types. A value of type T takes a single src value, and an error
// returned by fn is returned as is. The function takes precedence over the
// [encoding.TextUnmarshaler] method of T and the decoders the package
// provides, [time.Time] and [time.Duration] included. For a pointer T, fn
// decodes the type T points to, and a nil pointer it returns is an
// [ErrInvValue] error. For an interface T, fn may return nil, which sets a
// nil interface.
func WithDecFunc[T any](fn func(string) (T, error)) DecoderOption {
	typ := reflect.TypeFor[T]()
	isPtr := typ.Kind() == reflect.Pointer
	if isPtr {
		typ = typ.Elem() // The decoder dereferences pointers first.
	}
	return func(dec *Decoder) {
		if dec.reg == nil {
			dec.reg = make(decReg)
		}
		dec.reg[typ] = func(vls ...string) (any, error) {
			val, err := single(vls)
			if err != nil {
				return nil, err
			}
			res, err := fn(val)
			if err != nil || !isPtr {
				return res, err
			}
			ptr := reflect.ValueOf(res)
			if ptr.IsNil() {
				const format = "%w: decoder returned nil %#q"
				code := xrr.WithCode(ECInvValue)
				name := ptr.Type().String()
				return nil, xrr.Newf(format, ErrInvValue, name, code)
			}
			return ptr.Elem().Interface(), nil
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
// [WithDecMaxIndex] sets, is an [ErrInvValue] field error, and so is an index
// adding more slice elements, gaps included, than the indexed keys of one call
// may add: as many as the limit plus one, and one more per src key. Among the
// fields of a struct and the structs it embeds, a key names the field Go
// promotion selects, as in [encoding/json]. Errors of the src values are
// returned as [xrr.FieldErrors] keyed by the src keys; the other values are
// still set, and a key with an error changes nothing but what a failing
// UnmarshalText method wrote through the pointers and maps its value holds. The
// first key reaching a slice or an array replaces it: the values of one key
// replace the elements, indexed keys fill a new slice whose gaps are zero
// values, and array elements no key sets are zeroed; a field with no key keeps
// its value. The src keys are applied in sorted order; a key reaching a value
// another key sets, or one key setting the values of a slice or array other
// keys index, is an [ErrInvValue] field error on the later key. Keys are added
// to an existing map, and the values of a map are updated in place; keys
// decoding to equal values name one entry, and a key that is or holds a NaN is
// an [ErrInvValue] field error. A nil pointer to a nested struct is allocated
// only when a src key leads into it. A field with the "required" option is an
// [ErrRequired] error when it is empty after decoding: a zero number, an empty
// string, slice or map, a nil pointer or a pointer to an empty value, or a zero
// struct; a bool is never empty. The required fields of a nil struct pointer,
// an absent struct, are not checked; those of the structs in maps, slices and
// arrays are, under paths like "items[k].name". A src key leading into an
// embedded nil pointer to a struct, which the decoder cannot allocate because
// the field is unexported, is an [ErrUnexported] field error. A struct tag of
// dst that does not parse, or that names a field with a bracket ("ids[]") or an
// embedded struct, is an [ErrInvTag] error. Pointers to pointers to structs are
// not supported.
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
	keys := make(map[string]bool) // The keys of ers.
	setErr := func(pth string, err error) {
		if ers == nil {
			ers = &xrr.FieldErrors{}
		}
		ers.Set(pth, err)
		keys[pth] = true
	}

	stt := newState()
	// Indexed keys may add one full slice of elements, and one per key.
	if lim := dec.indexLimit(); lim < math.MaxInt-len(src)-1 {
		stt.budget = max(lim, 0) + 1 + len(src)
	}
	for _, key := range slices.Sorted(maps.Keys(src)) {
		vls := src[key]
		segs, ok := parseKey(key)
		var steps []step
		var err error
		if !ok {
			err = ErrUnkFieldPath
		} else {
			steps, err = dec.plan(val.Type(), segs, stt)
		}
		// Only the key names nothing; a value error may wrap the sentinel.
		unknown := errors.Is(err, ErrUnkFieldPath)
		if err == nil {
			mark := len(stt.log)
			if err = dec.apply(val, steps, vls, stt, ""); err != nil {
				stt.undo(mark) // A key that fails changes nothing.
			}
		}
		switch {
		case err == nil, errors.Is(err, errIgnored):

		case unknown:
			if !dec.allowUnknown {
				setErr(key, ErrUnkFieldPath)
			}

		default:
			setErr(key, err)
		}
	}

	// A field with a value error keeps that error.
	lvl := stt.level(sv.Type(), dec.tagName())
	seen := map[ptrKey]bool{}
	dec.required("", sv, lvl, seen, stt, func(pth string, err error) {
		if !stt.failed[pth] && !keys[pth] {
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
// error wrapping [ErrInvValue]. The levels of the structs come from stt.
func (dec *Decoder) plan(
	typ reflect.Type,
	segs []segment,
	stt *state,
) ([]step, error) {

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
			lvl := stt.level(typ, dec.tagName())
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

// state is the state of one [Decoder.Decode] call.
type state struct {
	reset   map[string]bool        // Canonical paths of the reset containers.
	log     []resetLog             // The resets and saves, in the order made.
	entries map[string]mapEntry    // Map entries reached, by entry identity.
	maps    map[string]bool        // Canonical paths of the maps reached.
	failed  map[string]bool        // Canonical paths of the values that failed.
	set     map[string]bool        // Canonical paths of the values keys set.
	indexed map[string]bool        // Canonical paths of the indexed containers.
	budget  int                    // Slice elements indexed keys may still add.
	levels  map[reflect.Type]level // Levels of the struct types met.
}

// resetLog is a reset of a slice or array, or a value a src key sets, or a
// value saved before a src key changes it.
type resetLog struct {
	pth   string        // Canonical path of the value.
	val   reflect.Value // The value, when the reset zeroed it or it was saved.
	old   reflect.Value // A copy of val from before the change.
	saved bool          // The value was saved, not reset.
}

// mapEntry is a map entry src keys reach.
type mapEntry struct {
	key   reflect.Value // The map key, decoded from the first src key.
	text  string        // The key text in the canonical path of the entry.
	named bool          // A src key named the entry.
}

// newState returns the state of a new [Decoder.Decode] call, with no element
// budget.
func newState() *state {
	return &state{
		reset:   make(map[string]bool),
		entries: make(map[string]mapEntry),
		maps:    make(map[string]bool),
		failed:  make(map[string]bool),
		set:     make(map[string]bool),
		indexed: make(map[string]bool),
		budget:  math.MaxInt,
		levels:  make(map[reflect.Type]level),
	}
}

// level returns the level of the typ struct for the key struct tag, built on
// the first call for the type.
func (stt *state) level(typ reflect.Type, key string) level {
	lvl, ok := stt.levels[typ]
	if !ok {
		lvl = newLevel(typ, key)
		stt.levels[typ] = lvl
	}
	return lvl
}

// entry returns the entry of the map at the canonical path pth that the
// decoded key names; txt is the src text of the key. Keys decoding to equal
// values, by their encoded text or, when the key type has no encoder, by
// their Go syntax, name one entry: its key is the first one decoded, so
// pointer keys and keys holding pointers find it again, and its text is the
// encoded text or, when the key type has no encoder or keep is true, the src
// text of the first key. Pass keep for a key type with a registered decoder,
// whose src text may differ from the encoded one. The pointer keys already in
// the map m, which no lookup by value finds, name their entries too, the first
// time a key reaches m, so the value of such an entry is updated in place and
// its text comes from the first src key naming it; pointer keys there with one
// text name no entry.
func (stt *state) entry(
	pth string,
	m reflect.Value,
	key reflect.Value,
	txt string,
	keep bool,
) mapEntry {

	if !stt.maps[pth] {
		stt.maps[pth] = true
		for it := m.MapRange(); it.Next(); {
			mk := it.Key()
			if !isPtrKey(mk) {
				continue
			}
			id := pth + "[" + keyText(mk, "%#v") + "]"
			ent, ok := stt.entries[id]
			if ok {
				ent.key = reflect.Value{} // Keys with one text are ambiguous.
			} else {
				ent = mapEntry{key: mk, text: keyText(mk, "%v")}
			}
			stt.entries[id] = ent
		}
	}
	id := pth + "[" + keyText(key, "%#v") + "]"
	ent, ok := stt.entries[id]
	if ok && ent.named {
		return ent
	}
	if !ok || !ent.key.IsValid() || !isPtrKey(key) {
		ent.key = key
	}
	ent.text, ent.named = txt, true
	// A nil interface key, which a decoder may return, has no encoding.
	if !isNil(key) && !keep {
		if enc, err := encMapKey(key, nil); err == nil {
			ent.text = enc
		}
	}
	stt.entries[id] = ent
	return ent
}

// markReset records the canonical path pth as reset. A valid val is the
// value at pth, which is saved and zeroed, so undo can set it back.
func (stt *state) markReset(pth string, val reflect.Value) {
	if stt.reset[pth] {
		return
	}
	stt.reset[pth] = true
	ent := resetLog{pth: pth}
	if val.IsValid() {
		ent.val = val
		ent.old = reflect.New(val.Type()).Elem()
		ent.old.Set(val)
		val.SetZero()
	}
	stt.log = append(stt.log, ent)
}

// save logs a copy of val, so undo can set it back.
func (stt *state) save(val reflect.Value) {
	old := reflect.New(val.Type()).Elem()
	old.Set(val)
	stt.log = append(stt.log, resetLog{val: val, old: old, saved: true})
}

// undo forgets the resets made since the log held n entries, and sets back
// the values they zeroed or saved, latest first.
func (stt *state) undo(n int) {
	for i := len(stt.log) - 1; i >= n; i-- {
		ent := stt.log[i]
		if !ent.saved {
			delete(stt.reset, ent.pth)
		}
		if ent.val.IsValid() {
			ent.val.Set(ent.old)
		}
	}
	stt.log = stt.log[:n]
}

// apply moves from dst along the steps and sets the value reached to vls. Nil
// pointers on the way are allocated, and map entries are written back after the
// value in them is set. The first step into a slice or array in a
// [Decoder.Decode] call resets it; stt holds the canonical paths reset so far,
// and pth is the canonical path of dst. A key whose value fails writes no map
// entry and adds no slice elements, and the resets it made in a map entry are
// undone. A key setting a value another key set, or setting the values of a
// slice or array other keys index, or the other way round, returns an error
// wrapping [ErrInvValue].
func (dec *Decoder) apply(
	dst reflect.Value,
	steps []step,
	vls []string,
	stt *state,
	pth string,
) error {

	if len(steps) == 0 {
		if stt.set[pth] || stt.indexed[pth] {
			return keyConflict()
		}
		stt.set[pth] = true
		stt.markReset(pth, reflect.Value{})
		val, err := deref(dst, stt.save)
		if err == nil && isUnmarshal(val) && dec.reg[val.Type()] == nil {
			stt.save(val) // UnmarshalText may change val before it fails.
		}
		ok := true
		if err == nil {
			ok, err = setDst(val, dec.reg, stt.save, vls...)
		}
		if !ok {
			err = decNotFound(dst.Type())
		}
		if err != nil {
			stt.failed[pth] = true
		}
		return err
	}
	dst, err := deref(dst, stt.save)
	if err != nil {
		return err
	}
	stp := steps[0]
	switch stp.kind {
	case stepField:
		for n, i := range stp.idx {
			if n > 0 {
				if dst, err = deref(dst, stt.save); err != nil {
					return err
				}
			}
			dst = dst.Field(i)
		}
		return dec.apply(dst, steps[1:], vls, stt, fieldPath(pth, stp.name))

	case stepKey:
		kt := dst.Type().Key()
		key, err := decKey(kt, dec.reg, stp.key)
		if err != nil {
			return err
		}
		if !key.Comparable() {
			// A decoder may return a value no map can hold as a key.
			const format = "%w: map key not comparable"
			return xrr.Newf(format, ErrInvValue, xrr.WithCode(ECInvValue))
		}
		if isNaNKey(key) {
			// No lookup finds a key holding a NaN, so its entry cannot be
			// read back.
			const format = "%w: NaN map key"
			return xrr.Newf(format, ErrInvValue, xrr.WithCode(ECInvValue))
		}
		keep := dec.reg[indirect(kt)] != nil
		ent := stt.entry(pth, dst, key, stp.key, keep)
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
			stt.undo(mark) // A value the map shares keeps its data.
			return err
		}
		if dst.IsNil() {
			dst.Set(reflect.MakeMap(dst.Type())) // Made for its first entry.
		}
		dst.SetMapIndex(key, val)
		return nil

	default:
		if stt.set[pth] {
			return keyConflict()
		}
		stt.indexed[pth] = true
		stt.markReset(pth, dst)
		size := dst.Len()
		var prev reflect.Value // The slice before it grew.
		if dst.Kind() == reflect.Slice && size <= stp.pos {
			add := stp.pos + 1 - size
			if add > stt.budget {
				const format = "%w: index %d adds too many elements"
				code := xrr.WithCode(ECInvValue)
				return xrr.Newf(format, ErrInvValue, stp.pos, code)
			}
			prev = reflect.New(dst.Type()).Elem()
			prev.Set(dst)
			stt.budget -= add
			dst.Grow(add)
			dst.SetLen(stp.pos + 1)
		}
		elem := dst.Index(stp.pos)
		err = dec.apply(elem, steps[1:], vls, stt, pth+stp.name)
		if err != nil && prev.IsValid() {
			for i := size; i < dst.Len(); i++ {
				dst.Index(i).SetZero()
			}
			stt.budget += dst.Len() - size
			dst.Set(prev) // The capacity it grew is dropped too.
		}
		return err
	}
}

// keyConflict returns the error of a src key reaching a value another src
// key sets.
func keyConflict() error {
	const format = "%w: another key sets the value"
	return xrr.Newf(format, ErrInvValue, xrr.WithCode(ECInvValue))
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
// the struct pointers, maps and slices on the path to dst, so a cycle ends.
// The paths are canonical; stt gives the text of the map keys src keys
// reached.
func (dec *Decoder) required(
	prefix string,
	dst *mirror.StructValue,
	lvl level,
	seen map[ptrKey]bool,
	stt *state,
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
		name := tag.NameOrField()
		// The level names no unexported field and no embedded interface.
		if (!anon || !isStruct) && !lvl.names(name, i) {
			continue
		}
		if anon && isStruct {
			// The path to a field that comes from an embedded struct does
			// not include the name of the embedded struct.
			nxt := lvl.embedded(i)
			val := fld.Value()
			if val.Kind() != reflect.Pointer {
				dec.required(prefix, fld.StructValue(), nxt, seen, stt, setErr)
				continue
			}
			key := ptrKey{ptr: val.Pointer(), typ: val.Type()}
			if val.IsNil() || seen[key] {
				continue // An absent struct, or a pointer cycle.
			}
			seen[key] = true
			dec.required(prefix, fld.StructValue(), nxt, seen, stt, setErr)
			delete(seen, key)
			continue
		}
		fldPath := fieldPath(prefix, name)
		if tag.Contains("required") && isEmpty(fld.Value()) {
			setErr(fldPath, ErrRequired)
		}
		dec.requiredIn(fldPath, fld.Value(), seen, stt, setErr)
	}
}

// requiredIn checks the required fields of the structs held in val, a slice,
// array or map, or a pointer to one, and in the containers nested in it; pth
// is the canonical path of val, and seen and stt are as for required.
func (dec *Decoder) requiredIn(
	pth string,
	val reflect.Value,
	seen map[ptrKey]bool,
	stt *state,
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
	case reflect.Slice, reflect.Array, reflect.Map:
		if dec.isLeafType(indirect(val.Type().Elem())) {
			return // No element holds a required field.
		}
		if val.Kind() != reflect.Array && val.Len() > 0 {
			// A map or slice holding itself is a cycle without a pointer.
			key := ptrKey{ptr: val.Pointer(), typ: val.Type(), len: val.Len()}
			if seen[key] {
				return
			}
			seen[key] = true
			defer delete(seen, key)
		}
	}
	switch val.Kind() { //nolint:exhaustive
	case reflect.Struct:
		if !val.CanAddr() {
			val = addressable(val)
		}
		sv := mirror.NewStructValue(val.Addr().Interface())
		lvl := stt.level(val.Type(), dec.tagName())
		dec.required(pth, sv, lvl, seen, stt, setErr)

	case reflect.Slice, reflect.Array:
		for i := 0; i < val.Len(); i++ {
			ipt := pth + "[" + strconv.Itoa(i) + "]"
			dec.requiredIn(ipt, val.Index(i), seen, stt, setErr)
		}

	case reflect.Map:
		for it := val.MapRange(); it.Next(); {
			txt := keyText(it.Key(), "%v")
			id := pth + "[" + keyText(it.Key(), "%#v") + "]"
			if ent, ok := stt.entries[id]; ok {
				txt = ent.text // The key text the src key gave.
			}
			kpt := pth + "[" + txt + "]"
			dec.requiredIn(kpt, it.Value(), seen, stt, setErr)
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
// exactly one value. The dst must not be a pointer. The nil pointers a struct
// dst gets the method through are allocated first (see allocPath), each one
// passed to a non-nil save before.
func useUnmarshal(
	dst reflect.Value,
	save func(reflect.Value),
	vls ...string,
) (bool, error) {

	if !isUnmarshal(dst) {
		return false, nil
	}
	val, err := single(vls)
	if err != nil {
		return true, err
	}
	if dst.Kind() == reflect.Struct && hasNilEmbedded(dst, map[ptrKey]bool{}) {
		// The method may be promoted through a nil embedded pointer.
		if err = allocPath(dst, "UnmarshalText", save); err != nil {
			return true, err
		}
	}
	unm, _ := dst.Addr().Interface().(encoding.TextUnmarshaler)
	return true, unm.UnmarshalText([]byte(val))
}

// allocPath allocates the nil pointers on the path of embedded fields the
// struct dst gets the method name through (see methodPath), so the method
// has a receiver it can set; past an embedded interface, the path goes on in
// the struct pointer it holds. A nil pointer on the path that cannot be set,
// an embedded pointer to an unexported type, returns [ErrUnexported], and a
// nil interface, or one holding a value whose method would be reached through
// a nil embedded field, returns an error wrapping [ErrDecNotFound]. A non-nil
// save gets each nil pointer before it is allocated.
func allocPath(dst reflect.Value, name string, save func(reflect.Value)) error {
	pth, _ := methodPath(dst.Type(), name, map[reflect.Type]bool{})
	val := dst
	for _, i := range pth {
		val = val.Field(i)
		switch val.Kind() { //nolint:exhaustive
		case reflect.Interface:
			if val.IsNil() {
				return decNotFound(dst.Type())
			}
			// The rest of the path is in the value the interface holds.
			held := val.Elem()
			if held.Kind() == reflect.Pointer && !held.IsNil() &&
				held.Elem().Kind() == reflect.Struct {

				return allocPath(held.Elem(), name, save)
			}
			// Nothing can be allocated in a value the interface holds.
			sub, _ := methodPath(held.Type(), name, map[reflect.Type]bool{})
			if len(sub) > 0 && nilOnPath(held, name) {
				return decNotFound(dst.Type())
			}
			return nil

		case reflect.Pointer:
			if val.IsNil() {
				if !val.CanSet() {
					return ErrUnexported
				}
				if save != nil {
					save(val)
				}
				val.Set(reflect.New(val.Type().Elem()))
			}
			val = val.Elem()
		}
	}
	return nil
}

// nilOnPath returns true if the path of embedded fields v, a struct or a
// pointer to one, gets the method name through (see methodPath) holds a nil
// pointer or interface, the value a non-nil interface on it holds included.
func nilOnPath(v reflect.Value, name string) bool {
	if v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return true
		}
		v = v.Elem()
	}
	pth, _ := methodPath(v.Type(), name, map[reflect.Type]bool{})
	for _, i := range pth {
		v = v.Field(i)
		switch v.Kind() { //nolint:exhaustive
		case reflect.Interface:
			return v.IsNil() || nilOnPath(v.Elem(), name)

		case reflect.Pointer:
			if v.IsNil() {
				return true
			}
			v = v.Elem()
		}
	}
	return false
}

// setDst sets dst with the given values. It checks first if dst implements the
// [encoding.TextUnmarshaler] interface, then it uses decFor with the reg
// registry, then it decodes a slice or an array element by element. If one of
// them was used, it returns true and the error returned by it. If none can
// decode the value, it returns false with a nil error. Nil pointers are
// allocated, as deref does with save, before dst is set.
func setDst(
	dst reflect.Value,
	reg decReg,
	save func(reflect.Value),
	vls ...string,
) (bool, error) {

	dst, err := deref(dst, save)
	if err != nil {
		return true, err
	}
	if reg[dst.Type()] == nil {
		if ok, err := useUnmarshal(dst, save, vls...); ok {
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
		if !vv.IsValid() && typ.Kind() == reflect.Interface {
			dst.SetZero() // The decoder returned a nil interface.
			return true, nil
		}
		if !vv.IsValid() || !vv.CanConvert(typ) {
			// This may happen only if decoder returns type which is in
			// fact not the one it should be decoding.
			const format = "%w: expected decoder to return %#q got `%T`"
			code := xrr.WithCode(ECInvType)
			return true, xrr.Newf(format, ErrType, typ.String(), val, code)
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
// zeroed. The dst is set only when every value decodes.
func setSliceOrArray(dst reflect.Value, reg decReg, vls ...string) error {
	et := dst.Type().Elem() // Element type.
	tmp := reflect.New(dst.Type()).Elem()
	if dst.Kind() == reflect.Array {
		if dst.Len() < len(vls) {
			const format = "%w: too many values for an array"
			return xrr.Newf(format, ErrInvValue, xrr.WithCode(ECInvValue))
		}
	} else {
		tmp.Set(reflect.MakeSlice(dst.Type(), 0, len(vls)))
	}

	for i, val := range vls {
		ne := reflect.New(indirect(et))
		ok, err := setDst(ne.Elem(), reg, nil, val)
		if err != nil {
			return err
		}
		if !ok {
			return decNotFound(et)
		}
		if et.Kind() != reflect.Pointer {
			ne = ne.Elem() // Slice element is not a pointer type.
		}
		if tmp.Kind() == reflect.Slice {
			tmp.Set(reflect.Append(tmp, ne))
		} else {
			tmp.Index(i).Set(ne)
		}
	}
	dst.Set(tmp)
	return nil
}

// decKey returns a map key of the kt type decoded from key. A float key of -0
// is returned as 0, the same key in a map.
func decKey(kt reflect.Type, reg decReg, key string) (reflect.Value, error) {
	nk := reflect.New(indirect(kt))
	ok, err := setDst(nk.Elem(), reg, nil, key)
	if !ok {
		return reflect.Value{}, decNotFound(kt)
	}
	if err != nil {
		return reflect.Value{}, err
	}
	if val := nk.Elem(); val.CanFloat() && val.Float() == 0 {
		val.SetFloat(0) // A -0 key names the entry of 0.
	}
	if kt.Kind() != reflect.Pointer {
		nk = nk.Elem() // Map key is not a pointer type.
	}
	return nk, nil
}

// deref returns the value val points to, through any number of pointers,
// allocating the nil ones; a non-nil save gets each of them first. A nil
// pointer that cannot be set, an embedded pointer to an unexported struct
// type, returns [ErrUnexported].
func deref(val reflect.Value, save func(reflect.Value)) (reflect.Value, error) {
	for val.Kind() == reflect.Pointer {
		if val.IsNil() {
			if !val.CanSet() {
				return val, ErrUnexported
			}
			if save != nil {
				save(val)
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
