package docker

import (
	"strings"

	cerrdefs "github.com/containerd/errdefs"
)

func isNotFound(err error) bool {
	if err == nil {
		return false
	}
	if cerrdefs.IsNotFound(err) {
		return true
	}
	return strings.Contains(strings.ToLower(err.Error()), "not found")
}
