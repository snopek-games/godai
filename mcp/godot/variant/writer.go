package variant

import (
	"bufio"
	"encoding/base64"
	"fmt"
	"io"
	"math"
	"reflect"
	"strconv"
	"strings"
)

type Writer struct {
	w *bufio.Writer
}

func NewWriter(w io.Writer) *Writer {
	bw, ok := w.(*bufio.Writer)
	if !ok {
		bw = bufio.NewWriter(w)
	}
	return &Writer{
		w: bw,
	}
}

func (w *Writer) Flush() error {
	return w.w.Flush()
}

func (w *Writer) WriteValue(x any) error {
	err := w.writeValueInternal(x)
	if err != nil {
		return err
	}
	return nil
}

func (w *Writer) WriteComment(s string) error {
	if len(s) == 0 || s[0] != ';' {
		if _, err := w.w.WriteRune(';'); err != nil {
			return err
		}
	}
	if _, err := w.w.WriteString(s); err != nil {
		return err
	}
	if _, err := w.w.WriteRune('\n'); err != nil {
		return err
	}
	return nil
}

func (w *Writer) WriteTag(name string, fields map[string]any) error {
	if _, err := w.w.WriteRune('['); err != nil {
		return err
	}
	if _, err := w.w.WriteString(name); err != nil {
		return err
	}
	if len(fields) > 0 {
		for k, v := range fields {
			if _, err := w.w.WriteRune(' '); err != nil {
				return err
			}
			if _, err := w.w.WriteString(k); err != nil {
				return err
			}
			if _, err := w.w.WriteRune('='); err != nil {
				return err
			}
			if err := w.writeValueInternal(v); err != nil {
				return err
			}
		}
	}
	if _, err := w.w.WriteString("]\n"); err != nil {
		return err
	}
	return nil
}

func (w *Writer) WriteAssignment(name string, value any) error {
	if _, err := w.w.WriteString(name); err != nil {
		return err
	}
	if _, err := w.w.WriteRune('='); err != nil {
		return err
	}
	if err := w.writeValueInternal(value); err != nil {
		return err
	}
	if _, err := w.w.WriteRune('\n'); err != nil {
		return err
	}
	return nil
}

func (w *Writer) writeValueInternal(x any) error {
	switch v := x.(type) {
	case nil:
		if _, err := w.w.WriteString("null"); err != nil {
			return err
		}
	case bool:
		if v {
			if _, err := w.w.WriteString("true"); err != nil {
				return err
			}
		} else {
			if _, err := w.w.WriteString("false"); err != nil {
				return err
			}
		}
	case int, int8, int16, int32, int64:
		i := reflect.ValueOf(v).Int()
		if _, err := w.w.WriteString(itos64(i)); err != nil {
			return err
		}
	case float32:
		if _, err := w.w.WriteString(explicitFloatString(rtos32(v))); err != nil {
			return err
		}
	case float64:
		if _, err := w.w.WriteString(explicitFloatString(rtos64(v))); err != nil {
			return err
		}
	case string:
		if _, err := w.w.WriteString(escapedStringMultiline(v)); err != nil {
			return err
		}
	case Vector2:
		err := writeConstruct(w.w, "Vector2", rtos64, []float64{v.X, v.Y})
		if err != nil {
			return err
		}
	case Vector2i:
		err := writeConstruct(w.w, "Vector2i", itos32, []int32{v.X, v.Y})
		if err != nil {
			return err
		}
	case Rect2:
		err := writeConstruct(w.w, "Rect2", rtos64, []float64{v.Position.X, v.Position.Y, v.Size.X, v.Size.Y})
		if err != nil {
			return err
		}
	case Rect2i:
		err := writeConstruct(w.w, "Rect2i", itos32, []int32{v.Position.X, v.Position.Y, v.Size.X, v.Size.Y})
		if err != nil {
			return err
		}
	case Vector3:
		err := writeConstruct(w.w, "Vector3", rtos64, []float64{v.X, v.Y, v.Z})
		if err != nil {
			return err
		}
	case Vector3i:
		err := writeConstruct(w.w, "Vector3i", itos32, []int32{v.X, v.Y, v.Z})
		if err != nil {
			return err
		}
	case Vector4:
		err := writeConstruct(w.w, "Vector4", rtos64, []float64{v.X, v.Y, v.Z, v.W})
		if err != nil {
			return err
		}
	case Vector4i:
		err := writeConstruct(w.w, "Vector4i", itos32, []int32{v.X, v.Y, v.Z, v.W})
		if err != nil {
			return err
		}
	case Plane:
		err := writeConstruct(w.w, "Plane", rtos64, []float64{v.Normal.X, v.Normal.Y, v.Normal.Z, v.D})
		if err != nil {
			return err
		}
	case AABB:
		err := writeConstruct(w.w, "AABB", rtos64, []float64{v.Position.X, v.Position.Y, v.Position.Z, v.Size.X, v.Size.Y, v.Size.Z})
		if err != nil {
			return err
		}
	case Quaternion:
		err := writeConstruct(w.w, "Quaternion", rtos64, []float64{v.X, v.Y, v.Z, v.W})
		if err != nil {
			return err
		}
	case Transform2D:
		err := writeConstruct(w.w, "Transform2D", rtos64, []float64{
			v.Columns[0].X,
			v.Columns[0].Y,
			v.Columns[1].X,
			v.Columns[1].Y,
			v.Columns[2].X,
			v.Columns[2].Y,
		})
		if err != nil {
			return err
		}
	case Basis:
		err := writeConstruct(w.w, "Basis", rtos64, []float64{
			v.Rows[0].X,
			v.Rows[0].Y,
			v.Rows[0].Z,
			v.Rows[1].X,
			v.Rows[1].Y,
			v.Rows[1].Z,
			v.Rows[2].X,
			v.Rows[2].Y,
			v.Rows[2].Z,
		})
		if err != nil {
			return err
		}
	case Transform3D:
		err := writeConstruct(w.w, "Transform3D", rtos64, []float64{
			v.Basis.Rows[0].X,
			v.Basis.Rows[0].Y,
			v.Basis.Rows[0].Z,
			v.Basis.Rows[1].X,
			v.Basis.Rows[1].Y,
			v.Basis.Rows[1].Z,
			v.Basis.Rows[2].X,
			v.Basis.Rows[2].Y,
			v.Basis.Rows[2].Z,
			v.Origin.X,
			v.Origin.Y,
			v.Origin.Z,
		})
		if err != nil {
			return err
		}
	case Projection:
		err := writeConstruct(w.w, "Projection", rtos64, []float64{
			v.Columns[0].X,
			v.Columns[0].Y,
			v.Columns[0].Z,
			v.Columns[0].W,
			v.Columns[1].X,
			v.Columns[1].Y,
			v.Columns[1].Z,
			v.Columns[1].W,
			v.Columns[2].X,
			v.Columns[2].Y,
			v.Columns[2].Z,
			v.Columns[2].W,
			v.Columns[3].X,
			v.Columns[3].Y,
			v.Columns[3].Z,
			v.Columns[3].W,
		})
		if err != nil {
			return err
		}
	case Color:
		err := writeConstruct(w.w, "Color", rtos32, []float32{v.R, v.G, v.B, v.A})
		if err != nil {
			return err
		}
	case StringName:
		if _, err := w.w.WriteRune('&'); err != nil {
			return err
		}
		if _, err := w.w.WriteString(escapedString(string(v))); err != nil {
			return err
		}
	case NodePath:
		s := v.String()
		if _, err := w.w.WriteString("NodePath("); err != nil {
			return err
		}
		if _, err := w.w.WriteString(escapedString(s)); err != nil {
			return err
		}
		if _, err := w.w.WriteRune(')'); err != nil {
			return err
		}
	case RID:
		if v == 0 {
			if _, err := w.w.WriteString("RID()"); err != nil {
				return err
			}
		} else {
			if _, err := w.w.WriteString("RID("); err != nil {
				return err
			}
			if _, err := w.w.WriteString(itos64(int64(v))); err != nil {
				return err
			}
			if _, err := w.w.WriteRune(')'); err != nil {
				return err
			}
		}
	case Signal:
		if _, err := w.w.WriteString("Signal()"); err != nil {
			return err
		}
	case Callable:
		if _, err := w.w.WriteString("Callable()"); err != nil {
			return err
		}
	case Object:
		if _, err := w.w.WriteString("Object("); err != nil {
			return err
		}
		if _, err := w.w.WriteString(v.ClassName); err != nil {
			return err
		}
		for _, item := range v.Properties {
			if _, err := w.w.WriteRune(','); err != nil {
				return err
			}
			if err := w.writeValueInternal(item.Key); err != nil {
				return err
			}
			if _, err := w.w.WriteRune(':'); err != nil {
				return err
			}
			if err := w.writeValueInternal(item.Value); err != nil {
				return err
			}
		}
		if _, err := w.w.WriteRune(')'); err != nil {
			return err
		}
	case Dictionary:
		if _, err := w.w.WriteString("{\n"); err != nil {
			return err
		}
		first := true
		for _, item := range v {
			if !first {
				if _, err := w.w.WriteString(",\n"); err != nil {
					return err
				}
			}
			if err := w.writeValueInternal(item.Key); err != nil {
				return err
			}
			if _, err := w.w.WriteString(": "); err != nil {
				return err
			}
			if err := w.writeValueInternal(item.Value); err != nil {
				return err
			}
			first = false
		}
		if _, err := w.w.WriteString("\n}"); err != nil {
			return err
		}
	case Array:
		if len(v) > 0 {
			if _, err := w.w.WriteRune('['); err != nil {
				return err
			}
			first := true
			for _, item := range v {
				if !first {
					if _, err := w.w.WriteString("\n, "); err != nil {
						return err
					}
				}
				if err := w.writeValueInternal(item); err != nil {
					return err
				}
				first = false
			}
			if _, err := w.w.WriteString("\n]"); err != nil {
				return err
			}
		} else {
			if _, err := w.w.WriteString("[]"); err != nil {
				return err
			}
		}
	// @todo Resource
	// @todo TypedDictionary
	// @todo TypedArray
	case PackedByteArray:
		// @todo Support the "compat" mode too where it doesn't do base64.
		if _, err := w.w.WriteString("PackedByteArray(\""); err != nil {
			return err
		}
		s := base64.StdEncoding.EncodeToString([]byte(v))
		if _, err := w.w.WriteString(s); err != nil {
			return err
		}
		if _, err := w.w.WriteString("\")"); err != nil {
			return err
		}
	case PackedInt32Array:
		err := writeConstruct(w.w, "PackedInt32Array", itos32, []int32(v))
		if err != nil {
			return err
		}
	case PackedInt64Array:
		err := writeConstruct(w.w, "PackedInt64Array", itos64, []int64(v))
		if err != nil {
			return err
		}
	case PackedFloat32Array:
		err := writeConstruct(w.w, "PackedFloat32Array", rtos32, []float32(v))
		if err != nil {
			return err
		}
	case PackedFloat64Array:
		err := writeConstruct(w.w, "PackedFloat64Array", rtos64, []float64(v))
		if err != nil {
			return err
		}
	case PackedStringArray:
		err := writeConstruct(w.w, "PackedStringArray", escapedString, []string(v))
		if err != nil {
			return err
		}
	case PackedVector2Array:
		arr := make([]float64, 0, len(v)*2)
		for _, a := range v {
			arr = append(arr, a.X, a.Y)
		}
		err := writeConstruct(w.w, "PackedVector2Array", rtos64, arr)
		if err != nil {
			return err
		}
	case PackedVector3Array:
		arr := make([]float64, 0, len(v)*3)
		for _, a := range v {
			arr = append(arr, a.X, a.Y, a.Z)
		}
		err := writeConstruct(w.w, "PackedVector3Array", rtos64, arr)
		if err != nil {
			return err
		}
	case PackedVector4Array:
		arr := make([]float64, 0, len(v)*4)
		for _, a := range v {
			arr = append(arr, a.X, a.Y, a.Z, a.W)
		}
		err := writeConstruct(w.w, "PackedVector4Array", rtos64, arr)
		if err != nil {
			return err
		}
	case PackedColorArray:
		arr := make([]float32, 0, len(v)*4)
		for _, a := range v {
			arr = append(arr, a.R, a.G, a.B, a.A)
		}
		err := writeConstruct(w.w, "PackedColorArray", rtos32, arr)
		if err != nil {
			return err
		}
	default:
		return fmt.Errorf("unable to write '%T'", x)
	}

	return nil
}

func itos32(v int32) string {
	return strconv.FormatInt(int64(v), 10)
}

func itos64(v int64) string {
	return strconv.FormatInt(v, 10)
}

func rtosv(v float64, prec int, bits int) string {
	if v == 0 {
		return "0"
	} else if math.IsInf(v, 1) {
		return "inf"
	} else if math.IsInf(v, -1) {
		return "-inf"
	} else if math.IsNaN(v) {
		return "nan"
	}

	return strconv.FormatFloat(v, 'g', prec, bits)
}

func rtos64(v float64) string {
	return rtosv(v, 16, 64)
}

func rtos32(v float32) string {
	return rtosv(float64(v), 7, 32)
}

func explicitFloatString(s string) string {
	if s != "inf" && s != "-inf" && s != "nan" && !strings.Contains(s, ".") && !strings.Contains(s, "e") {
		s += ".0"
	}
	return s
}

func escapedStringMultiline(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "\"", "\\\"")
	return "\"" + s + "\""
}

func escapedString(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "\a", "\\a")
	s = strings.ReplaceAll(s, "\b", "\\b")
	s = strings.ReplaceAll(s, "\f", "\\f")
	s = strings.ReplaceAll(s, "\n", "\\n")
	s = strings.ReplaceAll(s, "\r", "\\r")
	s = strings.ReplaceAll(s, "\t", "\\t")
	s = strings.ReplaceAll(s, "\v", "\\v")
	s = strings.ReplaceAll(s, "'", "\\'")
	s = strings.ReplaceAll(s, "\"", "\\\"")
	return "\"" + s + "\""
}

func writeConstruct[T any](w *bufio.Writer, s string, f func(v T) string, l []T) error {
	if _, err := w.WriteString(s); err != nil {
		return err
	}
	if _, err := w.WriteRune('('); err != nil {
		return err
	}

	first := true
	for _, v := range l {
		if !first {
			if _, err := w.WriteString(", "); err != nil {
				return err
			}
		}

		if _, err := w.WriteString(f(v)); err != nil {
			return err
		}

		first = false
	}

	if _, err := w.WriteRune(')'); err != nil {
		return err
	}

	return nil
}
