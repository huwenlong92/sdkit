package captcha

import (
	"encoding/json"
	"strconv"
	"strings"
)

type point struct {
	X int `json:"x"`
	Y int `json:"y"`
}

func parseSliderAnswer(raw string) (point, int, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return point{}, 0, false
	}
	if x, err := strconv.Atoi(raw); err == nil {
		return point{X: x}, 0, true
	}
	request := struct {
		X          int     `json:"x"`
		Y          int     `json:"y"`
		DurationMS int     `json:"duration_ms"`
		Duration   int     `json:"duration"`
		Track      [][]int `json:"track"`
	}{}
	if err := json.Unmarshal([]byte(raw), &request); err != nil {
		return point{}, 0, false
	}
	duration := request.DurationMS
	if duration <= 0 {
		duration = request.Duration
	}
	if duration <= 0 && len(request.Track) > 1 {
		start := request.Track[0]
		end := request.Track[len(request.Track)-1]
		if len(start) >= 3 && len(end) >= 3 {
			duration = end[2] - start[2]
		}
	}
	return point{X: request.X, Y: request.Y}, duration, true
}

func parseClickAnswer(raw string) ([]point, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, false
	}
	request := struct {
		Points []point `json:"points"`
	}{}
	if err := json.Unmarshal([]byte(raw), &request); err != nil {
		return nil, false
	}
	return request.Points, len(request.Points) > 0
}

func absInt(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
