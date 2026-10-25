// SPDX-FileCopyrightText: (c) 2026 Rafal Zajac
// SPDX-License-Identifier: MIT

package xbind

import (
	"maps"
	"reflect"
	"sync"
	"sync/atomic"

	"github.com/ctx42/mirror/pkg/mirror"
)

// typeCache holds what an [Encoder] or a [Decoder] finds out about the types
// it meets, each fact built once per type; it is safe for concurrent use. The
// methods of a nil cache build the facts on every call, so a zero-value
// [Encoder] or [Decoder] works without one.
type typeCache struct {
	infos    typeMap[typeInfo]      // The facts about interfaces of each type.
	tags     typeMap[tagCheck]      // The tag check of each src or dst type.
	leaves   typeMap[bool]          // Whether each type is a leaf.
	values   typeMap[bool]          // Whether each type is a value.
	keys     typeMap[bool]          // Whether each map key type is plain.
	parts    typeMap[bool]          // Whether each type is encoded by parts.
	addrs    typeMap[bool]          // Whether each src type needs an address.
	loops    typeMap[bool]          // Whether each src type can hold a cycle.
	levels   typeMap[*mirror.Names] // The names of each struct level.
	fields   typeMap[keyFields]     // The key fields of each struct level.
	fast     typeMap[[]fastField]   // The fast fields of each dst type.
	mapFacts typeMap[*mapFacts]     // How directMap sets each map type.
	plans    typeMap[[]encField]    // The encode plan of each struct type.
	plains   typeMap[encFn]         // The encoder of each plain value type.
	maps     typeMap[*sureMap]      // How each sure map type is encoded.
	slices   typeMap[*slicePlan]    // How each slice type is encoded.
}

// typeMap maps types to values; it is safe for concurrent use. Made for values
// stored once and read many times, it is read without locks: a store copies
// the map. The zero value is an empty map.
type typeMap[V any] struct {
	mx sync.Mutex                    // Serializes the stores.
	m  atomic.Pointer[map[uintptr]V] // The current map, never changed.
}

// typeKey returns the key of typ in a typeMap: the address of the type, which
// the runtime never frees or moves, so it tells the type for good. A number
// hashes faster than the interface.
func typeKey(typ reflect.Type) uintptr { return reflect.ValueOf(typ).Pointer() }

// Load returns the value stored for typ, and false when there is none.
func (tm *typeMap[V]) Load(typ reflect.Type) (V, bool) {
	if m := tm.m.Load(); m != nil {
		val, ok := (*m)[typeKey(typ)]
		return val, ok
	}
	var zero V
	return zero, false
}

// Store stores val for typ.
func (tm *typeMap[V]) Store(typ reflect.Type, val V) {
	tm.mx.Lock()
	defer tm.mx.Unlock()
	tm.store(typ, val)
}

// LoadOrStore returns the value stored for typ and true, or stores val for typ
// and returns it and false when there is none.
func (tm *typeMap[V]) LoadOrStore(typ reflect.Type, val V) (V, bool) {
	tm.mx.Lock()
	defer tm.mx.Unlock()
	if cur, ok := tm.Load(typ); ok {
		return cur, true
	}
	tm.store(typ, val)
	return val, false
}

// store stores val for typ in a copy of the map; the caller holds the mutex.
func (tm *typeMap[V]) store(typ reflect.Type, val V) {
	var m map[uintptr]V
	if cur := tm.m.Load(); cur != nil {
		m = maps.Clone(*cur)
	} else {
		m = make(map[uintptr]V, 1)
	}
	m[typeKey(typ)] = val
	tm.m.Store(&m)
}

// keyField is the field a key name selects in a struct level.
type keyField struct {
	idx  []int        // Index path of the field, shared with mirror.Names.
	typ  reflect.Type // Field type.
	leaf bool         // The type the field is or points to is a leaf.
	unm  bool         // The type the field holds through pointers unmarshals.
	set  setFn        // Sets a built-in type nothing else decodes, or nil.
}

// unmFact returns what the field tells of its value being set by its
// UnmarshalText method: unmYes or unmNo.
func (kf keyField) unmFact() uint8 {
	if kf.unm {
		return unmYes
	}
	return unmNo
}

// keyFields maps the key names of a struct level to the fields they select.
type keyFields map[string]keyField

// typeInfo tells which of the interfaces the package looks for a type, and a
// pointer to it, implement.
type typeInfo struct {
	mar   bool // The type implements Marshaler.
	text  bool // It implements encoding.TextMarshaler.
	unm   bool // It implements encoding.TextUnmarshaler.
	pmar  bool // A pointer to it implements Marshaler.
	ptext bool // A pointer to it implements encoding.TextMarshaler.
	punm  bool // A pointer to it implements encoding.TextUnmarshaler.
}

// tagCheck is the result of checkTags for a src or dst type and, for a dst
// type whose tags parse, whether it has fields to check.
type tagCheck struct {
	err    error
	checks bool
}

// newTypeInfo returns the typeInfo of typ.
func newTypeInfo(typ reflect.Type) typeInfo {
	inf := typeInfo{
		mar:  typ.Implements(iMarshaler),
		text: typ.Implements(iTextMarshaler),
		unm:  typ.Implements(iUnmarshaler),
	}
	if typ.Kind() != reflect.Interface {
		ptr := reflect.PointerTo(typ)
		inf.pmar = ptr.Implements(iMarshaler)
		inf.ptext = ptr.Implements(iTextMarshaler)
		inf.punm = ptr.Implements(iUnmarshaler)
	}
	return inf
}

// info returns the typeInfo of typ.
func (tc *typeCache) info(typ reflect.Type) typeInfo {
	if tc == nil {
		return newTypeInfo(typ)
	}
	if inf, ok := tc.infos.Load(typ); ok {
		return inf
	}
	inf := newTypeInfo(typ)
	tc.infos.Store(typ, inf)
	return inf
}

// isMarshaler returns true if typ implements [Marshaler] or
// [encoding.TextMarshaler].
func (tc *typeCache) isMarshaler(typ reflect.Type) bool {
	inf := tc.info(typ)
	return inf.mar || inf.text
}

// textUnmarshaler reports whether typ, or a pointer to the type it is or
// points to, implements [encoding.TextUnmarshaler].
func (tc *typeCache) textUnmarshaler(typ reflect.Type) bool {
	if typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	inf := tc.info(typ)
	return inf.unm || inf.punm
}

// checkTags returns what checkTags returns for the typ struct, the key tag
// and the leaf function, which must not change between the calls on one
// cache.
func (tc *typeCache) checkTags(
	typ reflect.Type,
	key string,
	leaf func(reflect.Type) bool,
) error {

	if tc != nil {
		if res, ok := tc.tags.Load(typ); ok {
			return res.err
		}
	}
	err := checkTags("", typ, key, leaf, map[reflect.Type]bool{})
	if tc != nil {
		tc.tags.Store(typ, tagCheck{err: err})
	}
	return err
}

// checkDst returns what checks returns for the typ dst struct and what
// checkTags returns for it, the key tag and the leaf function; checks is not
// called when the tags do not parse. The key, leaf and checks must not change
// between the calls on one cache.
func (tc *typeCache) checkDst(
	typ reflect.Type,
	key string,
	leaf func(reflect.Type) bool,
	checks func(reflect.Type) bool,
) (bool, error) {

	if tc != nil {
		if res, ok := tc.tags.Load(typ); ok {
			return res.checks, res.err
		}
	}
	err := checkTags("", typ, key, leaf, map[reflect.Type]bool{})
	has := err == nil && checks(typ)
	if tc != nil {
		tc.tags.Store(typ, tagCheck{err: err, checks: has})
	}
	return has, err
}

// leaf returns fn(typ), remembered per type; fn must not change between the
// calls on one cache.
func (tc *typeCache) leaf(typ reflect.Type, fn func(reflect.Type) bool) bool {
	if tc == nil {
		return fn(typ)
	}
	return memo(&tc.leaves, typ, fn)
}

// value returns fn(typ), remembered per type; fn must not change between the
// calls on one cache.
func (tc *typeCache) value(typ reflect.Type, fn func(reflect.Type) bool) bool {
	if tc == nil {
		return fn(typ)
	}
	return memo(&tc.values, typ, fn)
}

// plainKey returns fn(typ), remembered per type; fn must not change between
// the calls on one cache.
func (tc *typeCache) plainKey(
	typ reflect.Type,
	fn func(reflect.Type) bool,
) bool {

	if tc == nil {
		return fn(typ)
	}
	return memo(&tc.keys, typ, fn)
}

// composite returns fn(typ), remembered per type; fn must not change between
// the calls on one cache.
func (tc *typeCache) composite(
	typ reflect.Type,
	fn func(reflect.Type) bool,
) bool {

	if tc == nil {
		return fn(typ)
	}
	return memo(&tc.parts, typ, fn)
}

// addr returns fn(typ), remembered per type; fn must not change between the
// calls on one cache.
func (tc *typeCache) addr(typ reflect.Type, fn func(reflect.Type) bool) bool {
	if tc == nil {
		return fn(typ)
	}
	return memo(&tc.addrs, typ, fn)
}

// loop returns fn(typ), remembered per type; fn must not change between the
// calls on one cache.
func (tc *typeCache) loop(typ reflect.Type, fn func(reflect.Type) bool) bool {
	if tc == nil {
		return fn(typ)
	}
	return memo(&tc.loops, typ, fn)
}

// level returns the level of the typ struct for the key struct tag, which
// must not change between the calls on one cache.
func (tc *typeCache) level(typ reflect.Type, key string) level {
	if tc == nil {
		return newLevel(typ, key)
	}
	if nms, ok := tc.levels.Load(typ); ok {
		return level{nms: nms}
	}
	lvl := newLevel(typ, key)
	tc.levels.Store(typ, lvl.nms)
	return lvl
}

// keyFields returns the key fields of the typ struct for the key struct tag;
// leaf tells the leaf types, unm the types set by their UnmarshalText method,
// and reg the registered decoders. The key, leaf, unm and reg must not change
// between the calls on one cache.
func (tc *typeCache) keyFields(
	typ reflect.Type,
	key string,
	leaf func(reflect.Type) bool,
	unm func(reflect.Type) bool,
	reg decReg,
) keyFields {

	if tc != nil {
		if kfs, ok := tc.fields.Load(typ); ok {
			return kfs
		}
	}
	kfs := keyFields{}
	for name, idx := range tc.level(typ, key).nms.All() {
		kf := newKeyField(typ, idx, leaf, unm)
		if !kf.unm && reg.get(kf.typ) == nil {
			kf.set = builtinSetter(kf.typ) // Nil for a pointer.
		}
		kfs[name] = kf
	}
	if tc != nil {
		kfs, _ = tc.fields.LoadOrStore(typ, kfs)
	}
	return kfs
}

// newKeyField returns the keyField of the field of the typ struct at the idx
// index path; leaf tells the leaf types, and unm the types set by their
// UnmarshalText method.
func newKeyField(
	typ reflect.Type,
	idx []int,
	leaf func(reflect.Type) bool,
	unm func(reflect.Type) bool,
) keyField {

	ft := typ.FieldByIndex(idx).Type
	base := ft
	for base.Kind() == reflect.Pointer {
		base = base.Elem()
	}
	return keyField{idx: idx, typ: ft, leaf: leaf(indirect(ft)), unm: unm(base)}
}

// memo returns the bool m holds for typ, storing fn(typ) there first when it
// holds none.
func memo(m *typeMap[bool], typ reflect.Type, fn func(reflect.Type) bool) bool {
	if res, ok := m.Load(typ); ok {
		return res
	}
	res := fn(typ)
	m.Store(typ, res)
	return res
}
