package tracing

type Attr struct {
	Key   string
	Value any
}

func String(key string, value string) Attr {
	return Attr{Key: key, Value: value}
}

func Int(key string, value int) Attr {
	return Attr{Key: key, Value: value}
}

func Int64(key string, value int64) Attr {
	return Attr{Key: key, Value: value}
}

func Float64(key string, value float64) Attr {
	return Attr{Key: key, Value: value}
}

func Bool(key string, value bool) Attr {
	return Attr{Key: key, Value: value}
}
