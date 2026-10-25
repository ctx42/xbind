// SPDX-FileCopyrightText: (c) 2026 Rafal Zajac
// SPDX-License-Identifier: MIT

package bench

import (
	"errors"
	"fmt"
	"net/url"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ajg/form"
	"github.com/gin-gonic/gin/binding"
	pform "github.com/go-playground/form/v4"
	"github.com/google/go-querystring/query"
	"github.com/gorilla/schema"
	"github.com/monoculum/formam/v3"

	"github.com/ctx42/testing/pkg/assert"

	"github.com/ctx42/xbind/pkg/xbind"
)

// Flat is a struct of base types.
type Flat struct {
	Name   string    `url:"name" form:"name"`
	Age    int       `url:"age" form:"age"`
	Count  uint      `url:"count" form:"count"`
	Score  float64   `url:"score" form:"score"`
	Active bool      `url:"active" form:"active"`
	Born   time.Time `url:"born" form:"born"`
}

// Page is a struct nested in [Nested].
type Page struct {
	Size int `url:"size" form:"size"`
	Num  int `url:"num" form:"num"`
}

// Sort is a struct nested in [Nested].
type Sort struct {
	Field string `url:"field" form:"field"`
	Desc  bool   `url:"desc" form:"desc"`
}

// Nested is a struct of structs.
type Nested struct {
	Page Page `url:"page" form:"page"`
	Sort Sort `url:"sort" form:"sort"`
}

// Slices is a struct of slices of base types.
type Slices struct {
	Tags []string `url:"tag" form:"tag"`
	IDs  []int    `url:"id" form:"id"`
}

// Maps is a struct of maps of base types.
type Maps struct {
	Attr map[string]string `url:"attr" form:"attr"`
	Nums map[string]int    `url:"num" form:"num"`
}

// Item is a struct in the maps and slices of [Structs] and [List].
type Item struct {
	Size int    `url:"size" form:"size"`
	Name string `url:"name" form:"name"`
}

// Structs is a struct of a map and a slice of structs.
type Structs struct {
	Items map[string]Item `url:"items" form:"items"`
	List  []Item          `url:"list" form:"list"`
}

// List is a struct of a long slice of structs.
type List struct {
	List []Item `url:"list" form:"list"`
}

// workload is one decoding or encoding case.
type workload struct {
	name string
	src  url.Values // Keys with fields after dots, others in brackets.
	want any        // Value src decodes to; nil when decoding must fail.
	dst  func() any // Returns a new pointer to decode src to.
}

// workloads are the cases every library runs; the ones with a nil want are
// decoded only.
var workloads = []workload{
	{
		name: "flat",
		src: url.Values{
			"name":   {"alice"},
			"age":    {"42"},
			"count":  {"7"},
			"score":  {"1.5"},
			"active": {"true"},
			"born":   {"2001-02-03T04:05:06Z"},
		},
		want: Flat{
			Name:   "alice",
			Age:    42,
			Count:  7,
			Score:  1.5,
			Active: true,
			Born:   time.Date(2001, 2, 3, 4, 5, 6, 0, time.UTC),
		},
		dst: func() any { return &Flat{} },
	},
	{
		name: "nested",
		src: url.Values{
			"page.size":  {"20"},
			"page.num":   {"3"},
			"sort.field": {"name"},
			"sort.desc":  {"true"},
		},
		want: Nested{
			Page: Page{Size: 20, Num: 3},
			Sort: Sort{Field: "name", Desc: true},
		},
		dst: func() any { return &Nested{} },
	},
	{
		name: "slices",
		src: url.Values{
			"tag": {"a", "b", "c", "d", "e"},
			"id":  {"1", "2", "3", "4", "5"},
		},
		want: Slices{
			Tags: []string{"a", "b", "c", "d", "e"},
			IDs:  []int{1, 2, 3, 4, 5},
		},
		dst: func() any { return &Slices{} },
	},
	{
		name: "maps",
		src: url.Values{
			"attr[a]": {"1"},
			"attr[b]": {"2"},
			"attr[c]": {"3"},
			"num[x]":  {"1"},
			"num[y]":  {"2"},
			"num[z]":  {"3"},
		},
		want: Maps{
			Attr: map[string]string{"a": "1", "b": "2", "c": "3"},
			Nums: map[string]int{"x": 1, "y": 2, "z": 3},
		},
		dst: func() any { return &Maps{} },
	},
	{
		name: "structs",
		src: url.Values{
			"items[a].size": {"1"},
			"items[a].name": {"x"},
			"items[b].size": {"2"},
			"items[b].name": {"y"},
			"list[0].size":  {"3"},
			"list[0].name":  {"z"},
			"list[1].size":  {"4"},
			"list[1].name":  {"w"},
		},
		want: Structs{
			Items: map[string]Item{"a": {1, "x"}, "b": {2, "y"}},
			List:  []Item{{3, "z"}, {4, "w"}},
		},
		dst: func() any { return &Structs{} },
	},
	newList(100),
	{
		name: "err-value",
		src:  url.Values{"age": {"abc"}},
		dst:  func() any { return &Flat{} },
	},
	{
		name: "err-unknown",
		src:  url.Values{"nope": {"1"}},
		dst:  func() any { return &Flat{} },
	},
}

// newList returns a workload of a [List] with n items.
func newList(n int) workload {
	src := url.Values{}
	lst := List{List: make([]Item, n)}
	for i := range n {
		idx := strconv.Itoa(i)
		src["list["+idx+"].size"] = []string{strconv.Itoa(i + 1)}
		src["list["+idx+"].name"] = []string{"n" + idx}
		lst.List[i] = Item{Size: i + 1, Name: "n" + idx}
	}
	return workload{
		name: "list" + strconv.Itoa(n),
		src:  src,
		want: lst,
		dst:  func() any { return &List{} },
	}
}

// lib is a form library under comparison.
type lib struct {
	name    string
	key     func(string) string                 // Workload key to its syntax.
	indexed bool                                // Indexes slices of values.
	decode  func(dst any, src url.Values) error // Nil when it does not decode.
	encode  func(src any) (url.Values, error)   // Nil when it does not encode.
}

// libs are the libraries under comparison, xbind first.
var libs = []lib{
	func() lib {
		dec := xbind.NewDecoder()
		enc := xbind.NewEncoder()
		return lib{
			name:   "xbind",
			key:    func(key string) string { return key },
			decode: dec.Decode,
			encode: func(src any) (url.Values, error) {
				dst := url.Values{}
				return dst, enc.Encode(dst, src)
			},
		}
	}(),
	func() lib {
		dec := pform.NewDecoder()
		enc := pform.NewEncoder()
		return lib{
			name:   "go-playground",
			key:    func(key string) string { return key },
			decode: dec.Decode,
			encode: enc.Encode,
		}
	}(),
	func() lib {
		dec := schema.NewDecoder()
		dec.SetAliasTag("form")
		enc := schema.NewEncoder()
		enc.SetAliasTag("form")
		return lib{
			name: "gorilla",
			key:  dotKey,
			decode: func(dst any, src url.Values) error {
				return dec.Decode(dst, src)
			},
			encode: func(src any) (url.Values, error) {
				dst := url.Values{}
				return dst, enc.Encode(src, dst)
			},
		}
	}(),
	{
		name:    "ajg",
		key:     dotKey,
		indexed: true,
		decode:  form.DecodeValues,
		encode: func(src any) (url.Values, error) {
			return form.EncodeToValues(src)
		},
	},
	{
		name: "formam",
		key:  func(key string) string { return key },
		decode: func(dst any, src url.Values) error {
			opts := &formam.DecoderOptions{TagName: "form"}
			return formam.NewDecoder(opts).Decode(src, dst)
		},
	},
	{
		name: "gin",
		key:  lastKey,
		decode: func(dst any, src url.Values) error {
			return binding.MapFormWithTag(dst, src, "form")
		},
	},
	{
		name:   "go-querystring",
		key:    nestKey,
		encode: query.Values,
	},
}

// lastKey returns the part of a workload key after the last dot:
// "page.size" becomes "size".
func lastKey(key string) string {
	return key[strings.LastIndex(key, ".")+1:]
}

// dotKey turns brackets in a workload key into dots: "list[0].size" becomes
// "list.0.size".
func dotKey(key string) string {
	return strings.NewReplacer("[", ".", "]", "").Replace(key)
}

// nestKey turns dots in a workload key into brackets: "page.size" becomes
// "page[size]".
func nestKey(key string) string {
	head, rest, _ := strings.Cut(key, ".")
	for _, seg := range strings.Split(rest, ".") {
		if seg != "" {
			head += "[" + seg + "]"
		}
	}
	return head
}

// values returns a copy of src with its keys in the library's syntax, or an
// error when the syntax gives two keys one name.
func (lbr lib) values(src url.Values) (url.Values, error) {
	out := make(url.Values, len(src))
	for key, vals := range src {
		name := lbr.key(key)
		if !lbr.indexed || len(vals) == 1 {
			if _, ok := out[name]; ok {
				return nil, fmt.Errorf("cannot name key %q", key)
			}
			out[name] = slices.Clone(vals)
			continue
		}
		for i, val := range vals {
			out[name+"."+strconv.Itoa(i)] = []string{val}
		}
	}
	return out, nil
}

// checkDecode returns why the library fails the decoding of wrk, or nil.
func (lbr lib) checkDecode(wrk workload) error {
	if lbr.decode == nil {
		return errors.New("no decoder")
	}
	src, err := lbr.values(wrk.src)
	if err != nil {
		return err
	}
	dst := wrk.dst()
	err = lbr.decode(dst, src)
	if want, _ := lbr.values(wrk.src); !reflect.DeepEqual(src, want) {
		return fmt.Errorf("changes src to %v", src)
	}
	if wrk.want == nil {
		if err == nil {
			return errors.New("no error returned")
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("decode error: %w", err)
	}
	if have := reflect.ValueOf(dst).Elem().Interface(); !reflect.DeepEqual(
		have,
		wrk.want,
	) {
		return fmt.Errorf("decodes to %+v", have)
	}
	return nil
}

// checkEncode returns why the library fails the encoding of wrk, or nil.
func (lbr lib) checkEncode(wrk workload) error {
	if lbr.encode == nil {
		return errors.New("no encoder")
	}
	want, err := lbr.values(wrk.src)
	if err != nil {
		return err
	}
	have, err := lbr.encode(wrk.want)
	if err != nil {
		return fmt.Errorf("encode error: %w", err)
	}
	if !reflect.DeepEqual(have, want) {
		return fmt.Errorf("encodes to %v", have)
	}
	return nil
}

func Test_workloads_decode(t *testing.T) {
	for _, wrk := range workloads {
		t.Run(wrk.name, func(t *testing.T) {
			// --- When ---
			err := libs[0].checkDecode(wrk)

			// --- Then ---
			assert.NoError(t, err)
		})
	}
}

func Test_workloads_encode(t *testing.T) {
	for _, wrk := range workloads {
		if wrk.want == nil {
			continue
		}
		t.Run(wrk.name, func(t *testing.T) {
			// --- When ---
			err := libs[0].checkEncode(wrk)

			// --- Then ---
			assert.NoError(t, err)
		})
	}
}

func Benchmark_Decode(b *testing.B) {
	for _, wrk := range workloads {
		for _, lbr := range libs {
			b.Run(wrk.name+"/"+lbr.name, func(b *testing.B) {
				if err := lbr.checkDecode(wrk); err != nil {
					b.Skip(err)
				}
				src, _ := lbr.values(wrk.src)
				b.ReportAllocs()
				for b.Loop() {
					_ = lbr.decode(wrk.dst(), src)
				}
			})
		}
	}
}

func Benchmark_Encode(b *testing.B) {
	for _, wrk := range workloads {
		if wrk.want == nil {
			continue
		}
		for _, lbr := range libs {
			b.Run(wrk.name+"/"+lbr.name, func(b *testing.B) {
				if err := lbr.checkEncode(wrk); err != nil {
					b.Skip(err)
				}
				b.ReportAllocs()
				for b.Loop() {
					_, _ = lbr.encode(wrk.want)
				}
			})
		}
	}
}
