package hashstructure

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/cespare/xxhash/v2"
)

func TestHash_identity(t *testing.T) {
	cases := []interface{}{
		nil,
		"foo",
		42,
		true,
		false,
		[]string{"foo", "bar"},
		[]interface{}{1, nil, "foo"},
		map[string]string{"foo": "bar"},
		map[interface{}]string{"foo": "bar"},
		map[interface{}]interface{}{"foo": "bar", "bar": 0},
		struct {
			Foo string
			Bar []interface{}
		}{
			Foo: "foo",
			Bar: []interface{}{nil, nil, nil},
		},
		&struct {
			Foo string
			Bar []interface{}
		}{
			Foo: "foo",
			Bar: []interface{}{nil, nil, nil},
		},
	}

	for _, tc := range cases {
		// We run the test 100 times to try to tease out variability
		// in the runtime in terms of ordering.
		valuelist := make([]uint64, 100)
		for i := range valuelist {
			v, err := Hash(tc, nil)
			if err != nil {
				t.Fatalf("Error: %s\n\n%#v", err, tc)
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
				t.Fatalf("non-matching: %d, %d\n\n%#v", i, 0, tc)
			}
		}
	}
}

func TestHash_equal(t *testing.T) {
	type testFoo struct{ Name string }
	type testBar struct{ Name string }

	now := time.Now()

	cases := []struct {
		One, Two interface{}
		Match    bool
	}{
		{
			map[string]string{"foo": "bar"},
			map[interface{}]string{"foo": "bar"},
			true,
		},

		{
			map[string]interface{}{"1": "1"},
			map[string]interface{}{"1": "1", "2": "2"},
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
				t.Fatalf("Failed to hash %#v: %s", tc.One, err)
			}
			t.Logf("Hashing: %#v", tc.Two)
			two, err := Hash(tc.Two, nil)
			t.Logf("Result: %d", two)
			if err != nil {
				t.Fatalf("Failed to hash %#v: %s", tc.Two, err)
			}

			// Zero is always wrong
			if one == 0 {
				t.Fatalf("zero hash: %#v", tc.One)
			}

			// Compare
			if (one == two) != tc.Match {
				t.Fatalf("bad, expected: %#v\n\n%#v\n\n%#v", tc.Match, tc.One, tc.Two)
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
		One, Two interface{}
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
	}

	for _, tc := range cases {
		one, err := Hash(tc.One, nil)
		if err != nil {
			t.Fatalf("Failed to hash %#v: %s", tc.One, err)
		}
		two, err := Hash(tc.Two, nil)
		if err != nil {
			t.Fatalf("Failed to hash %#v: %s", tc.Two, err)
		}

		// Zero is always wrong
		if one == 0 {
			t.Fatalf("zero hash: %#v", tc.One)
		}

		// Compare
		if (one == two) != tc.Match {
			t.Fatalf("bad, expected: %#v\n\n%#v\n\n%#v", tc.Match, tc.One, tc.Two)
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
		Test  interface{}
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
			if ens, ok := err.(*ErrNotStringer); ok {
				if ens.Field != tc.Field {
					t.Fatalf("did not get expected field %#v: got %s wanted %s", tc.Test, ens.Field, tc.Field)
				}
			} else {
				t.Fatalf("unknown error %#v: got %s", tc, err)
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
		One, Two interface{}
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
			t.Fatalf("Failed to hash %#v: %s", tc.One, err)
		}
		two, err := Hash(tc.Two, &HashOptions{ZeroNil: tc.ZeroNil})
		if err != nil {
			t.Fatalf("Failed to hash %#v: %s", tc.Two, err)
		}

		// Zero is always wrong
		if one == 0 {
			t.Fatalf("zero hash: %#v", tc.One)
		}

		// Compare
		if (one == two) != tc.Match {
			t.Fatalf("bad, expected: %#v\n\n%#v\n\n%#v", tc.Match, tc.One, tc.Two)
		}
	}
}

func TestHash_equalSet(t *testing.T) {
	type Test struct {
		Name    string
		Friends []string `hash:"set"`
	}

	cases := []struct {
		One, Two interface{}
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
			t.Fatalf("Failed to hash %#v: %s", tc.One, err)
		}
		two, err := Hash(tc.Two, nil)
		if err != nil {
			t.Fatalf("Failed to hash %#v: %s", tc.Two, err)
		}

		// Zero is always wrong
		if one == 0 {
			t.Fatalf("zero hash: %#v", tc.One)
		}

		// Compare
		if (one == two) != tc.Match {
			t.Fatalf("bad, expected: %#v\n\n%#v\n\n%#v", tc.Match, tc.One, tc.Two)
		}
	}
}

func TestHash_includable(t *testing.T) {
	cases := []struct {
		One, Two interface{}
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
			t.Fatalf("Failed to hash %#v: %s", tc.One, err)
		}
		two, err := Hash(tc.Two, nil)
		if err != nil {
			t.Fatalf("Failed to hash %#v: %s", tc.Two, err)
		}

		// Zero is always wrong
		if one == 0 {
			t.Fatalf("zero hash: %#v", tc.One)
		}

		// Compare
		if (one == two) != tc.Match {
			t.Fatalf("bad, expected: %#v\n\n%#v\n\n%#v", tc.Match, tc.One, tc.Two)
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
			t.Fatalf("Failed to hash %#v: %s", structA, err)
		}
		hashB, err := Hash(structB, &HashOptions{IgnoreZeroValue: tc.IgnoreZeroValue})
		if err != nil {
			t.Fatalf("Failed to hash %#v: %s", structB, err)
		}
		if (hashA == hashB) != tc.IgnoreZeroValue {
			t.Fatalf("bad, expected: %#v\n\n%d\n\n%d", tc.IgnoreZeroValue, hashA, hashB)
		}
	}
}

func TestHash_includableMap(t *testing.T) {
	cases := []struct {
		One, Two interface{}
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
			t.Fatalf("Failed to hash %#v: %s", tc.One, err)
		}
		two, err := Hash(tc.Two, nil)
		if err != nil {
			t.Fatalf("Failed to hash %#v: %s", tc.Two, err)
		}

		// Zero is always wrong
		if one == 0 {
			t.Fatalf("zero hash: %#v", tc.One)
		}

		// Compare
		if (one == two) != tc.Match {
			t.Fatalf("bad, expected: %#v\n\n%#v\n\n%#v", tc.Match, tc.One, tc.Two)
		}
	}
}

func TestHash_hashable(t *testing.T) {
	cases := []struct {
		One, Two interface{}
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
					t.Fatal("expected error")
				}

				if !strings.Contains(err.Error(), tc.Err) {
					t.Fatalf("expected error to contain %q, got: %s", tc.Err, err)
				}

				return
			}
			if err != nil {
				t.Fatalf("Failed to hash %#v: %s", tc.One, err)
			}

			two, err := Hash(tc.Two, nil)
			if err != nil {
				t.Fatalf("Failed to hash %#v: %s", tc.Two, err)
			}

			// Zero is always wrong
			if one == 0 {
				t.Fatalf("zero hash: %#v", tc.One)
			}

			// Compare
			if (one == two) != tc.Match {
				t.Fatalf("bad, expected: %#v\n\n%#v\n\n%#v", tc.Match, tc.One, tc.Two)
			}
		})
	}
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
			In:     int32(42),
			Expect: 843871326190827175,
		},
		{
			In:     int64(42),
			Expect: 11375694726533372055,
		},

		{
			In:     uint16(42),
			Expect: 590708257076254031,
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
			In:     complex64(42),
			Expect: 13187391128804187615,
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
			In:     []interface{}{1, nil, "foo"},
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
			In:     map[interface{}]string{"foo": "bar"},
			Expect: 5334326627423288605,
		},
		{
			In:     map[interface{}]interface{}{"foo": "bar", "bar": 0},
			Expect: 10207098687398820730,
		},
		{
			In:     map[interface{}]interface{}{"foo": "bar", "bar": map[interface{}]interface{}{"foo": "bar", "bar": map[interface{}]interface{}{"foo": "bar", "bar": map[interface{}]interface{}{&foo: "bar", "bar": 0}}}},
			Expect: 18346441822047112296,
		},
		{
			In: struct {
				Foo string
				Bar []interface{}
			}{
				Foo: "foo",
				Bar: []interface{}{nil, nil, nil},
			},
			Expect: 14887393564066082535,
		},
	}

	for i, tc := range cases {
		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
			got, err := Hash(tc.In, nil)
			if err != nil {
				t.Fatalf("unexpected error: %s", err)
			}

			if got != tc.Expect {
				t.Fatalf("expected: %d, got: %d", tc.Expect, got)
			}
		})
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
			Bar []interface{}
		}{
			Foo: "foo",
			Bar: []interface{}{nil, nil, nil},
		},
	}

	for i := 0; i < b.N; i++ {
		Hash(m, nil)
	}
}

func BenchmarkString(b *testing.B) {
	s := "lorem ipsum dolor sit amet"
	b.Run("default", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			Hash(s, nil)
		}
	})

	b.Run("xxhash", func(b *testing.B) {
		opts := &HashOptions{Hasher: xxhash.New()}
		for i := 0; i < b.N; i++ {
			Hash(s, opts)
		}
	})
}

type testIncludable struct {
	Value  string
	Ignore string
}

func (t testIncludable) HashInclude(field string, v interface{}) (bool, error) {
	return field != "Ignore", nil
}

type testIncludableMap struct {
	Map map[string]string
}

func (t testIncludableMap) HashIncludeMap(field string, k, v interface{}) (bool, error) {
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

func (t testIncludableMapMap) HashIncludeMap(_ string, k, _ interface{}) (bool, error) {
	return k.(string) != "ignore", nil
}
