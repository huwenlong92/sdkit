package audit

type Event struct {
	Scene    string
	Event    string
	Level    string
	UID      int64
	IP       string
	DeviceID string
	Score    int
	Action   string
	Reason   map[string]any
	Extra    map[string]any
}
