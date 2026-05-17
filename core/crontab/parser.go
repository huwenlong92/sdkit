package crontab

import (
	"time"

	"github.com/robfig/cron/v3"
)

func ValidateSpec(spec string) error {
	_, err := cronParser().Parse(spec)
	return err
}

func NextRuns(spec string, n int) ([]time.Time, error) {
	if n <= 0 {
		n = 5
	}
	schedule, err := cronParser().Parse(spec)
	if err != nil {
		return nil, err
	}
	out := make([]time.Time, 0, n)
	next := time.Now()
	for i := 0; i < n; i++ {
		next = schedule.Next(next)
		out = append(out, next)
	}
	return out, nil
}

func cronParser() cron.Parser {
	return cron.NewParser(
		cron.SecondOptional | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor,
	)
}
