// SPDX-FileCopyrightText: (c) 2026 Rafal Zajac
// SPDX-License-Identifier: MIT

// Package xbind binds HTTP request data to Go structs and back.
//
// A [Decoder] sets struct fields from [url.Values] (query strings, form
// bodies) and an [Encoder] turns a struct into [url.Values]. Both read the
// "url" struct tag for the field name and options:
//
//	type Query struct {
//		Name  string            `url:"name,required"`  // Must not be empty.
//		Tags  []string          `url:"tag"`            // Repeated key.
//		Attrs map[string]string `url:"attr"`           // Keys attr[<key>].
//		Page  *Page             `url:"page"`           // Keys page.<field>.
//		Note  string            `url:"note,omitempty"` // Skipped when empty.
//		Skip  int               `url:"-"`              // Never bound.
//	}
//
// A key is the path to a field: struct field names after dots, map keys and
// slice or array indexes in brackets ("page.size", "attr[color]",
// "list[0].size"); the fields of embedded structs belong to the outer struct.
// A slice or array of values takes one key with many values; maps, slices
// and arrays of structs, and any nesting of them, are supported. The value
// types are bool, string, the int, uint (except uintptr) and float kinds,
// their named variants, [time.Time] (RFC 3339), [time.Duration], and types
// implementing [encoding.TextUnmarshaler], [encoding.TextMarshaler] when the
// type also unmarshals text, or [Marshaler].
//
// [UnmarshalJSON], [UnmarshalJSONReader] and [UnmarshalJSONBody] decode JSON
// bodies strictly (unknown fields and trailing data are errors) and translate
// [encoding/json] errors with [HUE].
//
// Errors are [xrr] errors with codes; errors returned by marshalers and
// unmarshalers pass through. Decoding problems of individual values come
// back as [xrr.FieldErrors] keyed by the source key; pass them to
// [xrr.Envelop] with [ErrBind] as the lead error for an API response.
package xbind

import (
	"net/url"
	"reflect"
	"slices"
	"strings"

	"github.com/ctx42/xrr/pkg/xrr"

	"github.com/ctx42/mirror/pkg/mirror"
)

// defaultTag is the struct tag name read by [Encoder] and [Decoder].
const defaultTag = "url"

// Limits of the src keys [Decoder] takes, unless options set others.
const (
	// DefaultMaxIndex is the largest slice or array index a src key may use
	// unless [WithDecMaxIndex] sets another limit.
	DefaultMaxIndex = 1000

	// DefaultMaxDepth is the largest number of steps a src key may take into
	// dst unless [WithDecMaxDepth] sets another limit.
	DefaultMaxDepth = 32
)

// Marshaler is the interface implemented by types that can marshal themselves
// into [url.Values]. The [Encoder] does not call it on a nil pointer, a nil
// Marshaler interface, or a Marshaler interface holding a nil pointer; such a
// value adds nothing. A method promoted from a nil embedded pointer to a type
// declaring it with a pointer receiver is called with a nil receiver; one
// promoted through a nil embedded interface, or one the call would need to
// dereference a nil embedded pointer for, is not called, and the value adds
// nothing.
type Marshaler interface {
	// MarshalValues adds the values of the type to dst under the prefix key,
	// or under keys starting with the prefix. The prefix is empty for the
	// struct passed to [Encoder.Encode] and for the structs it embeds.
	MarshalValues(dst url.Values, prefix string) error
}

// Error codes.
const (
	ECBinding = "ECBinding"

	ECReqStructPtr = "ECReqStructPtr"

	ECReqStruct = "ECReqStruct"

	ECUnkFieldPath = "ECUnkFieldPath"

	// ECDecNotFound represents the missing type decoder error code.
	ECDecNotFound = "ECDecNotFound"

	// ECEncNotFound represents the missing type encoder error code.
	ECEncNotFound = "ECEncNotFound"

	// ECRequired represents the error code for a required field that is
	// empty after decoding. Only [Decoder] sets it.
	ECRequired = "ECRequired"

	// ECMissing represents the error code for a field with the "present"
	// option that no src key reaches. Only [Decoder] sets it.
	ECMissing = "ECMissing"

	// ECInvTimeFormat represents the error code for field values with an
	// invalid RFC 3339 time format.
	ECInvTimeFormat = "ECInvTimeFormat"

	// ECInvDurFormat represents the error code for field values with an
	// invalid [time.Duration] string representation.
	ECInvDurFormat = "ECInvDurFormat"

	ECInvType = "ECInvType"

	// ECInvValue represents the error code for values that cannot be decoded
	// from a string to a type.
	ECInvValue = "ECInvValue"

	// ECReqSingle represents the error code used when a single value is
	// expected but none or more than one were given.
	ECReqSingle = "ECReqSingle"

	// ECUnexported represents the error code for an embedded struct the
	// decoder cannot reach because it is an unexported nil pointer.
	ECUnexported = "ECUnexported"

	// ECCycle represents the error code for a pointer to a struct, or a map
	// or slice, already being encoded. Only [Encoder] sets it.
	ECCycle = "ECCycle"

	// ECInvTag represents the error code for a struct tag that does not
	// parse.
	ECInvTag = "ECInvTag"

	// ECNilDst represents the error code for a nil [url.Values] given to
	// [Encoder.Encode].
	ECNilDst = "ECNilDst"
)

// Binding errors.
var (
	// ErrBind represents a binding error either from the request body or from
	// the query parameters.
	ErrBind = xrr.New("binding error", ECBinding)

	// ErrReqStructPtr represents a [Decoder.Decode] destination that is not a
	// pointer to a struct.
	ErrReqStructPtr = xrr.New("struct pointer required", ECReqStructPtr)

	// ErrReqStruct represents an [Encoder.Encode] source that is neither a
	// struct nor a pointer to one.
	ErrReqStruct = xrr.New("struct or struct pointer required", ECReqStruct)

	// ErrType represents a mismatched field type during binding.
	ErrType = xrr.New("invalid type", ECInvType)

	// ErrTimeFormat represents an invalid time format when binding.
	ErrTimeFormat = xrr.New("invalid RFC3339 time", ECInvTimeFormat)

	// ErrDurFormat represents an invalid [time.Duration] string representation
	// format when binding.
	ErrDurFormat = xrr.New("invalid duration string", ECInvDurFormat)

	// ErrRequired represents an error for required fields left empty.
	ErrRequired = xrr.New("cannot be blank", ECRequired)

	// ErrMissing represents an error for present fields no src key reached.
	ErrMissing = xrr.New("is missing", ECMissing)

	// ErrInvValue represents an error for an invalid field value.
	ErrInvValue = xrr.New("invalid value", ECInvValue)

	// ErrReqSingleValue represents an error when none or many values are
	// provided for a field taking a single value.
	ErrReqSingleValue = xrr.New("require single value", ECReqSingle)

	// ErrUnexported represents a nil pointer to an embedded unexported
	// struct, which the decoder can neither allocate nor descend into.
	ErrUnexported = xrr.New("unexported field", ECUnexported)

	// ErrCycle represents a pointer cycle: a pointer to a struct, or a map or
	// slice, the encoder is already encoding.
	ErrCycle = xrr.New("pointer cycle", ECCycle)

	// ErrUnkFieldPath represents a source key that does not match a field.
	ErrUnkFieldPath = xrr.New("invalid field path", ECUnkFieldPath)

	// ErrDecNotFound represents a type the [Decoder] cannot decode.
	ErrDecNotFound = xrr.New("decoder not found", ECDecNotFound)

	// ErrEncNotFound represents a type the [Encoder] cannot encode.
	ErrEncNotFound = xrr.New("encoder not found", ECEncNotFound)

	// ErrInvTag represents a struct tag that does not parse.
	ErrInvTag = xrr.New("invalid struct tag", ECInvTag)

	// ErrNilDst represents a nil [url.Values] given to [Encoder.Encode].
	ErrNilDst = xrr.New("nil destination", ECNilDst)
)

// checkTags returns an error wrapping [ErrInvTag], with the Go path to the
// field, for the first field of the typ struct, or of a struct it nests, embeds
// or holds in slices, arrays and maps, whose struct tag does not parse, or
// whose key tag names it with a bracket or names an embedded struct; the tag of
// every key counts, as mirror parses the tag as a whole. The key tag decides
// which fields are ignored. Unexported fields, other than embedded ones, are
// skipped, as [Decoder] and [Encoder] skip them, and so are the fields of a
// struct that is not embedded and that leaf reports, or that a type leaf
// reports holds, the caller decodes or encodes as one value, as by its
// UnmarshalText method. The seen map holds
// the struct types checked so far, so a recursive type ends.
func checkTags(
	prefix string,
	typ reflect.Type,
	key string,
	leaf func(reflect.Type) bool,
	seen map[reflect.Type]bool,
) error {

	md := mirror.ReflectType(typ)
	if seen[md.Type()] {
		return nil
	}
	seen[md.Type()] = true
	for _, fld := range md.Fields() {
		if !fld.IsAnonymous() && !fld.IsExported() {
			continue
		}
		pth := buildPath(prefix, fld.Name())
		tag, err := fld.Tag(key)
		if err != nil {
			code := xrr.WithCode(ECInvTag)
			return xrr.Newf("%w: %s", ErrInvTag, pth, code)
		}
		if strings.ContainsAny(tag.Name(), "[]") {
			const format = "%w: %s: bracket in name"
			return xrr.Newf(format, ErrInvTag, pth, xrr.WithCode(ECInvTag))
		}
		base := baseType(fld.Type())
		if tag.IsIgnored() || base.Kind() != reflect.Struct {
			continue
		}
		raw := fld.StructField().Tag.Get(key)
		named := raw != "" && !strings.HasPrefix(raw, ",")
		isStruct := indirect(fld.Type()).Kind() == reflect.Struct
		if fld.IsAnonymous() && isStruct && named {
			// The fields of an embedded struct belong to the outer struct.
			const format = "%w: %s: name on embedded struct"
			return xrr.Newf(format, ErrInvTag, pth, xrr.WithCode(ECInvTag))
		}
		if !fld.IsAnonymous() && holdsLeaf(fld.Type(), leaf) {
			continue // The caller never reads its fields.
		}
		if err = checkTags(pth, base, key, leaf, seen); err != nil {
			return err
		}
	}
	return nil
}

// level is the position of a struct among the structs whose fields share
// one set of keys: the dst or src struct, or a struct reached by a key
// segment, together with the structs it embeds.
type level struct {
	vis map[string][]int // Index path of the field each key names.
	ign map[string]bool  // Keys naming only ignored fields.
	idx []int            // Index path of the struct in the level.
}

// newLevel returns the level of the typ struct for the key struct tag. A key
// names the field Go promotion selects, as [encoding/json] does: the shallowest
// field with that name or, among equally shallow ones, the only one whose
// struct tag names it; when there is no such field, the key names none. Ignored
// and unexported fields name no key, embedded interfaces are skipped and
// embedded structs walked; the names of ignored fields that no other field
// takes or shares are recorded as ignored keys. A struct type met at a
// shallower depth is not walked again, so a recursive type ends.
func newLevel(typ reflect.Type, key string) level {
	type node struct {
		typ reflect.Type // Struct type to walk.
		idx []int        // Its index path in the level.
	}
	type cand struct {
		idx    []int // Index path of the field.
		tagged bool  // The struct tag names the field.
	}

	vis := make(map[string][]int)
	ign := make(map[string]bool)
	done := make(map[string]bool)       // Keys decided at a shallower depth.
	seen := make(map[reflect.Type]bool) // Struct types of shallower depths.
	for next := []node{{typ: typ}}; len(next) > 0; {
		cur := next
		next = nil
		cands := make(map[string][]cand)
		walked := make(map[reflect.Type]bool)
		for _, nod := range cur {
			md := mirror.ReflectType(nod.typ)
			if seen[md.Type()] {
				continue
			}
			walked[md.Type()] = true
			for i, fld := range md.Fields() {
				tag, _ := fld.Tag(key) // The caller checked the tags.
				if tag.IsIgnored() {
					// An ignored embedded struct names no key of its own.
					if !fld.IsAnonymous() ||
						indirect(fld.Type()).Kind() != reflect.Struct {

						ign[tag.NameOrField()] = true
					}
					continue
				}
				idx := append(slices.Clip(nod.idx), i)
				if fld.IsAnonymous() {
					if fld.IsInterface() {
						continue
					}
					if indirect(fld.Type()).Kind() == reflect.Struct {
						next = append(next, node{typ: fld.Type(), idx: idx})
						continue
					}
				}
				if !fld.IsExported() {
					continue
				}
				raw := fld.StructField().Tag.Get(key)
				tagged := raw != "" && !strings.HasPrefix(raw, ",")
				name := tag.NameOrField()
				cands[name] = append(cands[name], cand{idx, tagged})
			}
		}
		for wtp := range walked {
			seen[wtp] = true
		}
		for name, cds := range cands {
			if done[name] {
				continue
			}
			done[name] = true
			if len(cds) == 1 {
				vis[name] = cds[0].idx
				continue
			}
			var tagged []cand
			for _, cnd := range cds {
				if cnd.tagged {
					tagged = append(tagged, cnd)
				}
			}
			if len(tagged) == 1 {
				vis[name] = tagged[0].idx
			}
		}
	}
	for name := range done {
		delete(ign, name) // A key some field takes, or several fields share.
	}
	return level{vis: vis, ign: ign}
}

// names returns true if the key name names field i of the struct at the
// level position.
func (lvl level) names(name string, i int) bool {
	return slices.Equal(lvl.vis[name], append(slices.Clip(lvl.idx), i))
}

// embedded returns the level position of the struct embedded as field i.
func (lvl level) embedded(i int) level {
	return level{vis: lvl.vis, idx: append(slices.Clip(lvl.idx), i)}
}
