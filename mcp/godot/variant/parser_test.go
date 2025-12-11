package variant

import (
	"errors"
	"io"
	"math"
	"strings"
	"testing"

	"github.com/matryer/is"
)

func TestParseValue(t *testing.T) {
	cases := []struct {
		name  string
		input string
		value any
	}{
		{
			"Empty",
			"",
			io.EOF,
		},
		{
			"String",
			`"teststring"`,
			"teststring",
		},
		{
			"StringUnterminated",
			`"a`,
			ErrUnterminatedString,
		},
		{
			"StringNewline",
			"\"test\nstring\"",
			"test\nstring",
		},
		{
			"StringEscaped",
			`"ab\"c\b\t\n\f\r\u03a9\U01f600"`,
			"ab\"c\b\t\n\f\r\u03a9\U0001f600",
		},
		{
			"StringEscapedUnterminated",
			`"ab\`,
			ErrUnterminatedString,
		},
		{
			"StringEscapedUnterminatedUnicode",
			`"ab\u00`,
			ErrUnterminatedString,
		},
		{
			"StringEscapedMalformedUnicode",
			`"ab\uzz`,
			errors.New("malformed hex constant in string"),
		},
		{
			"StringUTF16Surrogate",
			`"\uD83D\uDE00"`,
			"\U0001F600",
		},
		{
			"StringUTF16SurrogateUnpaired1",
			`"\uD83D"`,
			ErrUnpairedUTF16Surrogate,
		},
		{
			"StringUTF16SurrogateUnpaired2",
			`"\uD83D\uD83D"`,
			ErrUnpairedUTF16Surrogate,
		},
		{
			"StringUTF16SurrogateUnpaired3",
			`"\uDE00"`,
			ErrUnpairedUTF16Surrogate,
		},
		{
			"StringUTF16SurrogateUnpaired4",
			`"\uD83Dabc"`,
			ErrUnpairedUTF16Surrogate,
		},
		{
			"StringUTF16SurrogateUnpaired5",
			`"\uD83D\t"`,
			ErrUnpairedUTF16Surrogate,
		},
		{
			"LeadingComment",
			"; Test comment\n\"mystring\"",
			"mystring",
		},
		{
			"LeadingNewline",
			"\n\"mystring\"",
			"mystring",
		},
		{
			"LeadingSpace",
			"\t\t  \"mystring\"",
			"mystring",
		},
		{
			"CommentEOF",
			"; Test comment",
			io.EOF,
		},
		{
			"StringName",
			`&"name"`,
			StringName("name"),
		},
		{
			"StringNameLegacy",
			`@"name"`,
			StringName("name"),
		},
		{
			"StringNameEOF",
			`&`,
			ErrUnterminatedString,
		},
		{
			"StringNameNoQuote",
			`&blah`,
			errors.New("expected '\"' after '&'"),
		},
		{
			"StringNameNoQuoteLegacy",
			`@blah`,
			errors.New("expected '\"' after '@'"),
		},
		{
			"IntPositive",
			"5",
			int64(5),
		},
		{
			"IntNegative",
			"-27",
			int64(-27),
		},
		{
			"IntTrailing",
			"1; Comment",
			int64(1),
		},
		{
			"Float",
			"5.5",
			5.5,
		},
		{
			"FloatExp",
			"1.5e3",
			1.5e3,
		},
		{
			"FloatNegExp",
			"1.5e-3",
			1.5e-3,
		},
		{
			"FloatExpNoDote",
			"1e+2",
			1e+2,
		},
		{
			"FloatTrailing",
			"1.0; Comment",
			1.0,
		},
		{
			"FloatExpTrailing",
			"1.0e1; Comment",
			1.0e1,
		},
		{
			"BoolTrue",
			"true",
			true,
		},
		{
			"BoolFalse",
			"false",
			false,
		},
		{
			"Color3",
			"#abc",
			Color8(170, 187, 204, 255),
		},
		{
			"Color4",
			"#abcd",
			Color8(170, 187, 204, 221),
		},
		{
			"Color6",
			"#010204",
			Color8(1, 2, 4, 255),
		},
		{
			"Color8",
			"#10204080",
			Color8(16, 32, 64, 128),
		},
		{
			"ColorWithTrailing",
			"#eee;Comment",
			Color8(238, 238, 238, 255),
		},
		{
			"BoolTrue",
			"true",
			true,
		},
		{
			"BoolFalse",
			"false",
			false,
		},
		{
			"Null",
			"null",
			nil,
		},
		{
			"Nil",
			"nil",
			nil,
		},
		{
			"Inf",
			"inf",
			math.Inf(1),
		},
		{
			"InfNeg1",
			"-inf",
			math.Inf(-1),
		},
		{
			"InfNeg2",
			"inf_neg",
			math.Inf(-1),
		},
		{
			"PackedStringArrayEmpty",
			`PackedStringArray()`,
			PackedStringArray{},
		},
		{
			"PackedStringArraySingle",
			`PackedStringArray("hello")`,
			PackedStringArray{"hello"},
		},
		{
			"PackedStringArrayMultiple",
			`PackedStringArray("hello", "world")`,
			PackedStringArray{"hello", "world"},
		},
		{
			"PackedStringArrayNotString",
			`PackedStringArray(1, 2)`,
			errors.New("expected string"),
		},
		{
			"ParseConstructInvalid1",
			`PackedStringArray["hello"]`,
			errors.New("expected '('"),
		},
		{
			"ParseConstructInvalid2",
			`PackedStringArray("hello" . "world")`,
			errors.New("expected ',' or ')'"),
		},
		{
			"ParseConstructInvalid3",
			`PackedStringArray(/)`,
			errors.New("unexpected character '/'"),
		},
		{
			"ParseConstructInvalid4",
			`PackedStringArray("hello" /)`,
			errors.New("unexpected character '/'"),
		},
		{
			"ParseConstructInvalid5",
			`PackedStringArray/`,
			errors.New("unexpected character '/'"),
		},
		{
			"Vector2",
			`Vector2(12.34, 56.78)`,
			Vector2{12.34, 56.78},
		},
		{
			"Vector2WithInt",
			`Vector2(1.2, 3)`,
			Vector2{1.2, 3.0},
		},
		{
			"Vector2InvalidString",
			`Vector2(1.2, "hi")`,
			errors.New("expected number"),
		},
		{
			"Vector2InvalidCount",
			`Vector2(1.2, 3.4, 5.6)`,
			errors.New("expected 2 values (found 3)"),
		},
		{
			"Vector2i",
			`Vector2i(1, 2)`,
			Vector2i{1, 2},
		},
		{
			"Vector2iInvalidString",
			`Vector2i(1, "hi")`,
			errors.New("expected number"),
		},
		{
			"Vector2iInvalidFloat",
			`Vector2i(1, 2.3)`,
			errors.New("expected integer"),
		},
		{
			"Vector2iInvalidOverflow",
			`Vector2i(1, 3000000000)`,
			errors.New("integer overflow"),
		},
		{
			"Vector2iInvalidCount",
			`Vector2i(1, 2, 3)`,
			errors.New("expected 2 values (found 3)"),
		},
		{
			"Rect2",
			`Rect2(1.2, 2.3, 4.5, 6.7)`,
			Rect2{Vector2{1.2, 2.3}, Vector2{4.5, 6.7}},
		},
		{
			"Rect2InvalidCount",
			`Rect2(1.2, 2.3, 4.5, 6.7, 8.9)`,
			errors.New("expected 4 values (found 5)"),
		},
		{
			"Rect2i",
			`Rect2i(1, 2, 3, 4)`,
			Rect2i{Vector2i{1, 2}, Vector2i{3, 4}},
		},
		{
			"Rect2iInvalidCount",
			`Rect2i(1, 2, 3, 4, 5)`,
			errors.New("expected 4 values (found 5)"),
		},
		{
			"Vector3",
			`Vector3(12.34, 56.78, 90.12)`,
			Vector3{12.34, 56.78, 90.12},
		},
		{
			"Vector3i",
			`Vector3i(1, 2, 3)`,
			Vector3i{1, 2, 3},
		},
		{
			"Vector3InvalidCount",
			`Vector3(1.0)`,
			errors.New("expected 3 values (found 1)"),
		},
		{
			"Vector3iInvalidCount",
			`Vector3i(1)`,
			errors.New("expected 3 values (found 1)"),
		},
		{
			"Vector4",
			`Vector4(12.34, 56.78, 90.12, 34.56)`,
			Vector4{12.34, 56.78, 90.12, 34.56},
		},
		{
			"Vector4i",
			`Vector4i(1, 2, 3, 4)`,
			Vector4i{1, 2, 3, 4},
		},
		{
			"Vector4InvalidCount",
			`Vector4(1.0)`,
			errors.New("expected 4 values (found 1)"),
		},
		{
			"Vector4iInvalidCount",
			`Vector4i(1)`,
			errors.New("expected 4 values (found 1)"),
		},
		{
			"Transform2D",
			`Transform2D(1.0, 0.0, 0.0, 1.0, 2.0, 3.0)`,
			Transform2D{
				Columns: [3]Vector2{
					{1.0, 0.0},
					{0.0, 1.0},
					{2.0, 3.0},
				},
			},
		},
		{
			"Transform2DInvalidCount",
			`Transform2D(1.0)`,
			errors.New("expected 6 values (found 1)"),
		},
		{
			"Plane",
			`Plane(0.0, 1.0, 0.0, 4.0)`,
			Plane{
				Normal: Vector3{0.0, 1.0, 0.0},
				D:      4.0,
			},
		},
		{
			"PlaneInvalidCount",
			`Plane(1.0)`,
			errors.New("expected 4 values (found 1)"),
		},
		{
			"Quaternion",
			`Quaternion(1.2, 3.4, 5.6, 7.8)`,
			Quaternion{1.2, 3.4, 5.6, 7.8},
		},
		{
			"QuaternionInvalidCount",
			`Quaternion(1.0)`,
			errors.New("expected 4 values (found 1)"),
		},
		{
			"AABB",
			`AABB(1.0, 2.0, 3.0, 4.0, 5.0, 6.0)`,
			AABB{
				Position: Vector3{1.0, 2.0, 3.0},
				Size:     Vector3{4.0, 5.0, 6.0},
			},
		},
		{
			"AABBInvalidCount",
			`AABB(1.0)`,
			errors.New("expected 6 values (found 1)"),
		},
		{
			"Basis",
			`Basis(1.0, 0.0, 0.0, 0.0, 1.0, 0.0, 0.0, 0.0, 1.0)`,
			Basis{
				Rows: [3]Vector3{
					{1.0, 0.0, 0.0},
					{0.0, 1.0, 0.0},
					{0.0, 0.0, 1.0},
				},
			},
		},
		{
			"BasisInvalidCount",
			`Basis(1.0)`,
			errors.New("expected 9 values (found 1)"),
		},
		{
			"Transform3D",
			`Transform3D(1.0, 0.0, 0.0, 0.0, 1.0, 0.0, 0.0, 0.0, 1.0, 1.0, 2.0, 3.0)`,
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
		},
		{
			"Transform3DInvalidCount",
			`Transform3D(1.0)`,
			errors.New("expected 12 values (found 1)"),
		},
		{
			"Projection",
			`Projection(1.0, 0.0, 0.0, 0.0, 0.0, 1.0, 0.0, 0.0, 0.0, 0.0, 1.0, 0.0, 0.0, 0.0, 0.0, 1.0)`,
			Projection{
				Columns: [4]Vector4{
					{1.0, 0.0, 0.0, 0.0},
					{0.0, 1.0, 0.0, 0.0},
					{0.0, 0.0, 1.0, 0.0},
					{0.0, 0.0, 0.0, 1.0},
				},
			},
		},
		{
			"ProjectionInvalidCount",
			`Projection(1.0)`,
			errors.New("expected 16 values (found 1)"),
		},
		{
			"Color",
			`Color(0.1, 0.2, 0.3, 0.4)`,
			Color{0.1, 0.2, 0.3, 0.4},
		},
		{
			"ColorWithInt",
			`Color(0, 0, 0, 1)`,
			Color{0.0, 0.0, 0.0, 1.0},
		},
		{
			"ColorInvalidCount",
			`Color(1.0)`,
			errors.New("expected 4 values (found 1)"),
		},
		{
			"NodePath1",
			`NodePath("/root/blah/zah:prop1:prop2")`,
			NodePath{
				Path:     []StringName{"root", "blah", "zah"},
				Subpath:  []StringName{"prop1", "prop2"},
				Absolute: true,
			},
		},
		{
			"RID",
			`RID(123)`,
			RID(123),
		},
		{
			"RIDInvalidCount",
			`RID(123, 456)`,
			errors.New("expected 1 values (found 2)"),
		},
		{
			"Signal",
			`Signal()`,
			Signal{},
		},
		{
			"SignalInvalidCount",
			`Signal(1)`,
			errors.New("expected empty"),
		},
		{
			"Callable",
			`Callable()`,
			Callable{},
		},
		{
			"CallableInvalidCount",
			`Callable(1)`,
			errors.New("expected empty"),
		},
		{
			"Object",
			`Object(InputEventKey,"physical_keycode":4194305,"pressed":true)`,
			Object{
				ClassName: "InputEventKey",
				Properties: OrderedMap[string, any]{
					{"physical_keycode", int64(4194305)},
					{"pressed", true},
				},
			},
		},
		{
			"ObjectInvalid1",
			`Object[InputEventKey]`,
			errors.New("expected '('"),
		},
		{
			"ObjectInvalid2",
			`Object(123)`,
			errors.New("expected class name"),
		},
		{
			"ObjectInvalid3",
			`Object(InputEventKey, "name": 123 123)`,
			errors.New("expected ',' or ')'"),
		},
		{
			"ObjectInvalid4",
			`Object(InputEventKey, 123)`,
			errors.New("expected property name as string"),
		},
		{
			"ObjectInvalid5",
			`Object(InputEventKey, "name" 123)`,
			errors.New("expected ':'"),
		},
		{
			"ObjectInvalid6",
			`Object(InputEventKey "name")`,
			errors.New("expected ','"),
		},
		{
			"Array",
			`[1, "2", 3.0]`,
			Array{int64(1), "2", 3.0},
		},
		{
			"ArrayInvalid1",
			`[1 "2"]`,
			errors.New("expected ','"),
		},
		{
			"Dictionary",
			`{"a": 1, 5: "five"}`,
			Dictionary{
				{"a", int64(1)},
				{int64(5), "five"},
			},
		},
		{
			"DictionaryInvalid1",
			`{"a" 1}`,
			errors.New("expected ':'"),
		},
		{
			"DictionaryInvalid2",
			`{"a": 1 2}`,
			errors.New("expected ',' or '}'"),
		},
		{
			"PackedInt32Array",
			`PackedInt32Array(1, 2, 3)`,
			PackedInt32Array{1, 2, 3},
		},
		{
			"PackedInt64Array",
			`PackedInt64Array(1, 2, 3)`,
			PackedInt64Array{1, 2, 3},
		},
		{
			"PackedFloat32Array",
			`PackedFloat32Array(1.0, 2.0, 3.0)`,
			PackedFloat32Array{1.0, 2.0, 3.0},
		},
		{
			"PackedFloat64Array",
			`PackedFloat64Array(1.0, 2.0, 3.0)`,
			PackedFloat64Array{1.0, 2.0, 3.0},
		},
		{
			"PackedVector2Array",
			`PackedVector2Array(1.2, 3.4, 5.6, 7.8)`,
			PackedVector2Array{Vector2{1.2, 3.4}, Vector2{5.6, 7.8}},
		},
		{
			"PackedVector2ArrayInvalidCount",
			`PackedVector2Array(1.2, 3.4, 5.6)`,
			errors.New("expected count divisible by 2"),
		},
		{
			"PackedVector3Array",
			`PackedVector3Array(1.2, 3.4, 5.6, 7.8, 9.0, 1.2)`,
			PackedVector3Array{Vector3{1.2, 3.4, 5.6}, Vector3{7.8, 9.0, 1.2}},
		},
		{
			"PackedVector3ArrayInvalidCount",
			`PackedVector3Array(1.2, 3.4, 5.6, 7.8, 9.0)`,
			errors.New("expected count divisible by 3"),
		},
		{
			"PackedVector4Array",
			`PackedVector4Array(1.2, 3.4, 5.6, 7.8, 9.0, 1.2, 3.4, 5.6)`,
			PackedVector4Array{Vector4{1.2, 3.4, 5.6, 7.8}, Vector4{9.0, 1.2, 3.4, 5.6}},
		},
		{
			"PackedVector4ArrayInvalidCount",
			`PackedVector4Array(1.2, 3.4, 5.6, 7.8, 9.0, 1.2, 3.4)`,
			errors.New("expected count divisible by 4"),
		},
		{
			"PackedColorArray",
			`PackedColorArray(0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.7, 0.8)`,
			PackedColorArray{Color{0.1, 0.2, 0.3, 0.4}, Color{0.5, 0.6, 0.7, 0.8}},
		},
		{
			"PackedColorArrayInvalidCount",
			`PackedColorArray(0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.7)`,
			errors.New("expected count divisible by 4"),
		},
		{
			"PackedByteArray",
			`PackedByteArray(1, 2, 3)`,
			PackedByteArray{1, 2, 3},
		},
		{
			"PackedByteArrayBase64",
			`PackedByteArray("AQID")`,
			PackedByteArray{1, 2, 3},
		},
		{
			"PackedByteArrayInvalid1",
			`PackedByteArray(abc)`,
			errors.New("expected integer"),
		},
		{
			"PackedByteArrayInvalid2",
			`PackedByteArray(1.2)`,
			errors.New("expected integer"),
		},
		{
			"PackedByteArrayInvalid3",
			`PackedByteArray(1000)`,
			errors.New("integer overflow"),
		},
		{
			"PackedByteArrayInvalid4",
			`PackedByteArray(1 2)`,
			errors.New("expected ',' or ')'"),
		},
		{
			"PackedByteArrayInvalid5",
			`PackedByteArray("AQID" 123)`,
			errors.New("expected ')'"),
		},
		{
			"PackedByteArrayInvalid6",
			`PackedByteArray[abc]`,
			errors.New("expected '('"),
		},
		{
			"PackedByteArrayInvalid6",
			`PackedByteArray("xxx")`,
			errors.New("illegal base64 data at input byte 0"),
		},
		{
			"UnknownIdentifier",
			"aoeu",
			errors.New("invalid identifier: aoeu"),
		},
		{
			"UnableToParse",
			",",
			errors.New("unable to parse"),
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			is := is.New(t)

			p := NewParser(strings.NewReader(tc.input))
			v, err := p.ParseValue()

			if expectedError, ok := tc.value.(error); ok {
				is.Equal(v, nil)
				is.Equal(err.Error(), expectedError.Error())
			} else {
				is.NoErr(err)
				is.Equal(v, tc.value)
			}
		})
	}
}

func TestParseValueNaN(t *testing.T) {
	is := is.New(t)

	p := NewParser(strings.NewReader("nan"))
	v, err := p.ParseValue()
	is.NoErr(err)

	is.True(math.IsNaN(v.(float64)))
}

func TestParseStatement(t *testing.T) {
	cases := []struct {
		name  string
		input string
		value any
	}{
		{
			"Comment",
			`; Comment`,
			Statement{StatementTypeComment, "; Comment", nil},
		},
		{
			"Tag",
			`[section]`,
			Statement{StatementTypeTag, "section", nil},
		},
		{
			"TagEscaping",
			`[sec\t\ion\]]`,
			Statement{StatementTypeTag, "section]", nil},
		},
		{
			"Assignment",
			`blah=true`,
			Statement{StatementTypeAssignment, "blah", true},
		},
		{
			"AssignmentSlash",
			`blah/de/blah=true`,
			Statement{StatementTypeAssignment, "blah/de/blah", true},
		},
		{
			"AssignmentQuoted",
			"\"even/with/special/\t/char\"=true",
			Statement{StatementTypeAssignment, "even/with/special/\t/char", true},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			is := is.New(t)

			p := NewParser(strings.NewReader(tc.input))
			p.SetSimpleTag(true)

			v, err := p.ParseStatement()

			if expectedError, ok := tc.value.(error); ok {
				is.Equal(v, nil)
				is.Equal(err.Error(), expectedError.Error())
			} else {
				is.NoErr(err)
				is.Equal(v, tc.value)
			}
		})
	}
}

func TestParseStatementComplexTag(t *testing.T) {
	cases := []struct {
		name  string
		input string
		value any
	}{
		{
			"Simple",
			`[tag]`,
			Statement{StatementTypeTag, "tag", map[string]any{}},
		},
		{
			"Periods",
			`[tag.with.periods]`,
			Statement{StatementTypeTag, "tag.with.periods", map[string]any{}},
		},
		{
			"Colons",
			`[tag:with:colons]`,
			Statement{StatementTypeTag, "tag:with:colons", map[string]any{}},
		},
		{
			"PeriodsAndColons",
			`[tag:with.both]`,
			Statement{StatementTypeTag, "tag:with.both", map[string]any{}},
		},
		{
			"Fields",
			`[tag value=true other="thing"]`,
			Statement{StatementTypeTag, "tag", map[string]any{"value": true, "other": "thing"}},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			is := is.New(t)

			p := NewParser(strings.NewReader(tc.input))
			p.SetSimpleTag(false)

			v, err := p.ParseStatement()

			if expectedError, ok := tc.value.(error); ok {
				is.Equal(v, nil)
				is.Equal(err.Error(), expectedError.Error())
			} else {
				is.NoErr(err)
				is.Equal(v, tc.value)
			}
		})
	}
}
