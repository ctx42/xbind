// SPDX-FileCopyrightText: (c) 2026 Rafal Zajac
// SPDX-License-Identifier: MIT

package xbind

import (
	"database/sql/driver"
	"encoding"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Interfaces implemented by the test types.
var (
	_ Marshaler                = (*TStructMar)(nil)
	_ Marshaler                = (*TMarValPtr)(nil)
	_ Marshaler                = TMarValVal{}
	_ Marshaler                = TAliasMar("")
	_ encoding.TextMarshaler   = (*TTextPtr)(nil)
	_ encoding.TextUnmarshaler = (*TTextPtr)(nil)
	_ encoding.TextMarshaler   = TTextVal(0)
	_ encoding.TextUnmarshaler = (*ArrImp)(nil)
	_ Marshaler                = (*ArrImp)(nil)
	_ Marshaler                = (*MapImp)(nil)
	_ io.ReadCloser            = (*TRC)(nil)
	_ encoding.TextUnmarshaler = (*TItf)(nil)
	_ encoding.TextUnmarshaler = (*TUnmMap)(nil)
	_ encoding.TextMarshaler   = (*TUnmMap)(nil)
	_ encoding.TextMarshaler   = tTextA{}
	_ encoding.TextMarshaler   = tTextB{}
	_ encoding.TextUnmarshaler = (*tUnmInner)(nil)
	_ encoding.TextMarshaler   = TOwnText{}
	_ encoding.TextUnmarshaler = (*TOwnUnm)(nil)
	_ encoding.TextMarshaler   = TLevel(0)
	_ encoding.TextUnmarshaler = (*TLevel)(nil)
	_ encoding.TextMarshaler   = TFltKey{}
	_ encoding.TextMarshaler   = TGenText[int]{}
	_ zeroer                   = TOwnZero{}
)

// TString is a test struct with string fields.
type TString struct {
	FStr   string         `url:"f_str"`
	FpStr  *string        `url:"f_p_str"`
	FsStr  []string       `url:"f_s_str"`
	FspStr []*string      `url:"f_s_p_str"`
	FmStr  map[int]string `url:"f_m_int_str"`
	FaStr  [2]string      `url:"f_a_str"`
	FapStr [2]*string     `url:"f_a_p_str"`
}

// Named types based on base types.
type (
	IntAlias int
	FltAlias float64
	StrAlias string
)

// TAliases is a test struct with fields of named types.
type TAliases struct {
	FIntAlias  IntAlias   `url:"f_int_alias"`
	FsIntAlias []IntAlias `url:"f_s_int_alias"`
	FFltAlias  FltAlias   `url:"f_flt_alias"`
	FsFltAlias []FltAlias `url:"f_s_flt_alias"`
	FStrAlias  StrAlias   `url:"f_str_alias"`
	FsStrAlias []StrAlias `url:"f_s_str_alias"`
}

// TMultiEmbedded is a test struct embedding [TString] and [TAliases].
type TMultiEmbedded struct {
	TString
	TAliases
}

// TStructMar is a struct implementing the [Marshaler] interface.
type TStructMar struct {
	fStr  string
	fInt  int
	fsInt []int
}

func (tsm *TStructMar) MarshalValues(dst url.Values, prefix string) error {
	if tsm == nil {
		return nil
	}
	key := buildPath(prefix, "f_str")
	dst[key] = append(dst[key], tsm.fStr)

	key = buildPath(prefix, "f_int")
	dst[key] = append(dst[key], strconv.Itoa(tsm.fInt))

	var is []string
	for _, v := range tsm.fsInt {
		is = append(is, strconv.Itoa(v))
	}

	key = buildPath(prefix, "f_s_int")
	dst[key] = append(dst[key], is...)
	return nil
}

// TMarValPtr is a struct implementing [Marshaler] with one field.
type TMarValPtr struct {
	fStr string
}

func (mvp *TMarValPtr) MarshalValues(dst url.Values, prefix string) error {
	key := buildPath(prefix, "f_str")
	dst[key] = append(dst[key], "mar_"+mvp.fStr)
	return nil
}

// TMarValVal is a struct implementing [Marshaler] with a value receiver
// and one field.
type TMarValVal struct {
	fStr string
}

func (mvv TMarValVal) MarshalValues(dst url.Values, prefix string) error {
	if len(mvv.fStr) > 3 {
		return errors.New("string too long")
	}
	key := buildPath(prefix, "f_str")
	dst[key] = append(dst[key], "mar_"+mvv.fStr)
	return nil
}

// TAliasMar is a custom type implementing the [Marshaler] interface.
type TAliasMar string

func (tam TAliasMar) MarshalValues(dst url.Values, prefix string) error {
	dst[prefix] = []string{"mar_" + string(tam)}
	return nil
}

// TTextPtr implements the [encoding.TextMarshaler] and
// [encoding.TextUnmarshaler] interfaces with pointer receivers.
type TTextPtr byte

func (ttp *TTextPtr) MarshalText() (text []byte, err error) {
	v := byte(*ttp)
	if v < 32 || v > 126 {
		return nil, errors.New("non-printable ascii character")
	}
	vs := string(v)
	return []byte(vs), nil
}

func (ttp *TTextPtr) UnmarshalText(text []byte) error {
	val := string(text)
	valByte, err := strconv.ParseUint(val, 16, 8)
	if err != nil {
		return convertErr(val, "byte")
	}
	*ttp = TTextPtr(valByte)
	return nil
}

// TSWithUnm has [encoding.TextUnmarshaler] fields with pointer receivers.
type TSWithUnm struct {
	FUm   TTextPtr     `url:"f_um"`
	FpUm  *TTextPtr    `url:"f_p_um"`
	FapUm [1]*TTextPtr `url:"f_a_p_um"`
	FspUm []*TTextPtr  `url:"f_s_p_um"`
}

// TTextVal implements the [encoding.TextMarshaler] interface with a value
// receiver.
type TTextVal byte

func (ttv TTextVal) MarshalText() (text []byte, err error) {
	v := byte(ttv)
	if v < 32 || v > 126 {
		return nil, errors.New("non-printable ascii character")
	}
	return []byte(string(v)), nil
}

// TRequired is a struct with a required field.
type TRequired struct {
	FStr string `url:"f_str,required"`
	FInt int    `url:"f_int"`
}

// TReqUnm is a struct with a required field implementing
// [encoding.TextUnmarshaler], so it is decoded as a whole.
type TReqUnm struct {
	S string `url:"s,required"`
}

func (tru *TReqUnm) UnmarshalText(text []byte) error {
	tru.S = string(text)
	return nil
}

// TWithTime is a struct with a time field.
type TWithTime struct {
	FTime time.Time `url:"f_time"`
}

// TWithMarField is a struct with fields implementing [Marshaler].
type TWithMarField struct {
	FStr TMarValVal  `url:"f_mar"`
	FMar *TMarValPtr `url:"f_p_mar"`
}

// TWithMarSlice is a struct with slice fields of types implementing
// [Marshaler].
type TWithMarSlice struct {
	FsMar  []TMarValVal  `url:"f_s_mar"`
	FspMar []*TMarValPtr `url:"f_s_p_mar"`
}

// TWithMarMap is a struct with map fields of types implementing [Marshaler].
type TWithMarMap struct {
	FsMar  map[int]TMarValVal  `url:"f_m_mar"`
	FspMar map[int]*TMarValPtr `url:"f_m_p_mar"`
}

// Anonymous is a struct with an anonymous field.
type Anonymous struct {
	TWithTime
}

// AnonymousPtr is a struct with an anonymous field.
type AnonymousPtr struct {
	*TWithTime
}

// ArrImp represents a test type implementing the [encoding.TextUnmarshaler]
// and [Marshaler] interfaces.
type ArrImp [3]byte

// UnmarshalText stores up to three bytes of text in uppercase.
func (arr *ArrImp) UnmarshalText(b []byte) error {
	if len(b) > 3 {
		return errors.New("string too long")
	}
	str := strings.ToUpper(string(b))
	for i := 0; i < len(str) && i < len(arr); i++ {
		arr[i] = str[i]
	}
	return nil
}

// MarshalValues adds the bytes in lowercase under the prefix key.
func (arr *ArrImp) MarshalValues(dst url.Values, prefix string) error {
	str := string(arr[:])
	dst[prefix] = append(dst[prefix], strings.ToLower(str))
	return nil
}

// MapImp represents a test map type implementing the [Marshaler] interface.
type MapImp map[string]string

// MarshalValues adds each entry with both the key and the value doubled, under
// the key prefix followed by a dot and the doubled key.
func (mim *MapImp) MarshalValues(dst url.Values, prefix string) error {
	for k, v := range *mim {
		key := prefix + "." + k + k
		dst[key] = append(dst[key], v+v)
	}
	return nil
}

// TRC implements [io.ReadCloser]; it reads a simple JSON string and its Close
// method sets the Closed field to true.
type TRC struct {
	Closed bool
	read   bool // Set after the JSON string was read.
}

func (trc *TRC) Read(p []byte) (int, error) {
	if trc.read {
		return 0, io.EOF
	}
	trc.read = true
	return copy(p, `{"data": 123}`), nil
}

func (trc *TRC) Close() error {
	trc.Closed = true
	return nil
}

// TMixed is a struct with mixed types.
type TMixed struct {
	TRequired
	FStr  string    `url:"f_mix_str"`
	FTime time.Time `url:"f_mix_time"`
	FInt  int       `url:"f_mix_int"`
}

// tReq is an alias for [TRequired] to test unexported embedded structs.
type tReq = TRequired

// TNotExpEmb represents a struct with an unexported embedded struct that has
// exported fields.
type TNotExpEmb struct {
	tReq
	FStr string `url:"f_mix_str"`
}

// tStr is an alias for [TString] to test unexported embedded structs.
type tStr = TString

// TUnexpPtr represents a struct with an embedded unexported pointer to a
// struct, which the decoder can neither allocate nor descend into.
type TUnexpPtr struct {
	FInt int `url:"f_int"`
	*tStr
}

// TComplex represents a struct with a struct field implementing the
// [encoding.TextUnmarshaler] interface.
type TComplex struct {
	FItf TItf `url:"f_itf,required"`
	FInt int  `url:"f_int"`
}

// TItf represents a struct implementing the [encoding.TextUnmarshaler]
// interface.
type TItf struct{ Str string }

func (itf *TItf) UnmarshalText(text []byte) error {
	itf.Str = string(text)
	return nil
}

// TUnmMap is a map type implementing [encoding.TextUnmarshaler] and
// [encoding.TextMarshaler] with "key=value" text.
type TUnmMap map[string]string

func (tum *TUnmMap) UnmarshalText(text []byte) error {
	key, val, _ := strings.Cut(string(text), "=")
	*tum = TUnmMap{key: val}
	return nil
}

func (tum *TUnmMap) MarshalText() ([]byte, error) {
	for key, val := range *tum {
		return []byte(key + "=" + val), nil
	}
	return nil, nil
}

// tTextA and tTextB are unexported types implementing
// [encoding.TextMarshaler]; embedded together, their methods collide and are
// not promoted.
type (
	tTextA struct{ A string }
	tTextB struct{ B string }
)

func (tta tTextA) MarshalText() ([]byte, error) { return []byte(tta.A), nil }
func (ttb tTextB) MarshalText() ([]byte, error) { return []byte(ttb.B), nil }

// TPtrZero implements the IsZero method with a pointer receiver; it is zero
// when N is negative.
type TPtrZero struct {
	N int `url:"n"`
}

func (tpz *TPtrZero) IsZero() bool { return tpz.N < 0 }

// tUnmInner is an unexported struct implementing [encoding.TextUnmarshaler]
// with a pointer receiver.
type tUnmInner struct {
	X int `url:"x"`
}

func (tui *tUnmInner) UnmarshalText([]byte) error { return nil }

// TPromZero embeds a pointer to [time.Time], so it has the IsZero method
// promoted through a pointer that may be nil.
type TPromZero struct{ *time.Time }

// TOwnText embeds a pointer to [time.Time] and declares its own MarshalText
// method, which needs no embedded value.
type TOwnText struct{ *time.Time }

func (tot TOwnText) MarshalText() ([]byte, error) { return []byte("own"), nil }

// TOwnUnm declares its own UnmarshalText method and embeds a pointer to
// [TItem], which the method does not need.
type TOwnUnm struct {
	*TItem
	S string
}

func (tou *TOwnUnm) UnmarshalText(text []byte) error {
	tou.S = string(text)
	return nil
}

// TLevel is an int implementing [encoding.TextMarshaler] and
// [encoding.TextUnmarshaler] with "L<n>" text.
type TLevel int

func (tlv TLevel) MarshalText() ([]byte, error) {
	return []byte("L" + strconv.Itoa(int(tlv))), nil
}

func (tlv *TLevel) UnmarshalText(text []byte) error {
	val, err := strconv.Atoi(strings.TrimPrefix(string(text), "L"))
	*tlv = TLevel(val)
	return err
}

// TGenText is a generic type declaring its own MarshalText method.
type TGenText[T any] struct{ V T }

func (tgt TGenText[T]) MarshalText() ([]byte, error) { return nil, nil }

// TOwnZero embeds an interface and declares its own IsZero method, which
// reports it zero unless Set is true.
type TOwnZero struct {
	fmt.Stringer
	Set bool
}

func (toz TOwnZero) IsZero() bool { return !toz.Set }

// TCycle embeds a pointer to itself, so a walk over embedded fields can loop.
type TCycle struct{ *TCycle }

// TValuer implements [driver.Valuer] returning its value, or an error when
// the value is "error".
type TValuer string

func (tvl TValuer) Value() (driver.Value, error) {
	if tvl == "error" {
		return nil, errTest
	}
	return string(tvl), nil
}

// TSelfValuer implements [driver.Valuer] returning itself, which breaks the
// contract of the interface.
type TSelfValuer string

func (tsv TSelfValuer) Value() (driver.Value, error) { return tsv, nil }

// TAmbA and [TAmbB] both have an X field with the same key, so a struct
// embedding both has no field for the key x.
type TAmbA struct {
	X string `url:"x"`
	A string `url:"a"`
}

// TAmbB is the second half of [TAmbA].
type TAmbB struct {
	X string `url:"x"`
}

// TAmbig embeds [TAmbA] and [TAmbB] at the same depth.
type TAmbig struct {
	TAmbA
	TAmbB
}

// TTagA has an X field whose tag names it; [TTagB] has an untagged one.
type TTagA struct {
	X string `url:"X"`
}

// TTagB has an untagged X field.
type TTagB struct{ X string }

// TTagged embeds [TTagA] and [TTagB] at the same depth; the tagged field
// names the key X.
type TTagged struct {
	TTagA
	TTagB
}

// TWrapA embeds [TAmbB]; with [TWrapB] it puts TAmbB twice at one depth.
type TWrapA struct{ TAmbB }

// TWrapB embeds [TAmbB].
type TWrapB struct{ TAmbB }

// TDouble embeds [TAmbB] twice at depth two, so its X field is ambiguous.
type TDouble struct {
	TWrapA
	TWrapB
}

// Types holding themselves, so a walk over the types they hold can loop.
type (
	TRecMap   map[string]TRecMap
	TRecSlice []TRecSlice
	TRecPtr   *TRecPtr
)

// TDual implements [encoding.TextMarshaler] and [Marshaler], which adds its
// text under the prefix and ".v".
type TDual string

func (tdu TDual) MarshalText() ([]byte, error) { return []byte(tdu), nil }

func (tdu TDual) MarshalValues(dst url.Values, prefix string) error {
	dst.Add(prefix+".v", string(tdu))
	return nil
}

// TFltKey is a map key type holding a float, encoded as the float text.
type TFltKey struct{ F float64 }

func (tfk TFltKey) MarshalText() ([]byte, error) {
	return []byte(strconv.FormatFloat(tfk.F, 'f', -1, 64)), nil
}

// TItem is a struct used as an element of maps, slices and arrays.
type TItem struct {
	Size int    `url:"size"`
	Name string `url:"name"`
}

// TBox holds containers of [TItem] in every supported shape.
type TBox struct {
	Items  map[string]TItem   `url:"items"`
	PItems map[string]*TItem  `url:"pitems"`
	List   []TItem            `url:"list"`
	PList  []*TItem           `url:"plist"`
	Arr    [2]TItem           `url:"arr"`
	Nest   map[string][]TItem `url:"nest"`
	Grid   [][]int            `url:"grid"`
	Attr   map[string]string  `url:"attr"`
	Dotted int                `url:"page.size"`
}

// TPoint is a struct decoded and encoded as one "X,Y" value by the functions
// decPoint and encPoint.
type TPoint struct{ X, Y int }

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

// TNode is a recursive struct: a linked list node.
type TNode struct {
	Val  int    `url:"val"`
	Req  string `url:"req,required"`
	Next *TNode `url:"next"`
}

// TUnexpNode is a recursive struct embedding a pointer to an unexported
// struct, which the decoder cannot allocate in the nodes it allocates.
type TUnexpNode struct {
	Next *TUnexpNode `url:"next"`
	*tStr
}

// tSelfEmb embeds an unexported pointer to itself, whose fields its own
// fields shadow.
type tSelfEmb struct {
	*tSelfEmb
	X int `url:"x"`
}

// TSelfEmb embeds tSelfEmb by value.
type TSelfEmb struct{ tSelfEmb }

// TBuilder has a field of a type with a pointer to itself.
type TBuilder struct {
	B strings.Builder `url:"b"`
	S string          `url:"s"`
}

// TFuzzEmb is embedded in the fuzz test types: their S field shadows its S
// field, and its E field has a key of its own.
type TFuzzEmb struct {
	S string `url:"s"`
	E string `url:"e"`
}

// TFuzzDec is the destination of the decoder fuzz test: one field of every
// supported kind of value, a field with a registered codec, an embedded
// struct with a shadowed field, and a pointer to its own type.
type TFuzzDec struct {
	TRequired
	TFuzzEmb

	S    string            `url:"s"`
	I    int8              `url:"i"`
	U    uint16            `url:"u"`
	F    float32           `url:"f"`
	B    bool              `url:"b"`
	D    time.Duration     `url:"d"`
	T    time.Time         `url:"t"`
	P    *int              `url:"p"`
	SS   []string          `url:"ss"`
	A    [2]int            `url:"a"`
	M    map[string]int    `url:"m"`
	MK   map[TLevel]TLevel `url:"mk"`
	L    TLevel            `url:"l"`
	PL   *TLevel           `url:"pl"`
	Sub  *TString          `url:"sub"`
	PT   TPoint            `url:"pt"`
	LS   []TItem           `url:"ls"`
	MS   map[string]TItem  `url:"ms"`
	Next *TFuzzDec         `url:"next"`
	Ign  string            `url:"-"`
}

// TFuzzEnc is the source of the encoder fuzz test: one field of every
// supported kind of value, pointers, a field with a registered codec, and an
// embedded struct with a shadowed field.
type TFuzzEnc struct {
	TFuzzEmb

	S  string            `url:"s"`
	I  int64             `url:"i"`
	U  uint64            `url:"u"`
	F  float64           `url:"f"`
	B  bool              `url:"b"`
	D  time.Duration     `url:"d"`
	T  time.Time         `url:"t"`
	L  TLevel            `url:"l"`
	SS []string          `url:"ss"`
	M  map[string]string `url:"m"`
	MK map[TLevel]string `url:"mk"`
	P  *int64            `url:"p"`
	PS *string           `url:"ps"`
	PT TPoint            `url:"pt"`
	LS []TItem           `url:"ls"`
	MS map[string]TItem  `url:"ms"`
}
