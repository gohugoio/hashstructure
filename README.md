[![Tests on Linux, MacOS and Windows](https://github.com/gohugoio/hashstructure/workflows/Test/badge.svg)](https://github.com/gohugoio/hashstructure/actions?query=workflow:Test)
[![GoDoc](https://godoc.org/github.com/gohugoio/gohugoio?status.svg)](https://godoc.org/github.com/gohugoio/hashstructure)
[![Release](https://img.shields.io/github/release/gohugoio/hashstructure.svg?style=flat-square)](https://github.com/bep/hashstructure/releases/latest)

hashstructure is a Go library for creating a unique hash value
for arbitrary values in Go.

This can be used to key values in a hash (for use in a map, set, etc.)
that are complex. The most common use case is comparing two values without
sending data across the network, caching values locally (de-dup), and so on.

## Fork Info

This is a fork of [mitchellh/hashstructure](https://github.com/mitchellh/hashstructure) (now archived and read-only), but we produce the same hashes (in `mitchellh/hashstructure` annotated as `FormatV2`)[^bug-set]

We have added some new minor features, but the most notable difference is performance:

| Benchmark | time/op | Δ | B/op | Δ | allocs/op | Δ |
|---|---|---|---|---|---|---|
| Map/default | 2.96µ → 1.96µ | −34% | 1630 → 64 | −96% | 101 → 4 | −96% |
| Struct/value | 278n → 169n | −39% | 160 → 16 | −90% | 11 → 1 | −91% |
| Struct/pointer | 277n → 147n | −47% | 160 → 0 | −100% | 11 → 0 | −100% |
| Struct/pointer predefined hasher | 269n → 138n | −49% | 152 → 0 | −100% | 10 → 0 | −100% |
| String/default | 58.2n → 38.7n | −33% | 40 → 0 | −100% | 2 → 0 | −100% |
| String/xxhash | 32.9n → 13.3n | −60% | 32 → 0 | −100% | 1 → 0 | −100% |
| **geomean** | 221n → 184n | **−44%** | | | | |

[^bug-set]: Note that if you use the `SlicesAsSets` option you may see a difference, as we have fixed the bug in [#10](https://github.com/gohugoio/hashstructure/issues/10).


## Features

  * Hash any arbitrary Go value, including complex types.

  * Tag a struct field to ignore it and not affect the hash value.

  * Tag a slice type struct field to treat it as a set where ordering
    doesn't affect the hash code but the field itself is still taken into
    account to create the hash value.

  * Optionally, specify a custom hash function to optimize for speed, collision
    avoidance for your data set, etc.

  * Optionally, hash the output of `.String()` on structs that implement fmt.Stringer,
    allowing effective hashing of time.Time

  * Optionally, override the hashing process by implementing `Hashable`.

  * Optionally, provide a `UnwrapFunc` to override the hashing process (this is often simpler than using `Hashable`)

## Installation

Standard `go get`:

```
$ go get github.com/gohugoio/hashstructure
```

## Usage & Example

For usage and examples see the [Godoc](http://godoc.org/github.com/mitchellh/hashstructure).

A quick code example is shown below:

```go
type ComplexStruct struct {
    Name     string
    Age      uint
    Metadata map[string]interface{}
}

v := ComplexStruct{
    Name: "mitchellh",
    Age:  64,
    Metadata: map[string]interface{}{
        "car":      true,
        "location": "California",
        "siblings": []string{"Bob", "John"},
    },
}

hash, err := hashstructure.Hash(v, nil)
if err != nil {
    panic(err)
}

fmt.Printf("%d", hash)
// Output:
// 2307517237273902113
```
