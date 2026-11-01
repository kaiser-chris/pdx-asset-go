package mesh

import (
	"encoding/binary"
	"fmt"
	"math"
)

// Encode writes a tree of objects as a binary mesh file, the way Decode reads
// one: the root stands for the file, so its properties come before the first
// object and its children are the objects of the top level.
//
// Only what the format cannot hold is refused: a property name longer than
// the one byte its length is written in, a name of an object with a zero byte
// in it, or a property of an unknown kind.
func Encode(root *Object) ([]byte, error) {
	data := []byte(magic)

	var err error

	data, err = encodeProperties(data, root)
	if err != nil {
		return nil, err
	}

	for _, child := range root.Children {
		data, err = encodeObject(data, child, 1)
		if err != nil {
			return nil, err
		}
	}

	return data, nil
}

// encodeObject appends an object, nested depth levels deep, and everything
// below it.
func encodeObject(data []byte, object *Object, depth int) ([]byte, error) {
	if depth > MaxDepth {
		return nil, fmt.Errorf("object %s is nested %d deep, more than the %d a file may hold", object.Name, depth, MaxDepth)
	}

	for index := range len(object.Name) {
		if object.Name[index] == 0 {
			return nil, fmt.Errorf("the name of object %q holds a zero byte, which would end it", object.Name)
		}
	}

	for range depth {
		data = append(data, '[')
	}

	data = append(data, object.Name...)
	data = append(data, 0)

	data, err := encodeProperties(data, object)
	if err != nil {
		return nil, err
	}

	for _, child := range object.Children {
		data, err = encodeObject(data, child, depth+1)
		if err != nil {
			return nil, err
		}
	}

	return data, nil
}

// encodeProperties appends the properties of an object.
func encodeProperties(data []byte, object *Object) ([]byte, error) {
	for _, property := range object.Properties {
		if len(property.Name) > math.MaxUint8 {
			return nil, fmt.Errorf("property %s of object %s has a name longer than the %d bytes a file can hold", property.Name, object.Name, math.MaxUint8)
		}

		data = append(data, '!', byte(len(property.Name)))
		data = append(data, property.Name...)
		data = append(data, byte(property.Kind))
		data = binary.LittleEndian.AppendUint32(data, uint32(property.Len()))

		switch property.Kind {
		case Floats:
			for _, value := range property.Floats {
				data = binary.LittleEndian.AppendUint32(data, math.Float32bits(value))
			}

		case Ints:
			for _, value := range property.Ints {
				data = binary.LittleEndian.AppendUint32(data, uint32(value))
			}

		case Strings:
			// The length of a string counts the zero that ends it, as the
			// shipped files write it.
			for _, value := range property.Strings {
				data = binary.LittleEndian.AppendUint32(data, uint32(len(value)+1))
				data = append(data, value...)
				data = append(data, 0)
			}

		default:
			return nil, fmt.Errorf("property %s of object %s is of unknown %s", property.Name, object.Name, property.Kind)
		}
	}

	return data, nil
}
