// SPDX-FileCopyrightText: (c) 2026 Rafal Zajac
// SPDX-License-Identifier: MIT

package xbind

import (
	"errors"
	"fmt"
	"reflect"
	"time"

	"github.com/ctx42/testing/pkg/must"
)

// errTest is a general error used in tests.
var errTest = errors.New("test error")

// badTag returns a pointer to a new struct with a string field N, whose
// struct tag tag may not parse, inside an F field when nested is true. The
// type is built at run time, so go vet does not see the tag.
func badTag(tag reflect.StructTag, nested bool) any {
	typ := reflect.StructOf([]reflect.StructField{
		{Name: "N", Type: reflect.TypeFor[string](), Tag: tag},
	})
	if nested {
		typ = reflect.StructOf([]reflect.StructField{
			{Name: "F", Type: reflect.PointerTo(typ)},
		})
	}
	return reflect.New(typ).Interface()
}

// noLeaf is a checkTags leaf predicate reporting no type as a leaf.
func noLeaf(reflect.Type) bool { return false }

// waw represents the Warsaw timezone.
var waw = must.Value(time.LoadLocation("Europe/Warsaw"))

// decPoint decodes a [TPoint] from "X,Y".
func decPoint(s string) (TPoint, error) {
	var pnt TPoint
	_, err := fmt.Sscanf(s, "%d,%d", &pnt.X, &pnt.Y)
	return pnt, err
}

// encPoint encodes a [TPoint] as "X,Y".
func encPoint(pnt TPoint) (string, error) {
	return fmt.Sprintf("%d,%d", pnt.X, pnt.Y), nil
}

// noSetters returns a copy of the steps without their set functions and the
// field types that go with them, which equality cannot compare.
func noSetters(steps []step) []step {
	out := make([]step, len(steps))
	for i, stp := range steps {
		stp.set, stp.ftyp = nil, nil
		out[i] = stp
	}
	return out
}
