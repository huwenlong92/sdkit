package blacklist

type Entry struct {
	Type      string
	Value     string
	Reason    string
	Source    string
	ExpiredAt int64
}
