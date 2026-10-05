package mesh

import (
	"encoding/binary"
	"fmt"
	"math"
)

// magic starts every binary mesh file.
const magic = "@@b@"

// MaxDepth bounds how deeply objects nest. The shipped files go four deep; a
// file that claims more is broken.
const MaxDepth = 64

// Object is one object of a mesh file: a name, the properties it holds and the
// objects below it.
type Object struct {
	Name       string
	Properties []Property
	Children   []*Object
}

// Property is one property of an object: a name and an array of values, which
// are numbers with a fraction, whole numbers or strings.
type Property struct {
	Name string
	Kind Kind

	// Only the slice of the property's kind is filled.
	Floats  []float32
	Ints    []int32
	Strings []string
}

// Kind is what a property holds.
type Kind byte

const (
	Floats  Kind = 'f'
	Ints    Kind = 'i'
	Strings Kind = 's'
)

func (k Kind) String() string {
	switch k {
	case Floats:
		return "floats"
	case Ints:
		return "ints"
	case Strings:
		return "strings"
	}

	return fmt.Sprintf("kind %q", byte(k))
}

// Len is how many values the property holds.
func (p Property) Len() int {
	switch p.Kind {
	case Floats:
		return len(p.Floats)
	case Ints:
		return len(p.Ints)
	case Strings:
		return len(p.Strings)
	}

	return 0
}

// Property returns the property of the given name.
func (o *Object) Property(name string) (Property, bool) {
	for _, property := range o.Properties {
		if property.Name == name {
			return property, true
		}
	}

	return Property{}, false
}

// Child returns the first object below this one with the given name.
func (o *Object) Child(name string) (*Object, bool) {
	for _, child := range o.Children {
		if child.Name == name {
			return child, true
		}
	}

	return nil, false
}

// Decode reads a binary mesh file into its tree of objects. The returned
// object stands for the file itself: it holds the properties written before
// the first object, and the objects of the top level as its children.
//
// The file is a sequence of two kinds of record. An object starts with one [
// for each level it is nested at, then its name, ending at a zero byte; the
// records after it belong to it until an object at its own level or above
// starts. A property starts with !, then the length of its name in one byte,
// the name, one byte for its kind, a count of its values, and the values:
// four bytes each for numbers, and for strings a length followed by that many
// bytes, the last of them a zero.
//
// A file that does not keep to that is refused, with where it went wrong. A
// count is checked against what is left of the file before anything is
// allocated for it, so a broken count cannot ask for gigabytes.
func Decode(data []byte) (*Object, error) {
	if len(data) < len(magic) || string(data[:len(magic)]) != magic {
		return nil, fmt.Errorf("not a binary mesh file: it does not start with %q", magic)
	}

	reader := &reader{data: data, offset: len(magic)}
	root := &Object{}

	// open holds the objects that records currently belong to, the file
	// itself first.
	open := []*Object{root}

	for reader.offset < len(data) {
		switch data[reader.offset] {
		case '[':
			depth, name, err := reader.object()
			if err != nil {
				return nil, err
			}

			if depth > len(open) {
				return nil, fmt.Errorf("object %s at %d is nested %d deep below an object %d deep", name, reader.offset, depth, len(open)-1)
			}

			if depth > MaxDepth {
				return nil, fmt.Errorf("object %s at %d is nested %d deep, more than the %d this reads", name, reader.offset, depth, MaxDepth)
			}

			object := &Object{Name: name}
			parent := open[depth-1]
			parent.Children = append(parent.Children, object)
			open = append(open[:depth], object)

		case '!':
			property, err := reader.property()
			if err != nil {
				return nil, err
			}

			current := open[len(open)-1]
			current.Properties = append(current.Properties, property)

		default:
			return nil, fmt.Errorf("unexpected byte %#x at %d, where an object or a property should start", data[reader.offset], reader.offset)
		}
	}

	return root, nil
}

// reader walks a file.
type reader struct {
	data   []byte
	offset int
}

// left is how many bytes there are still to read.
func (r *reader) left() int {
	return len(r.data) - r.offset
}

// take returns the next count bytes.
func (r *reader) take(count int) ([]byte, bool) {
	if count < 0 || count > r.left() {
		return nil, false
	}

	taken := r.data[r.offset : r.offset+count]
	r.offset += count

	return taken, true
}

// object reads the start of an object: one bracket per level, then its name.
func (r *reader) object() (depth int, name string, err error) {
	start := r.offset

	for r.offset < len(r.data) && r.data[r.offset] == '[' {
		depth++
		r.offset++
	}

	for index := r.offset; index < len(r.data); index++ {
		if r.data[index] == 0 {
			name = string(r.data[r.offset:index])
			r.offset = index + 1

			return depth, name, nil
		}
	}

	return 0, "", fmt.Errorf("the name of the object at %d runs to the end of the file", start)
}

// property reads one property.
func (r *reader) property() (Property, error) {
	start := r.offset
	r.offset++ // the exclamation mark

	length, ok := r.take(1)
	if !ok {
		return Property{}, fmt.Errorf("the property at %d has no name", start)
	}

	name, ok := r.take(int(length[0]))
	if !ok {
		return Property{}, fmt.Errorf("the name of the property at %d runs to the end of the file", start)
	}

	property := Property{Name: string(name)}

	kind, ok := r.take(1)
	if !ok {
		return Property{}, fmt.Errorf("property %s at %d has no kind", property.Name, start)
	}

	property.Kind = Kind(kind[0])

	count, err := r.count()
	if err != nil {
		return Property{}, fmt.Errorf("property %s at %d: %w", property.Name, start, err)
	}

	switch property.Kind {
	case Floats, Ints:
		values, ok := r.take(count * 4)
		if !ok {
			return Property{}, fmt.Errorf("property %s at %d wants %d numbers, more than the file holds", property.Name, start, count)
		}

		if property.Kind == Floats {
			property.Floats = make([]float32, count)
			for index := range property.Floats {
				property.Floats[index] = math.Float32frombits(binary.LittleEndian.Uint32(values[index*4:]))
			}
		} else {
			property.Ints = make([]int32, count)
			for index := range property.Ints {
				property.Ints[index] = int32(binary.LittleEndian.Uint32(values[index*4:]))
			}
		}

	case Strings:
		// Every string takes at least the four bytes of its length.
		if count > r.left()/4 {
			return Property{}, fmt.Errorf("property %s at %d wants %d strings, more than the file holds", property.Name, start, count)
		}

		property.Strings = make([]string, 0, count)

		for range count {
			size, err := r.count()
			if err != nil {
				return Property{}, fmt.Errorf("property %s at %d: %w", property.Name, start, err)
			}

			text, ok := r.take(size)
			if !ok {
				return Property{}, fmt.Errorf("a string of property %s at %d wants %d bytes, more than the file holds", property.Name, start, size)
			}

			// The length counts the zero that ends the string.
			if len(text) > 0 && text[len(text)-1] == 0 {
				text = text[:len(text)-1]
			}

			property.Strings = append(property.Strings, string(text))
		}

	default:
		return Property{}, fmt.Errorf("property %s at %d is of unknown kind %q", property.Name, start, kind[0])
	}

	return property, nil
}

// count reads one count, which the file writes as four bytes.
func (r *reader) count() (int, error) {
	value, ok := r.take(4)
	if !ok {
		return 0, fmt.Errorf("a count at %d runs to the end of the file", r.offset)
	}

	count := binary.LittleEndian.Uint32(value)

	// Anything larger than the file itself cannot be backed by it, whatever
	// it counts, and refusing it here keeps the arithmetic on it in range.
	if int64(count) > int64(len(r.data)) {
		return 0, fmt.Errorf("a count of %d at %d is larger than the file", count, r.offset-4)
	}

	return int(count), nil
}
