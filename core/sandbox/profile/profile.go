package profile

import "fmt"

type Profile struct {
	Language   string
	Image      string
	WorkingDir string
	CompileCmd []string
	RunCmd     []string
	Env        []string
}

type Request struct {
	Language   string
	Image      string
	WorkingDir string
	CompileCmd []string
	RunCmd     []string
	Env        []string
}

func Apply(req Request) (Profile, error) {
	base, ok := defaults[req.Language]
	if !ok {
		return Profile{}, fmt.Errorf("unsupported language profile %q", req.Language)
	}
	if req.Image != "" {
		base.Image = req.Image
	}
	if req.WorkingDir != "" {
		base.WorkingDir = req.WorkingDir
	}
	if req.CompileCmd != nil {
		base.CompileCmd = req.CompileCmd
	}
	if len(req.RunCmd) > 0 {
		base.RunCmd = req.RunCmd
	}
	if len(req.Env) > 0 {
		base.Env = append(append([]string{}, base.Env...), req.Env...)
	}
	return base, nil
}

var defaults = map[string]Profile{
	"python": Python(),
	"golang": Go(),
	"cpp":    CPP(),
}
