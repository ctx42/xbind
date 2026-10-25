// SPDX-FileCopyrightText: (c) 2026 Rafal Zajac
// SPDX-License-Identifier: MIT

package difftest

import (
	"math/rand/v2"
	"net/url"
	"reflect"
	"testing"

	"github.com/ctx42/testing/pkg/assert"

	"github.com/ctx42/xbind/internal/difftest/ref"
	"github.com/ctx42/xbind/pkg/xbind"
)

// Number of random calls each test compares.
const (
	decodes = 300_000
	encodes = 200_000
)

// decoder is implemented by xbind.Decoder and ref.Decoder.
type decoder interface {
	Decode(dst any, src url.Values) error
}

// encoder is implemented by xbind.Encoder and ref.Encoder.
type encoder interface {
	Encode(dst url.Values, src any) error
}

func Test_Decoder_Decode(t *testing.T) {
	// --- Given ---
	rnd := rand.New(rand.NewPCG(1, 2))
	pairs := []struct {
		have decoder
		want decoder
	}{
		{xbind.NewDecoder(), ref.NewDecoder()},
		{&xbind.Decoder{}, &ref.Decoder{}},
		{
			xbind.NewDecoder(
				xbind.WithDecAllowUnknown(),
				xbind.WithDecMaxIndex(2),
			),
			ref.NewDecoder(ref.WithDecAllowUnknown(), ref.WithDecMaxIndex(2)),
		},
		{
			xbind.NewDecoder(xbind.WithDecMaxDepth(0)),
			ref.NewDecoder(ref.WithDecMaxDepth(0)),
		},
		{
			xbind.NewDecoder(xbind.WithDecMaxDepth(1)),
			ref.NewDecoder(ref.WithDecMaxDepth(1)),
		},
	}

	for i := range decodes {
		src := randSrc(rnd)
		pair := pairs[i%len(pairs)]
		fill := rnd.IntN(2) == 0
		var hDst, wDst any = &T{}, &T{}
		if rnd.IntN(3) == 0 {
			hTC, wTC := &TC{}, &TC{}
			if fill {
				prefill(&hTC.T)
				prefill(&wTC.T)
			}
			hDst, wDst = hTC, wTC
		} else if fill {
			prefill(hDst.(*T))
			prefill(wDst.(*T))
		}

		// --- When ---
		hErr := pair.have.Decode(hDst, src)

		// --- Then ---
		want := outcome(pair.want.Decode(wDst, src), wDst)
		if have := outcome(hErr, hDst); have != want {
			assert.Equal(t, want, have)
			t.Logf("call %d, decoder %d, src: %v", i, i%len(pairs), src)
			return
		}
	}
}

func Test_Encoder_Encode(t *testing.T) {
	// --- Given ---
	rnd := rand.New(rand.NewPCG(3, 4))
	dec := ref.NewDecoder(ref.WithDecAllowUnknown())
	pairs := []struct {
		have encoder
		want encoder
	}{
		{xbind.NewEncoder(), ref.NewEncoder()},
		{&xbind.Encoder{}, &ref.Encoder{}},
	}

	for i := range encodes {
		var val any = &T{}
		tv := val.(*T)
		if rnd.IntN(3) == 0 {
			tc := &TC{}
			val, tv = tc, &tc.T
		}
		if rnd.IntN(2) == 0 {
			prefill(tv)
		}
		_ = dec.Decode(val, randSrc(rnd)) // A value is set even on error.
		if rnd.IntN(10) == 0 {
			late(rnd, tv)
		}
		src := val
		if rnd.IntN(2) == 0 {
			src = reflect.ValueOf(val).Elem().Interface() // By value.
		}
		pair := pairs[i%len(pairs)]
		hDst, wDst := url.Values{}, url.Values{}

		// --- When ---
		hErr := pair.have.Encode(hDst, src)

		// --- Then ---
		want := outcome(pair.want.Encode(wDst, src), wDst)
		if have := outcome(hErr, hDst); have != want {
			assert.Equal(t, want, have)
			t.Logf("call %d, encoder %d, src: %+v", i, i%len(pairs), src)
			return
		}
	}
}
