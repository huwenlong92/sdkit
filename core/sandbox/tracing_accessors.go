package sandbox

func (r *RunRequest) GetSubmissionID() string {
	if r == nil {
		return ""
	}
	return r.SubmissionID
}

func (r *RunRequest) GetImage() string {
	if r == nil {
		return ""
	}
	return r.Image
}

func (r *RunRequest) GetTimeoutSeconds() float64 {
	if r == nil {
		return 0
	}
	return r.Timeout.Seconds()
}

func (r *RunRequest) GetMemoryBytes() int64 {
	if r == nil {
		return 0
	}
	return r.MemoryBytes
}

func (r *RunResult) GetContainerID() string {
	if r == nil {
		return ""
	}
	return r.ContainerID
}

func (r *RunResult) GetExitCode() int {
	if r == nil {
		return 0
	}
	return r.ExitCode
}

func (r *RunResult) GetTimedOut() bool {
	if r == nil {
		return false
	}
	return r.TimedOut
}

func (r *RunResult) GetMemoryUsed() uint64 {
	if r == nil {
		return 0
	}
	return r.MemoryUsed
}

func (r *RunResult) GetCPUUsed() float64 {
	if r == nil {
		return 0
	}
	return r.CPUUsed
}
