package docker

import (
	"context"

	"github.com/huwenlong92/sdkit/core/sandbox/tracing"

	"go.opentelemetry.io/otel/attribute"
	oteltrace "go.opentelemetry.io/otel/trace"
)

func startDockerStep(ctx context.Context, name string, spec *Spec, phase string) (context.Context, oteltrace.Span) {
	attrs := []attribute.KeyValue{}
	if spec != nil {
		attrs = append(attrs,
			attribute.String("image", spec.Image),
			attribute.String("submission.id", spec.SubmissionID),
		)
	}
	if phase != "" {
		attrs = append(attrs, attribute.String("phase", phase))
	}
	c, span := tracing.StartStep(ctx, name, attrs...)
	return c, span
}
