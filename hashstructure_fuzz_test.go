package hashstructure

import (
	"testing"
	"time"

	fuzz "github.com/AdaLogics/go-fuzz-headers"
)

func FuzzHash(f *testing.F) {
	type Test struct {
		Str         string
		Int         int
		In64        int64
		Float64     float64
		MapStr      map[string]string
		MapStruct   map[string]Test
		SliceStr    []string
		SliceStrSet []string `hash:"set"`
		SliceByte   []byte
		StrPtr      *string
		IntPtr      *int
		Time        time.Time
		Duration    time.Duration
		UUID        string `hash:"ignore"`
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		fuzzConsumer := fuzz.NewConsumer(data)
		targetStruct := &Test{}
		err := fuzzConsumer.GenerateStruct(targetStruct)
		if err != nil {
			return
		}
		_, err = Hash(targetStruct, nil)
		if err != nil {
			t.Fatal(err)
		}
	})
}
