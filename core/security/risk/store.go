package risk

import "context"

type Store interface {
	LoadScene(ctx context.Context, service string, scene string) (*Scene, error)
	MatchList(ctx context.Context, event Event, listType string) (*ListRule, error)
	ListFrequencyRules(ctx context.Context, event Event) ([]FrequencyRule, error)
	CountEvents(ctx context.Context, query EventCountQuery) (int64, error)
	SaveDecision(ctx context.Context, event Event, decision *Decision) error
}
