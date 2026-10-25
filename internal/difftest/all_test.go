// SPDX-FileCopyrightText: (c) 2026 Rafal Zajac
// SPDX-License-Identifier: MIT

package difftest

import (
	"fmt"
	"math/rand/v2"
	"net/url"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/ctx42/xrr/pkg/xrr"
)

// keys are the src keys randSrc draws from: canonical and alias spellings,
// conflicts, bad syntax, unknown and ignored names, and indexes past limits.
var keys = []string{
	"a", "[a]", "b", "s", "f", "bo", "tm", "d", "ids", "ids[0]", "ids[1]",
	"ids.0", "ids[01]", "ids[5]", "arr", "arr[0]", "arr[2]", "arr[3]",
	"m[k]", "m.k", "m[k.z]", "m.k.z", "m[j]", "mi[1]", "mi[01]", "mi[+1]",
	"mi[2]", "mf[0]", "mf[-0]", "mf[1.5]", "mf[NaN]", "ms[k].size",
	"ms[k][size]", "ms[k].tags", "ms[k].tags[0]", "ms[j].name", "l[0].size",
	"l.0.size", "l[1].name", "l[0][size]", "l[0].tags[1]", "p.size",
	"p.tags[0]", "p.tags", "x.y", "[x.y]", "lv", "lp", "e", "dup", "ign",
	"Ign", "unknown", "a.b", "a[0]", "m", "l", "p", "", "[", "a]", "l[0]",
	"req", "pres", "pi.need", "pi.here", "lc[0].need", "lc[1].here",
	"lc[0]", "ms[k]", "ids[1000]", "ids[1001]", "mc[1].need", "mc[01].need",
	"mc[+1].here", "mc[2].here", "mc.1.need", "n.size", "n.name", "n.tags",
	"n[size]", "n.tags[0]", "[n].size", "nd[v.w]", "nd.v.w", "n.size.x",
}

// vals are the src values randSrc draws from.
var vals = []string{
	"1", "2", "0", "-1", "abc", "", "true", "2001-02-03T04:05:06Z", "1s",
	"info", "warn", "1.5", "NaN",
}

// randSrc returns src values of a few random keys, now and then of more than
// the decoder sorts on the stack.
func randSrc(rnd *rand.Rand) url.Values {
	src := url.Values{}
	n := 1 + rnd.IntN(6)
	if rnd.IntN(10) == 0 {
		n = 17 + rnd.IntN(30)
	}
	for range n {
		key := keys[rnd.IntN(len(keys))]
		vs := []string{vals[rnd.IntN(len(vals))]}
		if rnd.IntN(5) == 0 {
			vs = append(vs, vals[rnd.IntN(len(vals))])
		}
		src[key] = vs
	}
	return src
}

// prefill sets fields of dst a decode may change, reset or keep.
func prefill(dst *T) {
	b := 3
	dst.B = &b
	dst.Ids = []int{9, 9}
	dst.Arr = [3]int{7, 7, 7}
	dst.M = map[string]int{"k": 5, "q": 6}
	dst.MI = map[int]string{1: "one"}
	dst.MS = map[string]Inner{"k": {Size: 4, Tags: []string{"x"}}}
	dst.L = []Inner{{Size: 7}, {Size: 8}}
	dst.P = &Inner{Size: 1}
}

// late sets one random time of dst, when dst has it, to a year past 9999,
// which encoding fails on.
func late(rnd *rand.Rand, dst *T) {
	tim := time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC)
	switch rnd.IntN(4) {
	case 0:
		dst.Tm = tim

	case 1:
		if len(dst.L) > 0 {
			dst.L[len(dst.L)-1].At = tim
		}

	case 2:
		for key, val := range dst.MS {
			val.At = tim
			dst.MS[key] = val
			break
		}

	default:
		if dst.P != nil {
			dst.P.At = tim
		}
	}
}

// outcome returns the text of the error and of the value a call left, which
// two calls give alike when they behave alike.
func outcome(err error, val any) string {
	var sb strings.Builder
	errText(&sb, err)
	sb.WriteString("\n")
	dump(&sb, reflect.ValueOf(val))
	return sb.String()
}

// errText writes the text and code of the error to sb, field by field for
// field errors.
func errText(sb *strings.Builder, err error) {
	if err == nil {
		sb.WriteString("<nil>")
		return
	}
	fs := xrr.GetFields(err)
	if fs == nil {
		_, _ = fmt.Fprintf(sb, "%s [%s]", err, xrr.GetCode(err))
		return
	}
	names := make([]string, 0, len(fs))
	for name := range fs {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		fe := fs[name]
		_, _ = fmt.Fprintf(sb, "%s=%s[%s]; ", name, fe, xrr.GetCode(fe))
	}
}

// dump writes v to sb, map entries in key order and pointers followed.
func dump(sb *strings.Builder, v reflect.Value) {
	switch v.Kind() {
	case reflect.Invalid:
		sb.WriteString("<invalid>")

	case reflect.Pointer, reflect.Interface:
		if v.IsNil() {
			sb.WriteString("nil")
			return
		}
		sb.WriteString("&")
		dump(sb, v.Elem())

	case reflect.Struct:
		if tim, ok := reflect.TypeAssert[time.Time](v); ok {
			sb.WriteString(tim.String())
			return
		}
		sb.WriteString("{")
		for i := range v.NumField() {
			sb.WriteString(v.Type().Field(i).Name + ":")
			dump(sb, v.Field(i))
			sb.WriteString(" ")
		}
		sb.WriteString("}")

	case reflect.Slice:
		if v.IsNil() {
			sb.WriteString("nil")
			return
		}
		dumpList(sb, v)

	case reflect.Array:
		dumpList(sb, v)

	case reflect.Map:
		if v.IsNil() {
			sb.WriteString("nil")
			return
		}
		var mks []string
		ents := map[string]reflect.Value{}
		for it := v.MapRange(); it.Next(); {
			mk := fmt.Sprint(it.Key())
			mks = append(mks, mk)
			ents[mk] = it.Value()
		}
		sort.Strings(mks)
		sb.WriteString("map[")
		for _, mk := range mks {
			sb.WriteString(mk + ":")
			dump(sb, ents[mk])
			sb.WriteString(" ")
		}
		sb.WriteString("]")

	default:
		_, _ = fmt.Fprint(sb, v)
	}
}

// dumpList writes the elements of the v slice or array to sb.
func dumpList(sb *strings.Builder, v reflect.Value) {
	sb.WriteString("[")
	for i := range v.Len() {
		dump(sb, v.Index(i))
		sb.WriteString(" ")
	}
	sb.WriteString("]")
}
