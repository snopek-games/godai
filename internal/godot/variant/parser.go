package variant

import (
	"bufio"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
	"unicode"
)

type Parser struct {
	r          *bufio.Reader
	buf        strings.Builder
	savedToken *token
	line       int
	simpleTag  bool
	done       bool
}

type StatementType int

const (
	StatementTypeUnknown StatementType = iota
	StatementTypeComment
	StatementTypeTag
	StatementTypeAssignment
)

type Statement struct {
	Type  StatementType
	Name  string
	Value any
}

var ErrUnexpectedEOF = errors.New("unexpected EOF")
var ErrUnterminatedString = errors.New("unterminated string")
var ErrUnpairedUTF16Surrogate = errors.New("invalid UTF-16 sequence in string: unpaired surrogate")

type tokenType int

const (
	tokenTypeError tokenType = iota
	tokenTypeCurlyBracketOpen
	tokenTypeCurlyBracketClose
	tokenTypeBracketOpen
	tokenTypeBracketClose
	tokenTypeParenthesisOpen
	tokenTypeParenthesisClose
	tokenTypeIdentifier
	tokenTypeString
	tokenTypeStringName
	tokenTypeNumber
	tokenTypeColor
	tokenTypeColon
	tokenTypeComma
	tokenTypePeriod
	tokenTypeEqual
	tokenTypeEOF
)

type token struct {
	TokenType tokenType
	Value     any
}

func NewParser(r io.Reader) *Parser {
	br, ok := r.(*bufio.Reader)
	if !ok {
		br = bufio.NewReader(r)
	}

	return &Parser{
		r: br,
	}
}

func (p *Parser) SetSimpleTag(simpleTag bool) {
	p.simpleTag = simpleTag
}

func (p *Parser) GetLine() int {
	return p.line
}

func (p *Parser) ParseStatement() (Statement, error) {
	if p.done {
		return Statement{}, errors.New("parser already done")
	}

	v, err := p.parseStatementInternal()
	if err != nil {
		p.done = true
	}

	return v, err
}

func (p *Parser) ParseValue() (any, error) {
	if p.done {
		return nil, errors.New("parser already done")
	}

	v, err := p.parseValueInternal()
	if err != nil {
		p.done = true
	}

	return v, err
}

func (p *Parser) parseStatementInternal() (Statement, error) {
	ret := Statement{}

	for {
		c, _, err := p.r.ReadRune()
		if err != nil {
			return ret, err
		}

		if c == '\n' {
			p.line++
			continue
		}
		if unicode.IsSpace(c) {
			continue
		}

		if c == ';' {
			p.r.UnreadRune()
			b, err := p.r.ReadBytes('\n')
			if err != nil {
				if err != io.EOF {
					return ret, err
				}
			}
			var s string
			if b[len(b)-1] == '\n' {
				s = string(b[:len(b)-1])
			} else {
				s = string(b)
			}

			p.line++

			ret.Type = StatementTypeComment
			ret.Name = s
			return ret, nil
		} else if c == '[' {
			p.r.UnreadRune()

			var s string
			var fields OrderedMap[string, any]
			if p.simpleTag {
				s, err = p.parseSimpleTag()
			} else {
				s, fields, err = p.parseTag()
			}
			if err != nil {
				return ret, err
			}

			ret.Type = StatementTypeTag
			ret.Name = s
			if fields != nil {
				ret.Value = fields
			}
			return ret, nil
		} else if unicode.IsPrint(c) {
			p.r.UnreadRune()

			name, value, err := p.parseAssignment()
			if err != nil {
				return ret, err
			}

			ret.Type = StatementTypeAssignment
			ret.Name = name
			ret.Value = value
			return ret, nil

		}
	}
}

func (p *Parser) parseSimpleTag() (string, error) {
	t, err := p.getToken()
	if err != nil {
		return "", err
	}
	if t.TokenType != tokenTypeBracketOpen {
		return "", errors.New("expected '['")
	}

	// Only "\]" is an escape; any other backslash stands for itself, so it's
	// held back for one rune and written out again if no ']' follows.
	escaping := false
	for {
		c, _, err := p.r.ReadRune()
		if err != nil {
			if err == io.EOF {
				return "", ErrUnexpectedEOF
			}
			return "", err
		}

		if escaping {
			escaping = false
			if c == ']' {
				p.buf.WriteRune(c)
				continue
			}
			p.buf.WriteRune('\\')
		}

		if c == '\\' {
			escaping = true
			continue
		}
		if c == ']' {
			break
		}

		p.buf.WriteRune(c)
	}

	return p.popString(), nil
}

func (p *Parser) parseTag() (string, OrderedMap[string, any], error) {
	t, err := p.getToken()
	if err != nil {
		return "", nil, err
	}
	if t.TokenType != tokenTypeBracketOpen {
		return "", nil, errors.New("expected '['")
	}

	t, err = p.getToken()
	if err != nil {
		return "", nil, err
	}
	if t.TokenType != tokenTypeIdentifier {
		return "", nil, errors.New("expected identifier")
	}

	tmpbuf := strings.Builder{}
	tmpbuf.WriteString(t.Value.(string))

	parsingName := true
	fields := OrderedMap[string, any]{}

	for {
		t, err = p.getToken()
		if err != nil {
			return "", nil, err
		}
		if t.TokenType == tokenTypeBracketClose {
			break
		}

		if parsingName && t.TokenType == tokenTypePeriod {
			tmpbuf.WriteRune('.')
			t, err = p.getToken()
			if err != nil {
				return "", nil, err
			}
		} else if parsingName && t.TokenType == tokenTypeColon {
			tmpbuf.WriteRune(':')
			t, err = p.getToken()
			if err != nil {
				return "", nil, err
			}
		} else {
			parsingName = false
		}

		if t.TokenType != tokenTypeIdentifier {
			return "", nil, errors.New("expected identifier")
		}

		s := t.Value.(string)

		if parsingName {
			tmpbuf.WriteString(s)
			continue
		}

		t, err = p.getToken()
		if err != nil {
			return "", nil, err
		}
		if t.TokenType != tokenTypeEqual {
			return "", nil, errors.New("expected '='")
		}

		v, err := p.parseValueInternal()
		if err != nil {
			return "", nil, err
		}

		fields.Set(s, v)
	}

	return tmpbuf.String(), fields, nil
}

func (p *Parser) parseAssignment() (string, any, error) {
	tmpbuf := strings.Builder{}

	for {
		c, _, err := p.r.ReadRune()
		if err != nil {
			if err == io.EOF {
				break
			}
			return "", nil, err
		}

		if c == '"' {
			p.r.UnreadRune()
			t, err := p.getToken()
			if err != nil {
				return "", nil, err
			}
			if t.TokenType != tokenTypeString {
				return "", nil, errors.New("error reading quoted string")
			}
			tmpbuf.WriteString(t.Value.(string))
		} else if c == '=' {
			p.r.UnreadRune()
			break
		} else {
			tmpbuf.WriteRune(c)
		}
	}

	name := tmpbuf.String()

	t, err := p.getToken()
	if err != nil {
		return "", nil, err
	}
	if t.TokenType != tokenTypeEqual {
		return "", nil, errors.New("expected '='")
	}

	v, err := p.parseValueInternal()
	if err != nil {
		return "", nil, err
	}

	return name, v, nil
}

func (p *Parser) parseValueInternal() (any, error) {
	t, err := p.getToken()
	if err != nil {
		return nil, err
	}

	switch t.TokenType {
	case tokenTypeEOF:
		return nil, io.EOF
	case tokenTypeString, tokenTypeNumber, tokenTypeColor:
		return t.Value, nil
	case tokenTypeCurlyBracketOpen:
		err = p.saveToken(&t)
		if err != nil {
			return nil, err
		}

		dict := Dictionary{}
		f := func(k any, v any) error {
			dict.Set(k, v)
			return nil
		}
		err = p.parseDictionary(f)
		if err != nil {
			return nil, err
		}
		return dict, nil
	case tokenTypeBracketOpen:
		err = p.saveToken(&t)
		if err != nil {
			return nil, err
		}

		array := Array{}
		f := func(v any) error {
			array = append(array, v)
			return nil
		}
		err = p.parseArray(f)
		if err != nil {
			return nil, err
		}
		return array, nil
	case tokenTypeIdentifier:
		switch t.Value {
		case "true":
			return true, nil
		case "false":
			return false, nil
		case "null", "nil":
			return nil, nil
		case "inf":
			return math.Inf(1), nil
		case "-inf", "inf_neg":
			return math.Inf(-1), nil
		case "nan":
			return math.NaN(), nil
		case "Vector2":
			values, err := p.parseConstructFloat64(2)
			if err != nil {
				return nil, err
			}
			return Vector2{X: values[0], Y: values[1]}, nil
		case "Vector2i":
			values, err := p.parseConstructInt32(2)
			if err != nil {
				return nil, err
			}
			return Vector2i{X: values[0], Y: values[1]}, nil
		case "Rect2":
			values, err := p.parseConstructFloat64(4)
			if err != nil {
				return nil, err
			}
			return Rect2{
				Position: Vector2{values[0], values[1]},
				Size:     Vector2{values[2], values[3]},
			}, nil
		case "Rect2i":
			values, err := p.parseConstructInt32(4)
			if err != nil {
				return nil, err
			}
			return Rect2i{
				Position: Vector2i{values[0], values[1]},
				Size:     Vector2i{values[2], values[3]},
			}, nil
		case "Vector3":
			values, err := p.parseConstructFloat64(3)
			if err != nil {
				return nil, err
			}
			return Vector3{X: values[0], Y: values[1], Z: values[2]}, nil
		case "Vector3i":
			values, err := p.parseConstructInt32(3)
			if err != nil {
				return nil, err
			}
			return Vector3i{X: values[0], Y: values[1], Z: values[2]}, nil
		case "Vector4":
			values, err := p.parseConstructFloat64(4)
			if err != nil {
				return nil, err
			}
			return Vector4{X: values[0], Y: values[1], Z: values[2], W: values[3]}, nil
		case "Vector4i":
			values, err := p.parseConstructInt32(4)
			if err != nil {
				return nil, err
			}
			return Vector4i{X: values[0], Y: values[1], Z: values[2], W: values[3]}, nil
		case "Transform2D", "Matrix32":
			values, err := p.parseConstructFloat64(6)
			if err != nil {
				return nil, err
			}
			return Transform2D{
				Columns: [3]Vector2{
					{values[0], values[1]},
					{values[2], values[3]},
					{values[4], values[5]},
				},
			}, nil
		case "Plane":
			values, err := p.parseConstructFloat64(4)
			if err != nil {
				return nil, err
			}
			return Plane{
				Normal: Vector3{values[0], values[1], values[2]},
				D:      values[3],
			}, nil
		case "Quaternion", "Quat":
			values, err := p.parseConstructFloat64(4)
			if err != nil {
				return nil, err
			}
			return Quaternion{X: values[0], Y: values[1], Z: values[2], W: values[3]}, nil
		case "AABB", "Rect3":
			values, err := p.parseConstructFloat64(6)
			if err != nil {
				return nil, err
			}
			return AABB{
				Position: Vector3{values[0], values[1], values[2]},
				Size:     Vector3{values[3], values[4], values[5]},
			}, nil
		case "Basis", "Matrix3":
			values, err := p.parseConstructFloat64(9)
			if err != nil {
				return nil, err
			}
			return Basis{
				Rows: [3]Vector3{
					{values[0], values[1], values[2]},
					{values[3], values[4], values[5]},
					{values[6], values[7], values[8]},
				},
			}, nil
		case "Transform3D", "Transform":
			values, err := p.parseConstructFloat64(12)
			if err != nil {
				return nil, err
			}
			return Transform3D{
				Basis: Basis{
					Rows: [3]Vector3{
						{values[0], values[1], values[2]},
						{values[3], values[4], values[5]},
						{values[6], values[7], values[8]},
					},
				},
				Origin: Vector3{values[9], values[10], values[11]},
			}, nil
		case "Projection":
			values, err := p.parseConstructFloat64(16)
			if err != nil {
				return nil, err
			}
			return Projection{
				Columns: [4]Vector4{
					{values[0], values[1], values[2], values[3]},
					{values[4], values[5], values[6], values[7]},
					{values[8], values[9], values[10], values[11]},
					{values[12], values[13], values[14], values[15]},
				},
			}, nil
		case "Color":
			values, err := p.parseConstructFloat32(4)
			if err != nil {
				return nil, err
			}
			return Color{R: values[0], G: values[1], B: values[2], A: values[3]}, nil
		case "NodePath":
			values, err := p.parseConstructString(1)
			if err != nil {
				return nil, err
			}
			v, err := ParseNodePath(values[0])
			if err != nil {
				return nil, fmt.Errorf("unable to parse NodePath: %w", err)
			}
			return v, nil
		case "RID":
			values, err := p.parseConstructInt64(1)
			if err != nil {
				return nil, err
			}
			return RID(values[0]), nil
		case "Signal":
			err := p.parseConstructEmpty()
			if err != nil {
				return nil, err
			}
			return Signal{}, nil
		case "Callable":
			err := p.parseConstructEmpty()
			if err != nil {
				return nil, err
			}
			return Callable{}, nil
		case "Object":
			value, err := p.parseObject()
			if err != nil {
				return nil, err
			}
			return *value, nil
		// @todo Resource types ("Resource", "SubResource", and "ExtResource")
		//case "Dictionary":
		//	// @todo for TypedDictionary
		//case "Array":
		//	// @todo for TypedArray
		case "PackedByteArray", "PoolByteArray", "ByteArray":
			values, err := p.parseByteArray()
			if err != nil {
				return nil, err
			}
			return values, nil
		case "PackedInt32Array", "PackedIntArray", "PoolIntArray", "IntArray":
			values, err := p.parseConstructInt32(0)
			if err != nil {
				return nil, err
			}
			return PackedInt32Array(values), nil
		case "PackedInt64Array":
			values, err := p.parseConstructInt64(0)
			if err != nil {
				return nil, err
			}
			return PackedInt64Array(values), nil
		case "PackedFloat32Array", "PackedRealArray", "PoolRealArray", "FloatArray":
			values, err := p.parseConstructFloat32(0)
			if err != nil {
				return nil, err
			}
			return PackedFloat32Array(values), nil
		case "PackedFloat64Array":
			values, err := p.parseConstructFloat64(0)
			if err != nil {
				return nil, err
			}
			return PackedFloat64Array(values), nil
		case "PackedStringArray", "PoolStringArray", "StringArray":
			values, err := p.parseConstructString(0)
			if err != nil {
				return nil, err
			}
			return PackedStringArray(values), nil
		case "PackedVector2Array", "PoolVector2Array", "Vector2Array":
			values, err := p.parseConstructFloat64(0)
			if err != nil {
				return nil, err
			}

			if len(values)%2 != 0 {
				return nil, errors.New("expected count divisible by 2")
			}

			l := len(values) / 2
			array := make(PackedVector2Array, 0, l)
			for i := 0; i < l; i++ {
				array = append(array, Vector2{
					X: values[i*2+0],
					Y: values[i*2+1],
				})
			}

			return array, nil
		case "PackedVector3Array", "PoolVector3Array", "Vector3Array":
			values, err := p.parseConstructFloat64(0)
			if err != nil {
				return nil, err
			}

			if len(values)%3 != 0 {
				return nil, errors.New("expected count divisible by 3")
			}

			l := len(values) / 3
			array := make(PackedVector3Array, 0, l)
			for i := 0; i < l; i++ {
				array = append(array, Vector3{
					X: values[i*3+0],
					Y: values[i*3+1],
					Z: values[i*3+2],
				})
			}

			return array, nil
		case "PackedVector4Array", "PoolVector4Array", "Vector4Array":
			values, err := p.parseConstructFloat64(0)
			if err != nil {
				return nil, err
			}

			if len(values)%4 != 0 {
				return nil, errors.New("expected count divisible by 4")
			}

			l := len(values) / 4
			array := make(PackedVector4Array, 0, l)
			for i := 0; i < l; i++ {
				array = append(array, Vector4{
					X: values[i*4+0],
					Y: values[i*4+1],
					Z: values[i*4+2],
					W: values[i*4+3],
				})
			}

			return array, nil
		case "PackedColorArray", "PoolColorArray", "ColorArray":
			values, err := p.parseConstructFloat32(0)
			if err != nil {
				return nil, err
			}

			if len(values)%4 != 0 {
				return nil, errors.New("expected count divisible by 4")
			}

			l := len(values) / 4
			array := make(PackedColorArray, 0, l)
			for i := 0; i < l; i++ {
				array = append(array, Color{
					R: values[i*4+0],
					G: values[i*4+1],
					B: values[i*4+2],
					A: values[i*4+3],
				})
			}

			return array, nil
		default:
			return nil, fmt.Errorf("invalid identifier: %s", t.Value)
		}
	default:
	}

	return nil, errors.New("unable to parse")
}

func (p *Parser) parseDictionary(f func(k any, v any) error) error {
	t, err := p.getToken()
	if err != nil {
		return err
	}
	if t.TokenType != tokenTypeCurlyBracketOpen {
		return errors.New("expected '{'")
	}

	var key any
	at_key := true
	need_comma := false

	for {
		if at_key {
			t, err = p.getToken()
			if err != nil {
				return err
			}
			if t.TokenType == tokenTypeCurlyBracketClose {
				break
			}

			if need_comma {
				if t.TokenType != tokenTypeComma {
					return errors.New("expected ',' or '}'")
				} else {
					need_comma = false
					continue
				}
			}

			err := p.saveToken(&t)
			if err != nil {
				return err
			}

			key, err = p.parseValueInternal()
			if err != nil {
				return err
			}
			t, err = p.getToken()
			if err != nil {
				return err
			}
			if t.TokenType != tokenTypeColon {
				return errors.New("expected ':'")
			}

			at_key = false
		} else {
			v, err := p.parseValueInternal()
			if err != nil {
				return err
			}

			err = f(key, v)
			if err != nil {
				return err
			}

			need_comma = true
			at_key = true
		}
	}

	return nil
}

func (p *Parser) parseArray(f func(v any) error) error {
	t, err := p.getToken()
	if err != nil {
		return err
	}
	if t.TokenType != tokenTypeBracketOpen {
		return errors.New("expected '['")
	}

	need_comma := false

	for {
		t, err := p.getToken()
		if err != nil {
			return err
		}

		if t.TokenType == tokenTypeBracketClose {
			break
		}

		if need_comma {
			if t.TokenType != tokenTypeComma {
				return errors.New("expected ','")
			} else {
				need_comma = false
				continue
			}
		}

		err = p.saveToken(&t)
		if err != nil {
			return err
		}

		v, err := p.parseValueInternal()
		if err != nil {
			return err
		}

		err = f(v)
		if err != nil {
			return err
		}

		need_comma = true
	}

	return nil
}

func (p *Parser) parseConstruct(f func(t token) error) error {
	t, err := p.getToken()
	if err != nil {
		return err
	}
	if t.TokenType != tokenTypeParenthesisOpen {
		return errors.New("expected '('")
	}

	first := true
	for {
		if !first {
			t, err = p.getToken()
			if err != nil {
				return err
			}
			if t.TokenType == tokenTypeComma {
				// pass
			} else if t.TokenType == tokenTypeParenthesisClose {
				break
			} else {
				return errors.New("expected ',' or ')'")
			}
		}

		t, err = p.getToken()
		if err != nil {
			return err
		}

		if t.TokenType == tokenTypeParenthesisClose {
			break
		}

		err = f(t)
		if err != nil {
			return err
		}

		first = false
	}

	return nil
}

func (p *Parser) parseConstructEmpty() error {
	f := func(t token) error {
		return errors.New("expected empty")
	}
	if err := p.parseConstruct(f); err != nil {
		return err
	}
	return nil
}

func (p *Parser) parseConstructFloat64(expectedCount int) ([]float64, error) {
	values := []float64{}
	f := func(t token) error {
		if t.TokenType != tokenTypeNumber {
			return errors.New("expected number")
		}
		var f float64
		switch v := t.Value.(type) {
		case float64:
			f = v
		case int64:
			f = float64(v)
		}
		values = append(values, f)
		return nil
	}
	if err := p.parseConstruct(f); err != nil {
		return nil, err
	}
	if expectedCount > 0 && len(values) != expectedCount {
		return nil, fmt.Errorf("expected %d values (found %d)", expectedCount, len(values))
	}
	return values, nil
}

func (p *Parser) parseConstructFloat32(expectedCount int) ([]float32, error) {
	values := []float32{}
	f := func(t token) error {
		if t.TokenType != tokenTypeNumber {
			return errors.New("expected number")
		}
		var f float32
		switch v := t.Value.(type) {
		case float64:
			f = float32(v)
		case int64:
			f = float32(v)
		}
		values = append(values, f)
		return nil
	}
	if err := p.parseConstruct(f); err != nil {
		return nil, err
	}
	if expectedCount > 0 && len(values) != expectedCount {
		return nil, fmt.Errorf("expected %d values (found %d)", expectedCount, len(values))
	}
	return values, nil
}

func (p *Parser) parseConstructInt64(expectedCount int) ([]int64, error) {
	values := []int64{}
	f := func(t token) error {
		if t.TokenType != tokenTypeNumber {
			return errors.New("expected number")
		}
		v, ok := t.Value.(int64)
		if !ok {
			return errors.New("expected integer")
		}
		values = append(values, v)
		return nil
	}
	if err := p.parseConstruct(f); err != nil {
		return nil, err
	}
	if expectedCount > 0 && len(values) != expectedCount {
		return nil, fmt.Errorf("expected %d values (found %d)", expectedCount, len(values))
	}
	return values, nil
}

func (p *Parser) parseConstructInt32(expectedCount int) ([]int32, error) {
	values := []int32{}
	f := func(t token) error {
		if t.TokenType != tokenTypeNumber {
			return errors.New("expected number")
		}
		v, ok := t.Value.(int64)
		if !ok {
			return errors.New("expected integer")
		}
		i32 := int32(v)
		if int64(i32) != v {
			return errors.New("integer overflow")
		}
		values = append(values, i32)
		return nil
	}
	if err := p.parseConstruct(f); err != nil {
		return nil, err
	}
	if expectedCount > 0 && len(values) != expectedCount {
		return nil, fmt.Errorf("expected %d values (found %d)", expectedCount, len(values))
	}
	return values, nil
}

func (p *Parser) parseConstructString(expectedCount int) ([]string, error) {
	values := []string{}
	f := func(t token) error {
		if t.TokenType != tokenTypeString {
			return errors.New("expected string")
		}
		v, ok := t.Value.(string)
		if !ok {
			return errors.New("expected string")
		}
		values = append(values, v)
		return nil
	}
	if err := p.parseConstruct(f); err != nil {
		return nil, err
	}
	if expectedCount > 0 && len(values) != expectedCount {
		return nil, fmt.Errorf("expected %d values (found %d)", expectedCount, len(values))
	}
	return values, nil
}

func (p *Parser) parseByteArray() (PackedByteArray, error) {
	t, err := p.getToken()
	if err != nil {
		return nil, err
	}
	if t.TokenType != tokenTypeParenthesisOpen {
		return nil, errors.New("expected '('")
	}

	t, err = p.getToken()
	if err != nil {
		return nil, err
	}

	if t.TokenType == tokenTypeString {
		s := t.Value.(string)
		d, err := base64.StdEncoding.DecodeString(s)
		if err != nil {
			return nil, err
		}

		t, err := p.getToken()
		if err != nil {
			return nil, err
		}
		if t.TokenType != tokenTypeParenthesisClose {
			return nil, errors.New("expected ')'")
		}

		return PackedByteArray(d), nil
	}

	array := PackedByteArray{}
	for {
		if t.TokenType != tokenTypeNumber {
			return nil, errors.New("expected integer")
		}

		orig, ok := t.Value.(int64)
		if !ok {
			return nil, errors.New("expected integer")
		}

		v := byte(orig)
		if int64(v) != orig {
			return nil, errors.New("integer overflow")
		}

		array = append(array, v)

		t, err = p.getToken()
		if err != nil {
			return nil, err
		}

		if t.TokenType == tokenTypeParenthesisClose {
			break
		}
		if t.TokenType != tokenTypeComma {
			return nil, errors.New("expected ',' or ')'")
		}

		t, err = p.getToken()
		if err != nil {
			return nil, err
		}
	}

	return array, nil
}

func (p *Parser) parseObject() (*Object, error) {
	t, err := p.getToken()
	if err != nil {
		return nil, err
	}
	if t.TokenType != tokenTypeParenthesisOpen {
		return nil, errors.New("expected '('")
	}

	t, err = p.getToken()
	if err != nil {
		return nil, err
	}
	if t.TokenType != tokenTypeIdentifier {
		return nil, errors.New("expected class name")
	}
	className := t.Value.(string)

	t, err = p.getToken()
	if err != nil {
		return nil, err
	}
	if t.TokenType != tokenTypeComma {
		return nil, errors.New("expected ','")
	}

	props := OrderedMap[string, any]{}

	at_key := true
	key := ""
	need_comma := false

	for {
		if at_key {
			t, err = p.getToken()
			if err != nil {
				return nil, err
			}
			if t.TokenType == tokenTypeParenthesisClose {
				break
			}

			if need_comma {
				if t.TokenType != tokenTypeComma {
					return nil, errors.New("expected ',' or ')'")
				} else {
					need_comma = false
					continue
				}
			}

			if t.TokenType != tokenTypeString {
				return nil, errors.New("expected property name as string")
			}

			key = t.Value.(string)

			t, err = p.getToken()
			if err != nil {
				return nil, err
			}
			if t.TokenType != tokenTypeColon {
				return nil, errors.New("expected ':'")
			}

			at_key = false
		} else {
			v, err := p.parseValueInternal()
			if err != nil {
				return nil, err
			}

			props.Set(key, v)
			need_comma = true
			at_key = true
		}
	}

	return &Object{
		ClassName:  className,
		Properties: props,
	}, nil
}

func (p *Parser) saveToken(saved *token) error {
	if p.savedToken != nil {
		return errors.New("internal error: unused saved token")
	}
	p.savedToken = saved
	return nil
}

func (p *Parser) getToken() (token, error) {
	if p.savedToken != nil {
		saved := p.savedToken
		p.savedToken = nil
		return *saved, nil
	}

	t := token{}
	is_stringname := false
	for {
		c, _, err := p.r.ReadRune()
		if err != nil {
			if err == io.EOF {
				t.TokenType = tokenTypeEOF
				return t, nil
			}
			return t, err
		}

		switch c {
		case '\n':
			p.line++
		case '{':
			t.TokenType = tokenTypeCurlyBracketOpen
			return t, nil
		case '}':
			t.TokenType = tokenTypeCurlyBracketClose
			return t, nil
		case '[':
			t.TokenType = tokenTypeBracketOpen
			return t, nil
		case ']':
			t.TokenType = tokenTypeBracketClose
			return t, nil
		case '(':
			t.TokenType = tokenTypeParenthesisOpen
			return t, nil
		case ')':
			t.TokenType = tokenTypeParenthesisClose
			return t, nil
		case ':':
			t.TokenType = tokenTypeColon
			return t, nil
		case ';':
			// Skip to end of line with comment.
			for {
				c, _, err = p.r.ReadRune()
				if err != nil {
					if err == io.EOF {
						t.TokenType = tokenTypeEOF
						return t, nil
					}
					return t, err
				}
				if c == '\n' {
					p.line++
					break
				}
			}
		case ',':
			t.TokenType = tokenTypeComma
			return t, nil
		case '.':
			t.TokenType = tokenTypePeriod
			return t, nil
		case '=':
			t.TokenType = tokenTypeEqual
			return t, nil
		case '#':
			for {
				c, _, err = p.r.ReadRune()
				if err != nil {
					if err == io.EOF {
						break
					}
					return t, err

				}
				if isAsciiHexChar(c) {
					p.buf.WriteRune(c)
				} else {
					p.r.UnreadRune()
					break
				}
			}

			s := p.popString()
			color, err := ColorFromHTML(s)
			if err != nil {
				return t, err
			}

			t.TokenType = tokenTypeColor
			t.Value = color
			return t, nil
		case '&', '@':
			is_stringname = true
			orig_c := c

			c, _, err := p.r.ReadRune()
			if err != nil {
				if err == io.EOF {
					return t, ErrUnterminatedString
				}
				return t, err
			}
			if c != '"' {
				return t, fmt.Errorf("expected '\"' after '%c'", orig_c)
			}

			fallthrough
		case '"':
			var prev rune
			for {
				c, _, err := p.r.ReadRune()
				if err != nil {
					if err == io.EOF {
						return t, ErrUnterminatedString
					}
					return t, err
				}

				if c == '"' {
					if prev != 0 {
						return t, ErrUnpairedUTF16Surrogate
					}
					t.TokenType = tokenTypeString
					s := p.popString()
					if is_stringname {
						t.Value = StringName(s)
					} else {
						t.Value = s
					}
					return t, nil
				} else if c == '\\' {
					// Escaped characters.
					c, _, err = p.r.ReadRune()
					if err != nil {
						if err == io.EOF {
							return t, ErrUnterminatedString
						}
						return t, err
					}

					switch c {
					case 'b':
						c = '\b'
					case 't':
						c = '\t'
					case 'n':
						c = '\n'
					case 'f':
						c = '\f'
					case 'r':
						c = '\r'
					case 'U', 'u':
						hex_len := 4
						if c == 'U' {
							hex_len = 6
						}
						tmpbuf := strings.Builder{}
						for j := 0; j < hex_len; j++ {
							c, _, err = p.r.ReadRune()
							if err != nil {
								if err == io.EOF {
									return t, ErrUnterminatedString
								}
								return t, err
							}
							if !isAsciiHexChar(c) {
								return t, errors.New("malformed hex constant in string")
							}
							tmpbuf.WriteRune(c)
						}
						s := tmpbuf.String()
						v, err := strconv.ParseInt(s, 16, 32)
						if err != nil {
							return t, err
						}
						c = rune(v)
					}

					// Make sequential `\uXXXX` characters into UTF-16 surrogate pairs.
					if (uint32(c) & uint32(0xfffffc00)) == 0xd800 {
						if prev == 0 {
							prev = c
							continue
						}
						return t, ErrUnpairedUTF16Surrogate
					} else if uint32(c)&uint32(0xfffffc00) == 0xdc00 {
						if prev == 0 {
							return t, ErrUnpairedUTF16Surrogate
						}
						c = rune((uint32(prev) << 10) + uint32(c) - ((0xd800 << 10) + 0xdc00 - 0x10000))
						prev = 0
					}
					if prev != 0 {
						return t, ErrUnpairedUTF16Surrogate
					}
				} else {
					if prev != 0 {
						return t, ErrUnpairedUTF16Surrogate
					}
					if c == '\n' {
						p.line++
					}
				}

				p.buf.WriteRune(c)
			}
		default:
		}

		if !unicode.IsPrint(c) {
			continue
		}

		if c == '-' {
			p.buf.WriteRune(c)
			continue
		}

		if unicode.IsDigit(c) {
			const (
				readingInt = iota
				readingDec
				readingExp
				readingDone
			)

			reading := readingInt
			is_float := false
			exp_sign := false
			exp_beg := false

			for {
				switch reading {
				case readingInt:
					if unicode.IsDigit(c) {
						// pass
					} else if c == '.' {
						reading = readingDec
						is_float = true
					} else if c == 'e' || c == 'E' {
						reading = readingExp
						is_float = true
					} else {
						p.r.UnreadRune()
						reading = readingDone
					}
				case readingDec:
					if unicode.IsDigit(c) {
						// pass
					} else if c == 'e' || c == 'E' {
						reading = readingExp
					} else {
						p.r.UnreadRune()
						reading = readingDone
					}
				case readingExp:
					if unicode.IsDigit(c) {
						exp_beg = true
					} else if (c == '-' || c == '+') && !exp_sign && !exp_beg {
						exp_sign = true
					} else {
						p.r.UnreadRune()
						reading = readingDone
					}
				}

				if reading == readingDone {
					break
				}

				p.buf.WriteRune(c)

				c, _, err = p.r.ReadRune()
				if err != nil {
					if err == io.EOF {
						break
					}
					return t, err
				}
			}

			s := p.popString()
			if is_float {
				v, err := strconv.ParseFloat(s, 64)
				if err != nil {
					return t, err
				}
				t.Value = v
			} else {
				v, err := strconv.ParseInt(s, 10, 64)
				if err != nil {
					return t, err
				}
				t.Value = v
			}
			t.TokenType = tokenTypeNumber
			return t, nil
		} else if isAsciiAlphabetChar(c) || c == '_' {
			first := true
			for isAsciiAlphabetChar(c) || c == '_' || (!first && unicode.IsDigit(c)) {
				p.buf.WriteRune(c)
				c, _, err = p.r.ReadRune()
				if err != nil {
					if err == io.EOF {
						break
					}
					return t, err
				}
				first = false
			}
			p.r.UnreadRune()
			t.TokenType = tokenTypeIdentifier
			t.Value = p.popString()
			return t, nil
		} else if unicode.IsSpace(c) {
			// pass
		} else {
			return t, fmt.Errorf("unexpected character '%c'", c)
		}
	}
}

func (p *Parser) popString() string {
	s := p.buf.String()
	p.buf.Reset()
	return s
}

func isAsciiAlphabetChar(c rune) bool {
	return (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z')
}

func isAsciiHexChar(c rune) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}
