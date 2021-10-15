package trace

import (
	"github.com/timescale/promscale/pkg/clockcache"
)

const (
	urlCacheSize       = 1000
	operationCacheSize = 1000
	instLibCacheSize   = 1000
	tagCacheSize       = 1000
)

func newSchemaCache() *clockcache.Cache {
	return clockcache.WithMax(urlCacheSize)
}

func newOperationCache() *clockcache.Cache {
	return clockcache.WithMax(operationCacheSize)
}

func newInstrumentationLibraryCache() *clockcache.Cache {
	return clockcache.WithMax(instLibCacheSize)
}

func newTagCache() *clockcache.Cache {
	return clockcache.WithMax(tagCacheSize)
}
