# AGENTS.md

This file provides guidance to coding agents when working with code in this
repository.

## Overview

Module `github.com/ctx42/xbind` is a Go library binding `url.Values` (query
strings, form bodies) to structs and back, and decoding JSON request bodies
strictly. There is no executable. It has two packages:

- `pkg/xbind` — `Decoder`, `Encoder`, the JSON helpers, and errors.
- `pkg/xbind/xbindtest` — `MustDecode` and `MustEncode`, which panic on
  error.

Dependencies are limited to ctx42 modules: `mirror` and `xrr`, `testing` in
tests and in `xbindtest`, and `testkit` in tests. Do not add third-party
modules.

`internal/bench` is a separate module (own `go.mod`, `replace` to `../..`)
benchmarking xbind against other form libraries; third-party modules belong
there only. Root `./...` commands skip it; its package doc says how to run it
and `report.go` turns the output into tables.

`internal/difftest` is another such module: it decodes and encodes random
inputs with xbind and with `ref`, a copy of `pkg/xbind` from before the
decode and encode shortcuts, and fails on any difference in values or
errors. CI runs it. A behavior change made on purpose goes into `ref` too.

## Commands

```bash
go test -count=1 -race ./...                       # run all tests
go test -count=1 -run Test_Decoder_Decode ./pkg/xbind/       # one test
go test -count=1 -run 'Test_Decoder_Decode/struct' ./pkg/xbind/  # subtest
GOTOOLCHAIN=go1.26.0 go test -count=1 ./...        # oldest supported Go
go vet ./...                                       # vet
golangci-lint run ./...                            # lint
go test ./pkg/xbind/ -run XXX -fuzz '^Fuzz_Decoder_Decode$' -fuzztime 60s
gomake :doc:mce                                    # re-inject README examples
(cd internal/difftest && go test -count=1 ./...)   # differential test
```

`go.mod` says `go 1.26`; the code must pass on Go 1.26 and on the newest
release (CI runs both). `encoding/json` error shapes differ between them, see
`MaybeHUE`.

## Architecture

All source is in `pkg/xbind/`.

- **`xbind.go`** — package doc, error codes and sentinels, `Marshaler`,
  `checkTags` (rejects tags that do not parse, run first by `Decode` and
  `Encode`), and `level`/`newLevel` (which field each key names, from
  `mirror.Names`).
- **`cache.go`** — `typeCache`, the per-instance cache `NewDecoder` and
  `NewEncoder` create (a zero value has none and computes per call): type
  facts, tag checks, levels, the decoder's key fields and the encoder's field
  plans, in copy-on-write `typeMap`s read without locks.
- **`decoder.go`** — `Decoder`, its options (`WithDecTag`,
  `WithDecAllowUnknown`, `WithDecMaxIndex`, `WithDecMaxDepth`, `WithDecFunc`).
  `decode` sorts the src keys, splits each into segments (`parseKey`), turns
  them into steps by type without side effects (`plan`), performs the steps
  on `dst` (`apply`, which allocates, writes map values back and resets
  slices once per call), then runs the `required` walk (`required`,
  `requiredIn`) over `dst` when the type has such fields. The per-call
  `state` comes from a pool in the `Decoder`; a canonical key (the path of
  its value) records its path only when another key could alias it
  (`setAt`).
- **`decoders.go`** — built-in decoders for base kinds, `time.Time` and
  `time.Duration`; `decFor` looks in the registry first.
- **`shortcuts.go`** — the decode shortcuts. When every key is a canonical
  path of struct fields, `decodeFast` decodes field by field without
  sorting; otherwise a plain key naming a leaf field skips planning
  (`direct`), and so does a canonical entry of a map of built-in values
  (`directMap`). They must give exactly what the key-driven path gives,
  which `internal/difftest` checks.
- **`encoder.go`** — `Encoder`, its options (`WithEncTag`, `WithEncFunc`).
  `encode` walks values (`encodeFields`, `encodeSlice`, `encodeMap`);
  `encSrc` encodes one value: registered function, then `Marshaler`, then
  `TextMarshaler`, then `encFor`. Field plans (`fields`, holding the plans
  of nested structs), plain value types (`plain`, `time.Time` included),
  slice plans and composite types (`isComposite`) skip `encSrc` where it
  would find nothing; a map that cannot fail is encoded in map order
  (`encodeMapSure`); cycles are tracked only for types that can loop
  (`canLoop`), and `src` is copied only when a method needs an address
  (`needsAddr`).
- **`encoders.go`** — built-in encoders and `encReg.lookup`.
- **`json.go`** — `UnmarshalJSON`, `UnmarshalJSONReader`,
  `UnmarshalJSONBody`, and `HUE`/`MaybeHUE`, which translate `encoding/json`
  errors to xrr errors; `HUEFor` first drops the embedded struct names
  encoding/json puts in a type error's field path (`jsonPath`).
- **`helpers.go`** — type-agnostic helpers: `isZero` (`omitempty`),
  `isEmpty` (`required`), `buildPath`, `indirect`, and the like.

## Invariants

- Key syntax: struct field names after dots, map keys and slice or array
  indexes in brackets (`page.size`, `attr[k]`, `list[0].name`); a bracketed
  segment may contain dots. The encoder writes exactly what the decoder reads;
  the decoder also accepts `attr.a.b` for a map of values and `list.0`.
- `Encode` output decodes back with `Decode` to the same value; the fuzz
  tests in `decoder_test.go` and `encoder_test.go` check it. Keep the two
  sides symmetric.
- Keys follow Go's promotion rules, as in `encoding/json`: `newLevel`
  (`mirror.Names`) picks the shallowest field per key, a tagged one among
  equals, or none. The
  decoder, the `required` walk and the encoder must all go through it.
- `required` fails a field that is empty after decoding. `isEmpty` copies the
  rules of `IsEmpty` in `github.com/ctx42/verax`; keep them in step.
- `present` fails a field no src key reached (`state.reached`, failing keys
  included), checked in dst and in the structs keys reached.
- A nil nested struct pointer is allocated only when a key leads into it; the
  fields of a nil one are not checked for `required`.
- `TextMarshaler` and `WithEncFunc` functions apply only to values with a
  key; the src struct and the structs it embeds are encoded field by field.
  `TextMarshaler` is used only when the type also implements
  `TextUnmarshaler`.
- Errors are xrr sentinels matched with `errors.Is`; each code's value equals
  its const name.

## Conventions

- Tests use `github.com/ctx42/testing` (`assert`, `must`) and
  `github.com/ctx42/xrr/pkg/xrr/xrrtest`, with
  `// --- Given ---` / `// --- When ---` / `// --- Then ---` blocks and
  `have`/`want` names.
- Shared test fixtures live in `all_types_test.go`, shared helpers in
  `all_test.go`. A struct tag that does not parse is built at run time
  (`badTag`), so `go vet` stays clean.
- Runnable examples live in `pkg/xbind/example_test.go`; the README embeds
  them through `gmmce` markers, so re-run `gomake :doc:mce` after editing one.
- Line width is 80 columns, tab width 4 (`.editorconfig`).
