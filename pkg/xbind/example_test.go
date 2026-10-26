// SPDX-FileCopyrightText: (c) 2026 Rafal Zajac
// SPDX-License-Identifier: MIT

package xbind_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"time"

	"github.com/ctx42/xbind/pkg/xbind"
	"github.com/ctx42/xrr/pkg/xrr"
)

func ExampleDecoder_Decode() {
	type Page struct {
		Size   int `url:"size"`
		Number int `url:"number"`
	}

	type Query struct {
		Name  string            `url:"name,required"`
		Tags  []string          `url:"tag"`
		Attrs map[string]string `url:"attr"`
		Since time.Time         `url:"since"`
		Page  *Page             `url:"page"`
	}

	src, _ := url.ParseQuery("" +
		"name=report&tag=a&tag=b&attr[color]=red&" +
		"since=2026-01-02T03:04:05Z&page.size=10&page.number=2",
	)

	dst := &Query{}
	if err := xbind.NewDecoder().Decode(dst, src); err != nil {
		panic(err)
	}

	fmt.Println(dst.Name, dst.Tags, dst.Attrs, dst.Since.Year(), *dst.Page)
	// Output:
	// report [a b] map[color:red] 2026 {10 2}
}

func ExampleDecoder_Decode_errors() {
	type Query struct {
		Name  string `url:"name,required"`
		Limit int    `url:"limit"`
	}

	src := url.Values{"limit": {"ten"}, "other": {"x"}}

	err := xbind.NewDecoder().Decode(&Query{}, src)

	for _, field := range xrr.FieldNames(err) {
		fe := xrr.GetFieldError(err, field)
		fmt.Printf("%s: %s (%s)\n", field, fe, xrr.GetCode(fe))
	}
	// Output:
	// limit: invalid value: cannot convert `ten` to int (ECInvValue)
	// name: cannot be blank (ECRequired)
	// other: invalid field path (ECUnkFieldPath)
}

func ExampleEncoder_Encode() {
	type Page struct {
		Size   int `url:"size"`
		Number int `url:"number,omitempty"`
	}

	type Query struct {
		Name  string            `url:"name"`
		Tags  []string          `url:"tag"`
		Attrs map[string]string `url:"attr"`
		Page  Page              `url:"page"`
		Skip  string            `url:"-"`
	}

	src := Query{
		Name:  "report",
		Tags:  []string{"a", "b"},
		Attrs: map[string]string{"color": "red"},
		Page:  Page{Size: 10},
		Skip:  "never",
	}

	dst := url.Values{}
	if err := xbind.NewEncoder().Encode(dst, src); err != nil {
		panic(err)
	}

	qry, _ := url.QueryUnescape(dst.Encode())
	fmt.Println(qry)
	// Output:
	// attr[color]=red&name=report&page.size=10&tag=a&tag=b
}

func ExampleUnmarshalJSON() {
	type Body struct {
		Name string `json:"name"`
	}

	err := xbind.UnmarshalJSON([]byte(`{"name": "a", "age": 1}`), &Body{})

	fmt.Println(errors.Is(err, xbind.ErrBind))
	fmt.Println(err)
	// Output:
	// true
	// json: unknown field "age"
}

func ExampleHUEFor() {
	type Body struct {
		Name string `json:"name"`
	}
	type T1 Body
	dst := &struct{ *T1 }{}

	err := json.Unmarshal([]byte(`{"name": 1}`), dst)

	fmt.Println(xbind.HUE(err))
	fmt.Println(xbind.HUEFor(dst, err))
	// Output:
	// T1.name: invalid type
	// name: invalid type
}

func ExampleWithDecFunc() {
	type Query struct {
		Day time.Time `url:"day"`
	}

	dateOnly := func(s string) (time.Time, error) {
		return time.Parse(time.DateOnly, s)
	}
	dec := xbind.NewDecoder(xbind.WithDecFunc(dateOnly))

	dst := &Query{}
	if err := dec.Decode(dst, url.Values{"day": {"2026-01-02"}}); err != nil {
		panic(err)
	}

	fmt.Println(dst.Day.Format(time.RFC3339))
	// Output:
	// 2026-01-02T00:00:00Z
}

func ExampleWithEncFunc() {
	type Query struct {
		Day time.Time `url:"day"`
	}

	dateOnly := func(t time.Time) (string, error) {
		return t.Format(time.DateOnly), nil
	}
	enc := xbind.NewEncoder(xbind.WithEncFunc(dateOnly))

	dst := url.Values{}
	src := Query{Day: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)}
	if err := enc.Encode(dst, src); err != nil {
		panic(err)
	}

	fmt.Println(dst.Encode())
	// Output:
	// day=2026-01-02
}
