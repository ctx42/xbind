// SPDX-FileCopyrightText: (c) 2026 Rafal Zajac
// SPDX-License-Identifier: MIT

package xbind

import (
	"errors"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/ctx42/testing/pkg/must"
)

// errTest is a general error used in tests.
var errTest = errors.New("test error")

// waw represents the Warsaw timezone.
var waw = must.Value(time.LoadLocation("Europe/Warsaw"))

// dropNullPtrs deletes the keys of the nil pointer fields named "p" or "pl"
// from vls. Nil pointers encode as "null", which the decoder does not read
// back as nil.
func dropNullPtrs(vls url.Values) {
	for key, val := range vls {
		name := key[strings.LastIndex(key, ".")+1:]
		isPtr := name == "p" || name == "pl"
		if isPtr && slices.Equal(val, []string{"null"}) {
			delete(vls, key)
		}
	}
}
