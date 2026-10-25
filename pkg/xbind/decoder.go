// SPDX-FileCopyrightText: (c) 2026 Rafal Zajac
// SPDX-License-Identifier: MIT

package xbind

import (
	"encoding"
	"errors"
	"math"
	"net/url"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync"

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

// WithDecMaxDepth is a [NewDecoder] option setting the largest number of steps
// a src key may take into dst, one per struct field, map key and slice or array
// index ("list[0].name" takes three, and the rest of a key a map of values
// takes as its key is one); a deeper key is an [ErrInvValue] field error. A
// zero or negative n rejects every key that names a field. A key costs memory
// quadratic in its depth, so a large n lets a small src use a lot of memory
// when dst has a recursive type. The default is [DefaultMaxDepth].
func WithDecMaxDepth(n int) DecoderOption {
	return func(dec *Decoder) { dec.maxDepth = &n }
}

// WithDecFunc is a [NewDecoder] option decoding every value of type T with fn:
// fields of type T or *T, and slice, array and map elements and map keys of
// those types. A value of type T takes a single src value, and an error
// returned by fn is returned as is. The function takes precedence over the
// [encoding.TextUnmarshaler] method of T and the decoders the package provides,
// [time.Time] and [time.Duration] included. For a pointer T, fn decodes the
// type T points to, and a nil pointer it returns is an [ErrInvValue] error.
// When functions are registered for both T and *T, the later one decodes both.
// For an interface T, fn may return nil, which sets a nil interface.
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
	maxDepth     *int   // Largest key depth; nil means DefaultMaxDepth.
	reg          decReg // Decoders registered with WithDecFunc.

	cache *typeCache // Facts about the types met; nil builds them per call.
	pool  *sync.Pool // States of finished calls; nil makes one per call.
}

// NewDecoder returns a new instance of [Decoder] reading the "url" struct tag
// and rejecting unknown keys unless the options say otherwise. The decoder
// caches what it learns about each type it decodes, so later calls for the
// type do less work; a zero-value [Decoder] caches nothing.
func NewDecoder(opts ...DecoderOption) *Decoder {
	dec := &Decoder{
		tag:   defaultTag,
		cache: &typeCache{},
		pool:  &sync.Pool{New: func() any { return newState() }},
	}
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
// may add, the keys that fail included: as many as the limit plus one, and one
// more per src key. A key that fails after resetting an array spends the length
// of the array from that budget, and once the budget is spent, a key that would
// reset an array is an [ErrInvValue] field error. Among the fields of a struct
// and the structs it embeds, a key names the field Go promotion selects, as in
// [encoding/json]. Errors of the src values are returned as [xrr.FieldErrors]
// keyed by the src keys; the other values are still set, and a key with an
// error changes nothing but what a failing UnmarshalText method wrote through
// the pointers and maps its value holds. The first key reaching a slice or an
// array replaces it: the values of one key replace the elements, indexed keys
// fill a new slice whose gaps are zero values, and array elements no key sets
// are zeroed; a field with no key keeps its value. The src keys are applied in
// sorted order; a key reaching a value another key sets, or one key setting the
// values of a slice or array other keys index, is an [ErrInvValue] field error
// on the later key. Conflicts, and the replacing of slices and arrays, follow
// the paths of the keys, so a value a prefilled dst shares under two paths,
// through two pointers to it, is reached as two values, in the sorted key
// order. Keys are added to an existing map, and the values of a map are updated
// in place; keys decoding to equal values name one entry, and a key that is or
// holds a NaN is an [ErrInvValue] field error. A nil pointer to a nested struct
// is allocated only when a src key leads into it. A field with the "required"
// option is an [ErrRequired] error when it is empty after decoding: a zero
// number, an empty string, slice or map, a nil pointer or a pointer to an empty
// value, or a zero struct; a bool is never empty. A field with the "present"
// option is an [ErrMissing] error when no src key reaches it, whatever value
// the key gives, a zero one included; it is checked in dst and in the structs
// src keys reach. The required fields of a nil struct pointer, an absent
// struct, are not checked; those of the structs in maps, slices and arrays are,
// under paths like "items[k].name". A src key leading into an embedded nil
// pointer to a struct, which the decoder cannot allocate because the field is
// unexported, is an [ErrUnexported] field error. A struct tag of dst that does
// not parse, or that names a field with a bracket ("ids[]") or an embedded
// struct, is an [ErrInvTag] error; the tags inside a type decoded as one value,
// as by its UnmarshalText method, are not checked. A key taking more steps than
// [DefaultMaxDepth], or the limit [WithDecMaxDepth] sets, is an [ErrInvValue]
// field error. Pointers to pointers to structs are not supported.
func (dec *Decoder) Decode(dst any, src url.Values) error {
	val := reflect.ValueOf(dst)
	if val.Kind() != reflect.Pointer || val.Elem().Kind() != reflect.Struct {
		return ErrReqStructPtr
	}
	val = val.Elem()
	leaf, has := dec.isLeafType, dec.hasChecks
	checks, err := dec.cache.checkDst(val.Type(), dec.tagName(), leaf, has)
	if err != nil {
		return err
	}
	return dec.decode(val, src, checks)
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

// depthLimit returns the largest number of steps a src key may take.
func (dec *Decoder) depthLimit() int {
	if dec.maxDepth == nil {
		return DefaultMaxDepth
	}
	return *dec.maxDepth
}

// decode decodes src to the val struct, and returns the errors of the src
// values and of the required fields left empty, as [xrr.FieldErrors] or nil;
// checks tells the struct has fields to check (see hasChecks).
func (dec *Decoder) decode(
	val reflect.Value,
	src url.Values,
	checks bool,
) error {

	if !checks && dec.cache != nil && len(src) <= fastKeys {
		if ok, err := dec.decodeFast(val, src); ok {
			return err
		}
	}
	stt := dec.state()
	defer dec.release(stt)
	stt.track = checks
	stt.src = src

	var ers *xrr.FieldErrors
	var keys map[string]bool // The keys of ers, for the required walk.
	var nested bool          // An error of ers holds field errors.
	setErr := func(pth string, err error) {
		if ers == nil {
			ers = &xrr.FieldErrors{}
		}
		if stt.track {
			if keys == nil {
				keys = make(map[string]bool)
			}
			keys[pth] = true
		}
		if _, ok := err.(xrr.Fielder); ok {
			nested = true
		}
		ers.Set(pth, err)
	}
	// Indexed keys may add one full slice of elements, and one per key.
	if lim := dec.indexLimit(); lim < math.MaxInt-len(src)-1 {
		stt.budget = max(lim, 0) + 1 + len(src)
	}
	// Buffers on the stack, for most calls, take no write barriers.
	var keyBuf [16]string
	var vlsBuf [16][]string
	var segBuf [4]segment
	var stepBuf [4]step
	few := len(src) <= len(keyBuf)
	if few {
		// A few keys sort with their values, as slices.Sort sorts them.
		sorted := keyBuf[:0]
		for key, vls := range src {
			i := len(sorted)
			sorted = append(sorted, key)
			for ; i > 0 && key < sorted[i-1]; i-- {
				sorted[i], vlsBuf[i] = sorted[i-1], vlsBuf[i-1]
			}
			sorted[i], vlsBuf[i] = key, vls
		}
	} else {
		// Many keys sort by their index in the pooled buffers, so moving
		// them writes no pointers and takes no write barriers.
		stt.keys = slices.Grow(stt.keys[:0], len(src))
		for key := range src {
			stt.keys = append(stt.keys, key)
		}
		stt.order = slices.Grow(stt.order[:0], len(src))[:len(src)]
		for i := range stt.order {
			stt.order[i] = i
		}
		keys := stt.keys
		slices.SortFunc(stt.order, func(a, b int) int {
			return strings.Compare(keys[a], keys[b])
		})
	}
	for n := range len(src) {
		var key string
		var vls []string
		if few {
			key, vls = keyBuf[n], vlsBuf[n]
		} else {
			key = stt.keys[stt.order[n]]
			vls = src[key]
		}
		stt.key, stt.off = key, false
		var unknown bool // The key names nothing.
		var err error
		if done, merr := dec.directMap(val, key, vls, stt); done {
			err = merr
		} else if kf, ok := dec.direct(val.Type(), key, stt); ok {
			// The one step the plan would take, applied as apply does.
			mark := len(stt.log)
			stt.reach("")
			fld := val.Field(kf.idx[0])
			if kf.set != nil {
				err = dec.setLeaf(fld, vls, stt, key, kf.typ, kf.set)
			} else {
				err = dec.applyLeaf(fld, vls, stt, key, kf.unmFact())
			}
			if err != nil {
				stt.undo(mark) // A key that fails changes nothing.
			}
		} else {
			segs, ok := parseKey(key, segBuf[:0])
			var steps []step
			if !ok {
				err = ErrUnkFieldPath
			} else {
				steps, err = dec.plan(val.Type(), segs, stepBuf[:0], stt)
			}
			// A value error may wrap the sentinel.
			unknown = err != nil && errors.Is(err, ErrUnkFieldPath)
			if err != nil && !unknown && errors.Is(err, ErrInvValue) {
				stt.markReached(steps) // A key the plan rejects was sent.
			}
			if err == nil {
				mark := len(stt.log)
				if err = dec.apply(val, steps, vls, stt, ""); err != nil {
					stt.undo(mark) // A key that fails changes nothing.
				}
			}
		}
		switch {
		case err == nil || errors.Is(err, errIgnored):

		case unknown:
			if !dec.allowUnknown {
				setErr(key, ErrUnkFieldPath)
			}

		default:
			setErr(key, err)
		}
	}

	if !stt.track {
		return fieldErrors(ers, nested) // No field to check.
	}
	// A field with a value error keeps that error.
	lvl := stt.level(val.Type(), dec.tagName())
	seen := map[ptrKey]bool{}
	sv := mirror.NewStructValue(val.Addr().Interface())
	dec.required("", sv, lvl, seen, stt, func(pth string, err error) {
		if !stt.has(pth, pathFailed) && !keys[pth] {
			setErr(pth, err)
		}
	})
	return fieldErrors(ers, nested)
}

// fieldErrors returns ers as an error, nil when it is nil. When nested is
// true, an error of ers holds field errors, and ers is returned filtered (see
// [xrr.FieldErrors.Filter]); else no error of ers is filtered out.
func fieldErrors(ers *xrr.FieldErrors, nested bool) error {
	switch {
	case ers == nil:
		return nil

	case nested:
		return ers.Filter()

	default:
		return ers
	}
}

// state returns the state for a new [Decoder.Decode] call, with no element
// budget, from the pool of the decoder when it has one.
func (dec *Decoder) state() *state {
	var stt *state
	if dec.pool != nil {
		stt = dec.pool.Get().(*state) //nolint:forcetypeassert
	} else {
		stt = newState()
	}
	if stt.tc != dec.cache {
		stt.tc = dec.cache
		stt.ftyp, stt.fkfs = [2]reflect.Type{}, [2]keyFields{}
		stt.ltyp, stt.lleaf = nil, false
	}
	return stt
}

// release returns the stt state of a finished [Decoder.Decode] call to the
// pool of the decoder, cleared, unless the decoder has no pool or the state
// grew past poolLimit entries.
func (dec *Decoder) release(stt *state) {
	if dec.pool == nil || len(stt.paths) > poolLimit {
		return
	}
	stt.clear()
	dec.pool.Put(stt)
}

// hasChecks returns true if decoding a value of the typ type may reach a field
// with the "required" or "present" option: typ is, holds or points to a struct
// with such a field, or one with a field of such a type, embedded fields
// included. It errs on the side of true: the fields of types decoded as one
// value count too.
func (dec *Decoder) hasChecks(typ reflect.Type) bool {
	return dec.checksIn(typ, map[reflect.Type]bool{})
}

// checksIn is hasChecks for the typ type; the seen map holds the struct
// types walked so far, so a recursive type ends.
func (dec *Decoder) checksIn(
	typ reflect.Type,
	seen map[reflect.Type]bool,
) bool {

	typ = baseType(typ)
	if typ.Kind() != reflect.Struct || seen[typ] {
		return false
	}
	seen[typ] = true
	md := mirror.ReflectType(typ)
	for i := range md.NumField() {
		fld := md.FieldByIndex(i)
		tag, _ := fld.Tag(dec.tagName()) // Decode checked the tags.
		if tag.IsIgnored() {
			continue
		}
		if tag.Contains("required") || tag.Contains("present") {
			return true
		}
		if dec.checksIn(fld.Type(), seen) {
			return true
		}
	}
	return false
}

// segment is one part of a src key: a name, or a literal written in
// brackets, which may contain dots.
type segment struct {
	text    string // Segment text.
	bracket bool   // The segment was written in brackets.
	end     int    // Offset in the key past the segment, its bracket included.
}

// parseKey splits the src key into segments, appended to segs: names separated
// by dots, and literals in brackets ("a.b[c.d][0]"). It returns false for an
// empty key or segment, a bracket that does not close, a bracket in brackets,
// and text or a dot that does not follow the rules.
func parseKey(key string, segs []segment) ([]segment, bool) {
	for i := 0; i < len(key); {
		if key[i] == '[' {
			end := i + 1
			for end < len(key) && key[end] != '[' && key[end] != ']' {
				end++
			}
			if end == i+1 || end == len(key) || key[end] != ']' {
				return nil, false
			}
			txt := key[i+1 : end]
			i = end + 1
			segs = append(segs, segment{text: txt, bracket: true, end: i})
		} else {
			end := i
			for end < len(key) && !isKeyMark(key[end]) {
				end++
			}
			if end == i {
				return nil, false
			}
			segs = append(segs, segment{text: key[i:end], end: end})
			i = end
		}
		if i == len(key) || key[i] == '[' {
			continue
		}
		// Only a dot followed by a name may come next.
		if key[i] != '.' || i+1 == len(key) || isKeyMark(key[i+1]) {

			return nil, false
		}
		i++
	}
	if len(segs) == 0 {
		return nil, false
	}
	return segs, true
}

// isKeyMark returns true if the byte is a dot or a bracket, which separate
// the segments of a src key.
func isKeyMark(b byte) bool { return b == '.' || b == '[' || b == ']' }

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
	idx  []int        // Field index path, through embedded structs.
	key  string       // Map key text.
	pos  int          // Slice or array index.
	name string       // Field name.
	unm  uint8        // For a field, whether its value is set by UnmarshalText.
	set  setFn        // For a field, the keyField set function, maybe nil.
	ftyp reflect.Type // For a field with a set function, its type.

	// The offset in the src key past the step when the key up to there is the
	// canonical path of the value the step reaches, else zero. For a map
	// entry, the key text must also be the text of the entry.
	end int
}

// What a step tells of the value it reaches being set by its UnmarshalText
// method (see isUnmarshal).
const (
	unmUnknown uint8 = iota // The step does not tell.
	unmNo                   // It is not.
	unmYes                  // It is.
)

// path returns the canonical path of the value the step reaches from the
// value at the canonical path pth; txt is the text of the entry for a map
// step.
func (stp step) path(pth, txt string) string {
	switch stp.kind {
	case stepField:
		return fieldPath(pth, stp.name)

	case stepKey:
		return pth + "[" + txt + "]"

	default:
		return pth + "[" + strconv.Itoa(stp.pos) + "]"
	}
}

// errIgnored reports a src key naming an ignored field; the key is skipped.
var errIgnored = errors.New("ignored field")

// plan returns the steps a src key, split into segs, takes from a value of type
// typ. A struct segment names a field, a map segment is the key, and a slice or
// array segment is an index; a map of plain values given a key that is not
// bracketed takes the rest of the key as its key. The typ struct itself is
// always walked field by field, like the encoder does at the empty path, even
// when it has a promoted UnmarshalText method. A key naming nothing returns
// [ErrUnkFieldPath], one naming an ignored field returns errIgnored, and an
// index over the limit or past an array end, or a key taking more steps than
// the depth limit, returns an error wrapping [ErrInvValue], together with the
// steps it planned until then. The steps are appended to steps, and the
// levels of the structs come from stt.
func (dec *Decoder) plan(
	typ reflect.Type,
	segs []segment,
	steps []step,
	stt *state,
) ([]step, error) {

	canon := true        // The src key is canonical up to the planned steps.
	var leaf, known bool // The typ type is a leaf, known from a field.
	for {
		if len(steps) > dec.depthLimit() {
			const format = "%w: key depth over the limit %d"
			code := xrr.WithCode(ECInvValue)
			err := xrr.Newf(format, ErrInvValue, dec.depthLimit(), code)
			return steps, err
		}
		typ = indirect(typ)
		if !known {
			leaf = len(steps) > 0 && stt.isLeaf(typ, dec)
		}
		known = false
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
			tag := dec.tagName()
			kf, ok := stt.fields(typ, tag, dec)[seg.text]
			if !ok {
				if stt.level(typ, tag).nms.IsIgnored(seg.text) {
					return nil, errIgnored
				}
				return nil, ErrUnkFieldPath
			}
			stp := step{kind: stepField, idx: kf.idx, name: seg.text}
			stp.unm = kf.unmFact()
			if kf.set != nil {
				stp.set, stp.ftyp = kf.set, kf.typ
			}
			// A field name with a dot goes in brackets; a name out of brackets
			// has no dot.
			if seg.bracket {
				canon = canon && strings.Contains(seg.text, ".")
			}
			if canon {
				stp.end = seg.end
			}
			steps = append(steps, stp)
			typ, leaf, known = kf.typ, kf.leaf, true
			segs = segs[1:]

		case typ.Kind() == reflect.Map:
			key := seg.text
			segs = segs[1:]
			if !seg.bracket && dec.isLeafValue(typ.Elem()) && len(segs) > 0 {
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
			stp := step{kind: stepKey, key: key}
			canon = canon && seg.bracket
			if canon {
				stp.end = seg.end
			}
			steps = append(steps, stp)
			typ = typ.Elem()

		default: // A slice or an array, the other types that are not leaves.
			pos, err := dec.index(typ, seg.text)
			if err != nil {
				return steps, err
			}
			stp := step{kind: stepIndex, pos: pos}
			// An index has no leading zeros.
			lead := seg.text == "0" || seg.text[0] != '0'
			canon = canon && seg.bracket && lead
			if canon {
				stp.end = seg.end
			}
			steps = append(steps, stp)
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
	for i := 0; i < len(txt); i++ {
		if txt[i] < '0' || txt[i] > '9' {
			return 0, ErrUnkFieldPath
		}
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

// poolLimit is the number of paths a [Decoder.Decode] call may reach for its
// state to go back to the pool; a larger one would keep its memory, and make
// clearing it slow, for every later call.
const poolLimit = 1024

// pathFlag is a fact about a canonical path in a [Decoder.Decode] call.
type pathFlag uint8

// Facts about canonical paths.
const (
	pathReset   pathFlag = 1 << iota // A reset container.
	pathMap                          // A map reached.
	pathFailed                       // A value that failed.
	pathSet                          // A value a key set.
	pathIndexed                      // A container keys indexed.
	pathReached                      // A path a src key went through.
)

// state is the state of one [Decoder.Decode] call.
type state struct {
	paths   map[string]pathFlag  // The facts of the canonical paths met.
	log     []resetLog           // The resets and saves, in the order made.
	entries map[entryID]mapEntry // Map entries reached, by entry identity.
	keyed   map[string][]entryID // Entry ids of each map, in the order met.
	budget  int                  // Slice elements indexed keys may still add.
	track   bool                 // Mark the paths src keys reach.
	tc      *typeCache           // Facts about types, may be nil.

	src   url.Values // The src values.
	key   string     // The src key being applied.
	off   bool       // A map entry of the key has another text than the key.
	alias bool       // A key whose path differs from it was met.
	keys  []string   // Buffer for many src keys.
	order []int      // Buffer for the sorted order of keys.

	ftyp [2]reflect.Type // The struct types of the last two fields calls.
	fkfs [2]keyFields    // Their key fields.

	ltyp  reflect.Type // The type of the last isLeaf call.
	lleaf bool         // What it returned.

	mf *mapFacts     // The map type of the last mapScratch call.
	mk reflect.Value // Its key, set and copied by directMap.
	mv reflect.Value // Its value, set and copied by directMap.
}

// mapScratch returns a key and a value of the map type of mf, made on the
// first call for the type, which directMap sets and copies to the map.
func (stt *state) mapScratch(mf *mapFacts) (reflect.Value, reflect.Value) {
	if stt.mf != mf {
		stt.mf = mf
		stt.mk = reflect.New(mf.kt).Elem()
		stt.mv = reflect.New(mf.vt).Elem()
	}
	return stt.mk, stt.mv
}

// resetLog is a reset of a slice or array, or a value a src key sets, or a
// value saved before a src key changes it.
type resetLog struct {
	pth   string        // Canonical path of the value.
	val   reflect.Value // The value, when the reset zeroed it or it was saved.
	old   reflect.Value // A copy of val from before the change.
	saved bool          // The value was saved, not reset.
}

// entryID identifies a map entry: the canonical path of its map and the text
// of its key in Go syntax (see keyText).
type entryID struct{ pth, key string }

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
		paths:   make(map[string]pathFlag),
		entries: make(map[entryID]mapEntry),
		keyed:   make(map[string][]entryID),
		budget:  math.MaxInt,
		track:   true,
	}
}

// has returns true if the canonical path pth has the flag.
func (stt *state) has(pth string, flag pathFlag) bool {
	return stt.paths[pth]&flag != 0
}

// mark gives the canonical path pth the flag.
func (stt *state) mark(pth string, flag pathFlag) { stt.paths[pth] |= flag }

// reach marks the canonical path pth as reached, when the state tracks the
// paths src keys reach.
func (stt *state) reach(pth string) {
	if stt.track {
		stt.paths[pth] |= pathReached
	}
}

// clear readies the state for another call of the decoder it came from: as
// newState returns it, but keeping the memory of its maps and buffers, its
// typeCache and the types it took from it.
func (stt *state) clear() {
	clear(stt.paths)
	clear(stt.log) // Drop the values the entries hold.
	stt.log = stt.log[:0]
	clear(stt.entries)
	clear(stt.keyed)
	stt.budget = math.MaxInt
	stt.track = true
	stt.src, stt.key, stt.off, stt.alias = nil, "", false, false
	clear(stt.keys) // The keys of a call with many.
	stt.keys = stt.keys[:0]
}

// fields returns the key fields of the typ struct for the key struct tag of
// the dec decoder (see typeCache.keyFields); a call for the struct type of
// one of the two calls before takes them from the state.
func (stt *state) fields(typ reflect.Type, key string, dec *Decoder) keyFields {
	switch {
	case typ == stt.ftyp[0] && stt.fkfs[0] != nil:
		return stt.fkfs[0]

	case typ == stt.ftyp[1] && stt.fkfs[1] != nil:
		return stt.fkfs[1]
	}
	leaf, unm := dec.isLeafType, dec.isUnmType
	kfs := stt.tc.keyFields(typ, key, leaf, unm, dec.reg)
	stt.ftyp[1], stt.fkfs[1] = stt.ftyp[0], stt.fkfs[0]
	stt.ftyp[0], stt.fkfs[0] = typ, kfs
	return kfs
}

// isLeaf returns what dec.isLeafType returns for typ; a call for the type of
// the call before takes it from the state.
func (stt *state) isLeaf(typ reflect.Type, dec *Decoder) bool {
	if typ != stt.ltyp {
		stt.ltyp, stt.lleaf = typ, dec.isLeafType(typ)
	}
	return stt.lleaf
}

// level returns the level of the typ struct for the key struct tag.
func (stt *state) level(typ reflect.Type, key string) level {
	return stt.tc.level(typ, key)
}

// next returns the canonical path of the value the step stp of the src key
// reaches from the value at the canonical path pth, a part of the key when
// the key is canonical up to the step; txt is the text of the entry for a map
// step.
func (stt *state) next(stp step, pth, txt string) string {
	if stp.kind == stepKey && txt != stp.key {
		stt.off = true // The path takes another text than the key from here.
	}
	if stp.end > 0 && !stt.off && stp.end <= len(stt.key) {
		return stt.key[:stp.end]
	}
	return stp.path(pth, txt)
}

// entry returns the entry of the map at the canonical path pth that the
// decoded key names; txt is the src text of the key. Keys decoding to equal
// values, by their encoded text or, when the key type has no encoder, by
// their Go syntax, name one entry: its key is the first one decoded, so
// pointer keys and keys holding pointers find it again, and its text is the
// encoded text or, when the key type has no encoder or keep is true, the src
// text of the first key. Pass keep for a key type with a registered decoder,
// whose src text may differ from the encoded one. The keys already in the map
// m that hold pointers, which no lookup by value finds, name their entries
// too, the first time a key reaches m: a decoded key of the same type finds
// such an entry, its value is updated in place, and its text comes from the
// first src key naming it; keys there with one text name no entry. A key Go
// compares equal to one already reached names that entry even when its text
// differs, so a float of -0 inside a struct or an array is the same key as 0.
func (stt *state) entry(
	pth string,
	m reflect.Value,
	key reflect.Value,
	txt string,
	keep bool,
) mapEntry {

	if !stt.has(pth, pathMap) {
		stt.mark(pth, pathMap)
		for it := m.MapRange(); it.Next(); {
			mk := it.Key()
			if !holdsPtr(dynType(mk)) {
				continue
			}
			id := entryID{pth: pth, key: keyText(mk, "%#v", stt.tc)}
			ent, ok := stt.entries[id]
			if ok {
				ent.key = reflect.Value{} // Keys with one text are ambiguous.
			} else {
				ent = mapEntry{key: mk, text: keyText(mk, "%v", stt.tc)}
				stt.keyed[pth] = append(stt.keyed[pth], id)
			}
			stt.entries[id] = ent
		}
	}
	// A bool, an integer or a string key is never nil, and when it encodes,
	// its text is both its id and the text of its entry.
	plain := key.IsValid() && textKey(key.Kind())
	var enc string
	var encErr error
	if plain {
		enc, encErr = encMapKey(key, nil, stt.tc)
	}
	var id entryID
	if plain && encErr == nil {
		id = entryID{pth: pth, key: enc}
	} else {
		id = entryID{pth: pth, key: keyText(key, "%#v", stt.tc)}
	}
	// Formatted text misses keys Go treats as equal, such as -0 and 0; the
	// text of a bool, an integer or a string tells it apart.
	if key.IsValid() && !isNil(key) && !plain {
		for _, alt := range stt.keyed[pth] {
			prev := stt.entries[alt]
			if !prev.key.IsValid() || prev.key.Type() != key.Type() {
				continue
			}
			if prev.key.Equal(key) {
				id = alt
				break
			}
		}
	}
	ent, ok := stt.entries[id]
	if ok && ent.named {
		return ent
	}
	if !ok || !ent.key.IsValid() || dynType(ent.key) != dynType(key) {
		ent.key = key
	}
	ent.text, ent.named = txt, true
	// A nil interface key, which a decoder may return, has no encoding.
	switch {
	case keep || isNil(key):

	case plain:
		if encErr == nil {
			ent.text = enc
		}

	default:
		if enc, err := encMapKey(key, nil, stt.tc); err == nil {
			ent.text = enc
		}
	}
	if !ok && !plain {
		stt.keyed[pth] = append(stt.keyed[pth], id) // Only for the scan.
	}
	stt.entries[id] = ent
	return ent
}

// textKey returns true if map keys of the kind are equal exactly when their
// texts are: a bool, an integer or a string.
func textKey(kind reflect.Kind) bool {
	switch kind { //nolint:exhaustive
	case reflect.Bool, reflect.String,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32,
		reflect.Uint64, reflect.Uintptr:

		return true

	default:
		return false
	}
}

// markReset records the canonical path pth as reset. A valid val is the
// value at pth, which is saved and zeroed, so undo can set it back.
func (stt *state) markReset(pth string, val reflect.Value) {
	if stt.has(pth, pathReset) {
		return
	}
	stt.mark(pth, pathReset)
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

// markReached records the canonical paths of the steps, those a src key took
// before plan rejected it, as reached; a map key keeps its src text.
func (stt *state) markReached(steps []step) {
	pth := ""
	stt.reach(pth)
	for _, stp := range steps {
		pth = stp.path(pth, stp.key)
		stt.reach(pth)
	}
}

// undo forgets the resets made since the log held n entries, and sets back
// the values they zeroed or saved, latest first. The elements of an array a
// reset zeroed are charged to the element budget, so keys that fail cannot
// copy arrays without end.
func (stt *state) undo(n int) {
	for i := len(stt.log) - 1; i >= n; i-- {
		ent := stt.log[i]
		if !ent.saved {
			if flg := stt.paths[ent.pth] &^ pathReset; flg != 0 {
				stt.paths[ent.pth] = flg
			} else {
				delete(stt.paths, ent.pth)
			}
			if ent.val.Kind() == reflect.Array {
				stt.budget = max(stt.budget-ent.val.Len(), 0)
			}
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
		return dec.applyLeaf(dst, vls, stt, pth, unmUnknown)
	}
	stt.reach(pth) // A key failing further on was still sent.
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
		if len(steps) == 1 {
			nxt := stt.next(stp, pth, "")
			if stp.set != nil {
				return dec.setLeaf(dst, vls, stt, nxt, stp.ftyp, stp.set)
			}
			return dec.applyLeaf(dst, vls, stt, nxt, stp.unm)
		}
		return dec.apply(dst, steps[1:], vls, stt, stt.next(stp, pth, ""))

	case stepKey:
		kt := dst.Type().Key()
		var key reflect.Value
		var txt string // The text of the entry.
		if dec.isPlainKey(kt) {
			// Equal keys have equal texts, so the text names the entry.
			key = reflect.New(kt).Elem()
			one := [1]string{stp.key}
			if _, err = setBuiltin(key, one[:]); err != nil {
				return err
			}
			txt, _ = encFor(kt, nil)(key) // Encoding a plain key never fails.
		} else {
			if key, err = decKey(kt, dec.reg, dec.cache, stp.key); err != nil {
				return err
			}
			plain := textKey(key.Kind()) // Comparable, and never a NaN.
			if !plain && !key.Comparable() {
				// A decoder may return a value no map can hold as a key.
				const format = "%w: map key not comparable"
				code := xrr.WithCode(ECInvValue)
				return xrr.Newf(format, ErrInvValue, code)
			}
			if !plain && isNaNKey(key) {
				// No lookup finds a key holding a NaN, so its entry cannot
				// be read back.
				const format = "%w: NaN map key"
				code := xrr.WithCode(ECInvValue)
				return xrr.Newf(format, ErrInvValue, code)
			}
			keep := dec.reg.get(indirect(kt)) != nil
			ent := stt.entry(pth, dst, key, stp.key, keep)
			key, txt = ent.key, ent.text
		}
		kpt := stt.next(stp, pth, txt)
		val := reflect.New(dst.Type().Elem()).Elem()
		// A value the key sets as a whole is new, so a pointer in the map
		// keeps its pointee.
		if len(steps) > 1 {
			if cur := dst.MapIndex(key); cur.IsValid() {
				val.Set(cur)
			}
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
		flg := stt.paths[pth]
		if flg&pathSet != 0 {
			return keyConflict()
		}
		if dst.Kind() == reflect.Array && flg&pathReset == 0 &&
			stt.budget == 0 {

			// Keys that failed spent the budget resetting arrays.
			const format = "%w: index %d resets too many elements"
			code := xrr.WithCode(ECInvValue)
			return xrr.Newf(format, ErrInvValue, stp.pos, code)
		}
		if flg&pathIndexed == 0 {
			stt.mark(pth, pathIndexed)
		}
		if flg&pathReset == 0 {
			stt.markReset(pth, dst)
		}
		size := dst.Len()
		grown := false
		var prev reflect.Value // The slice before it grew, when it moves.
		if dst.Kind() == reflect.Slice && size <= stp.pos {
			add := stp.pos + 1 - size
			if add > stt.budget {
				const format = "%w: index %d adds too many elements"
				code := xrr.WithCode(ECInvValue)
				return xrr.Newf(format, ErrInvValue, stp.pos, code)
			}
			if dst.Cap() <= stp.pos {
				prev = reflect.New(dst.Type()).Elem()
				prev.Set(dst)
			}
			stt.budget -= add
			dst.Grow(add)
			dst.SetLen(stp.pos + 1)
			grown = true
		}
		elem := dst.Index(stp.pos)
		err = dec.apply(elem, steps[1:], vls, stt, stt.next(stp, pth, ""))
		if err != nil && grown {
			// The elements stay spent, so failing keys cannot grow slices
			// without end.
			for i := size; i < dst.Len(); i++ {
				dst.Index(i).SetZero()
			}
			if prev.IsValid() {
				dst.Set(prev) // The capacity it grew is dropped too.
			} else {
				dst.SetLen(size) // The slice did not move.
			}
		}
		return err
	}
}

// applyLeaf sets the value dst at the canonical path pth, reached by the steps
// of a src key, to vls (see apply); unm tells whether dst is set by its
// UnmarshalText method, when the last step knows.
func (dec *Decoder) applyLeaf(
	dst reflect.Value,
	vls []string,
	stt *state,
	pth string,
	unm uint8,
) error {

	if err := stt.setAt(pth, dst.Type()); err != nil {
		return err
	}
	err := dec.setLeafValue(dst, vls, stt, unm)
	if err != nil {
		stt.mark(pth, pathFailed)
	}
	return err
}

// setLeafValue sets the value dst, through any pointers, to vls, logging in
// stt the values it changes so undo can set them back; unm tells whether dst
// is set by its UnmarshalText method, when known.
func (dec *Decoder) setLeafValue(
	dst reflect.Value,
	vls []string,
	stt *state,
	unm uint8,
) error {

	val, err := deref(dst, stt.save)
	ok := true
	if err == nil {
		if unm == unmUnknown {
			unm = unmNo
			if dec.isUnmType(val.Type()) && val.CanAddr() {
				unm = unmYes
			}
		}
		if unm == unmYes {
			stt.save(val) // UnmarshalText may change val before it fails.
		}
		yes := unm == unmYes
		ok, err = setValue(val, dec.reg, dec.cache, yes, stt.save, vls)
	}
	if !ok {
		err = decNotFound(dst.Type())
	}
	return err
}

// setLeaf is applyLeaf for a value dst of the typ type, which is not a
// pointer, set by the set function, as setBuiltin would set it.
func (dec *Decoder) setLeaf(
	dst reflect.Value,
	vls []string,
	stt *state,
	pth string,
	typ reflect.Type,
	set setFn,
) error {

	if pth == stt.key && !stt.alias {
		stt.reach(pth) // What setAt does for a canonical key and such a type.
	} else if err := stt.setAt(pth, typ); err != nil {
		return err
	}
	err := set(dst, vls)
	if err != nil {
		stt.mark(pth, pathFailed)
	}
	return err
}

// setAt records that the src key sets the value of the typ type at the
// canonical path pth, and returns an error wrapping [ErrInvValue] when another
// key set it, or indexed it. Only a key of another text can reach the value
// a canonical key, the path of its value, sets, unless the value is a slice or
// an array, which other keys may index: the value of such a key is recorded
// and checked only once a key of another text was met, which checks too
// whether the canonical key sorting before it reached the value.
func (stt *state) setAt(pth string, typ reflect.Type) error {
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	canon := pth == stt.key
	kind := typ.Kind()
	record := !canon || kind == reflect.Slice || kind == reflect.Array
	if !record && !stt.alias {
		stt.reach(pth) // A key failing further on was still sent.
		return nil
	}
	flg := stt.paths[pth]
	conflict := flg&(pathSet|pathIndexed) != 0
	if !canon && !conflict && pth < stt.key {
		_, conflict = stt.src[pth]
	}
	if conflict {
		stt.reach(pth)
		return keyConflict()
	}
	if !canon {
		stt.alias = true
	}
	if stt.track {
		flg |= pathReached // A key failing further on was still sent.
	}
	if record {
		flg |= pathSet
	}
	if flg != 0 {
		stt.paths[pth] = flg
	}
	return nil
}

// isPlainKey returns true if map keys of the kt type are decoded and encoded
// by the built-in functions of their kind, a bool, an integer or a string
// kind: no function is registered for the type, and it, or a pointer to it,
// implements neither [encoding.TextUnmarshaler] nor [encoding.TextMarshaler].
func (dec *Decoder) isPlainKey(kt reflect.Type) bool {
	return dec.cache.plainKey(kt, dec.plainKey)
}

// plainKey is isPlainKey without the cache.
func (dec *Decoder) plainKey(kt reflect.Type) bool {
	if !textKey(kt.Kind()) || !isBuiltin(kt) || dec.reg.get(kt) != nil {
		return false
	}
	inf := dec.cache.info(kt)
	return !inf.unm && !inf.punm && !inf.text && !inf.ptext
}

// isUnmType returns true if an addressable value of the typ type, which is
// not a pointer, is set by its UnmarshalText method: no function is
// registered for the type, which isUnmarshal says is set so.
func (dec *Decoder) isUnmType(typ reflect.Type) bool {
	return unmType(typ, dec.reg, dec.cache)
}

// unmType is isUnmType for the reg registry, the facts about types coming
// from tc.
func unmType(typ reflect.Type, reg decReg, tc *typeCache) bool {
	if reg.get(typ) != nil || typ == typTime {
		return false
	}
	switch typ.Kind() { //nolint:exhaustive
	case reflect.Pointer, reflect.Interface:
		return false

	default:
		return tc.info(typ).punm
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
	return dec.cache.leaf(typ, dec.leafType)
}

// leafType is isLeafType without the cache.
func (dec *Decoder) leafType(typ reflect.Type) bool {
	if dec.reg.get(typ) != nil || typ == typTime {
		return true
	}
	if inf := dec.cache.info(typ); inf.unm || inf.punm {
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
// of dst whose value is empty (see isEmpty). It sets an [ErrMissing] error
// instead for each field with the "present" option that no src key reached (see
// state), in dst at the empty prefix or in a struct a src key reached. A nil
// struct pointer is an absent struct, so its fields are not checked, though the
// present fields of an embedded one are (see presentIn). Ignored fields and
// unexported fields, other than embedded structs, are skipped; an embedded
// struct has no path, so its required option is ignored. The lvl is the level
// position of dst; a field its key does not name there is skipped. The seen map
// holds the struct pointers, maps and slices on the path to dst, so a cycle
// ends. The paths are canonical; stt gives the text of the map keys src keys
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
			if val.IsNil() {
				// An absent struct, whose fields still name keys of dst.
				typ := indirect(fld.Type())
				types := map[reflect.Type]bool{}
				dec.presentIn(prefix, typ, nxt, types, stt, setErr)
				continue
			}
			key := ptrKey{ptr: val.Pointer(), typ: val.Type()}
			if seen[key] {
				continue // A pointer cycle.
			}
			seen[key] = true
			dec.required(prefix, fld.StructValue(), nxt, seen, stt, setErr)
			delete(seen, key)
			continue
		}
		fldPath := fieldPath(prefix, name)
		// A struct no key reached was not sent, so neither were its fields.
		sent := prefix == "" || stt.has(prefix, pathReached)
		switch {
		case tag.Contains("present") && sent && !stt.has(fldPath, pathReached):
			setErr(fldPath, ErrMissing)

		case tag.Contains("required") && isEmpty(fld.Value()):
			setErr(fldPath, ErrRequired)
		}
		dec.requiredIn(fldPath, fld.Value(), seen, stt, setErr)
	}
}

// presentIn sets an [ErrMissing] error, using setErr, for each field with the
// "present" option that no src key reached, of the typ struct embedded through
// a nil pointer at the lvl position of a struct at prefix, and of the structs
// typ embeds. The fields are checked only in dst, at the empty prefix, or in a
// struct a src key reached; the other fields of typ are in structs no key
// reached. The types map holds the struct types on the embedding path, so a
// recursive type ends.
func (dec *Decoder) presentIn(
	prefix string,
	typ reflect.Type,
	lvl level,
	types map[reflect.Type]bool,
	stt *state,
	setErr func(string, error),
) {

	if types[typ] || prefix != "" && !stt.has(prefix, pathReached) {
		return
	}
	types[typ] = true
	defer delete(types, typ)
	for i, fld := range mirror.ReflectType(typ).Fields() {
		tag, _ := fld.Tag(dec.tagName()) // Decode checked the tags.
		if tag.IsIgnored() {
			continue
		}
		if fld.IsAnonymous() && indirect(fld.Type()).Kind() == reflect.Struct {
			sub := indirect(fld.Type())
			dec.presentIn(prefix, sub, lvl.embedded(i), types, stt, setErr)
			continue
		}
		name := tag.NameOrField()
		if !lvl.names(name, i) {
			continue
		}
		fldPath := fieldPath(prefix, name)
		if tag.Contains("present") && !stt.has(fldPath, pathReached) {
			setErr(fldPath, ErrMissing)
		}
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
			txt := keyText(it.Key(), "%v", stt.tc)
			id := entryID{pth: pth, key: keyText(it.Key(), "%#v", stt.tc)}
			if ent, ok := stt.entries[id]; ok {
				txt = ent.text // The key text the src key gave.
			}
			kpt := pth + "[" + txt + "]"
			dec.requiredIn(kpt, it.Value(), seen, stt, setErr)
		}
	}
}

// isUnmarshal returns true if dst is addressable and its address implements
// the [encoding.TextUnmarshaler] interface, as tc tells. It returns false for
// [time.Time], which is decoded by a custom decoder, and for pointers and
// interfaces.
func isUnmarshal(dst reflect.Value, tc *typeCache) bool {
	if !dst.CanAddr() || dst.Type() == typTime {
		return false
	}
	switch dst.Kind() { //nolint:exhaustive
	case reflect.Pointer, reflect.Interface:
		return false

	default:
		return tc.info(dst.Type()).punm
	}
}

// unmarshalText sets dst, which must have an [encoding.TextUnmarshaler]
// implementation (see isUnmarshal), using it. Such a dst takes exactly one
// value. The nil pointers a struct dst gets the method through are allocated
// first (see allocPath), each one passed to a non-nil save before.
func unmarshalText(
	dst reflect.Value,
	save func(reflect.Value),
	vls []string,
) error {

	val, err := single(vls)
	if err != nil {
		return err
	}
	if dst.Kind() == reflect.Struct && hasNilEmbedded(dst, map[ptrKey]bool{}) {
		// The method may be promoted through a nil embedded pointer.
		if err = allocPath(dst, "UnmarshalText", save); err != nil {
			return err
		}
	}
	unm, _ := dst.Addr().Interface().(encoding.TextUnmarshaler)
	return unm.UnmarshalText([]byte(val))
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
			// Nothing can be allocated in a value the interface holds, and a
			// nil pointer there is no receiver the method can set.
			sub, _ := methodPath(held.Type(), name, map[reflect.Type]bool{})
			if isNil(held) || len(sub) > 0 && nilOnPath(held, name) {
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
// allocated, as deref does with save, before dst is set. The facts about
// types come from tc.
func setDst(
	dst reflect.Value,
	reg decReg,
	tc *typeCache,
	save func(reflect.Value),
	vls ...string,
) (bool, error) {

	dst, err := deref(dst, save)
	if err != nil {
		return true, err
	}
	unm := reg.get(dst.Type()) == nil && isUnmarshal(dst, tc)
	return setValue(dst, reg, tc, unm, save, vls)
}

// setValue is setDst for a dst that is not a pointer; unm tells dst is set by
// its [encoding.TextUnmarshaler] (see isUnmarshal), no function being
// registered for its type.
func setValue(
	dst reflect.Value,
	reg decReg,
	tc *typeCache,
	unm bool,
	save func(reflect.Value),
	vls []string,
) (bool, error) {

	if unm {
		return true, unmarshalText(dst, save, vls)
	}
	if reg.get(dst.Type()) == nil {
		if ok, err := setBuiltin(dst, vls); ok {
			return true, err
		}
	}

	typ := dst.Type()
	if dfn := decFor(typ, reg); dfn != nil {
		// A copy keeps vls on the stack of the callers: dfn may keep it.
		val, err := dfn(slices.Clone(vls)...)
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
		return true, setSliceOrArray(dst, reg, tc, vls...)

	default:
		return false, nil
	}
}

// setSliceOrArray expects dst to be a slice or an array. Each of the elements
// in vls will be decoded based on the array or slice type and set. A slice is
// replaced by a new one holding the values. An array gets the values on
// indexes in the same order as in vls; the elements past the values are
// zeroed. The dst is set only when every value decodes. The facts about types
// come from tc.
func setSliceOrArray(
	dst reflect.Value,
	reg decReg,
	tc *typeCache,
	vls ...string,
) error {

	et := dst.Type().Elem() // Element type.
	var tmp reflect.Value
	if dst.Kind() == reflect.Array {
		if dst.Len() < len(vls) {
			const format = "%w: too many values for an array"
			return xrr.Newf(format, ErrInvValue, xrr.WithCode(ECInvValue))
		}
		tmp = reflect.New(dst.Type()).Elem()
	} else {
		tmp = reflect.MakeSlice(dst.Type(), len(vls), len(vls))
	}

	if isBuiltin(et) && !unmType(et, reg, tc) && reg.get(et) == nil {
		// Elements of a built-in type are set as such.
		for i := range vls {
			if _, err := setBuiltin(tmp.Index(i), vls[i:i+1]); err != nil {
				return err
			}
		}
		dst.Set(tmp)
		return nil
	}
	for i := range vls {
		elem := tmp.Index(i)
		if et.Kind() == reflect.Pointer {
			elem.Set(reflect.New(et.Elem()))
			elem = elem.Elem()
		}
		ok, err := setDst(elem, reg, tc, nil, vls[i:i+1]...)
		if err != nil {
			return err
		}
		if !ok {
			return decNotFound(et)
		}
	}
	dst.Set(tmp)
	return nil
}

// decKey returns a map key of the kt type decoded from key. A float key of -0
// is returned as 0, the same key in a map. The facts about types come from tc.
func decKey(
	kt reflect.Type,
	reg decReg,
	tc *typeCache,
	key string,
) (reflect.Value, error) {

	vls := [1]string{key}
	if textKey(kt.Kind()) && reg.get(kt) == nil && !tc.info(kt).punm {
		// A bool, number or string key is set as such, when it can be.
		nk := reflect.New(kt).Elem()
		ok, err := setBuiltin(nk, vls[:])
		switch {
		case ok && err != nil:
			return reflect.Value{}, err

		case ok:
			return nk, nil
		}
	}
	nk := reflect.New(indirect(kt))
	ok, err := setDst(nk.Elem(), reg, tc, nil, vls[:]...)
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
