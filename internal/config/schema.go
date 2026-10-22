package config

import (
	"fmt"
	"math"
	"sort"
)

type kind int

const (
	kString kind = iota
	kInt
	kNumber
	kBool
	kObject
	kList
)

func (k kind) String() string {
	return [...]string{"a string", "an integer", "a number", "a boolean", "an object", "a list"}[k]
}

// typed lists the keys grapi reads, with the type it needs.
var typed = map[string]kind{
	"server.host":                kString,
	"server.port":                kInt,
	"server.url":                 kString,
	"server.proxy":               kBool,
	"server.app.keys":            kList,
	"api.rest.prefix":            kString,
	"api.rest.defaultLimit":      kInt,
	"api.rest.maxLimit":          kInt,
	"api.rest.withCount":         kBool,
	"admin.apiToken.salt":        kString,
	"admin.auth.secret":          kString,
	"database.connection":        kObject,
	"database.connection.client": kString,
}

func matches(v any, k kind) bool {
	switch k {
	case kString:
		_, ok := v.(string)
		return ok
	case kInt:
		switch x := v.(type) {
		case int:
			return true
		case float64:
			return x == math.Trunc(x) && !math.IsInf(x, 0)
		}
		return false
	case kNumber:
		switch v.(type) {
		case int, float64:
			return true
		}
		return false
	case kBool:
		_, ok := v.(bool)
		return ok
	case kObject:
		_, ok := v.(map[string]any)
		return ok
	case kList:
		_, ok := v.([]any)
		return ok
	}
	return false
}

func (c *Config) validate() error {
	keys := make([]string, 0, len(typed))
	for k := range typed {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, key := range keys {
		v := c.Get(key)
		if v == nil || matches(v, typed[key]) {
			continue
		}
		top := key
		for i := range key {
			if key[i] == '.' {
				top = key[:i]
				break
			}
		}
		src := c.sources[top]
		if src == "" {
			src = "defaults"
		}
		return fmt.Errorf("%s: %s must be %s, got %v", src, key, typed[key], v)
	}
	return nil
}
