# core/sandbox

`core/sandbox` provides a unified code execution sandbox for AI competition and online judge workloads.

The public API is intentionally small:

```go
runner, err := sandbox.New(sandbox.Options{
    AllowedImages: []string{"python:3.12-slim"},
})
if err != nil {
    return err
}

result, err := runner.Run(ctx, &sandbox.RunRequest{
    SubmissionID: "submission-1",
    Language:     sandbox.LanguagePython,
    Files: []sandbox.File{
        {Path: "main.py", Content: []byte(`print("ok")`)},
    },
})
```

Docker SDK usage is hidden in `core/sandbox/internal/docker`; callers must use `Sandbox.Run`.

Local demo:

```bash
go run ./examples/sandbox-demo
```

Optional local observability stack:

```bash
docker compose -f deploy/sandbox/docker-compose.yaml up -d
```

Jaeger UI: `http://127.0.0.1:16686`
