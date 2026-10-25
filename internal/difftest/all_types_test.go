// SPDX-FileCopyrightText: (c) 2026 Rafal Zajac
// SPDX-License-Identifier: MIT

package difftest

import (
	"errors"
	"time"
)

// Level decodes and encodes through its text methods.
type Level int

func (lvl *Level) UnmarshalText(b []byte) error {
	switch string(b) {
	case "info":
		*lvl = 1
	case "warn":
		*lvl = 2
	default:
		return errors.New("bad level")
	}
	return nil
}

func (lvl Level) MarshalText() ([]byte, error) {
	switch lvl {
	case 1:
		return []byte("info"), nil
	case 2:
		return []byte("warn"), nil
	}
	return []byte("none"), nil
}

// Inner is a struct held by value, by pointer, in slices and in maps.
type Inner struct {
	Size int      `url:"size"`
	Name string   `url:"name,omitempty"`
	Tags []string `url:"tags"`
}

// Emb is embedded in T; its Dup field is shadowed by the one of T.
type Emb struct {
	E   int   `url:"e"`
	Dup int   `url:"dup"`
	N   Inner `url:"n"`
	ND  struct {
		V int `url:"v.w"`
	} `url:"nd"`
}

// T has a field of every kind of value the decoder and the encoder handle.
type T struct {
	Emb
	A   int              `url:"a"`
	B   *int             `url:"b"`
	S   string           `url:"s"`
	F   float64          `url:"f"`
	Bo  bool             `url:"bo"`
	Tm  time.Time        `url:"tm"`
	D   time.Duration    `url:"d"`
	Ids []int            `url:"ids"`
	Arr [3]int           `url:"arr"`
	M   map[string]int   `url:"m"`
	MI  map[int]string   `url:"mi"`
	MF  map[float64]int  `url:"mf"`
	MS  map[string]Inner `url:"ms"`
	L   []Inner          `url:"l"`
	P   *Inner           `url:"p"`
	Dot int              `url:"x.y"`
	Lv  Level            `url:"lv"`
	LP  *Level           `url:"lp"`
	Ign int              `url:"-"`
	Dup int              `url:"dup"`
}

// TC is T with "required" and "present" fields, at the top and nested.
type TC struct {
	T
	Req  string         `url:"req,required"`
	Pres int            `url:"pres,present"`
	PI   *InnerC        `url:"pi"`
	LC   []InnerC       `url:"lc"`
	MC   map[int]InnerC `url:"mc"`
}

// InnerC is a nested struct with fields to check.
type InnerC struct {
	Need int `url:"need,required"`
	Here int `url:"here,present"`
}
