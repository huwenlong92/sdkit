package queue

import (
	"errors"
	"testing"
)

func TestIsLockNotAcquired(t *testing.T) {
	if !IsLockNotAcquired(ErrLockNotAcquired) {
		t.Fatal("ErrLockNotAcquired should be recognized")
	}
	if !IsLockNotAcquired(errors.Join(errors.New("wrapped"), ErrLockNotAcquired)) {
		t.Fatal("joined ErrLockNotAcquired should be recognized")
	}
	if IsLockNotAcquired(errors.New("other")) {
		t.Fatal("unrelated error should not be recognized")
	}
}
