// Copyright 2019 The ebml-go authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package ebml

import (
	"bytes"
	"errors"
	"io"
	"reflect"
)

// ErrUnknownElement means that a decoded element is not known.
var ErrUnknownElement = errors.New("unknown element")

// ErrIncompatibleType means that an element is not convertible to a corresponding struct field.
var ErrIncompatibleType = errors.New("marshal/unmarshal to incompatible type")

// ErrInvalidElementSize means that an element has inconsistent size. e.g. element size is larger than its parent element size.
var ErrInvalidElementSize = errors.New("invalid element size")

// ErrReadStopped is returned if unmarshaler finished to read element which has stop tag.
var ErrReadStopped = errors.New("read stopped")

// Element represents an EBML element.
type Element struct {
	Value    interface{}
	Name     string
	Type     ElementType
	Position uint64
	Size     uint64
	Parent   *Element
}

type Handler func(elem *Element)

func ReadElements(r io.Reader, handler Handler) error {
	var ret struct {
		Segment Segment `ebml:"Segment"`
	}

	vd := &valueDecoder{}
	vo := reflect.ValueOf(&ret)
	voe := vo.Elem()
	for {
		if _, err := vd.readElement(r, SizeUnknown, voe, 0, 0, nil, handler); err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
	}
}

func (vd *valueDecoder) readElement(r io.Reader, n int64, vo reflect.Value, depth int, pos uint64, parent *Element, handler Handler) (io.Reader, error) {
	pos0 := pos
	if n != SizeUnknown {
		r = io.LimitReader(r, n)
	}

	type fieldDef struct {
		v    reflect.Value
		stop bool
	}
	fieldMap := make(map[ElementType]fieldDef)
	switch vo.Kind() {
	case reflect.Struct:
		for i := 0; i < vo.NumField(); i++ {
			f := fieldDef{
				v: vo.Field(i),
			}
			var name string
			if n, ok := vo.Type().Field(i).Tag.Lookup("ebml"); ok {
				t, err := parseTag(n)
				if err != nil {
					return nil, err
				}
				name = t.name
				f.stop = t.stop
			}
			if name == "" {
				name = vo.Type().Field(i).Name
			}
			t, err := ElementTypeFromString(name)
			if err != nil {
				return nil, err
			}
			fieldMap[t] = f
		}
	}

	for {
		var headerSize uint64
		e, nb, err := vd.readVUInt(r)
		headerSize += uint64(nb)
		if err != nil {
			if nb == 0 && err == io.ErrUnexpectedEOF {
				return nil, io.EOF
			}
			return nil, err
		}
		v, ok := revTable[uint32(e)]
		if !ok {
			return nil, wrapErrorf(ErrUnknownElement, "unmarshalling element 0x%x", e)
		}

		size, nb, err := vd.readDataSize(r)
		headerSize += uint64(nb)

		if n != SizeUnknown && pos+headerSize+size > pos0+uint64(n) {
			err = ErrInvalidElementSize
		}

		if err != nil {
			return nil, err
		}

		var vnext reflect.Value
		var stopHere bool
		if vn, ok := fieldMap[v.e]; ok {
			vnext = vn.v
			stopHere = vn.stop
		}

		var elem *Element
		if vnext.IsValid() {
			elem = &Element{
				Name:     v.e.String(),
				Type:     v.e,
				Position: pos,
				Size:     size,
				Parent:   parent,
			}
		}

		switch v.t {
		case DataTypeMaster:
			if v.top && depth > 1 {
				b := bytes.Join([][]byte{table[v.e].b, encodeDataSize(size, uint64(nb))}, []byte{})
				return bytes.NewBuffer(b), io.EOF
			}
			var vn reflect.Value

			if vnext.IsValid() && vnext.CanSet() {
				switch vnext.Kind() {
				case reflect.Ptr:
					vnext.Set(reflect.New(vnext.Type().Elem()))
					vn = vnext.Elem()
				case reflect.Slice:
					vnext.Set(reflect.Append(vnext, reflect.New(vnext.Type().Elem()).Elem()))
					vn = vnext.Index(vnext.Len() - 1)
				default:
					vn = vnext
				}
			}

			if elem != nil {
				elem.Value = vn.Interface()
			}
			r0, err := vd.readElement(r, int64(size), vn, depth+1, pos+headerSize, elem, handler)
			if err != nil && err != io.EOF {
				return r0, err
			}
			if r0 != nil {
				r = io.MultiReader(r0, r)
			}
		default:
			val, err := vd.decode(v.t, r, size)
			if err != nil {
				return nil, err
			}
			vr := reflect.ValueOf(val)

			if elem != nil {
				elem.Value = vr.Interface()
			}
		}
		if elem != nil {
			handler(elem)
		}

		pos += headerSize + size
		if stopHere {
			return nil, ErrReadStopped
		}
	}
}
