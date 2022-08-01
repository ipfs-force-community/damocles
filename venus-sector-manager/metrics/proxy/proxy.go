package proxy

import (
	"context"
	"reflect"

	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/metrics"
)

func MetricedSealerAPI(a interface{}) interface{} {
	return proxy(a)
}

func proxy(in interface{}) interface{} {
	fields := []reflect.StructField{}

	valueIn := reflect.ValueOf(in)

	for i := 0; i < valueIn.NumMethod(); i++ {
		fields = append(fields, reflect.StructField{Name: valueIn.Type().Method(i).Name, Type: valueIn.Method(i).Type()})
	}

	internal := reflect.StructOf(fields)
	internalValue := reflect.New(internal).Elem()
	for i := 0; i < valueIn.NumMethod(); i++ {
		fn := valueIn.Method(i)
		funcName := valueIn.Type().Method(i).Name
		internalValue.Field(i).Set(reflect.MakeFunc(valueIn.Method(i).Type(), func(args []reflect.Value) (results []reflect.Value) {
			ctx := args[0].Interface().(context.Context)
			// upsert function name into context
			ctx, _ = metrics.New(ctx, metrics.Upsert(metrics.Endpoint, funcName))
			stop := metrics.Timer(ctx, metrics.APIRequestDuration, metrics.SinceInMilliseconds)
			defer stop()
			// pass tagged ctx back into function call
			args[0] = reflect.ValueOf(ctx)
			return fn.Call(args)
		}))
	}

	outStruct := reflect.StructOf([]reflect.StructField{reflect.StructField{
		Name: "Internal",
		Type: reflect.TypeOf(internalValue.Addr().Interface()).Elem(),
	}})

	outValue := reflect.New(outStruct).Elem()

	outValue.Field(0).Set(internalValue)

	return outValue.Addr().Interface()
}
