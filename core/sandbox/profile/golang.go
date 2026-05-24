package profile

func Go() Profile {
	return Profile{
		Language:   "golang",
		Image:      "golang:1.22-alpine",
		WorkingDir: "/workspace",
		CompileCmd: []string{"go", "build", "-o", "/workspace/main", "/workspace/main.go"},
		RunCmd:     []string{"/workspace/main"},
		Env:        []string{"GOCACHE=/tmp/go-build", "GOMODCACHE=/tmp/go-mod"},
	}
}
