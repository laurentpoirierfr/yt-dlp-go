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

// NewInstance returns a fresh copy of the registered prototype (extractors
// must not share per-extraction state). Configuration fields configured at
// registration time are copied; the runtime `self` pointer is reset so the
// shared lifecycle dispatches to the new instance.
func NewInstance(proto IE) IE {
	pt := reflect.TypeOf(proto).Elem()
	np := reflect.New(pt)
	np.Elem().Set(reflect.ValueOf(proto).Elem())
	ie, ok := np.Interface().(IE)
	if !ok {
		return nil
	}
	if s, ok := ie.(interface{ SetSelf(IE) }); ok {
		s.SetSelf(ie)
	}
	return ie
}
