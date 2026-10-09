package extractor

import (
	"reflect"
	"sync"
)

var (
	registryMu sync.RWMutex
	registry   []IE
)

// Register adds an extractor instance to the global registry.
// Extractor packages call it from init().
func Register(ie IE) {
	registryMu.Lock()
	defer registryMu.Unlock()
	registry = append(registry, ie)
}

// All returns all registered extractors.
func All() []IE {
	registryMu.RLock()
	defer registryMu.RUnlock()
	out := make([]IE, len(registry))
	copy(out, registry)
	return out
}

// ByKey returns the extractor with the given key, if any.
func ByKey(key string) IE {
	for _, ie := range All() {
		if ie.IEKey() == key {
			return ie
		}
	}
	return nil
}

// NewInstance returns a fresh zero instance of the same concrete type as the
// registered prototype (extractors must not share per-extraction state).
func NewInstance(proto IE) IE {
	v := reflect.New(reflect.TypeOf(proto).Elem()).Interface()
	ie, _ := v.(IE)
	return ie
}
