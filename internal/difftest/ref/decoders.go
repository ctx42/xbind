// SPDX-FileCopyrightText: (c) 2026 Rafal Zajac
// SPDX-License-Identifier: MIT

package ref

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
	if f, ok := reg[typ]; ok {
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

// decBool converts a string to a boolean. It accepts the values
// [strconv.ParseBool] accepts and "on" or "off", in any letter case.
func decBool(vls ...string) (any, error) {
	val, err := single(vls)
	if err != nil {
		return nil, err
	}
	low := strings.ToLower(val)
	switch low {
	case "on":
		return true, nil

	case "off":
		return false, nil

	default:
		v, err := strconv.ParseBool(low)
		if err != nil {
			return nil, convertErr(val, "bool")
		}
		return v, nil
	}
}

// decFloat32 converts a string to float32.
func decFloat32(vls ...string) (any, error) {
	val, err := single(vls)
	if err != nil {
		return nil, err
	}
	v, err := strconv.ParseFloat(val, 32)
	if err != nil {
		return nil, convertErr(val, "float32")
	}
	return float32(v), nil
}

// decFloat64 converts a string to float64.
func decFloat64(vls ...string) (any, error) {
	val, err := single(vls)
	if err != nil {
		return nil, err
	}
	v, err := strconv.ParseFloat(val, 64)
	if err != nil {
		return nil, convertErr(val, "float64")
	}
	return v, nil
}

// decInt converts a string to int.
func decInt(vls ...string) (any, error) {
	val, err := single(vls)
	if err != nil {
		return nil, err
	}
	v, err := strconv.ParseInt(val, 10, 0)
	if err != nil {
		return nil, convertErr(val, "int")
	}
	return int(v), nil
}

// decInt8 converts a string to int8.
func decInt8(vls ...string) (any, error) {
	val, err := single(vls)
	if err != nil {
		return nil, err
	}
	v, err := strconv.ParseInt(val, 10, 8)
	if err != nil {
		return nil, convertErr(val, "int8")
	}
	return int8(v), nil
}

// decInt16 converts a string to int16.
func decInt16(vls ...string) (any, error) {
	val, err := single(vls)
	if err != nil {
		return nil, err
	}
	v, err := strconv.ParseInt(val, 10, 16)
	if err != nil {
		return nil, convertErr(val, "int16")
	}
	return int16(v), nil
}

// decInt32 converts a string to int32.
func decInt32(vls ...string) (any, error) {
	val, err := single(vls)
	if err != nil {
		return nil, err
	}
	v, err := strconv.ParseInt(val, 10, 32)
	if err != nil {
		return nil, convertErr(val, "int32")
	}
	return int32(v), nil
}

// decInt64 converts a string to int64.
func decInt64(vls ...string) (any, error) {
	val, err := single(vls)
	if err != nil {
		return nil, err
	}
	v, err := strconv.ParseInt(val, 10, 64)
	if err != nil {
		return nil, convertErr(val, "int64")
	}
	return v, nil
}

// decUint converts a string to uint.
func decUint(vls ...string) (any, error) {
	val, err := single(vls)
	if err != nil {
		return nil, err
	}
	v, err := strconv.ParseUint(val, 10, 0)
	if err != nil {
		return nil, convertErr(val, "uint")
	}
	return uint(v), nil
}

// decUint8 converts a string to uint8.
func decUint8(vls ...string) (any, error) {
	val, err := single(vls)
	if err != nil {
		return nil, err
	}
	v, err := strconv.ParseUint(val, 10, 8)
	if err != nil {
		return nil, convertErr(val, "uint8")
	}
	return uint8(v), nil
}

// decUint16 converts a string to uint16.
func decUint16(vls ...string) (any, error) {
	val, err := single(vls)
	if err != nil {
		return nil, err
	}
	v, err := strconv.ParseUint(val, 10, 16)
	if err != nil {
		return nil, convertErr(val, "uint16")
	}
	return uint16(v), nil
}

// decUint32 converts a string to uint32.
func decUint32(vls ...string) (any, error) {
	val, err := single(vls)
	if err != nil {
		return nil, err
	}
	v, err := strconv.ParseUint(val, 10, 32)
	if err != nil {
		return nil, convertErr(val, "uint32")
	}
	return uint32(v), nil
}

// decUint64 converts a string to uint64.
func decUint64(vls ...string) (any, error) {
	val, err := single(vls)
	if err != nil {
		return nil, err
	}
	v, err := strconv.ParseUint(val, 10, 64)
	if err != nil {
		return nil, convertErr(val, "uint64")
	}
	return v, nil
}

// decTime converts an RFC 3339 string to [time.Time], keeping its offset. An
// offset hour over 23 or minute over 59, which [time.Parse] takes but
// [time.Time.MarshalText] rejects, is an error.
func decTime(vls ...string) (any, error) {
	val, err := single(vls)
	if err != nil {
		return nil, err
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
		return nil, xrr.Newf("%w: %#q", ErrTimeFormat, val, code)
	}
	return v, nil
}

// decDur converts a string to [time.Duration].
func decDur(vls ...string) (any, error) {
	val, err := single(vls)
	if err != nil {
		return nil, err
	}
	v, err := time.ParseDuration(val)
	if err != nil {
		code := xrr.WithCode(ECInvDurFormat)
		return nil, xrr.Newf("%w: %#q", ErrDurFormat, val, code)
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
