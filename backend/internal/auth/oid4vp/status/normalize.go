package status

func NormalizeAnyMap(raw any) (map[string]any, bool) {
	switch value := raw.(type) {
	case map[string]any:
		return normalizeMap(value), true
	case map[interface{}]interface{}:
		out := make(map[string]any, len(value))
		for key, item := range value {
			keyString, ok := key.(string)
			if !ok {
				continue
			}
			out[keyString] = normalizeAnyValue(item)
		}
		return out, true
	default:
		return nil, false
	}
}

func normalizeMap(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for key, value := range in {
		out[key] = normalizeAnyValue(value)
	}
	return out
}

func normalizeAnyValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return normalizeMap(typed)
	case map[interface{}]interface{}:
		if normalized, ok := NormalizeAnyMap(typed); ok {
			return normalized
		}
	case []any:
		items := make([]any, len(typed))
		for i, item := range typed {
			items[i] = normalizeAnyValue(item)
		}
		return items
	}
	return value
}
