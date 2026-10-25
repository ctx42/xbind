// SPDX-FileCopyrightText: (c) 2026 Rafal Zajac
// SPDX-License-Identifier: MIT

package ref

import (
	"reflect"
	"strconv"
	"time"
)

// encFn represents the function signature for encoding base type values.
type encFn func(reflect.Value) (string, error)

// encReg is a map of custom types to encoders.
type encReg map[reflect.Type]encFn

// lookup returns the encoder registered for typ, or for the type typ points
// to, or nil when none is registered. The encoder for a pointer type must not
// be called with a nil pointer.
func (reg encReg) lookup(typ reflect.Type) encFn {
	if efn, ok := reg[typ]; ok {
		return efn
	}
	if typ.Kind() == reflect.Pointer {
		if efn, ok := reg[typ.Elem()]; ok {
			return func(v reflect.Value) (string, error) {
				return efn(v.Elem())
			}
		}
	}
	return nil
}

// encFor returns the encoder function for the base type represented by typ,
// looking in the reg registry first, or nil if no encoder can be found. The
// encoder of a pointer type must not be called with a nil pointer.
//
//nolint:cyclop
func encFor(typ reflect.Type, reg encReg) encFn {
	if f, ok := reg[typ]; ok {
		return f
	}

	if typ == typDur {
		return encDur
	}

	switch typ.Kind() {
	case reflect.Bool:
		return encBool

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return encInt

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32,
		reflect.Uint64:
		return encUint

	case reflect.Float32:
		return encFloat32

	case reflect.Float64:
		return encFloat64

	case reflect.Pointer:
		enc := encFor(typ.Elem(), reg)
		if enc == nil {
			return nil
		}

		return func(v reflect.Value) (string, error) { return enc(v.Elem()) }

	case reflect.String:
		return encString

	default:
		return nil
	}
}

// encBool encodes a boolean [reflect.Value] to a string.
// It panics if the kind of v is not Bool.
func encBool(v reflect.Value) (string, error) {
	return strconv.FormatBool(v.Bool()), nil
}

// encInt encodes an integer [reflect.Value] to a string.
// It panics if the kind of v is not Int, Int8, Int16, Int32, or Int64.
func encInt(v reflect.Value) (string, error) {
	return strconv.FormatInt(v.Int(), 10), nil
}

// encUint encodes an unsigned integer [reflect.Value] to a string.
// It panics if the kind of v is not Uint, Uintptr, Uint8, Uint16, Uint32, or
// Uint64.
func encUint(v reflect.Value) (string, error) {
	return strconv.FormatUint(v.Uint(), 10), nil
}

// encFloat encodes a float [reflect.Value] of the given bit size to a string.
// It panics if the kind of v is not Float32 or Float64.
func encFloat(v reflect.Value, bits int) (string, error) {
	return strconv.FormatFloat(v.Float(), 'f', -1, bits), nil
}

// encFloat32 encodes a float32 [reflect.Value] to a string.
// It panics if the kind of v is not Float32 or Float64.
func encFloat32(v reflect.Value) (string, error) {
	return encFloat(v, 32)
}

// encFloat64 encodes a float64 [reflect.Value] to a string.
// It panics if the kind of v is not Float32 or Float64.
func encFloat64(v reflect.Value) (string, error) {
	return encFloat(v, 64)
}

// encString returns the string v holds.
// It panics if the kind of v is not String.
func encString(v reflect.Value) (string, error) {
	return v.String(), nil
}

// encDur encodes a [time.Duration] [reflect.Value] to a string.
// It panics if the kind of v is not Int64.
func encDur(v reflect.Value) (string, error) {
	return time.Duration(v.Int()).String(), nil
}
