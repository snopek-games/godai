package variant

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

type StringName string
type Vector2 struct {
	X, Y float64 // real_t
}

type Vector2i struct {
	X, Y int32
}

type Vector3 struct {
	X, Y, Z float64 // real_t
}

type Vector3i struct {
	X, Y, Z int32
}

type Vector4 struct {
	X, Y, Z, W float64 // real_t
}

type Vector4i struct {
	X, Y, Z, W int32
}

type Rect2 struct {
	Position, Size Vector2
}

type Rect2i struct {
	Position, Size Vector2i
}

type Plane struct {
	Normal Vector3
	D      float64 // real_t
}

type Quaternion struct {
	X, Y, Z, W float64 // real_t
}

type AABB struct {
	Position, Size Vector3
}

type Transform2D struct {
	Columns [3]Vector2
}

type Basis struct {
	Rows [3]Vector3
}

type Transform3D struct {
	Basis  Basis
	Origin Vector3
}

type Projection struct {
	Columns [4]Vector4
}

type Color struct {
	R, G, B, A float32
}

type NodePath struct {
	Path     []StringName
	Subpath  []StringName
	Absolute bool
}

type RID int64
type ObjectID int64

type Signal struct {
	Name     StringName
	ObjectID ObjectID
}

type Callable struct{}

type Object struct {
	ClassName  string
	Properties OrderedMap[string, any]
}

type Dictionary = OrderedMap[any, any]
type Array []any

type TypedArray[T any] struct {
	ClassName string
	Items     []T
}

type TypedDictionary[K comparable, V any] struct {
	KeyClassName   string
	ValueClassName string
	Items          map[K]V
}

type PackedByteArray []byte
type PackedInt32Array []int32
type PackedInt64Array []int64
type PackedFloat32Array []float32
type PackedFloat64Array []float64
type PackedStringArray []string
type PackedVector2Array []Vector2
type PackedVector3Array []Vector3
type PackedVector4Array []Vector4
type PackedColorArray []Color

var ErrInvalidColor = errors.New("invalid color")

func Color8(r, g, b, a uint8) Color {
	return Color{
		float32(r) / 255.0,
		float32(g) / 255.0,
		float32(b) / 255.0,
		float32(a) / 255.0,
	}
}

func ColorFromHTML(s string) (Color, error) {
	c := Color{0, 0, 0, 255}
	if len(s) == 0 {
		return c, ErrInvalidColor
	}

	if s[0] == '#' {
		s = s[1:]
	}

	var expanded string
	switch len(s) {
	case 3:
		expanded = fmt.Sprintf("%c%c%c%c%c%cFF", s[0], s[0], s[1], s[1], s[2], s[2])
	case 4:
		expanded = fmt.Sprintf("%c%c%c%c%c%c%c%c", s[0], s[0], s[1], s[1], s[2], s[2], s[3], s[3])
	case 6:
		expanded = s + "FF"
	case 8:
		expanded = s
	default:
		return c, ErrInvalidColor
	}

	v, err := strconv.ParseUint(expanded, 16, 32)
	if err != nil {
		return c, ErrInvalidColor
	}

	return Color8(
		uint8(v>>24),
		uint8(v>>16),
		uint8(v>>8),
		uint8(v),
	), nil
}

func ParseNodePath(path string) (NodePath, error) {
	if len(path) == 0 {
		return NodePath{}, nil
	}

	isAbsolute := (path[0] == '/')

	parts := strings.Split(path, ":")
	subpath_parts := parts[1:]
	path_parts := strings.Split(parts[0], "/")

	subpath_sname_parts := make([]StringName, 0, len(subpath_parts))
	for _, p := range subpath_parts {
		if len(p) == 0 {
			return NodePath{}, errors.New("empty subpath element")
		}
		subpath_sname_parts = append(subpath_sname_parts, StringName(p))
	}

	var i int
	if isAbsolute {
		i = 1
	}
	path_sname_parts := make([]StringName, 0, len(path_parts))
	for ; i < len(path_parts); i++ {
		p := path_parts[i]
		if len(p) == 0 {
			return NodePath{}, errors.New("empty path element")
		}
		path_sname_parts = append(path_sname_parts, StringName(p))
	}

	return NodePath{
		Path:     path_sname_parts,
		Subpath:  subpath_sname_parts,
		Absolute: isAbsolute,
	}, nil
}

func (np *NodePath) isEmpty() bool {
	return np.Path == nil && np.Subpath == nil && !np.Absolute
}

func (np *NodePath) String() string {
	sb := strings.Builder{}

	if np.Absolute {
		sb.WriteRune('/')
	}
	if len(np.Path) > 0 {
		for i, p := range np.Path {
			sb.WriteString(string(p))
			if i < len(np.Path)-1 {
				sb.WriteRune('/')
			}
		}
	}
	if len(np.Subpath) > 0 {
		for _, p := range np.Subpath {
			sb.WriteRune(':')
			sb.WriteString(string(p))
		}
	}
	return sb.String()
}
