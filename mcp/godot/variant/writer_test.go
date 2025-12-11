package variant

import (
	"math"
	"strings"
	"testing"

	"github.com/matryer/is"
)

func TestWriteValue(t *testing.T) {
	cases := []struct {
		name   string
		value  any
		output string
	}{
		{
			"Null",
			nil,
			"null",
		},
		{
			"True",
			true,
			"true",
		},
		{
			"False",
			false,
			"false",
		},
		{
			"Integer",
			int64(127),
			"127",
		},
		{
			"Float1",
			float64(1.2),
			"1.2",
		},
		{
			"Float2",
			float64(1.2e+150),
			"1.2e+150",
		},
		{
			"Float3",
			math.Inf(1),
			"inf",
		},
		{
			"Float4",
			math.Inf(-1),
			"-inf",
		},
		{
			"Float5",
			math.NaN(),
			"nan",
		},
		{
			"String",
			"hey\na string: \"test\"",
			"\"hey\na string: \\\"test\\\"\"",
		},
		{
			"Vector2",
			Vector2{1.2, 3.4},
			"Vector2(1.2, 3.4)",
		},
		{
			"Vector2i",
			Vector2i{1, 2},
			"Vector2i(1, 2)",
		},
		{
			"Rect2",
			Rect2{Vector2{1.2, 3.4}, Vector2{5.6, 7.8}},
			"Rect2(1.2, 3.4, 5.6, 7.8)",
		},
		{
			"Rect2i",
			Rect2i{Vector2i{1, 2}, Vector2i{3, 4}},
			"Rect2i(1, 2, 3, 4)",
		},
		{
			"Vector3",
			Vector3{1.2, 3.4, 5.6},
			"Vector3(1.2, 3.4, 5.6)",
		},
		{
			"Vector3i",
			Vector3i{1, 2, 3},
			"Vector3i(1, 2, 3)",
		},
		{
			"Vector4",
			Vector4{1.2, 3.4, 5.6, 7.8},
			"Vector4(1.2, 3.4, 5.6, 7.8)",
		},
		{
			"Vector4i",
			Vector4i{1, 2, 3, 4},
			"Vector4i(1, 2, 3, 4)",
		},
		{
			"Plane",
			Plane{Vector3{1.2, 3.4, 5.6}, 7.0},
			"Plane(1.2, 3.4, 5.6, 7)",
		},
		{
			"AABB",
			AABB{Vector3{1.2, 3.4, 5.6}, Vector3{7.8, 9.0, 1.2}},
			"AABB(1.2, 3.4, 5.6, 7.8, 9, 1.2)",
		},
		{
			"Quaternion",
			Quaternion{1.2, 3.4, 5.6, 7.8},
			"Quaternion(1.2, 3.4, 5.6, 7.8)",
		},
		{
			"Transform2D",
			Transform2D{
				Columns: [3]Vector2{
					{1.0, 0.0},
					{0.0, 1.0},
					{2.0, 3.0},
				},
			},
			`Transform2D(1, 0, 0, 1, 2, 3)`,
		},
		{
			"Basis",
			Basis{
				Rows: [3]Vector3{
					{1.0, 0.0, 0.0},
					{0.0, 1.0, 0.0},
					{0.0, 0.0, 1.0},
				},
			},
			`Basis(1, 0, 0, 0, 1, 0, 0, 0, 1)`,
		},
		{
			"Transform3D",
			Transform3D{
				Basis: Basis{
					Rows: [3]Vector3{
						{1.0, 0.0, 0.0},
						{0.0, 1.0, 0.0},
						{0.0, 0.0, 1.0},
					},
				},
				Origin: Vector3{1.0, 2.0, 3.0},
			},
			`Transform3D(1, 0, 0, 0, 1, 0, 0, 0, 1, 1, 2, 3)`,
		},
		{
			"Projection",
			Projection{
				Columns: [4]Vector4{
					{1.0, 0.0, 0.0, 0.0},
					{0.0, 1.0, 0.0, 0.0},
					{0.0, 0.0, 1.0, 0.0},
					{0.0, 0.0, 0.0, 1.0},
				},
			},
			`Projection(1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1)`,
		},
		{
			"Color",
			Color{0.1, 0.2, 0.3, 0.4},
			"Color(0.1, 0.2, 0.3, 0.4)",
		},
		{
			"StringName",
			StringName("name"),
			`&"name"`,
		},
		{
			"NodePath1",
			NodePath{
				Path:     []StringName{"root", "blah", "zah"},
				Subpath:  []StringName{"prop1", "prop2"},
				Absolute: true,
			},
			`NodePath("/root/blah/zah:prop1:prop2")`,
		},
		{
			"RID",
			RID(123),
			`RID(123)`,
		},
		{
			"Signal",
			Signal{},
			`Signal()`,
		},
		{
			"Callable",
			Callable{},
			`Callable()`,
		},
		{
			"PackedInt32Array",
			PackedInt32Array{1, 2, 3},
			`PackedInt32Array(1, 2, 3)`,
		},
		{
			"PackedInt64Array",
			PackedInt64Array{1, 2, 3},
			`PackedInt64Array(1, 2, 3)`,
		},
		{
			"PackedFloat32Array",
			PackedFloat32Array{1.2, 3.4, 5.6},
			`PackedFloat32Array(1.2, 3.4, 5.6)`,
		},
		{
			"PackedFloat64Array",
			PackedFloat64Array{1.2, 3.4, 5.6},
			`PackedFloat64Array(1.2, 3.4, 5.6)`,
		},
		{
			"PackedStringArray",
			PackedStringArray{"hello", "world"},
			`PackedStringArray("hello", "world")`,
		},
		{
			"PackedVector2Array",
			PackedVector2Array{Vector2{1.2, 3.4}, Vector2{5.6, 7.8}},
			`PackedVector2Array(1.2, 3.4, 5.6, 7.8)`,
		},
		{
			"PackedVector3Array",
			PackedVector3Array{Vector3{1.2, 3.4, 5.6}, Vector3{7.8, 9.0, 1.2}},
			`PackedVector3Array(1.2, 3.4, 5.6, 7.8, 9, 1.2)`,
		},
		{
			"PackedVector4Array",
			PackedVector4Array{Vector4{1.2, 3.4, 5.6, 7.8}, Vector4{9.0, 1.2, 3.4, 5.6}},
			`PackedVector4Array(1.2, 3.4, 5.6, 7.8, 9, 1.2, 3.4, 5.6)`,
		},
		{
			"PackedColorArray",
			PackedColorArray{Color{0.1, 0.2, 0.3, 0.4}, Color{0.5, 0.6, 0.7, 0.8}},
			`PackedColorArray(0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.7, 0.8)`,
		},
		{
			"Object",
			Object{
				ClassName: "InputEventKey",
				Properties: OrderedMap[string, any]{
					{"physical_keycode", int64(4194305)},
					{"pressed", true},
				},
			},
			`Object(InputEventKey,"physical_keycode":4194305,"pressed":true)`,
		},
		{
			"Dictionary",
			Dictionary{
				{"a", int64(1)},
				{int64(5), "five"},
			},
			"{\n\"a\": 1,\n5: \"five\"\n}",
		},
		{
			"Array",
			Array{int64(1), "2", 3.0},
			"[1\n, \"2\"\n, 3.0\n]",
		},
		{
			"ArrayEmpty",
			Array{},
			"[]",
		},
		{
			"PackedByteArrayBase64",
			PackedByteArray{1, 2, 3},
			`PackedByteArray("AQID")`,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			is := is.New(t)

			sb := strings.Builder{}
			w := NewWriter(&sb)

			err := w.WriteValue(tc.value)
			w.Flush()

			if expectedError, ok := tc.value.(error); ok {
				is.Equal(err.Error(), expectedError.Error())
			} else {
				is.NoErr(err)
				is.Equal(sb.String(), tc.output)
			}
		})
	}
}

func TestWriteComment(t *testing.T) {
	cases := []struct {
		name    string
		comment string
		output  string
	}{
		{
			"Simple",
			"My comment",
			";My comment\n",
		},
		{
			"SimpleSpace",
			" My comment",
			"; My comment\n",
		},
		{
			"Premarked",
			"; My comment",
			"; My comment\n",
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			is := is.New(t)

			sb := strings.Builder{}
			w := NewWriter(&sb)

			err := w.WriteComment(" My comment")
			w.Flush()

			is.NoErr(err)
			is.Equal(sb.String(), "; My comment\n")
		})
	}
}

func TestWriteTag(t *testing.T) {
	cases := []struct {
		name    string
		tagName string
		fields  map[string]any
		output  string
	}{
		{
			"Simple",
			"simpletag",
			nil,
			"[simpletag]\n",
		},
		{
			"WithFields",
			"complextag",
			map[string]any{
				"field1": 27,
				"field2": "hello",
			},
			"[complextag field1=27 field2=\"hello\"]\n",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			is := is.New(t)

			sb := strings.Builder{}
			w := NewWriter(&sb)

			err := w.WriteTag(tc.tagName, tc.fields)
			w.Flush()

			is.NoErr(err)
			is.Equal(sb.String(), tc.output)
		})
	}
}

func TestWriteAssignment(t *testing.T) {
	cases := []struct {
		name   string
		key    string
		value  any
		output string
	}{
		{
			"Simple1",
			"name",
			"David",
			"name=\"David\"\n",
		},
		{
			"Simple2",
			"with/a/deeper/path",
			27,
			"with/a/deeper/path=27\n",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			is := is.New(t)

			sb := strings.Builder{}
			w := NewWriter(&sb)

			err := w.WriteAssignment(tc.key, tc.value)
			w.Flush()

			is.NoErr(err)
			is.Equal(sb.String(), tc.output)
		})
	}
}
