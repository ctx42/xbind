[![Go](https://github.com/ctx42/xbind/actions/workflows/go.yml/badge.svg)](https://github.com/ctx42/xbind/actions/workflows/go.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/ctx42/xbind.svg)](https://pkg.go.dev/github.com/ctx42/xbind)
[![Go Version](https://img.shields.io/github/go-mod/go-version/ctx42/xbind)](go.mod)
[![License](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE.md)

# xbind

Bind URL query and form values to Go structs and back, and decode JSON request
bodies strictly.

xbind maps `url.Values` onto struct fields by key paths built from the `url`
struct tag (`page.size`, `attr[color]`, `items[0].name`), and encodes structs
back into `url.Values` with the same keys, so an HTTP handler can declare its
parameters as a struct. Problems with individual values come back as per-key
field errors with error codes, ready to send to an API client. A small JSON
helper set decodes request bodies, rejecting unknown fields and trailing data,
and translates `encoding/json` errors into the same error style.

## Features

- `Decoder`: `url.Values` to a struct, with `required` fields, ignored fields
  and an option to allow unknown keys.
- `Encoder`: a struct to `url.Values`, with `omitempty` and ignored fields.
- Nested structs, embedded structs, slices, arrays and maps, addressed by
  field names after dots and map keys and indexes in brackets (`page.size`,
  `attr[color]`, `items[0].name`).
- `bool`, `string`, integer (except `uintptr`) and float types and their
  named variants, `time.Time` (RFC 3339) and `time.Duration`.
- Custom types through `encoding.TextUnmarshaler` / `encoding.TextMarshaler`,
  whole-struct encoding through the `Marshaler` interface, and per-type
  functions registered with `WithDecFunc` / `WithEncFunc`.
- Structured errors built on [ctx42/xrr](https://github.com/ctx42/xrr): the
  package's errors carry codes, and value errors are field errors keyed by the
  source key.
- Strict JSON body decoding: `UnmarshalJSON`, `UnmarshalJSONReader`,
  `UnmarshalJSONBody`.

## Install

```bash
go get github.com/ctx42/xbind
```

```go
import "github.com/ctx42/xbind/pkg/xbind"
```

## Usage

### Decoding values

`Decode` takes a pointer to a struct and the values to bind, for example
`r.URL.Query()` or `r.PostForm`:

<!-- gmmce:pkg/xbind/ExampleDecoder_Decode -->
```go
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
```

### Keys and tags

A key is the path to a field: struct field names after dots, map keys and
slice indexes in brackets. The `url` tag sets the name of a field; an untagged
field uses its Go name.

| Field type          | Tag               | Keys                   |
|---------------------|-------------------|------------------------|
| `string`            | `url:"name"`      | `name=x`               |
| `[]string`          | `url:"tag"`       | `tag=a&tag=b`          |
| `map[string]string` | `url:"attr"`      | `attr[color]=red`      |
| `*Page` (a struct)  | `url:"page"`      | `page.size=10`         |
| `[]Item` (structs)  | `url:"items"`     | `items[0].name=x`      |
| `map[string]Item`   | `url:"byid"`      | `byid[a1].name=x`      |
| `int`               | `url:"page.size"` | `[page.size]=10`       |
| embedded struct     | none              | the keys of its fields |

Brackets may hold dots, so map keys and tag names with dots work. A tag name may
not hold a bracket: a tag such as `url:"ids[]"` makes `Decode` and `Encode`
return `ErrInvTag`, so tag the field `url:"ids"` and send `ids=1&ids=2`. An
embedded struct takes no name, as its fields belong to the outer struct: a tag
such as `url:"page"` on one is an `ErrInvTag` error too. The tags inside a type
handled as one value, by its own text methods or a registered function, are not
checked. The decoder also reads `attr.color` for a map of values, taking the
rest of the key as the map key, and `items.0.name` for an index. An index above
1000 is an error unless `WithDecMaxIndex` sets another limit, and the indexed
keys of one request, failing ones included, may add, gaps included, as many
elements as that limit plus one, and one more per key. A failing key that reset
an array spends its length from that budget too. The first key reaching a slice
replaces it, indexed keys fill it whatever their order, and keys are added to an
existing map.

A key takes one step per struct field, map key or index, and the rest of a key
a map of values takes as its key is one step. A key taking more than 32 steps is
an error unless `WithDecMaxDepth` sets another limit; it bounds the memory a
deep key into a recursive struct costs.

When a struct and the structs it embeds have fields with the same key, the
key names the field Go promotion selects, as in `encoding/json`: the
shallowest one or, among equally shallow ones, the only one whose tag names
it. A key no single field wins names no field.

Tag options:

| Option             | Effect                                            |
|--------------------|---------------------------------------------------|
| `url:"-"`          | The field is never decoded or encoded.            |
| `url:",required"`  | `Decode` reports the field when it is left empty. |
| `url:",present"`   | `Decode` reports the field when no key sends it.  |
| `url:",omitempty"` | `Encode` skips the field when it is zero.         |

A required field is empty when it is a zero number, an empty string, slice or
map, a nil pointer or a pointer to an empty value, or a struct whose fields are
all zero; a `bool` is never empty. The fields of a nil struct pointer are not
checked. `omitempty` skips a zero value, `false` and a nil pointer included, an
empty slice or map, and a struct whose `IsZero` method reports it zero. A
present field is sent when any key reaches it, whatever its value, so `present`
tells an absent `count` from `count=0` and an absent `bool` from `false`; it is
checked in the struct and in the structs keys reach.

A different tag name is set with `WithDecTag` and `WithEncTag`.

### Errors

`Decode` sets every value it can and reports the rest as field errors keyed
by the source key. The package's own errors carry a code; an error returned by
a type's `UnmarshalText` is passed through as it is:

<!-- gmmce:pkg/xbind/ExampleDecoder_Decode_errors -->
```go
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
```

Unknown keys are errors unless the decoder is built with
`NewDecoder(xbind.WithDecAllowUnknown())`. For an API response,
`xrr.Envelop(err, xbind.ErrBind)` puts the field errors under the `ErrBind`
lead error.

### Encoding values

`Encode` adds the fields of a struct, or a pointer to one, to `url.Values`
under the same keys `Decode` reads:

<!-- gmmce:pkg/xbind/ExampleEncoder_Encode -->
```go
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
```

A slice element, or a map value, that adds no keys, such as a nil pointer or a
struct whose fields are all empty and `omitempty`, is not written: such a map
entry, or such an element at the end of a slice, does not decode back, and a nil
in a slice of plain values moves the values after it up one place.

### Custom types

A field whose type implements `encoding.TextUnmarshaler` is decoded from a
single value with `UnmarshalText`. A field that also implements
`encoding.TextMarshaler` is encoded with `MarshalText`. A type that implements
only `MarshalText` is encoded by the built-in encoder, or field by field, so
the text decodes back to the same value. A type that needs more than one key
implements `Marshaler` and adds its own values under the given prefix:

```go
type Marshaler interface {
	MarshalValues(dst url.Values, prefix string) error
}
```

For a type you cannot add methods to, or to change how a type is read or
written (`time.Time` as a date only, say), register a function with
`WithDecFunc` or `WithEncFunc`. It takes precedence over the type's own
methods and over the package's codecs:

```go
dec := xbind.NewDecoder(xbind.WithDecFunc(func(s string) (time.Time, error) {
	return time.Parse(time.DateOnly, s)
}))
```

### JSON bodies

`UnmarshalJSONBody` decodes a request body and closes it; `UnmarshalJSON` and
`UnmarshalJSONReader` decode a byte slice and a reader. Unknown fields and data
after the JSON value are errors, and decoding errors are translated by `HUE`
into errors under the `ErrBind` or `xrr.ErrInvJSON` lead error. A nil or
non-pointer dst returns the `json.InvalidUnmarshalError` as is, and a nil
reader, request or body returns `ErrBind` itself:

<!-- gmmce:pkg/xbind/ExampleUnmarshalJSON -->
```go
type Body struct {
	Name string `json:"name"`
}

err := xbind.UnmarshalJSON([]byte(`{"name": "a", "age": 1}`), &Body{})

fmt.Println(errors.Is(err, xbind.ErrBind))
fmt.Println(err)
// Output:
// true
// json: unknown field "age"
```

### Testing

Package `xbindtest` wraps the default decoder and encoder in helpers that panic
on error, for building test fixtures:

```go
import "github.com/ctx42/xbind/pkg/xbind/xbindtest"
```

`xbindtest.MustEncode(src)` returns the `url.Values` of a struct, and
`xbindtest.MustDecode(dst, values)` decodes into a struct pointer.

## License

MIT; see [LICENSE.md](LICENSE.md).
