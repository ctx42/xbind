// SPDX-FileCopyrightText: (c) 2026 Rafal Zajac
// SPDX-License-Identifier: MIT

package xbind

import (
	"errors"
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

// waw represents the Warsaw timezone.
var waw = must.Value(time.LoadLocation("Europe/Warsaw"))
