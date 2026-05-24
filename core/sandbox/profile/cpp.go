package profile

func CPP() Profile {
	return Profile{
		Language:   "cpp",
		Image:      "gcc:13",
		WorkingDir: "/workspace",
		CompileCmd: []string{"g++", "-O2", "-std=c++17", "/workspace/main.cpp", "-o", "/workspace/main"},
		RunCmd:     []string{"/workspace/main"},
	}
}
