package docker

import (
	"context"

	"github.com/huwenlong92/sdkit/core/sandbox/tracing"
	coretracing "github.com/huwenlong92/sdkit/core/tracing"
)

func startDockerStep(ctx context.Context, name string, spec *Spec, phase string) (context.Context, coretracing.Span) {
	attrs := []coretracing.Attr{}
	if spec != nil {
		attrs = append(attrs,
			coretracing.String("image", spec.Image),
			coretracing.String("submission.id", spec.SubmissionID),
		)
	}
	if phase != "" {
		attrs = append(attrs, coretracing.String("phase", phase))
	}
	c, span := tracing.StartStep(ctx, name, attrs...)
	return c, span
}
