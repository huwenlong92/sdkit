package errors_test

import (
	stderrors "errors"
	"testing"

	apperrors "github.com/huwenlong92/sdkit/core/errors"
)

func TestAppErrorSupportsUnwrapAsAndIs(t *testing.T) {
	cause := stderrors.New("database failed")
	err := apperrors.Wrap(cause, apperrors.CodeInternal, apperrors.SubCodeInternal, "查询失败")

	var appErr *apperrors.AppError
	if !stderrors.As(err, &appErr) {
		t.Fatal("expected errors.As to find AppError")
	}
	if appErr.Code != apperrors.CodeInternal {
		t.Fatalf("unexpected code: %d", appErr.Code)
	}
	if !stderrors.Is(err, cause) {
		t.Fatal("expected errors.Is to match wrapped cause")
	}
	if !stderrors.Is(err, apperrors.ErrInternalServer) {
		t.Fatal("expected errors.Is to match AppError code and sub_code")
	}
}

func TestNewCodeUsesStableSubCode(t *testing.T) {
	err := apperrors.NewCodeWithData(apperrors.CodeQueueTaskConflict, "任务已存在", map[string]string{"id": "task-1"})
	if err.SubCode != apperrors.SubCodeQueueTaskConflict {
		t.Fatalf("sub_code = %q, want %q", err.SubCode, apperrors.SubCodeQueueTaskConflict)
	}

	custom := apperrors.NewCode(4092, "队列已暂停")
	if custom.SubCode != "CODE_4092" {
		t.Fatalf("custom sub_code = %q, want CODE_4092", custom.SubCode)
	}
}
