// SPDX-FileCopyrightText: (c) 2026 Rafal Zajac
// SPDX-License-Identifier: MIT

package xbind

import (
	"reflect"
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
		if hasNilEmbedded(v, map[uintptr]bool{}) {
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
// interface, directly or through embedded structs and struct pointers. The
// struct pointers in seen are skipped, so an embedding cycle ends.
func hasNilEmbedded(v reflect.Value, seen map[uintptr]bool) bool {
	for i := 0; i < v.NumField(); i++ {
		if !v.Type().Field(i).Anonymous {
			continue
		}
		fld := v.Field(i)
		if isNil(fld) {
			return true
		}
		if fld.Kind() == reflect.Pointer {
			if seen[fld.Pointer()] {
				continue
			}
			seen[fld.Pointer()] = true
			fld = fld.Elem()
		}
		if fld.Kind() == reflect.Struct && hasNilEmbedded(fld, seen) {
			return true
		}
	}
	return false
}

// indirect returns the type that typ points to.
// The original typ is returned if typ is not a pointer.
func indirect(typ reflect.Type) reflect.Type {
	if typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	return typ
}

// addressable returns an addressable copy of val.
func addressable(val reflect.Value) reflect.Value {
	cpy := reflect.New(val.Type()).Elem()
	cpy.Set(val)
	return cpy
}
