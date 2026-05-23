# Tests

所有测试统一放在仓库根目录 `tests/` 下，按被测模块镜像目录归档。

示例：

```text
tests/core/queue/...
tests/pkg/storage/...
tests/bootstrap/...
```

不要在 `core/`、`pkg/` 等业务包目录下新增 `*_test.go`。
