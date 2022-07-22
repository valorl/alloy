package value_test

import (
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/grafana/agent/pkg/river/internal/value"
	"github.com/stretchr/testify/require"
)

func TestDecode_Numbers(t *testing.T) {
	// There's a lot of values that can represent numbers, so we construct a
	// matrix dynamically of all the combinations here.
	vals := []interface{}{
		int(15), int8(15), int16(15), int32(15), int64(15),
		uint(15), uint8(15), uint16(15), uint32(15), uint64(15),
		float32(15), float64(15),
		string("15"), // string holding a valid number (which can be converted to a number)
	}

	for _, input := range vals {
		for _, expect := range vals {
			val := value.Encode(input)

			name := fmt.Sprintf(
				"%s to %s",
				reflect.TypeOf(input),
				reflect.TypeOf(expect),
			)

			t.Run(name, func(t *testing.T) {
				vPtr := reflect.New(reflect.TypeOf(expect)).Interface()
				require.NoError(t, value.Decode(val, vPtr))

				actual := reflect.ValueOf(vPtr).Elem().Interface()

				require.Equal(t, expect, actual)
			})
		}
	}
}

func TestDecode(t *testing.T) {
	// Declare some types to use for testing. Person2 is used as a struct
	// equivalent to Person, but with a different Go type to force casting.
	type Person struct {
		Name string `river:"name,attr"`
	}

	type Person2 struct {
		Name string `river:"name,attr"`
	}

	tt := []struct {
		input, expect interface{}
	}{
		{nil, (*int)(nil)},

		// Non-number primitives.
		{string("Hello!"), string("Hello!")},
		{bool(true), bool(true)},

		// Arrays
		{[]int{1, 2, 3}, []int{1, 2, 3}},
		{[]int{1, 2, 3}, [...]int{1, 2, 3}},
		{[...]int{1, 2, 3}, []int{1, 2, 3}},
		{[...]int{1, 2, 3}, [...]int{1, 2, 3}},

		// Maps
		{map[string]int{"year": 2022}, map[string]uint{"year": 2022}},
		{map[string]string{"name": "John"}, map[string]string{"name": "John"}},
		{map[string]string{"name": "John"}, Person{Name: "John"}},
		{Person{Name: "John"}, map[string]string{"name": "John"}},
		{Person{Name: "John"}, Person{Name: "John"}},
		{Person{Name: "John"}, Person2{Name: "John"}},
		{Person2{Name: "John"}, Person{Name: "John"}},

		// NOTE(rfratto): we don't test capsules or functions here because they're
		// not comparable in the same way as we do the other tests.
		//
		// See TestDecode_Functions and TestDecode_Capsules for specific decoding
		// tests of those types.
	}

	for _, tc := range tt {
		val := value.Encode(tc.input)

		name := fmt.Sprintf(
			"%s (%s) to %s",
			val.Type(),
			reflect.TypeOf(tc.input),
			reflect.TypeOf(tc.expect),
		)

		t.Run(name, func(t *testing.T) {
			vPtr := reflect.New(reflect.TypeOf(tc.expect)).Interface()
			require.NoError(t, value.Decode(val, vPtr))

			actual := reflect.ValueOf(vPtr).Elem().Interface()

			require.Equal(t, tc.expect, actual)
		})
	}
}

func TestDecode_EmbeddedField(t *testing.T) {
	t.Run("Non-pointer", func(t *testing.T) {
		type Phone struct {
			Brand string `river:"phone_brand,attr"`
			Year  int    `river:"phone_year,attr"`
		}
		type Person struct {
			Phone
			Name string `river:"name,attr"`
			Age  int    `river:"age,attr"`
		}

		input := value.Object(map[string]value.Value{
			"age":         value.Int(32),
			"phone_brand": value.String("Android"),
			"name":        value.String("John Doe"),
			"phone_year":  value.Int(2019),
		})

		var actual Person
		require.NoError(t, value.Decode(input, &actual))

		require.Equal(t, Person{
			Name:  "John Doe",
			Age:   32,
			Phone: Phone{Brand: "Android", Year: 2019},
		}, actual)
	})

	t.Run("Pointer", func(t *testing.T) {
		type Phone struct {
			Brand string `river:"phone_brand,attr"`
			Year  int    `river:"phone_year,attr"`
		}
		type Person struct {
			*Phone
			Name string `river:"name,attr"`
			Age  int    `river:"age,attr"`
		}

		input := value.Object(map[string]value.Value{
			"age":         value.Int(32),
			"phone_brand": value.String("Android"),
			"name":        value.String("John Doe"),
			"phone_year":  value.Int(2019),
		})

		var actual Person
		require.NoError(t, value.Decode(input, &actual))

		require.Equal(t, Person{
			Name:  "John Doe",
			Age:   32,
			Phone: &Phone{Brand: "Android", Year: 2019},
		}, actual)
	})
}

func TestDecode_Functions(t *testing.T) {
	val := value.Encode(func() int { return 15 })

	var f func() int
	require.NoError(t, value.Decode(val, &f))
	require.Equal(t, 15, f())
}

func TestDecode_Capsules(t *testing.T) {
	expect := make(chan int, 5)

	var actual chan int
	require.NoError(t, value.Decode(value.Encode(expect), &actual))
	require.Equal(t, expect, actual)
}

// TestDecode_SliceCopy ensures that copies are made during decoding instead of
// setting values directly.
func TestDecode_SliceCopy(t *testing.T) {
	orig := []int{1, 2, 3}

	var res []int
	require.NoError(t, value.Decode(value.Encode(orig), &res))

	res[0] = 10
	require.Equal(t, []int{1, 2, 3}, orig, "Original slice should not have been modified")
}

// TestDecode_ArrayCopy ensures that copies are made during decoding instead of
// setting values directly.
func TestDecode_ArrayCopy(t *testing.T) {
	orig := [...]int{1, 2, 3}

	var res [3]int
	require.NoError(t, value.Decode(value.Encode(orig), &res))

	res[0] = 10
	require.Equal(t, [3]int{1, 2, 3}, orig, "Original array should not have been modified")
}

func TestDecode_CustomTypes(t *testing.T) {
	t.Run("TextUnmarshaler", func(t *testing.T) {
		now := time.Now()
		nowBytes, _ := now.MarshalText()

		var actual time.Time
		require.NoError(t, value.Decode(value.String(string(nowBytes)), &actual))

		actualBytes, _ := actual.MarshalText()
		require.Equal(t, nowBytes, actualBytes)
	})

	t.Run("time.Duration", func(t *testing.T) {
		dur := 15 * time.Second

		var actual time.Duration
		require.NoError(t, value.Decode(value.String(dur.String()), &actual))
		require.Equal(t, dur.String(), actual.String())
	})
}

type textEnumType bool

func (et *textEnumType) UnmarshalText(text []byte) error {
	*et = false

	switch string(text) {
	case "accepted_value":
		*et = true
		return nil
	default:
		return fmt.Errorf("unrecognized value %q", string(text))
	}
}

func TestDecode_TextUnmarshaler(t *testing.T) {
	t.Run("valid type and value", func(t *testing.T) {
		var et textEnumType
		require.NoError(t, value.Decode(value.String("accepted_value"), &et))
		require.Equal(t, textEnumType(true), et)
	})

	t.Run("invalid type", func(t *testing.T) {
		var et textEnumType
		err := value.Decode(value.Bool(true), &et)
		require.EqualError(t, err, "expected string, got bool")
	})

	t.Run("invalid value", func(t *testing.T) {
		var et textEnumType
		err := value.Decode(value.String("bad_value"), &et)
		require.EqualError(t, err, `unrecognized value "bad_value"`)
	})

	t.Run("unmarshaler nested in other value", func(t *testing.T) {
		input := value.Array(
			value.String("accepted_value"),
			value.String("accepted_value"),
			value.String("accepted_value"),
		)

		var ett []textEnumType
		require.NoError(t, value.Decode(input, &ett))
		require.Equal(t, []textEnumType{true, true, true}, ett)
	})
}

func TestDecode_ErrorChain(t *testing.T) {
	type Target struct {
		Key struct {
			Object struct {
				Field1 []int `river:"field1,attr"`
			} `river:"object,attr"`
		} `river:"key,attr"`
	}

	val := value.Object(map[string]value.Value{
		"key": value.Object(map[string]value.Value{
			"object": value.Object(map[string]value.Value{
				"field1": value.Array(
					value.Int(15),
					value.Int(30),
					value.String("Hello, world!"),
				),
			}),
		}),
	})

	// NOTE(rfratto): strings of errors from the value package are fairly limited
	// in the amount of information they show, since the value package doesn't
	// have a great way to pretty-print the chain of errors.
	//
	// For example, with the error below, the message doesn't explain where the
	// string is coming from, even though the error values hold that context.
	//
	// Callers consuming errors should print the error chain with extra context
	// so it's more useful to users.
	err := value.Decode(val, &Target{})
	expectErr := `expected number, got string`
	require.EqualError(t, err, expectErr)
}

type boolish int

var _ value.ConvertibleFromCapsule = (*boolish)(nil)
var _ value.ConvertibleIntoCapsule = (boolish)(0)

func (b boolish) RiverCapsule() {}

func (b *boolish) ConvertFrom(src interface{}) error {
	switch v := src.(type) {
	case bool:
		if v {
			*b = 1
		} else {
			*b = 0
		}
		return nil
	}

	return value.ErrNoConversion
}

func (b boolish) ConvertInto(dst interface{}) error {
	switch d := dst.(type) {
	case *bool:
		if b == 0 {
			*d = false
		} else {
			*d = true
		}
		return nil
	}

	return value.ErrNoConversion
}

func TestDecode_CustomConvert(t *testing.T) {
	t.Run("compatible type to custom", func(t *testing.T) {
		var b boolish
		err := value.Decode(value.Bool(true), &b)
		require.NoError(t, err)
		require.Equal(t, boolish(1), b)
	})

	t.Run("custom to compatible type", func(t *testing.T) {
		var b bool
		err := value.Decode(value.Encapsulate(boolish(10)), &b)
		require.NoError(t, err)
		require.Equal(t, true, b)
	})

	t.Run("incompatible type to custom", func(t *testing.T) {
		var b boolish
		err := value.Decode(value.String("true"), &b)
		require.EqualError(t, err, "expected capsule, got string")
	})

	t.Run("custom to incompatible type", func(t *testing.T) {
		src := boolish(10)

		var s string
		err := value.Decode(value.Encapsulate(&src), &s)
		require.EqualError(t, err, "expected string, got capsule")
	})
}
