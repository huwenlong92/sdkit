package redisx

import "strings"

func (c *Client) Key(parts ...string) string {
	all := make([]string, 0, len(parts)+1)
	if c != nil && c.Prefix != "" {
		all = append(all, strings.Trim(c.Prefix, ":"))
	}
	for _, p := range parts {
		p = strings.Trim(p, ":")
		if p != "" {
			all = append(all, p)
		}
	}
	return strings.Join(all, ":")
}
