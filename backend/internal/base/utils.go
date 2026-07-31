package base

import (
	"os"
	"strconv"
)

func DerefInt(i *int) int {
	if i != nil {
		return *i
	}
	return 0
}

func DerefString(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

func GetEnvOrDefault[T string | bool](key string, def T) T {
	raw, ok := os.LookupEnv(key)
	if !ok || raw == "" {
		return def
	}

	var result any
	switch any(def).(type) {
	case string:
		result = raw
	case bool:
		b, err := strconv.ParseBool(raw)
		if err != nil {
			return def
		}
		result = b
	}
	return result.(T)
}
