package service

import (
	"testing"
	"time"
)

func TestTagLockerSerializesSameKeyAndCleansUp(t *testing.T) {
	locker := newTagLocker()
	unlockFirst := locker.lockMany([]string{"repo\x00latest"})
	acquired := make(chan func(), 1)
	go func() { acquired <- locker.lockMany([]string{"repo\x00latest"}) }()

	select {
	case unlock := <-acquired:
		unlock()
		t.Fatal("second caller acquired the same tag lock too early")
	case <-time.After(30 * time.Millisecond):
	}
	unlockFirst()

	select {
	case unlock := <-acquired:
		unlock()
	case <-time.After(time.Second):
		t.Fatal("second caller did not acquire released tag lock")
	}
	locker.mu.Lock()
	defer locker.mu.Unlock()
	if len(locker.entries) != 0 {
		t.Fatalf("lock entries leaked: %d", len(locker.entries))
	}
}

func TestTagLockerLockManyUsesStableOrder(t *testing.T) {
	locker := newTagLocker()
	first := make(chan func(), 1)
	second := make(chan func(), 1)
	go func() { first <- locker.lockMany([]string{"b", "a", "a"}) }()
	go func() { second <- locker.lockMany([]string{"a", "b"}) }()

	var unlock1 func()
	select {
	case unlock1 = <-first:
	case unlock1 = <-second:
	case <-time.After(time.Second):
		t.Fatal("neither multi-lock caller acquired; possible deadlock")
	}
	unlock1()
	select {
	case unlock := <-first:
		unlock()
	case unlock := <-second:
		unlock()
	case <-time.After(time.Second):
		t.Fatal("remaining multi-lock caller deadlocked")
	}
}
