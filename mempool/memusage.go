// Copyright (c) 2013-2016 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mempool

import (
	"fmt"
	"reflect"
)

var (
	dynamicMemUsageAssert   = false
	dynamicMemUsageDebug    = false
	dynamicMemUsageMaxDepth = 10
)

func dynamicMemUsage(v reflect.Value) uintptr {
	return dynamicMemUsageCrawl(v, 0)
}

func dynamicMemUsageCrawl(v reflect.Value, depth int) uintptr {
	t := v.Type()
	bytes := t.Size()
	if dynamicMemUsageDebug {
		println("[", depth, "]", t.Kind().String(), "(", t.String(), ") ->", t.Size())
	}

	if depth >= dynamicMemUsageMaxDepth {
		if dynamicMemUsageAssert {
			panic("crawl reached maximum depth")
		}
		return bytes
	}

	// For complex types, we need to peek inside slices/arrays/structs and chase pointers.
	switch t.Kind() {
	case reflect.Pointer, reflect.Interface:
		if !v.IsNil() {
			bytes += dynamicMemUsageCrawl(v.Elem(), depth+1)
		}
	case reflect.Array, reflect.Slice:
		for j := 0; j < v.Len(); j++ {
			vi := v.Index(j)
			k := vi.Type().Kind()
			if dynamicMemUsageDebug {
				println("[", depth, "] index:", j, "kind:", k.String())
			}
			elemBytes := uintptr(0)
			if t.Kind() == reflect.Array {
				if (k == reflect.Pointer || k == reflect.Interface) && !vi.IsNil() {
					elemBytes += dynamicMemUsageCrawl(vi.Elem(), depth+1)
				}
			} else { // slice
				elemBytes += dynamicMemUsageCrawl(vi, depth+1)
			}
			if k == reflect.Uint8 {
				// short circuit for byte slice/array
				bytes += elemBytes * uintptr(v.Len())
				if dynamicMemUsageDebug {
					println("...", v.Len(), "elements")
				}
				break
			}
			bytes += elemBytes
		}
	case reflect.Struct:
		for _, f := range reflect.VisibleFields(t) {
			vf := v.FieldByIndex(f.Index)
			k := vf.Type().Kind()
			if dynamicMemUsageDebug {
				println("[", depth, "] field:", f.Name, "kind:", k.String())
			}
			if (k == reflect.Pointer || k == reflect.Interface) && !vf.IsNil() {
				bytes += dynamicMemUsageCrawl(vf.Elem(), depth+1)
			} else if k == reflect.Array || k == reflect.Slice {
				bytes -= vf.Type().Size()
				bytes += dynamicMemUsageCrawl(vf, depth+1)
			}
		}
	case reflect.Uint8:
	default:
		if dynamicMemUsageAssert {
			panic(fmt.Sprintf("unsupported kind: %v", t.Kind()))
		}
	}

	return bytes
}
