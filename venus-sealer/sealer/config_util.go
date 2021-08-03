package sealer

import (
	"fmt"
	"reflect"
)

func checkOptionalConfig(original, optional reflect.Type) {
	fieldNum := original.NumField()
	for i := 0; i < fieldNum; i++ {
		fori := original.Field(i)
		fopt := optional.Field(i)
		if fori.Name != fopt.Name {
			panic(fmt.Errorf("field name not match: %s != %s", fori.Name, fopt.Name))
		}

		if fopt.Type.Kind() != reflect.Ptr || fopt.Type.Elem() != fori.Type {
			panic(fmt.Errorf("field type not match: %s vs %s", fori.Type, fopt.Type))
		}
	}
}
