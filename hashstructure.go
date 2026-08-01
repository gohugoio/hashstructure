package hashstructure

import (
	"encoding/binary"
	"fmt"
	"hash"
	"hash/fnv"
	"math"
	"reflect"
	"sync"
	"time"
	"unsafe"
)

// HashOptions are options that are available for hashing.
type HashOptions struct {
	// Hasher is the hash function to use. If this isn't set, it will
	// default to FNV.
	Hasher hash.Hash64

	// TagName is the struct tag to look at when hashing the structure.
	// By default this is "hash".
	TagName string

	// ZeroNil is flag determining if nil pointer should be treated equal
	// to a zero value of pointed type. By default this is false.
	ZeroNil bool

	// IgnoreZeroValue is determining if zero value fields should be
	// ignored for hash calculation.
	IgnoreZeroValue bool

	// SlicesAsSets assumes that a `set` tag is always present for slices.
	// Default is false (in which case the tag is used instead)
	SlicesAsSets bool

	// UseStringer will attempt to use fmt.Stringer always. If the struct
	// doesn't implement fmt.Stringer, it'll fall back to trying usual tricks.
	// If this is true, and the "string" tag is also set, the tag takes
	// precedence (meaning that if the type doesn't implement fmt.Stringer, we
	// panic)
	UseStringer bool

	// UnwrapFunc is a hook to replace a value with another value before hashing,
	// e.g. to hash a struct by some identity method instead of its exported fields.
	// It is called for every value visited, including nested values (struct fields,
	// slice/array elements and map keys/values), after any pointers and interfaces
	// have been dereferenced, so the hook typically wants to switch on the value's
	// Kind before doing any work.
	// Return the input value unchanged to hash it as-is.
	// A changed value replaces the original: it is dereferenced and unwrapped
	// again, then hashed in place of the original, bypassing any other handling
	// of the original value (e.g. the Hashable interface and struct tags).
	// Note that Hash has a fast path for top-level string values that bypasses
	// this hook; a string nested inside another value still visits it. Keep
	// this in mind if the hook rewrites strings.
	UnwrapFunc func(reflect.Value) (reflect.Value, error)
}

// Hash returns the hash value of an arbitrary value.
//
// If opts is nil, then default options will be used. See HashOptions
// for the default values. Hash treats opts as read-only, so the same
// *HashOptions value can be shared and reused, also concurrently, as
// long as its Hasher is nil: a hash.Hash64 is stateful, so a
// *HashOptions carrying a custom Hasher must not be used concurrently.
//
// Notes on the value:
//
//   - Unexported fields on structs are ignored and do not affect the
//     hash value.
//
//   - Adding an exported field to a struct with the zero value will change
//     the hash value.
//
// For structs, the hashing can be controlled using tags. For example:
//
//	struct {
//	    Name string
//	    UUID string `hash:"ignore"`
//	}
//
// The available tag values are:
//
//   - "ignore" or "-" - The field will be ignored and not affect the hash code.
//
//   - "set" - The field will be treated as a set, where ordering doesn't
//     affect the hash code. This only works for slices.
//
//   - "string" - The field will be hashed as a string, only works when the
//     field implements fmt.Stringer
func Hash(v any, opts *HashOptions) (uint64, error) {
	if opts == nil {
		opts = &HashOptions{}
	}

	hasher := opts.Hasher
	if hasher == nil {
		hasher = getFnv()
		defer putFnv(hasher)
	}

	// No Reset of the hasher here: every function that touches it
	// (hashString, hashDirect etc.) resets it before writing.
	// Keep it that way if you add a new write site.

	// Fast path for strings.
	// This deliberately bypasses UnwrapFunc; see its documentation.
	if s, ok := v.(string); ok {
		return hashString(hasher, s)
	}

	w := getWalker(hasher, opts)
	defer putWalker(w)

	return w.visit(reflect.ValueOf(v), nil)
}

type walker struct {
	h               hash.Hash64
	tag             string
	zeronil         bool
	ignorezerovalue bool
	sets            bool
	stringer        bool
	unwrap          func(reflect.Value) (reflect.Value, error)
	buf             [16]byte // Reusable buffer for binary encoding
}

type visitOpts struct {
	// Flags are a bitmask of flags to affect behavior of this visit
	Flags visitFlag

	// Information about the struct containing this field
	Struct      any
	StructField string
}

var timeType = reflect.TypeFor[time.Time]()

// maxUnwraps caps the number of times UnwrapFunc may rewrite a single value,
// as a guard against hooks that never converge.
const maxUnwraps = 32

// A direct hash calculation used for numeric and bool values.
func (w *walker) hashDirect(v any) (uint64, error) {
	w.h.Reset()

	// Use direct byte manipulation for numbers instead of binary.Write to avoid allocations
	switch val := v.(type) {
	case int64:
		binary.LittleEndian.PutUint64(w.buf[:8], uint64(val))
		w.h.Write(w.buf[:8])
	case uint64:
		binary.LittleEndian.PutUint64(w.buf[:8], val)
		w.h.Write(w.buf[:8])
	case int8:
		w.buf[0] = byte(val)
		w.h.Write(w.buf[:1])
	case uint8:
		w.buf[0] = val
		w.h.Write(w.buf[:1])
	case int16:
		binary.LittleEndian.PutUint16(w.buf[:2], uint16(val))
		w.h.Write(w.buf[:2])
	case uint16:
		binary.LittleEndian.PutUint16(w.buf[:2], val)
		w.h.Write(w.buf[:2])
	case int32:
		binary.LittleEndian.PutUint32(w.buf[:4], uint32(val))
		w.h.Write(w.buf[:4])
	case uint32:
		binary.LittleEndian.PutUint32(w.buf[:4], val)
		w.h.Write(w.buf[:4])
	case float32:
		binary.LittleEndian.PutUint32(w.buf[:4], math.Float32bits(val))
		w.h.Write(w.buf[:4])
	case float64:
		binary.LittleEndian.PutUint64(w.buf[:8], math.Float64bits(val))
		w.h.Write(w.buf[:8])
	case complex64:
		binary.LittleEndian.PutUint32(w.buf[:4], math.Float32bits(real(val)))
		binary.LittleEndian.PutUint32(w.buf[4:8], math.Float32bits(imag(val)))
		w.h.Write(w.buf[:8])
	case complex128:
		binary.LittleEndian.PutUint64(w.buf[:8], math.Float64bits(real(val)))
		binary.LittleEndian.PutUint64(w.buf[8:16], math.Float64bits(imag(val)))
		w.h.Write(w.buf[:16])
	default:
		// Fallback to binary.Write for unsupported types, for instance enums
		err := binary.Write(w.h, binary.LittleEndian, v)
		return w.h.Sum64(), err
	}

	return w.h.Sum64(), nil
}

// A direct hash calculation used for strings.
func (w *walker) hashString(s string) (uint64, error) {
	return hashString(w.h, s)
}

// A direct hash calculation used for strings.
func hashString(h hash.Hash64, s string) (uint64, error) {
	h.Reset()

	// Use zero-copy conversion from string to []byte using unsafe
	if len(s) > 0 {
		b := unsafe.Slice(unsafe.StringData(s), len(s))
		h.Write(b)
	}
	return h.Sum64(), nil
}

func (w *walker) visit(v reflect.Value, opts *visitOpts) (uint64, error) {
	t := reflect.TypeFor[int]()

	// Loop since these can be wrapped in multiple layers of pointers
	// and interfaces. An unwrapped value re-enters the loop, as it may
	// itself be wrapped or subject to further unwrapping.
	for unwraps := 0; ; {
		// If we have an interface, dereference it. We have to do this up
		// here because it might be a nil in there and the check below must
		// catch that.
		if v.Kind() == reflect.Interface {
			v = v.Elem()
			continue
		}

		if v.Kind() == reflect.Ptr {
			if w.zeronil {
				t = v.Type().Elem()
			}
			v = reflect.Indirect(v)
			continue
		}

		// If it is nil, treat it like a zero.
		if !v.IsValid() {
			v = reflect.Zero(t)
		}

		if w.unwrap != nil {
			vv, err := w.unwrap(v)
			if err != nil {
				return 0, err
			}
			if vv != v {
				unwraps++
				if unwraps > maxUnwraps {
					return 0, fmt.Errorf("unwrapped value %s more than %d times; check that UnwrapFunc returns its input unchanged when there is nothing to unwrap", v.Type(), maxUnwraps)
				}
				v = vv
				continue
			}
		}

		break
	}

	if v.CanInt() {
		i := v.Int()
		switch v.Kind() {
		case reflect.Int:
			return w.hashDirect(i)
		case reflect.Int8:
			return w.hashDirect(int8(i))
		case reflect.Int16:
			return w.hashDirect(int16(i))
		case reflect.Int32:
			return w.hashDirect(int32(i))
		case reflect.Int64:
			return w.hashDirect(i)
		}
	}

	if v.CanUint() {
		u := v.Uint()
		switch v.Kind() {
		case reflect.Uint:
			return w.hashDirect(u)
		case reflect.Uint8:
			return w.hashDirect(uint8(u))
		case reflect.Uint16:
			return w.hashDirect(uint16(u))
		case reflect.Uint32:
			return w.hashDirect(uint32(u))
		case reflect.Uint64:
			return w.hashDirect(u)
		}
	}

	if v.CanFloat() {
		f := v.Float()
		switch v.Kind() {
		case reflect.Float32:
			return w.hashDirect(float32(f))
		case reflect.Float64:
			return w.hashDirect(f)
		}
	}

	if v.CanComplex() {
		c := v.Complex()
		switch v.Kind() {
		case reflect.Complex64:
			return w.hashDirect(complex64(c))
		case reflect.Complex128:
			return w.hashDirect(c)
		}
	}

	k := v.Kind()

	if k == reflect.Bool {
		var tmp int8
		if v.Bool() {
			tmp = 1
		}
		return w.hashDirect(tmp)
	}

	switch v.Type() {
	case timeType:
		w.h.Reset()
		b, err := v.Interface().(time.Time).MarshalBinary()
		if err != nil {
			return 0, err
		}

		w.h.Write(b)
		return w.h.Sum64(), nil
	}

	switch k {
	case reflect.Array:
		var h uint64
		l := v.Len()
		for i := range l {
			current, err := w.visit(v.Index(i), nil)
			if err != nil {
				return 0, err
			}

			h = w.hashUpdateOrdered(h, current)
		}

		return h, nil

	case reflect.Map:
		var includeMap IncludableMap
		var field string

		if v, ok := v.Interface().(IncludableMap); ok {
			includeMap = v
		} else if opts != nil && opts.Struct != nil {
			if v, ok := opts.Struct.(IncludableMap); ok {
				includeMap, field = v, opts.StructField
			}
		}

		// Build the hash for the map. We do this by XOR-ing all the key
		// and value hashes. This makes it deterministic despite ordering.
		var h uint64

		k := reflect.New(v.Type().Key()).Elem()
		vv := reflect.New(v.Type().Elem()).Elem()

		iter := v.MapRange()

		for iter.Next() {
			k.SetIterKey(iter)
			vv.SetIterValue(iter)
			if includeMap != nil {
				incl, err := includeMap.HashIncludeMap(field, k.Interface(), vv.Interface())
				if err != nil {
					return 0, err
				}
				if !incl {
					continue
				}
			}

			kh, err := w.visit(k, nil)
			if err != nil {
				return 0, err
			}
			vh, err := w.visit(vv, nil)
			if err != nil {
				return 0, err
			}

			fieldHash := w.hashUpdateOrdered(kh, vh)
			h = w.hashUpdateUnordered(h, fieldHash)
		}

		// Important: read the docs for hashFinishUnordered
		h = w.hashFinishUnordered(h)

		return h, nil

	case reflect.Struct:
		var include Includable
		var parent any

		// Check if we can address this value first (more common case for pointer receivers)
		if v.CanAddr() {
			vptr := v.Addr()
			parentptr := vptr.Interface()
			if impl, ok := parentptr.(Includable); ok {
				include = impl
			}

			if impl, ok := parentptr.(Hashable); ok {
				return impl.Hash()
			}
			// Only set parent if we'll need it for IncludableMap
			parent = parentptr
		}

		// Only box the value if we haven't already found an implementation via pointer
		if include == nil && parent == nil {
			parent = v.Interface()
			if impl, ok := parent.(Includable); ok {
				include = impl
			}

			if impl, ok := parent.(Hashable); ok {
				return impl.Hash()
			}
		}

		t := v.Type()
		h, err := w.hashString(t.Name())
		if err != nil {
			return 0, err
		}

		l := v.NumField()
		var fieldOpts visitOpts
		// Defer boxing parent until we know we need it
		if parent == nil {
			parent = v.Interface()
		}
		fieldOpts.Struct = parent

		for i := range l {
			if innerV := v.Field(i); v.CanSet() || t.Field(i).Name != "_" {
				fieldType := t.Field(i)
				if fieldType.PkgPath != "" {
					// Unexported
					continue
				}

				tag := fieldType.Tag.Get(w.tag)
				if tag == "ignore" || tag == "-" {
					// Ignore this field
					continue
				}

				if w.ignorezerovalue {
					if innerV.IsZero() {
						continue
					}
				}

				// if string is set, use the string value
				if tag == "string" || w.stringer {
					if impl, ok := innerV.Interface().(fmt.Stringer); ok {
						innerV = reflect.ValueOf(impl.String())
					} else if tag == "string" {
						// We only show this error if the tag explicitly
						// requests a stringer.
						return 0, &ErrNotStringer{
							Field: v.Type().Field(i).Name,
						}
					}
				}

				// Check if we implement includable and check it
				if include != nil {
					incl, err := include.HashInclude(fieldType.Name, innerV)
					if err != nil {
						return 0, err
					}
					if !incl {
						continue
					}
				}

				fieldOpts.Flags = 0
				if tag == "set" {
					fieldOpts.Flags |= visitFlagSet
				}

				kh, err := w.hashString(fieldType.Name)
				if err != nil {
					return 0, err
				}

				fieldOpts.StructField = fieldType.Name
				vh, err := w.visit(innerV, &fieldOpts)
				if err != nil {
					return 0, err
				}

				fieldHash := w.hashUpdateOrdered(kh, vh)
				h = w.hashUpdateUnordered(h, fieldHash)
			}
			// Important: read the docs for hashFinishUnordered
			h = w.hashFinishUnordered(h)
		}

		return h, nil

	case reflect.Slice:
		// We have two behaviors here. If it isn't a set, then we just
		// visit all the elements. If it is a set, then we do a deterministic
		// hash code.
		var h uint64
		set := w.sets
		if opts != nil {
			set = set || (opts.Flags&visitFlagSet) != 0
		}
		l := v.Len()
		// In set mode, duplicate element hashes must be skipped: the hashes are
		// combined with XOR, so an element repeated an even number of times
		// cancels itself out, making e.g. {a, e, e} and {a, d, d} collide.
		var seen map[uint64]struct{}
		if set && l > 1 {
			seen = make(map[uint64]struct{}, l)
		}
		for i := range l {
			current, err := w.visit(v.Index(i), nil)
			if err != nil {
				return 0, err
			}

			if set {
				if seen != nil {
					if _, ok := seen[current]; ok {
						continue
					}
					seen[current] = struct{}{}
				}
				h = w.hashUpdateUnordered(h, current)
			} else {
				h = w.hashUpdateOrdered(h, current)
			}
		}

		if set {
			// Important: read the docs for hashFinishUnordered
			h = w.hashFinishUnordered(h)
		}

		return h, nil

	case reflect.String:
		return w.hashString(v.String())
	default:
		return 0, fmt.Errorf("unknown kind to hash: %s", k)
	}
}

func (w *walker) hashUpdateOrdered(a, b uint64) uint64 {
	// For ordered updates, use a real hash function
	w.h.Reset()

	binary.LittleEndian.PutUint64(w.buf[0:8], a)
	binary.LittleEndian.PutUint64(w.buf[8:16], b)
	w.h.Write(w.buf[:])

	return w.h.Sum64()
}

func (w *walker) hashUpdateUnordered(a, b uint64) uint64 {
	return a ^ b
}

// After mixing a group of unique hashes with hashUpdateUnordered, it's always
// necessary to call hashFinishUnordered. Why? Because hashUpdateUnordered
// is a simple XOR, and calling hashUpdateUnordered on hashes produced by
// hashUpdateUnordered can effectively cancel out a previous change to the hash
// result if the same hash value appears later on. For example, consider:
//
//	hashUpdateUnordered(hashUpdateUnordered("A", "B"), hashUpdateUnordered("A", "C")) =
//	H("A") ^ H("B")) ^ (H("A") ^ H("C")) =
//	(H("A") ^ H("A")) ^ (H("B") ^ H(C)) =
//	H(B) ^ H(C) =
//	hashUpdateUnordered(hashUpdateUnordered("Z", "B"), hashUpdateUnordered("Z", "C"))
//
// hashFinishUnordered "hardens" the result, so that encountering partially
// overlapping input data later on in a different context won't cancel out.
func (w *walker) hashFinishUnordered(a uint64) uint64 {
	w.h.Reset()

	binary.LittleEndian.PutUint64(w.buf[:8], a)
	w.h.Write(w.buf[:8])

	return w.h.Sum64()
}

// visitFlag is used as a bitmask for affecting visit behavior
type visitFlag uint

const (
	visitFlagInvalid visitFlag = iota
	visitFlagSet               = iota << 1
)

var walkerPool = sync.Pool{
	New: func() any {
		return &walker{}
	},
}

func getWalker(h hash.Hash64, opts *HashOptions) *walker {
	w := walkerPool.Get().(*walker)
	w.h = h
	w.tag = opts.TagName
	if w.tag == "" {
		w.tag = "hash"
	}
	w.zeronil = opts.ZeroNil
	w.ignorezerovalue = opts.IgnoreZeroValue
	w.sets = opts.SlicesAsSets
	w.stringer = opts.UseStringer
	w.unwrap = opts.UnwrapFunc
	return w
}

func putWalker(w *walker) {
	w.h = nil
	w.tag = ""
	w.zeronil = false
	w.ignorezerovalue = false
	w.sets = false
	w.stringer = false
	w.unwrap = nil

	walkerPool.Put(w)
}

var fnvPool = sync.Pool{
	New: func() any {
		return fnv.New64()
	},
}

func getFnv() hash.Hash64 {
	return fnvPool.Get().(hash.Hash64)
}

func putFnv(h hash.Hash64) {
	// It will be reset before it's used again.
	fnvPool.Put(h)
}
