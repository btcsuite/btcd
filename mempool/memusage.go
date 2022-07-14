// Copyright (c) 2013-2016 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mempool

import (
	"reflect"
)

func dynamicMemUsage(v reflect.Value) uintptr {
	return _dynamicMemUsage(v, false, 0)
}

func _dynamicMemUsage(v reflect.Value, debug bool, level int) uintptr {
	t := v.Type()
	bytes := t.Size()
	if debug {
		println("[", level, "]", t.Kind().String(), "(", t.String(), ") ->", t.Size())
	}

	// For complex types, we need to peek inside slices/arrays/structs/maps and chase pointers.
	switch t.Kind() {
	case reflect.Pointer, reflect.Interface:
		if !v.IsNil() {
			bytes += _dynamicMemUsage(v.Elem(), debug, level+1)
		}
	case reflect.Array, reflect.Slice:
		for j := 0; j < v.Len(); j++ {
			vi := v.Index(j)
			k := vi.Type().Kind()
			if debug {
				println("[", level, "] index:", j, "kind:", k.String())
			}
			elemB := uintptr(0)
			if t.Kind() == reflect.Array {
				if (k == reflect.Pointer || k == reflect.Interface) && !vi.IsNil() {
					elemB += _dynamicMemUsage(vi.Elem(), debug, level+1)
				}
			} else { // slice
				elemB += _dynamicMemUsage(vi, debug, level+1)
			}
			if k == reflect.Uint8 {
				// short circuit for byte slice/array
				bytes += elemB * uintptr(v.Len())
				if debug {
					println("...", v.Len(), "elements")
				}
				break
			}
			bytes += elemB
		}
	case reflect.Map:
		iter := v.MapRange()
		for iter.Next() {
			vk := iter.Key()
			vv := iter.Value()
			if debug {
				println("[", level, "] key:", vk.Type().Kind().String())
			}
			bytes += _dynamicMemUsage(vk, debug, level+1)
			if debug {
				println("[", level, "] value:", vv.Type().Kind().String())
			}
			bytes += _dynamicMemUsage(vv, debug, level+1)
			if debug {
				println("...", v.Len(), "map elements")
			}
			debug = false
		}
	case reflect.Struct:
		for _, f := range reflect.VisibleFields(t) {
			vf := v.FieldByIndex(f.Index)
			k := vf.Type().Kind()
			if debug {
				println("[", level, "] field:", f.Name, "kind:", k.String())
			}
			if (k == reflect.Pointer || k == reflect.Interface) && !vf.IsNil() {
				bytes += _dynamicMemUsage(vf.Elem(), debug, level+1)
			} else if k == reflect.Array || k == reflect.Slice {
				bytes -= vf.Type().Size()
				bytes += _dynamicMemUsage(vf, debug, level+1)
			}
		}
	}

	return bytes
}
