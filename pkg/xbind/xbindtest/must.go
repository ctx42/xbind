// SPDX-FileCopyrightText: (c) 2026 Rafal Zajac
// SPDX-License-Identifier: MIT

// Package xbindtest provides test helpers for package [xbind] that panic
// instead of returning errors.
package xbindtest

import (
	"net/url"

	"github.com/ctx42/testing/pkg/must"
	"github.com/ctx42/xbind/pkg/xbind"
)

// MustEncode encodes the src struct as [url.Values] with the default
// [xbind.Encoder]. It panics with the error Encode returns.
func MustEncode(src any) url.Values {
	dst := url.Values{}
	must.Nil(xbind.NewEncoder().Encode(dst, src))
	return dst
}

// MustDecode decodes vs to the struct pointed to by dst with the default
// [xbind.Decoder]. It panics with the error Decode returns.
func MustDecode(dst any, vs url.Values) {
	must.Nil(xbind.NewDecoder().Decode(dst, vs))
}
