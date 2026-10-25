// SPDX-FileCopyrightText: (c) 2026 Rafal Zajac
// SPDX-License-Identifier: MIT

package xbind

import (
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/ctx42/xrr/pkg/xrr"
)

// decFn represents the function signature for decoding string values.
type decFn func(vls ...string) (any, error)

// decReg is a map of custom types to decoders.
type decReg map[reflect.Type]decFn

// get returns the decoder registered for typ, or nil. An empty registry
// answers without hashing the type.
func (reg decReg) get(typ reflect.Type) decFn {
	if len(reg) == 0 {
		return nil
	}
	return reg[typ]
}

// Types with dedicated decoders; [time.Duration] also has a dedicated encoder.
var (
	typTime = reflect.TypeFor[time.Time]()
	typDur  = reflect.TypeFor[time.Duration]()
)

// decFor returns the decoder function for the base type represented by typ,
// looking in the reg registry first, or nil if no decoder can be found.
//
//nolint:cyclop
func decFor(typ reflect.Type, reg decReg) decFn {
	if f := reg.get(typ); f != nil {
		return f
	}

	switch typ {
	case typTime:
		return decTime

	case typDur:
		return decDur
	}

	switch typ.Kind() {
	case reflect.Bool:
		return decBool

	case reflect.Int:
		return decInt

	case reflect.Int8:
		return decInt8

	case reflect.Int16:
		return decInt16

	case reflect.Int32:
		return decInt32

	case reflect.Int64:
		return decInt64

	case reflect.Uint:
		return decUint

	case reflect.Uint8:
		return decUint8

	case reflect.Uint16:
		return decUint16

	case reflect.Uint32:
		return decUint32

	case reflect.Uint64:
		return decUint64

	case reflect.Float32:
		return decFloat32

	case reflect.Float64:
		return decFloat64

	case reflect.String:
		return decString

	default:
		return nil
	}
}

// isBuiltin returns true if setBuiltin sets a value of the typ type.
func isBuiltin(typ reflect.Type) bool {
	if typ == typTime || typ == typDur {
		return true
	}
	switch typ.Kind() { //nolint:exhaustive
	case reflect.Bool, reflect.String,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32,
		reflect.Uint64, reflect.Float32, reflect.Float64:

		return true

	default:
		return false
	}
}

// setFn sets dst, which must be settable, from vls.
type setFn func(dst reflect.Value, vls []string) error

// builtinSetter returns the function setting a value of the typ type as
// setBuiltin does, or nil when isBuiltin returns false for the type.
func builtinSetter(typ reflect.Type) setFn {
	switch typ {
	case typTime, typDur:
		return func(dst reflect.Value, vls []string) error {
			_, err := setBuiltin(dst, vls)
			return err
		}
	}
	switch kind := typ.Kind(); kind { //nolint:exhaustive
	case reflect.Bool:
		return func(dst reflect.Value, vls []string) error {
			v, err := parseBool(vls)
			if err == nil {
				dst.SetBool(v)
			}
			return err
		}

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return func(dst reflect.Value, vls []string) error {
			v, err := parseInt(vls, kind)
			if err == nil {
				dst.SetInt(v)
			}
			return err
		}

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32,
		reflect.Uint64:

		return func(dst reflect.Value, vls []string) error {
			v, err := parseUint(vls, kind)
			if err == nil {
				dst.SetUint(v)
			}
			return err
		}

	case reflect.Float32, reflect.Float64:
		return func(dst reflect.Value, vls []string) error {
			v, err := parseFloat(vls, kind)
			if err == nil {
				dst.SetFloat(v)
			}
			return err
		}

	case reflect.String:
		return func(dst reflect.Value, vls []string) error {
			v, err := single(vls)
			if err == nil {
				dst.SetString(v)
			}
			return err
		}

	default:
		return nil
	}
}

// setBuiltin sets dst, which must be settable, from vls as the built-in
// decoder of its type (see decFor) does, without boxing the value, and returns
// true and the error of the decoder. It returns false for a type with no
// built-in decoder.
func setBuiltin(dst reflect.Value, vls []string) (bool, error) {
	switch dst.Type() {
	case typTime:
		v, err := parseTime(vls)
		if err == nil {
			ptr, _ := reflect.TypeAssert[*time.Time](dst.Addr())
			*ptr = v
		}
		return true, err

	case typDur:
		v, err := parseDur(vls)
		if err == nil {
			dst.SetInt(int64(v))
		}
		return true, err
	}

	var err error
	switch kind := dst.Kind(); kind { //nolint:exhaustive
	case reflect.Bool:
		var v bool
		if v, err = parseBool(vls); err == nil {
			dst.SetBool(v)
		}

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		var v int64
		if v, err = parseInt(vls, kind); err == nil {
			dst.SetInt(v)
		}

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32,
		reflect.Uint64:

		var v uint64
		if v, err = parseUint(vls, kind); err == nil {
			dst.SetUint(v)
		}

	case reflect.Float32, reflect.Float64:
		var v float64
		if v, err = parseFloat(vls, kind); err == nil {
			dst.SetFloat(v)
		}

	case reflect.String:
		var v string
		if v, err = single(vls); err == nil {
			dst.SetString(v)
		}

	default:
		return false, nil
	}
	return true, err
}

// decBool converts a string to a boolean. It accepts the values
// [strconv.ParseBool] accepts and "on" or "off", in any letter case.
func decBool(vls ...string) (any, error) {
	v, err := parseBool(vls)
	if err != nil {
		return nil, err
	}
	return v, nil
}

// decFloat32 converts a string to float32.
func decFloat32(vls ...string) (any, error) {
	v, err := parseFloat(vls, reflect.Float32)
	if err != nil {
		return nil, err
	}
	return float32(v), nil
}

// decFloat64 converts a string to float64.
func decFloat64(vls ...string) (any, error) {
	v, err := parseFloat(vls, reflect.Float64)
	if err != nil {
		return nil, err
	}
	return v, nil
}

// decInt converts a string to int.
func decInt(vls ...string) (any, error) {
	v, err := parseInt(vls, reflect.Int)
	if err != nil {
		return nil, err
	}
	return int(v), nil
}

// decInt8 converts a string to int8.
func decInt8(vls ...string) (any, error) {
	v, err := parseInt(vls, reflect.Int8)
	if err != nil {
		return nil, err
	}
	return int8(v), nil
}

// decInt16 converts a string to int16.
func decInt16(vls ...string) (any, error) {
	v, err := parseInt(vls, reflect.Int16)
	if err != nil {
		return nil, err
	}
	return int16(v), nil
}

// decInt32 converts a string to int32.
func decInt32(vls ...string) (any, error) {
	v, err := parseInt(vls, reflect.Int32)
	if err != nil {
		return nil, err
	}
	return int32(v), nil
}

// decInt64 converts a string to int64.
func decInt64(vls ...string) (any, error) {
	v, err := parseInt(vls, reflect.Int64)
	if err != nil {
		return nil, err
	}
	return v, nil
}

// decUint converts a string to uint.
func decUint(vls ...string) (any, error) {
	v, err := parseUint(vls, reflect.Uint)
	if err != nil {
		return nil, err
	}
	return uint(v), nil
}

// decUint8 converts a string to uint8.
func decUint8(vls ...string) (any, error) {
	v, err := parseUint(vls, reflect.Uint8)
	if err != nil {
		return nil, err
	}
	return uint8(v), nil
}

// decUint16 converts a string to uint16.
func decUint16(vls ...string) (any, error) {
	v, err := parseUint(vls, reflect.Uint16)
	if err != nil {
		return nil, err
	}
	return uint16(v), nil
}

// decUint32 converts a string to uint32.
func decUint32(vls ...string) (any, error) {
	v, err := parseUint(vls, reflect.Uint32)
	if err != nil {
		return nil, err
	}
	return uint32(v), nil
}

// decUint64 converts a string to uint64.
func decUint64(vls ...string) (any, error) {
	v, err := parseUint(vls, reflect.Uint64)
	if err != nil {
		return nil, err
	}
	return v, nil
}

// decTime converts an RFC 3339 string to [time.Time], keeping its offset (see
// parseTime).
func decTime(vls ...string) (any, error) {
	v, err := parseTime(vls)
	if err != nil {
		return nil, err
	}
	return v, nil
}

// decDur converts a string to [time.Duration].
func decDur(vls ...string) (any, error) {
	v, err := parseDur(vls)
	if err != nil {
		return nil, err
	}
	return v, nil
}

// decString returns the only string in vls.
func decString(vls ...string) (any, error) {
	val, err := single(vls)
	if err != nil {
		return nil, err
	}
	return val, nil
}

// parseBool returns the boolean the only value in vls holds. It accepts the
// values [strconv.ParseBool] accepts and "on" or "off", in any letter case.
func parseBool(vls []string) (bool, error) {
	val, err := single(vls)
	if err != nil {
		return false, err
	}
	switch {
	case strings.EqualFold(val, "on"):
		return true, nil

	case strings.EqualFold(val, "off"):
		return false, nil

	default:
		v, err := strconv.ParseBool(strings.ToLower(val))
		if err != nil {
			return false, convertErr(val, "bool")
		}
		return v, nil
	}
}

// parseInt returns the integer of the kind the only value in vls holds.
func parseInt(vls []string, kind reflect.Kind) (int64, error) {
	val, err := single(vls)
	if err != nil {
		return 0, err
	}
	v, err := strconv.ParseInt(val, 10, bitSize(kind))
	if err != nil {
		return 0, convertErr(val, kind.String())
	}
	return v, nil
}

// parseUint returns the unsigned integer of the kind the only value in vls
// holds.
func parseUint(vls []string, kind reflect.Kind) (uint64, error) {
	val, err := single(vls)
	if err != nil {
		return 0, err
	}
	v, err := strconv.ParseUint(val, 10, bitSize(kind))
	if err != nil {
		return 0, convertErr(val, kind.String())
	}
	return v, nil
}

// parseFloat returns the float of the kind the only value in vls holds.
func parseFloat(vls []string, kind reflect.Kind) (float64, error) {
	val, err := single(vls)
	if err != nil {
		return 0, err
	}
	v, err := strconv.ParseFloat(val, bitSize(kind))
	if err != nil {
		return 0, convertErr(val, kind.String())
	}
	return v, nil
}

// bitSize returns the size in bits of a number of the kind for [strconv], zero
// for int and uint.
func bitSize(kind reflect.Kind) int {
	switch kind { //nolint:exhaustive
	case reflect.Int8, reflect.Uint8:
		return 8

	case reflect.Int16, reflect.Uint16:
		return 16

	case reflect.Int32, reflect.Uint32, reflect.Float32:
		return 32

	case reflect.Int64, reflect.Uint64, reflect.Float64:
		return 64

	default:
		return 0
	}
}

// parseTime returns the [time.Time] of the RFC 3339 only value in vls,
// keeping its offset. An offset hour over 23 or minute over 59, which
// [time.Parse] takes but [time.Time.MarshalText] rejects, is an error.
func parseTime(vls []string) (time.Time, error) {
	val, err := single(vls)
	if err != nil {
		return time.Time{}, err
	}
	v, err := time.Parse(time.RFC3339, val)
	if n := len(val); err == nil && strings.ContainsAny(val[n-6:n-5], "+-") {
		// The value ends in a "+hh:mm" offset.
		if val[n-5:n-3] > "23" || val[n-2:] > "59" {
			err = ErrTimeFormat
		}
	}
	if err != nil {
		code := xrr.WithCode(ECInvTimeFormat)
		return time.Time{}, xrr.Newf("%w: %#q", ErrTimeFormat, val, code)
	}
	return v, nil
}

// parseDur returns the [time.Duration] of the only value in vls.
func parseDur(vls []string) (time.Duration, error) {
	val, err := single(vls)
	if err != nil {
		return 0, err
	}
	v, err := time.ParseDuration(val)
	if err != nil {
		code := xrr.WithCode(ECInvDurFormat)
		return 0, xrr.Newf("%w: %#q", ErrDurFormat, val, code)
	}
	return v, nil
}

// single returns the only value in vls or an error wrapping
// [ErrReqSingleValue] when vls has none or more than one value.
func single(vls []string) (string, error) {
	if len(vls) != 1 {
		const format = "%w: received %d"
		code := xrr.WithCode(ECReqSingle)
		return "", xrr.Newf(format, ErrReqSingleValue, len(vls), code)
	}
	return vls[0], nil
}

// convertErr returns an error wrapping [ErrInvValue] saying the val cannot be
// converted to the typ type.
func convertErr(val, typ string) error {
	const format = "%w: cannot convert %#q to %s"
	return xrr.Newf(format, ErrInvValue, val, typ, xrr.WithCode(ECInvValue))
}
