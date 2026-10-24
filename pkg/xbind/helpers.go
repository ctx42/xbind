// SPDX-FileCopyrightText: (c) 2026 Rafal Zajac
// SPDX-License-Identifier: MIT

package xbind

import (
	"database/sql/driver"
	"fmt"
	"math"
	"reflect"
	"runtime"
	"strings"
)

// isNil returns true if v is a nil pointer or a nil interface.
func isNil(v reflect.Value) bool {
	switch v.Kind() { //nolint:exhaustive
	case reflect.Pointer, reflect.Interface:
		return v.IsNil()

	default:
		return false
	}
}

// buildPath returns name prefixed with pref and a dot. It returns the other
// one when either of them is empty.
func buildPath(pref, name string) string {
	if pref != "" {
		if name == "" {
			return pref
		}
		return pref + "." + name
	}
	return name
}

// fieldPath returns the path to the field name under the pref path: the
// name after a dot or, when the name contains a dot, in brackets.
func fieldPath(pref, name string) string {
	if strings.Contains(name, ".") {
		return pref + "[" + name + "]"
	}
	return buildPath(pref, name)
}

// keyText returns the text of the map key, as the encoder writes it when it
// can, else as the format verb of [fmt] prints it; the Go syntax of "%#v"
// quotes strings, so keys printing alike stay apart.
func keyText(key reflect.Value, format string) string {
	if key.Kind() == reflect.Interface && !key.IsNil() {
		key = key.Elem()
	}
	if !isNil(key) {
		if txt, err := encMapKey(key, nil); err == nil {
			return txt
		}
	}
	return fmt.Sprintf(format, key.Interface())
}

// isNaN returns true if v is a float NaN, or a pointer to one.
func isNaN(v reflect.Value) bool {
	v = reflect.Indirect(v)
	switch v.Kind() { //nolint:exhaustive
	case reflect.Float32, reflect.Float64:
		return math.IsNaN(v.Float())

	default:
		return false
	}
}

// isNaNKey returns true if no map lookup finds the map key: it is a float
// NaN, or a pointer to one, or a value holding one.
func isNaNKey(key reflect.Value) bool {
	return isNaN(key) || key.Kind() != reflect.Pointer && !key.Equal(key)
}

// dynType returns the type of the value v holds when v is a non-nil
// interface, else the type of v.
func dynType(v reflect.Value) reflect.Type {
	if v.Kind() == reflect.Interface && !v.IsNil() {
		return v.Elem().Type()
	}
	return v.Type()
}

// holdsPtr returns true if a value of the typ type is or holds a pointer, an
// interface or a channel, which a map lookup compares by address.
func holdsPtr(typ reflect.Type) bool {
	switch typ.Kind() { //nolint:exhaustive
	case reflect.Pointer, reflect.Interface, reflect.Chan,
		reflect.UnsafePointer:

		return true

	case reflect.Array:
		return holdsPtr(typ.Elem())

	case reflect.Struct:
		for i := 0; i < typ.NumField(); i++ {
			if holdsPtr(typ.Field(i).Type) {
				return true
			}
		}
		return false

	default:
		return false
	}
}

// zeroer is implemented by types reporting whether they are zero.
type zeroer interface{ IsZero() bool }

// isZero returns true if v is a zero value, an empty slice or an empty map.
// A struct implementing the IsZero method, on its value or on its address
// when v is addressable, is zero when the method says so; other structs,
// structs read through unexported fields, structs embedding a nil pointer or
// interface, and arrays are zero when all their elements are.
func isZero(v reflect.Value) bool {
	switch v.Kind() { //nolint:exhaustive
	case reflect.Map, reflect.Slice:
		return v.Len() == 0

	case reflect.Array:
		for i := 0; i < v.Len(); i++ {
			if !isZero(v.Index(i)) {
				return false
			}
		}
		return true

	case reflect.Struct:
		if hasNilEmbedded(v, map[ptrKey]bool{}) {
			// A method promoted through a nil pointer or interface would panic.
			return isZeroFields(v)
		}
		if v.CanAddr() && v.Addr().CanInterface() {
			if zer, ok := v.Addr().Interface().(zeroer); ok {
				return zer.IsZero()
			}
		}
		if v.CanInterface() {
			if zer, ok := v.Interface().(zeroer); ok {
				return zer.IsZero()
			}
		}
		return isZeroFields(v)

	default:
		return v.IsZero()
	}
}

// isZeroFields returns true if all the fields of the struct v are zero.
func isZeroFields(v reflect.Value) bool {
	for i := 0; i < v.NumField(); i++ {
		if !isZero(v.Field(i)) {
			return false
		}
	}
	return true
}

// hasNilEmbedded returns true if the struct v embeds a nil pointer or
// interface, directly or through embedded structs, struct pointers and the
// values embedded interfaces hold. The struct pointers in seen are skipped, so
// an embedding cycle ends.
func hasNilEmbedded(v reflect.Value, seen map[ptrKey]bool) bool {
	for i := 0; i < v.NumField(); i++ {
		if !v.Type().Field(i).Anonymous {
			continue
		}
		fld := v.Field(i)
		if isNil(fld) {
			return true
		}
		if fld.Kind() == reflect.Interface {
			if fld = fld.Elem(); isNil(fld) {
				return true
			}
		}
		if fld.Kind() == reflect.Pointer {
			key := ptrKey{ptr: fld.Pointer(), typ: fld.Type()}
			if seen[key] {
				continue
			}
			seen[key] = true
			fld = fld.Elem()
		}
		if fld.Kind() == reflect.Struct && hasNilEmbedded(fld, seen) {
			return true
		}
	}
	return false
}

// nilPromoted returns true if calling the method name on v, a struct or a
// pointer to one, would panic: the method is promoted through a nil embedded
// interface, through a nil embedded pointer to a type declaring it with a
// value receiver, or past a nil embedded pointer, either in v or in the value
// an embedded interface holds. A method promoted through a nil embedded
// pointer to a type declaring it with a pointer receiver is called with a nil
// receiver, so it returns false, and so does a method the struct type
// declares. A nil pointer v counts as one such embedded pointer.
func nilPromoted(v reflect.Value, name string) bool {
	if v.Kind() == reflect.Pointer && v.IsNil() {
		// Only the declaring type takes a nil pointer receiver.
		pth, _ := methodPath(v.Type(), name, map[reflect.Type]bool{})
		_, val := v.Type().Elem().MethodByName(name)
		return len(pth) > 0 || val
	}
	v = reflect.Indirect(v)
	if v.Kind() != reflect.Struct || !hasNilEmbedded(v, map[ptrKey]bool{}) {
		return false // With no nil embedded field, no method lookup is needed.
	}
	pth, _ := methodPath(v.Type(), name, map[reflect.Type]bool{})
	for n, i := range pth {
		v = v.Field(i)
		switch v.Kind() { //nolint:exhaustive
		case reflect.Interface:
			// The rest of the path is in the value the interface holds.
			return v.IsNil() || nilPromoted(v.Elem(), name)

		case reflect.Pointer:
			if v.IsNil() {
				// Only the declaring type takes a nil pointer receiver.
				_, val := v.Type().Elem().MethodByName(name)
				return n < len(pth)-1 || val
			}
			v = v.Elem()
		}
	}
	return false
}

// methodPath returns the index path of the embedded field whose type
// declares the method name the typ type, or the type it points to, has;
// the path is empty when the type declares the method itself. Like Go
// promotion, the shallowest field wins and two at that depth hide the
// method. The seen map holds the struct types on the path, so a recursive
// type ends. It returns false when there is no such method.
func methodPath(
	typ reflect.Type,
	name string,
	seen map[reflect.Type]bool,
) ([]int, bool) {

	typ = indirect(typ)
	if declares(typ, name) {
		return nil, true
	}
	if typ.Kind() != reflect.Struct || seen[typ] {
		return nil, false
	}
	seen[typ] = true
	defer delete(seen, typ)
	var best []int
	var found, ambiguous bool
	for i := 0; i < typ.NumField(); i++ {
		if !typ.Field(i).Anonymous {
			continue
		}
		sub, ok := methodPath(typ.Field(i).Type, name, seen)
		if !ok {
			continue
		}
		pth := append([]int{i}, sub...)
		switch {
		case !found || len(pth) < len(best):
			best, found, ambiguous = pth, true, false

		case len(pth) == len(best):
			ambiguous = true
		}
	}
	if !found || ambiguous {
		return nil, false
	}
	return best, true
}

// declares returns true if the typ type declares the method name, with a
// value or a pointer receiver; an interface declares its methods. A method
// promoted from an embedded field is not declared: its function is a
// wrapper the compiler generates, which the runtime places in the file
// "<autogenerated>".
func declares(typ reflect.Type, name string) bool {
	if typ.Kind() == reflect.Interface {
		_, ok := typ.MethodByName(name)
		return ok
	}
	for _, rcv := range []reflect.Type{typ, reflect.PointerTo(typ)} {
		if mth, ok := rcv.MethodByName(name); ok {
			pc := mth.Func.Pointer()
			file, _ := runtime.FuncForPC(pc).FileLine(pc)
			return file != "<autogenerated>"
		}
	}
	return false
}

// isEmpty returns true if v is empty: a zero number; a nil or zero-length
// string, array, slice, map or channel; a nil pointer, interface or function;
// a pointer or interface to an empty value; a value whose IsZero method
// returns true or whose [driver.Valuer] value is empty; any other zero value.
// A bool is never empty. The rules are those of IsEmpty in the
// github.com/ctx42/verax package. Methods are called only when v can be
// interfaced, and not on a struct embedding a nil pointer or interface, or a
// pointer or interface to one, whose promoted methods would panic; such a
// struct is empty when all its fields are zero. A chain of pointers and
// interfaces leading back to itself is not empty.
func isEmpty(v reflect.Value) bool {
	if !v.IsValid() {
		return true
	}
	switch v.Kind() { //nolint:exhaustive
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map,
		reflect.Pointer, reflect.Slice:

		if v.IsNil() {
			return true
		}
	}
	base := v
	var seen map[uintptr]bool // Pointers on the chain, so a cycle ends.
	for base.Kind() == reflect.Interface || base.Kind() == reflect.Pointer {
		if base.IsNil() {
			break
		}
		if base.Kind() == reflect.Pointer {
			if seen[base.Pointer()] {
				return false // A chain pointing to itself holds no value.
			}
			if seen == nil {
				seen = make(map[uintptr]bool)
			}
			seen[base.Pointer()] = true
		}
		base = base.Elem()
	}
	nilEmb := base.Kind() == reflect.Struct &&
		hasNilEmbedded(base, map[ptrKey]bool{})
	if v.CanInterface() && !nilEmb {
		if zer, ok := v.Interface().(zeroer); ok {
			return zer.IsZero()
		}
		if vlr, ok := v.Interface().(driver.Valuer); ok {
			val, err := vlr.Value()
			rv := reflect.ValueOf(val)
			// A value of its own type would be checked forever.
			if err == nil && (!rv.IsValid() || rv.Type() != v.Type()) {
				return isEmpty(rv)
			}
		}
	}
	switch v.Kind() { //nolint:exhaustive
	case reflect.Bool:
		return false

	case reflect.String, reflect.Array, reflect.Map, reflect.Slice,
		reflect.Chan:

		return v.Len() == 0

	case reflect.Interface, reflect.Pointer:
		return isEmpty(v.Elem())

	default:
		return v.IsZero()
	}
}

// indirect returns the type that typ points to.
// The original typ is returned if typ is not a pointer.
func indirect(typ reflect.Type) reflect.Type {
	if typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	return typ
}

// baseType returns the type typ holds through pointers, slices, arrays and
// maps, at any depth; for any other type it returns typ. For a type holding
// itself, such as a map of its own type, it returns the first type met twice.
func baseType(typ reflect.Type) reflect.Type {
	seen := make(map[reflect.Type]bool)
	for !seen[typ] {
		switch typ.Kind() { //nolint:exhaustive
		case reflect.Pointer, reflect.Slice, reflect.Array, reflect.Map:
			seen[typ] = true
			typ = typ.Elem()

		default:
			return typ
		}
	}
	return typ
}

// addressable returns an addressable copy of val.
func addressable(val reflect.Value) reflect.Value {
	cpy := reflect.New(val.Type()).Elem()
	cpy.Set(val)
	return cpy
}
