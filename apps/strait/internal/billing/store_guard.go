package billing

import "reflect"

func storeIsNil(store Store) bool {
	if store == nil {
		return true
	}

	v := reflect.ValueOf(store)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Ptr, reflect.Slice:
		return v.IsNil()
	default:
		return false
	}
}
