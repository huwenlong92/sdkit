package profile

func Python() Profile {
	return Profile{
		Language:   "python",
		Image:      "python:3.12-slim",
		WorkingDir: "/workspace",
		RunCmd:     []string{"python", "main.py"},
		Env:        []string{"PYTHONUNBUFFERED=1"},
	}
}
