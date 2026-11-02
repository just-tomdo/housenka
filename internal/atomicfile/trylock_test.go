package atomicfile

import (
	"path/filepath"
	"testing"
)

func TestTryLock(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".lock")
	unlock, ok, err := TryLock(path)
	if err != nil || !ok {
		t.Fatalf("first TryLock: ok=%v err=%v", ok, err)
	}
	if _, ok, err := TryLock(path); err != nil || ok {
		t.Errorf("second TryLock while held: ok=%v err=%v", ok, err)
	}
	unlock()
	unlock2, ok, err := TryLock(path)
	if err != nil || !ok {
		t.Fatalf("TryLock after unlock: ok=%v err=%v", ok, err)
	}
	unlock2()
}
