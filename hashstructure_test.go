package hashstructure

import (
	"errors"
	"fmt"
	"hash/fnv"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestHash_identity(t *testing.T) {
	cases := []any{
		nil,
		"foo",
		42,
		true,
		false,
		[]string{"foo", "bar"},
		[]any{1, nil, "foo"},
		map[string]string{"foo": "bar"},
		map[any]string{"foo": "bar"},
		map[any]any{"foo": "bar", "bar": 0},
		struct {
			Foo string
			Bar []any
		}{
			Foo: "foo",
			Bar: []any{nil, nil, nil},
		},
		&struct {
			Foo string
			Bar []any
		}{
			Foo: "foo",
			Bar: []any{nil, nil, nil},
		},
	}

	for _, tc := range cases {
		// We run the test 100 times to try to tease out variability
		// in the runtime in terms of ordering.
		valuelist := make([]uint64, 100)
		for i := range valuelist {
			v, err := Hash(tc, nil)
			if err != nil {
				t.Fatalf("unexpected error: %v for %#v", err, tc)
			}

			valuelist[i] = v
		}

		// Zero is always wrong
		if valuelist[0] == 0 {
			t.Fatalf("zero hash: %#v", tc)
		}

		// Make sure all the values match
		t.Logf("%#v: %d", tc, valuelist[0])
		for i := 1; i < len(valuelist); i++ {
			if valuelist[i] != valuelist[0] {
				t.Fatalf("non-matching hashes %d and %d: %#v", valuelist[i], valuelist[0], tc)
			}
		}
	}
}

func TestHash_equal(t *testing.T) {
	type testFoo struct{ Name string }
	type testBar struct{ Name string }

	now := time.Now()

	cases := []struct {
		One, Two any
		Match    bool
	}{
		{
			map[string]string{"foo": "bar"},
			map[any]string{"foo": "bar"},
			true,
		},

		{
			map[string]any{"1": "1"},
			map[string]any{"1": "1", "2": "2"},
			false,
		},

		{
			struct{ Fname, Lname string }{"foo", "bar"},
			struct{ Fname, Lname string }{"bar", "foo"},
			false,
		},

		{
			struct{ Lname, Fname string }{"foo", "bar"},
			struct{ Fname, Lname string }{"foo", "bar"},
			false,
		},

		{
			struct{ Lname, Fname string }{"foo", "bar"},
			struct{ Fname, Lname string }{"bar", "foo"},
			false,
		},

		{
			testFoo{"foo"},
			testBar{"foo"},
			false,
		},

		{
			struct {
				Foo        string
				unexported string
			}{
				Foo:        "bar",
				unexported: "baz",
			},
			struct {
				Foo        string
				unexported string
			}{
				Foo:        "bar",
				unexported: "bang",
			},
			true,
		},

		{
			struct {
				testFoo
				Foo string
			}{
				Foo:     "bar",
				testFoo: testFoo{Name: "baz"},
			},
			struct {
				testFoo
				Foo string
			}{
				Foo: "bar",
			},
			true,
		},

		{
			struct {
				Foo string
			}{
				Foo: "bar",
			},
			struct {
				testFoo
				Foo string
			}{
				Foo: "bar",
			},
			true,
		},
		{
			now, // contains monotonic clock
			time.Date(now.Year(), now.Month(), now.Day(), now.Hour(),
				now.Minute(), now.Second(), now.Nanosecond(), now.Location()), // does not contain monotonic clock
			true,
		},
	}

	for i, tc := range cases {
		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
			t.Logf("Hashing: %#v", tc.One)
			one, err := Hash(tc.One, nil)
			t.Logf("Result: %d", one)
			if err != nil {
				t.Fatalf("unexpected error: %v for %#v", err, tc.One)
			}
			t.Logf("Hashing: %#v", tc.Two)
			two, err := Hash(tc.Two, nil)
			t.Logf("Result: %d", two)
			if err != nil {
				t.Fatalf("unexpected error: %v for %#v", err, tc.Two)
			}

			// Zero is always wrong
			if one == 0 {
				t.Fatalf("zero hash: %#v", tc.One)
			}

			// Compare
			if (one == two) != tc.Match {
				t.Fatalf("expected match=%v: %#v\n\n%#v", tc.Match, tc.One, tc.Two)
			}
		})
	}
}

func TestHash_equalIgnore(t *testing.T) {
	type Test1 struct {
		Name string
		UUID string `hash:"ignore"`
	}

	type Test2 struct {
		Name string
		UUID string `hash:"-"`
	}

	type TestTime struct {
		Name string
		Time time.Time `hash:"string"`
	}

	type TestTime2 struct {
		Name string
		Time time.Time
	}

	now := time.Now()
	cases := []struct {
		One, Two any
		Match    bool
	}{
		{
			Test1{Name: "foo", UUID: "foo"},
			Test1{Name: "foo", UUID: "bar"},
			true,
		},

		{
			Test1{Name: "foo", UUID: "foo"},
			Test1{Name: "foo", UUID: "foo"},
			true,
		},

		{
			Test2{Name: "foo", UUID: "foo"},
			Test2{Name: "foo", UUID: "bar"},
			true,
		},

		{
			Test2{Name: "foo", UUID: "foo"},
			Test2{Name: "foo", UUID: "foo"},
			true,
		},
		{
			TestTime{Name: "foo", Time: now},
			TestTime{Name: "foo", Time: time.Time{}},
			false,
		},
		{
			TestTime{Name: "foo", Time: now},
			TestTime{Name: "foo", Time: now},
			true,
		},
		{
			TestTime2{Name: "foo", Time: now},
			TestTime2{Name: "foo", Time: time.Time{}},
			false,
		},
		{
			TestTime2{Name: "foo", Time: now},
			TestTime2{
				Name: "foo", Time: time.Date(now.Year(), now.Month(), now.Day(), now.Hour(),
					now.Minute(), now.Second(), now.Nanosecond(), now.Location()),
			},
			true,
		},
		{
			int16(-32768), uint16(32768), true,
		},
	}

	for _, tc := range cases {
		one, err := Hash(tc.One, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v for %#v", err, tc.One)
		}
		two, err := Hash(tc.Two, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v for %#v", err, tc.Two)
		}

		// Zero is always wrong
		if one == 0 {
			t.Fatalf("zero hash: %#v", tc.One)
		}

		// Compare
		if (one == two) != tc.Match {
			t.Fatalf("expected match=%v: %#v\n\n%#v", tc.Match, tc.One, tc.Two)
		}
	}
}

func TestHash_stringTagError(t *testing.T) {
	type Test1 struct {
		Name        string
		BrokenField string `hash:"string"`
	}

	type Test2 struct {
		Name        string
		BustedField int `hash:"string"`
	}

	type Test3 struct {
		Name string
		Time time.Time `hash:"string"`
	}

	cases := []struct {
		Test  any
		Field string
	}{
		{
			Test1{Name: "foo", BrokenField: "bar"},
			"BrokenField",
		},
		{
			Test2{Name: "foo", BustedField: 23},
			"BustedField",
		},
		{
			Test3{Name: "foo", Time: time.Now()},
			"",
		},
	}

	for _, tc := range cases {
		_, err := Hash(tc.Test, nil)
		if err != nil {
			var ens *ErrNotStringer
			if !errors.As(err, &ens) {
				t.Fatalf("expected ErrNotStringer, got %T (%v) for %#v", err, err, tc)
			}
			if ens.Field != tc.Field {
				t.Fatalf("expected field %q, got %q for %#v", tc.Field, ens.Field, tc.Test)
			}
		}
	}
}

func TestHash_equalNil(t *testing.T) {
	type Test struct {
		Str   *string
		Int   *int
		Map   map[string]string
		Slice []string
	}

	cases := []struct {
		One, Two any
		ZeroNil  bool
		Match    bool
	}{
		{
			Test{
				Str:   nil,
				Int:   nil,
				Map:   nil,
				Slice: nil,
			},
			Test{
				Str:   new(string),
				Int:   new(int),
				Map:   make(map[string]string),
				Slice: make([]string, 0),
			},
			true,
			true,
		},
		{
			Test{
				Str:   nil,
				Int:   nil,
				Map:   nil,
				Slice: nil,
			},
			Test{
				Str:   new(string),
				Int:   new(int),
				Map:   make(map[string]string),
				Slice: make([]string, 0),
			},
			false,
			false,
		},
		{
			nil,
			0,
			true,
			true,
		},
		{
			nil,
			0,
			false,
			true,
		},
	}

	for _, tc := range cases {
		one, err := Hash(tc.One, &HashOptions{ZeroNil: tc.ZeroNil})
		if err != nil {
			t.Fatalf("unexpected error: %v for %#v", err, tc.One)
		}
		two, err := Hash(tc.Two, &HashOptions{ZeroNil: tc.ZeroNil})
		if err != nil {
			t.Fatalf("unexpected error: %v for %#v", err, tc.Two)
		}

		// Zero is always wrong
		if one == 0 {
			t.Fatalf("zero hash: %#v", tc.One)
		}

		// Compare
		if (one == two) != tc.Match {
			t.Fatalf("expected match=%v: %#v\n\n%#v", tc.Match, tc.One, tc.Two)
		}
	}
}

func TestHash_equalSet(t *testing.T) {
	type Test struct {
		Name    string
		Friends []string `hash:"set"`
	}

	cases := []struct {
		One, Two any
		Match    bool
	}{
		{
			Test{Name: "foo", Friends: []string{"foo", "bar"}},
			Test{Name: "foo", Friends: []string{"bar", "foo"}},
			true,
		},

		{
			Test{Name: "foo", Friends: []string{"foo", "bar"}},
			Test{Name: "foo", Friends: []string{"foo", "bar"}},
			true,
		},
	}

	for _, tc := range cases {
		one, err := Hash(tc.One, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v for %#v", err, tc.One)
		}
		two, err := Hash(tc.Two, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v for %#v", err, tc.Two)
		}

		// Zero is always wrong
		if one == 0 {
			t.Fatalf("zero hash: %#v", tc.One)
		}

		// Compare
		if (one == two) != tc.Match {
			t.Fatalf("expected match=%v: %#v\n\n%#v", tc.Match, tc.One, tc.Two)
		}
	}
}

// Issue #10: two slices that differ only by an item repeated in both
// must not hash to the same value.
func TestHash_setWithDuplicates(t *testing.T) {
	type Test struct {
		Friends []string `hash:"set"`
	}

	cases := []struct {
		One, Two any
		Match    bool
	}{
		{
			[]string{"a", "b", "c", "e", "e"},
			[]string{"a", "b", "c", "d", "d"},
			false,
		},

		{
			Test{Friends: []string{"a", "b", "c", "e", "e"}},
			Test{Friends: []string{"a", "b", "c", "d", "d"}},
			false,
		},

		// Duplicates don't count in a set, and order doesn't matter.
		{
			[]string{"e", "a", "b", "c", "e"},
			[]string{"a", "b", "c", "e"},
			true,
		},

		{
			Test{Friends: []string{"e", "a", "b", "c", "e"}},
			Test{Friends: []string{"a", "b", "c", "e"}},
			true,
		},
	}

	for _, tc := range cases {
		opts := &HashOptions{SlicesAsSets: true}
		if _, ok := tc.One.(Test); ok {
			opts = nil
		}
		one, err := Hash(tc.One, opts)
		if err != nil {
			t.Fatalf("unexpected error: %v for %#v", err, tc.One)
		}
		two, err := Hash(tc.Two, opts)
		if err != nil {
			t.Fatalf("unexpected error: %v for %#v", err, tc.Two)
		}

		// Zero is always wrong
		if one == 0 {
			t.Fatalf("zero hash: %#v", tc.One)
		}

		// Compare
		if (one == two) != tc.Match {
			t.Fatalf("expected match=%v: %#v\n\n%#v", tc.Match, tc.One, tc.Two)
		}
	}
}

func TestHash_includable(t *testing.T) {
	cases := []struct {
		One, Two any
		Match    bool
	}{
		{
			testIncludable{Value: "foo"},
			testIncludable{Value: "foo"},
			true,
		},

		{
			testIncludable{Value: "foo", Ignore: "bar"},
			testIncludable{Value: "foo"},
			true,
		},

		{
			testIncludable{Value: "foo", Ignore: "bar"},
			testIncludable{Value: "bar"},
			false,
		},
	}

	for _, tc := range cases {
		one, err := Hash(tc.One, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v for %#v", err, tc.One)
		}
		two, err := Hash(tc.Two, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v for %#v", err, tc.Two)
		}

		// Zero is always wrong
		if one == 0 {
			t.Fatalf("zero hash: %#v", tc.One)
		}

		// Compare
		if (one == two) != tc.Match {
			t.Fatalf("expected match=%v: %#v\n\n%#v", tc.Match, tc.One, tc.Two)
		}
	}
}

func TestHash_ignoreZeroValue(t *testing.T) {
	cases := []struct {
		IgnoreZeroValue bool
	}{
		{
			IgnoreZeroValue: true,
		},
		{
			IgnoreZeroValue: false,
		},
	}
	structA := struct {
		Foo string
		Bar string
		Map map[string]int
	}{
		Foo: "foo",
		Bar: "bar",
	}
	structB := struct {
		Foo string
		Bar string
		Baz string
		Map map[string]int
	}{
		Foo: "foo",
		Bar: "bar",
	}

	for _, tc := range cases {
		hashA, err := Hash(structA, &HashOptions{IgnoreZeroValue: tc.IgnoreZeroValue})
		if err != nil {
			t.Fatalf("unexpected error: %v for %#v", err, structA)
		}
		hashB, err := Hash(structB, &HashOptions{IgnoreZeroValue: tc.IgnoreZeroValue})
		if err != nil {
			t.Fatalf("unexpected error: %v for %#v", err, structB)
		}
		if (hashA == hashB) != tc.IgnoreZeroValue {
			t.Fatalf("expected match=%v: %d\n\n%d", tc.IgnoreZeroValue, hashA, hashB)
		}
	}
}

func TestHash_includableMap(t *testing.T) {
	cases := []struct {
		One, Two any
		Match    bool
	}{
		{
			testIncludableMap{Map: map[string]string{"foo": "bar"}},
			testIncludableMap{Map: map[string]string{"foo": "bar"}},
			true,
		},

		{
			testIncludableMap{Map: map[string]string{"foo": "bar", "ignore": "true"}},
			testIncludableMap{Map: map[string]string{"foo": "bar"}},
			true,
		},

		{
			testIncludableMap{Map: map[string]string{"foo": "bar", "ignore": "true"}},
			testIncludableMap{Map: map[string]string{"bar": "baz"}},
			false,
		},
		{
			testIncludableMapMap{"foo": "bar"},
			testIncludableMapMap{"foo": "bar"},
			true,
		},

		{
			testIncludableMapMap{"foo": "bar", "ignore": "true"},
			testIncludableMapMap{"foo": "bar"},
			true,
		},

		{
			testIncludableMapMap{"foo": "bar", "ignore": "true"},
			testIncludableMapMap{"bar": "baz"},
			false,
		},
	}

	for _, tc := range cases {
		one, err := Hash(tc.One, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v for %#v", err, tc.One)
		}
		two, err := Hash(tc.Two, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v for %#v", err, tc.Two)
		}

		// Zero is always wrong
		if one == 0 {
			t.Fatalf("zero hash: %#v", tc.One)
		}

		// Compare
		if (one == two) != tc.Match {
			t.Fatalf("expected match=%v: %#v\n\n%#v", tc.Match, tc.One, tc.Two)
		}
	}
}

func TestHash_hashable(t *testing.T) {
	cases := []struct {
		One, Two any
		Match    bool
		Err      string
	}{
		{
			testHashable{Value: "foo"},
			&testHashablePointer{Value: "foo"},
			true,
			"",
		},

		{
			testHashable{Value: "foo1"},
			&testHashablePointer{Value: "foo2"},
			true,
			"",
		},
		{
			testHashable{Value: "foo"},
			&testHashablePointer{Value: "bar"},
			false,
			"",
		},
		{
			testHashable{Value: "nofoo"},
			&testHashablePointer{Value: "bar"},
			true,
			"",
		},
		{
			testHashable{Value: "bar", Err: fmt.Errorf("oh no")},
			testHashable{Value: "bar"},
			true,
			"oh no",
		},
	}

	for i, tc := range cases {
		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
			one, err := Hash(tc.One, nil)
			if tc.Err != "" {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if !strings.Contains(err.Error(), tc.Err) {
					t.Fatalf("expected error containing %q, got %q", tc.Err, err.Error())
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v for %#v", err, tc.One)
			}

			two, err := Hash(tc.Two, nil)
			if err != nil {
				t.Fatalf("unexpected error: %v for %#v", err, tc.Two)
			}

			// Zero is always wrong
			if one == 0 {
				t.Fatalf("zero hash: %#v", tc.One)
			}

			// Compare
			if (one == two) != tc.Match {
				t.Fatalf("expected match=%v: %#v\n\n%#v", tc.Match, tc.One, tc.Two)
			}
		})
	}
}

func TestHash_unwrapFunc(t *testing.T) {
	// Unwraps values with a Key() string method to the key, the common case
	// being structs whose identity lives in unexported fields.
	keyOpts := func() *HashOptions {
		return &HashOptions{
			UnwrapFunc: func(v reflect.Value) (reflect.Value, error) {
				if v.Kind() != reflect.Struct {
					return v, nil
				}
				var in any
				if v.CanAddr() {
					in = v.Addr().Interface()
				} else {
					in = v.Interface()
				}
				if k, ok := in.(interface{ Key() string }); ok {
					return reflect.ValueOf(k.Key()), nil
				}
				return v, nil
			},
		}
	}

	hash := func(t *testing.T, v any, opts *HashOptions) uint64 {
		t.Helper()
		h, err := Hash(v, opts)
		if err != nil {
			t.Fatalf("unexpected error: %v for %#v", err, v)
		}
		return h
	}

	t.Run("value receiver", func(t *testing.T) {
		one := hash(t, testUnwrapKeyer{key: "foo"}, keyOpts())
		two := hash(t, testUnwrapKeyer{key: "foo"}, keyOpts())
		three := hash(t, testUnwrapKeyer{key: "bar"}, keyOpts())
		if one != two {
			t.Fatalf("expected %d == %d", one, two)
		}
		if one == three {
			t.Fatalf("expected %d != %d", one, three)
		}
	})

	t.Run("pointer receiver", func(t *testing.T) {
		// The struct is reached through a pointer, so it's addressable and the
		// hook finds the pointer receiver method via Addr.
		one := hash(t, &testUnwrapKeyerPointer{key: "foo"}, keyOpts())
		two := hash(t, &testUnwrapKeyerPointer{key: "foo"}, keyOpts())
		three := hash(t, &testUnwrapKeyerPointer{key: "bar"}, keyOpts())
		if one != two {
			t.Fatalf("expected %d == %d", one, two)
		}
		if one == three {
			t.Fatalf("expected %d != %d", one, three)
		}
	})

	t.Run("hashes as the unwrapped value", func(t *testing.T) {
		one := hash(t, testUnwrapKeyer{key: "foo"}, keyOpts())
		two := hash(t, "foo", keyOpts())
		if one != two {
			t.Fatalf("expected %d == %d", one, two)
		}
	})

	t.Run("nested", func(t *testing.T) {
		one := hash(t, []any{"a", &testUnwrapKeyerPointer{key: "foo"}}, keyOpts())
		two := hash(t, []any{"a", &testUnwrapKeyerPointer{key: "bar"}}, keyOpts())
		if one == two {
			t.Fatalf("expected %d != %d", one, two)
		}

		three := hash(t, map[string]any{"a": testUnwrapKeyer{key: "foo"}}, keyOpts())
		four := hash(t, map[string]any{"a": testUnwrapKeyer{key: "bar"}}, keyOpts())
		if three == four {
			t.Fatalf("expected %d != %d", three, four)
		}

		// Without the hook, the unexported keys are invisible to the walker.
		five := hash(t, []any{"a", &testUnwrapKeyerPointer{key: "foo"}}, nil)
		six := hash(t, []any{"a", &testUnwrapKeyerPointer{key: "bar"}}, nil)
		if five != six {
			t.Fatalf("expected %d == %d", five, six)
		}
	})

	t.Run("no-op hook", func(t *testing.T) {
		noop := &HashOptions{UnwrapFunc: func(v reflect.Value) (reflect.Value, error) { return v, nil }}
		v := struct {
			A string
			B int
		}{"a", 32}
		one := hash(t, v, noop)
		two := hash(t, v, nil)
		if one != two {
			t.Fatalf("expected %d == %d", one, two)
		}
	})

	t.Run("unwrapped value is dereferenced", func(t *testing.T) {
		s := "foo"
		opts := &HashOptions{UnwrapFunc: func(v reflect.Value) (reflect.Value, error) {
			if v.Type() == reflect.TypeFor[testUnwrapKeyer]() {
				return reflect.ValueOf(&s), nil
			}
			return v, nil
		}}
		one := hash(t, testUnwrapKeyer{key: "ignored"}, opts)
		two := hash(t, "foo", opts)
		if one != two {
			t.Fatalf("expected %d == %d", one, two)
		}
	})

	t.Run("unwrapped value is unwrapped again", func(t *testing.T) {
		opts := &HashOptions{UnwrapFunc: func(v reflect.Value) (reflect.Value, error) {
			switch v.Type() {
			case reflect.TypeFor[testUnwrapChainA]():
				return reflect.ValueOf(testUnwrapChainB{}), nil
			case reflect.TypeFor[testUnwrapChainB]():
				return reflect.ValueOf("final"), nil
			}
			return v, nil
		}}
		one := hash(t, testUnwrapChainA{}, opts)
		two := hash(t, "final", opts)
		if one != two {
			t.Fatalf("expected %d == %d", one, two)
		}
	})

	t.Run("takes precedence over Hashable", func(t *testing.T) {
		v := &testUnwrapHashableKeyer{}
		one := hash(t, v, keyOpts())
		two := hash(t, "key", keyOpts())
		if one != two {
			t.Fatalf("expected %d == %d", one, two)
		}
		if got := hash(t, v, nil); got != 12345 {
			t.Fatalf("expected 12345, got %d", got)
		}
	})

	t.Run("error", func(t *testing.T) {
		opts := &HashOptions{UnwrapFunc: func(v reflect.Value) (reflect.Value, error) {
			if v.Kind() == reflect.Struct {
				return v, fmt.Errorf("unwrap failed")
			}
			return v, nil
		}}
		_, err := Hash(testUnwrapKeyer{key: "foo"}, opts)
		if err == nil || err.Error() != "unwrap failed" {
			t.Fatalf("expected error %q, got %v", "unwrap failed", err)
		}
	})

	t.Run("runaway hook errors", func(t *testing.T) {
		var n int
		opts := &HashOptions{UnwrapFunc: func(v reflect.Value) (reflect.Value, error) {
			n++
			return reflect.ValueOf(fmt.Sprintf("%d", n)), nil
		}}
		_, err := Hash(testUnwrapKeyer{key: "foo"}, opts)
		if err == nil || !strings.Contains(err.Error(), "more than 32 times") {
			t.Fatalf("expected error containing %q, got %v", "more than 32 times", err)
		}
	})
}

func TestHash_golden(t *testing.T) {
	foo := "foo"

	cases := []struct {
		In     any
		Expect uint64
	}{
		{
			In:     nil,
			Expect: 12161962213042174405,
		},
		{
			In:     "foo",
			Expect: 15621798640163566899,
		},
		{
			In:     42,
			Expect: 11375694726533372055,
		},
		{
			In:     uint8(42),
			Expect: 12638153115695167477,
		},
		{
			In:     int16(42),
			Expect: 590708257076254031,
		},
		{
			In:     int16(-32768),
			Expect: 590684067820433261,
		},
		{
			In:     int16(32767),
			Expect: 590474061099445023,
		},
		{
			In:     int32(42),
			Expect: 843871326190827175,
		},
		{
			In:     int64(42),
			Expect: 11375694726533372055,
		},
		{
			In:     uint16(0),
			Expect: 590684067820433389,
		},
		{
			In:     uint16(42),
			Expect: 590708257076254031,
		},
		{
			In:     uint16(65535),
			Expect: 590474061099445151,
		},
		{
			In:     uint32(42),
			Expect: 843871326190827175,
		},
		{
			In:     uint64(42),
			Expect: 11375694726533372055,
		},
		{
			In:     float32(42),
			Expect: 5558953217260120943,
		},
		{
			In:     float64(42),
			Expect: 12162027084228238918,
		},
		{
			In:     float64(3.14159265359),
			Expect: 999115755352816086,
		},
		{
			In:     complex64(42),
			Expect: 13187391128804187615,
		},
		{
			In:     complex64(complex(1.2, 3.4)),
			Expect: 12862333766589160118,
		},
		{
			In:     complex128(42),
			Expect: 4635205179288363782,
		},
		{
			In:     true,
			Expect: 12638153115695167454,
		},
		{
			In:     false,
			Expect: 12638153115695167455,
		},
		{
			In:     []string{"foo", "bar"},
			Expect: 18333885979647637445,
		},
		{
			In:     []any{1, nil, "foo"},
			Expect: 636613494442026145,
		},
		{
			In:     map[string]string{"foo": "bar"},
			Expect: 5334326627423288605,
		},
		{
			In:     map[string]*string{"foo": &foo},
			Expect: 4615367350888355399,
		},
		{
			In:     map[*string]string{&foo: "bar"},
			Expect: 5334326627423288605,
		},
		{
			In:     map[any]string{"foo": "bar"},
			Expect: 5334326627423288605,
		},
		{
			In:     map[any]any{"foo": "bar", "bar": 0},
			Expect: 10207098687398820730,
		},
		{
			In:     map[any]any{"foo": "bar", "bar": map[any]any{"foo": "bar", "bar": map[any]any{"foo": "bar", "bar": map[any]any{&foo: "bar", "bar": 0}}}},
			Expect: 18346441822047112296,
		},
		{
			In: struct {
				Foo string
				Bar []any
			}{
				Foo: "foo",
				Bar: []any{nil, nil, nil},
			},
			Expect: 14887393564066082535,
		},
	}

	for i, tc := range cases {
		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
			got, err := Hash(tc.In, nil)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.Expect {
				t.Fatalf("expected %d, got %d", tc.Expect, got)
			}

			h := fnv.New64()
			// Make sure the custom hasher is reset correctly.
			h.Write([]byte("hello"))
			opts := &HashOptions{Hasher: h}
			for range 2 {
				got, err := Hash(tc.In, opts)
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if got != tc.Expect {
					t.Fatalf("expected %d, got %d", tc.Expect, got)
				}
			}
		})
	}
}

func TestHash_slicesAsSet_golden(t *testing.T) {
	cases := []struct {
		In     any
		Expect uint64
	}{
		{
			In: struct {
				Foo string
				Bar []any
			}{
				Foo: "foo",
				Bar: []any{nil, nil, nil},
			},
			Expect: 3717100187917615858,
		},
		{
			In:     []any{1, 2, 3},
			Expect: 2044210338600952799,
		},
		{
			In:     []any{3, 2, 3, 3, 1},
			Expect: 2044210338600952799,
		},
		{
			In:     []string{"a", "b", "c", "e", "e"},
			Expect: 14856124470027295614,
		},
		{
			In:     []string{"a", "b", "c", "d", "d"},
			Expect: 10627943203888361049,
		},
	}

	for i, tc := range cases {
		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
			got, err := Hash(tc.In, &HashOptions{SlicesAsSets: true})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.Expect {
				t.Fatalf("expected %d, got %d", tc.Expect, got)
			}
		})
	}
}

func TestHash_concurrentSharedOptions(t *testing.T) {
	type testStruct struct {
		Foo string
	}
	v := testStruct{Foo: "foo"}

	expect, err := Hash(v, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Shared across goroutines without any warm-up call, so that any
	// write to it inside Hash (Hasher, TagName) is caught by -race.
	// See issue #23.
	opts := &HashOptions{}

	var wg sync.WaitGroup
	for range 4 {
		wg.Go(func() {
			for j := range 1000 {
				o := opts
				if j%2 == 0 {
					o = nil
				}
				got, err := Hash(v, o)
				if err != nil {
					t.Errorf("unexpected error: %v", err)
					return
				}
				if got != expect {
					t.Errorf("hash mismatch: got %d, want %d", got, expect)
					return
				}
			}
		})
	}
	wg.Wait()

	// Hash must have treated the shared options as read-only.
	if opts.Hasher != nil {
		t.Fatalf("expected nil Hasher, got %v", opts.Hasher)
	}
	if opts.TagName != "" {
		t.Fatalf("expected empty TagName, got %q", opts.TagName)
	}
}

func BenchmarkMap(b *testing.B) {
	m := map[string]any{
		"int16":      int16(42),
		"int32":      int32(42),
		"int64":      int64(42),
		"int":        int(42),
		"uint16":     uint16(42),
		"uint32":     uint32(42),
		"uint64":     uint64(42),
		"uint":       uint(42),
		"float32":    float32(42),
		"float64":    float64(42),
		"complex64":  complex64(42),
		"complex128": complex128(42),
		"string":     "foo",
		"bool":       true,
		"slice":      []string{"foo", "bar"},
		"sliceint":   []int{1, 2, 3},
		"map":        map[string]string{"foo": "bar"},
		"struct": struct {
			Foo string
			Bar []any
		}{
			Foo: "foo",
			Bar: []any{nil, nil, nil},
		},
	}

	b.Run("default", func(b *testing.B) {
		for b.Loop() {
			Hash(m, nil)
		}
	})

	b.Run("no-op unwrap", func(b *testing.B) {
		opts := &HashOptions{UnwrapFunc: func(v reflect.Value) (reflect.Value, error) { return v, nil }}
		for b.Loop() {
			Hash(m, opts)
		}
	})
}

func BenchmarkStruct(b *testing.B) {
	type testStruct struct {
		Foo string
	}

	b.Run("value", func(b *testing.B) {
		s := testStruct{Foo: "foo"}
		for b.Loop() {
			Hash(s, nil)
		}
	})

	b.Run("pointer", func(b *testing.B) {
		s := &testStruct{Foo: "foo"}
		for b.Loop() {
			Hash(s, nil)
		}
	})

	b.Run("pointer predefined hasher", func(b *testing.B) {
		s := &testStruct{Foo: "foo"}
		opts := &HashOptions{Hasher: fnv.New64()}
		for b.Loop() {
			Hash(s, opts)
		}
	})
}

func BenchmarkString(b *testing.B) {
	s := "lorem ipsum dolor sit amet"
	b.Run("default", func(b *testing.B) {
		for b.Loop() {
			Hash(s, nil)
		}
	})
}

type testIncludable struct {
	Value  string
	Ignore string
}

func (t testIncludable) HashInclude(field string, v any) (bool, error) {
	return field != "Ignore", nil
}

type testIncludableMap struct {
	Map map[string]string
}

func (t testIncludableMap) HashIncludeMap(field string, k, v any) (bool, error) {
	if field != "Map" {
		return true, nil
	}

	if s, ok := k.(string); ok && s == "ignore" {
		return false, nil
	}

	return true, nil
}

type testHashable struct {
	Value string
	Err   error
}

func (t testHashable) Hash() (uint64, error) {
	if t.Err != nil {
		return 0, t.Err
	}

	if strings.HasPrefix(t.Value, "foo") {
		return 500, nil
	}

	return 100, nil
}

type testHashablePointer struct {
	Value string
}

func (t *testHashablePointer) Hash() (uint64, error) {
	if strings.HasPrefix(t.Value, "foo") {
		return 500, nil
	}

	return 100, nil
}

type testIncludableMapMap map[string]string

func (t testIncludableMapMap) HashIncludeMap(_ string, k, _ any) (bool, error) {
	return k.(string) != "ignore", nil
}

type testUnwrapKeyer struct {
	key string
}

func (t testUnwrapKeyer) Key() string { return t.key }

type testUnwrapKeyerPointer struct {
	key string
}

func (t *testUnwrapKeyerPointer) Key() string { return t.key }

type testUnwrapChainA struct{}

type testUnwrapChainB struct{}

type testUnwrapHashableKeyer struct{}

func (t *testUnwrapHashableKeyer) Hash() (uint64, error) { return 12345, nil }

func (t *testUnwrapHashableKeyer) Key() string { return "key" }
