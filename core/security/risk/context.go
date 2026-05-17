package risk

type Context struct {
	Scene    string
	UID      int64
	IP       string
	UA       string
	DeviceID string
	Path     string
	Method   string
	Region   string
	Phone    string
	Body     []byte
	Headers  map[string]string
	Extra    map[string]any
}

func (c *Context) ExtraString(key string) string {
	if c == nil || c.Extra == nil {
		return ""
	}
	v, ok := c.Extra[key]
	if !ok || v == nil {
		return ""
	}
	s, _ := v.(string)
	return s
}
