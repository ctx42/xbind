// SPDX-FileCopyrightText: (c) 2026 Rafal Zajac
// SPDX-License-Identifier: MIT

package xbind

import (
	"net/url"
	"reflect"
	"slices"
	"strings"

	"github.com/ctx42/xrr/pkg/xrr"
)

// fastKeys is the largest number of src keys decodeFast takes.
const fastKeys = 16

// fastField is a canonical src key made of struct field names that leads, from
// a dst struct, through structs held by value, to a field the decoder sets as
// a whole.
type fastField struct {
	key string // The canonical key.
	idx []int  // Index path of the field.
	set setFn  // The keyField set function, maybe nil.
	unm uint8  // Whether the field is set by its UnmarshalText.
}

// decodeFast decodes src to the val struct field by field, as go-playground
// does, and returns true and the errors of the src values, when every src key
// is the key of a fastField of the struct type; it returns false, having
// changed nothing, otherwise. Such keys are distinct paths no other key
// reaches, which can neither conflict nor depend on the order they are
// applied in, so the result is what the key-driven decode gives.
func (dec *Decoder) decodeFast(
	val reflect.Value,
	src url.Values,
) (bool, error) {

	ffs := dec.fastFields(val.Type())
	if len(ffs) == 0 || len(ffs) > 4*len(src)+8 {
		return false, nil // Probing many fields for few keys costs more.
	}
	type hit struct {
		ff  *fastField
		vls []string
	}
	var hits [fastKeys]hit
	n := 0
	for i := range ffs {
		if vls, ok := src[ffs[i].key]; ok {
			hits[n] = hit{ff: &ffs[i], vls: vls}
			n++
		}
	}
	if n != len(src) {
		return false, nil // A key is not a fast field key.
	}

	var stt *state // Made for a field that may need undoing.
	var ers *xrr.FieldErrors
	var nested bool
	for _, hit := range hits[:n] {
		ff := hit.ff
		dst := val
		for _, i := range ff.idx {
			dst = dst.Field(i)
		}
		var err error
		if ff.set != nil {
			err = ff.set(dst, hit.vls)
		} else {
			if stt == nil {
				stt = dec.state()
				defer dec.release(stt)
			}
			mark := len(stt.log)
			if err = dec.setLeafValue(dst, hit.vls, stt, ff.unm); err != nil {
				stt.undo(mark) // A key that fails changes nothing.
			}
		}
		if err == nil {
			continue
		}
		if ers == nil {
			ers = &xrr.FieldErrors{}
		}
		if _, ok := err.(xrr.Fielder); ok {
			nested = true
		}
		ers.Set(ff.key, err)
	}
	return true, fieldErrors(ers, nested)
}

// fastFields returns the fastFields of the typ dst struct, built on the first
// call for the type.
func (dec *Decoder) fastFields(typ reflect.Type) []fastField {
	if ffs, ok := dec.cache.fast.Load(typ); ok {
		return ffs
	}
	ffs := dec.fastIn(typ, "", nil, 0, nil)
	ffs, _ = dec.cache.fast.LoadOrStore(typ, ffs)
	return ffs
}

// fastIn appends to ffs the fastFields of the typ struct at the canonical
// path prefix, of depth segments, and index path at, held by value in the dst
// struct, and returns it; keys deeper than the depth limit are left out.
func (dec *Decoder) fastIn(
	typ reflect.Type,
	prefix string,
	at []int,
	depth int,
	ffs []fastField,
) []fastField {

	if depth >= dec.depthLimit() {
		return ffs // A key one step deeper is over the limit.
	}
	leaf, unm := dec.isLeafType, dec.isUnmType
	kfs := dec.cache.keyFields(typ, dec.tagName(), leaf, unm, dec.reg)
	for name, kf := range kfs {
		if !valuePath(typ, kf.idx) {
			continue // Promoted through an embedded pointer.
		}
		key := fieldPath(prefix, name)
		idx := append(slices.Clip(at), kf.idx...)
		kind := kf.typ.Kind()
		switch {
		case kf.leaf || kind != reflect.Pointer && kind != reflect.Map &&
			kind != reflect.Struct:

			ff := fastField{key: key, idx: idx, set: kf.set, unm: kf.unmFact()}
			ffs = append(ffs, ff)

		case kind == reflect.Struct:
			ffs = dec.fastIn(kf.typ, key, idx, depth+1, ffs)
		}
		// Maps and pointers to structs, or to maps, slices and arrays, are
		// left to the key-driven decode.
	}
	return ffs
}

// valuePath returns true if the fields on the idx index path from the typ
// struct to the last one are structs held by value.
func valuePath(typ reflect.Type, idx []int) bool {
	for _, i := range idx[:len(idx)-1] {
		typ = typ.Field(i).Type
		if typ.Kind() != reflect.Struct {
			return false
		}
	}
	return true
}

// direct returns the field of the typ struct, the dst struct itself, the src
// key names, when the key is a name holding no dot and no bracket and the
// field, declared in typ, is a leaf or neither a struct nor a map: a key plan
// takes one step for, within the depth limit.
func (dec *Decoder) direct(
	typ reflect.Type,
	key string,
	stt *state,
) (keyField, bool) {

	if key == "" || dec.depthLimit() < 1 {
		return keyField{}, false
	}
	for i := 0; i < len(key); i++ {
		if isKeyMark(key[i]) {
			return keyField{}, false
		}
	}
	kf, ok := stt.fields(typ, dec.tagName(), dec)[key]
	if !ok || len(kf.idx) != 1 {
		return keyField{}, false
	}
	// Structs and maps take more of the key.
	if kind := indirect(kf.typ).Kind(); !kf.leaf &&
		(kind == reflect.Struct || kind == reflect.Map) {

		return keyField{}, false
	}
	return kf, true
}

// mapFacts is how directMap sets the entries of a map type.
type mapFacts struct {
	kt  reflect.Type // Key type.
	vt  reflect.Type // Value type.
	key setFn        // Sets a key.
	enc encFn        // Encodes a key.
	val setFn        // Sets a value.
}

// mapFactsOf returns the mapFacts of the typ map type, or nil when its keys
// are not plain keys (see isPlainKey) or its values are not of a built-in
// type no function or method decodes.
func (dec *Decoder) mapFactsOf(typ reflect.Type) *mapFacts {
	if mf, ok := dec.cache.mapFacts.Load(typ); ok {
		return mf
	}
	var mf *mapFacts
	kt, vt := typ.Key(), typ.Elem()
	if dec.isPlainKey(kt) && isBuiltin(vt) && dec.reg.get(vt) == nil &&
		!dec.isUnmType(vt) {

		mf = &mapFacts{kt: kt, vt: vt, key: builtinSetter(kt),
			enc: encFor(kt, nil), val: builtinSetter(vt)}
	}
	mf, _ = dec.cache.mapFacts.LoadOrStore(typ, mf)
	return mf
}

// directMap sets the entry of a map field of the val struct, the dst struct
// itself, that the src key names with the key text of the entry ("attr[k]")
// to vls, and returns true and the error of the key, as plan and apply would;
// the map must hold values of a built-in type under plain keys (see
// mapFactsOf). It returns false, having changed nothing, for any other key,
// and when the paths src keys reach are tracked.
func (dec *Decoder) directMap(
	val reflect.Value,
	key string,
	vls []string,
	stt *state,
) (bool, error) {

	if stt.track || dec.cache == nil || dec.depthLimit() < 2 {
		return false, nil
	}
	open := strings.IndexByte(key, '[')
	if open <= 0 || key[len(key)-1] != ']' || open+2 >= len(key) {
		return false, nil
	}
	name, txt := key[:open], key[open+1:len(key)-1]
	for i := 0; i < len(name); i++ {
		if isKeyMark(name[i]) {
			return false, nil
		}
	}
	if strings.ContainsAny(txt, "[]") {
		return false, nil
	}
	kf, ok := stt.fields(val.Type(), dec.tagName(), dec)[name]
	if !ok || len(kf.idx) != 1 || kf.typ.Kind() != reflect.Map {
		return false, nil
	}
	mf := dec.mapFactsOf(kf.typ)
	if mf == nil {
		return false, nil
	}

	mk, mv := stt.mapScratch(mf)
	one := [1]string{txt}
	if err := mf.key(mk, one[:]); err != nil {
		return true, err // As decKey fails.
	}
	if enc, _ := mf.enc(mk); enc != txt {
		return false, nil // The entry has another text than the key.
	}
	if err := stt.setAt(key, mf.vt); err != nil {
		return true, err
	}
	if err := mf.val(mv, vls); err != nil {
		stt.mark(key, pathFailed)
		return true, err
	}
	m := val.Field(kf.idx[0])
	if m.IsNil() {
		m.Set(reflect.MakeMap(m.Type())) // Made for its first entry.
	}
	m.SetMapIndex(mk, mv) // Copies the key and the value.
	return true, nil
}
