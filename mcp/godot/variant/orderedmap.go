package variant

type KeyValue[K comparable, V any] struct {
	Key   K
	Value V
}

type OrderedMap[K comparable, V any] []KeyValue[K, V]

func (o *OrderedMap[K, V]) Keys() []K {
	keys := make([]K, 0, len(*o))
	for _, item := range *o {
		keys = append(keys, item.Key)
	}
	return keys
}

func (o *OrderedMap[K, V]) Get(k K) (V, bool) {
	for _, item := range *o {
		if item.Key == k {
			return item.Value, true
		}
	}
	var v V
	return v, false
}

func (o *OrderedMap[K, V]) Set(k K, v V) {
	*o = append(*o, KeyValue[K, V]{k, v})
}

func (o *OrderedMap[K, V]) Erase(k K) {
	for i := range *o {
		if (*o)[i].Key == k {
			copy((*o)[i:], (*o)[i+1:])
			*o = (*o)[:len(*o)-1]
			return
		}
	}
}
