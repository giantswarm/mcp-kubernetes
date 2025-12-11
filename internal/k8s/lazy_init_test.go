package k8s

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
)

func TestLazyValueGet(t *testing.T) {
	tests := []struct {
		name      string
		initFn    func() (string, error)
		wantValue string
		wantErr   bool
	}{
		{
			name: "successful initialization",
			initFn: func() (string, error) {
				return "hello", nil
			},
			wantValue: "hello",
			wantErr:   false,
		},
		{
			name: "initialization error",
			initFn: func() (string, error) {
				return "", errors.New("init failed")
			},
			wantValue: "",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var lv lazyValue[string]

			got, err := lv.Get(tt.initFn)
			if (err != nil) != tt.wantErr {
				t.Errorf("Get() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.wantValue {
				t.Errorf("Get() = %v, want %v", got, tt.wantValue)
			}
		})
	}
}

func TestLazyValueGetCaching(t *testing.T) {
	var lv lazyValue[int]
	var callCount int

	initFn := func() (int, error) {
		callCount++
		return 42, nil
	}

	// First call should initialize
	v1, err := lv.Get(initFn)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v1 != 42 {
		t.Errorf("expected 42, got %d", v1)
	}
	if callCount != 1 {
		t.Errorf("expected 1 call, got %d", callCount)
	}

	// Second call should return cached value
	v2, err := lv.Get(initFn)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v2 != 42 {
		t.Errorf("expected 42, got %d", v2)
	}
	if callCount != 1 {
		t.Errorf("expected still 1 call (cached), got %d", callCount)
	}
}

func TestLazyValueGetConcurrent(t *testing.T) {
	var lv lazyValue[int]
	var initCount atomic.Int32

	initFn := func() (int, error) {
		initCount.Add(1)
		return 42, nil
	}

	const goroutines = 100
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			v, err := lv.Get(initFn)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if v != 42 {
				t.Errorf("expected 42, got %d", v)
			}
		}()
	}

	wg.Wait()

	// initFn should be called exactly once despite concurrent access
	if count := initCount.Load(); count != 1 {
		t.Errorf("expected initFn called once, got %d", count)
	}
}

func TestLazyValueIsSet(t *testing.T) {
	var lv lazyValue[string]

	if lv.IsSet() {
		t.Error("expected IsSet() = false before initialization")
	}

	_, _ = lv.Get(func() (string, error) {
		return "test", nil
	})

	if !lv.IsSet() {
		t.Error("expected IsSet() = true after initialization")
	}
}

func TestLazyValueErrorDoesNotCache(t *testing.T) {
	var lv lazyValue[string]
	var callCount int

	_, err := lv.Get(func() (string, error) {
		callCount++
		return "", errors.New("fail")
	})
	if err == nil {
		t.Error("expected error")
	}
	if callCount != 1 {
		t.Errorf("expected 1 call, got %d", callCount)
	}

	// Error should not be cached, so next call should retry
	_, err = lv.Get(func() (string, error) {
		callCount++
		return "success", nil
	})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if callCount != 2 {
		t.Errorf("expected 2 calls (error not cached), got %d", callCount)
	}

	// Now it should be cached
	_, err = lv.Get(func() (string, error) {
		callCount++
		return "ignored", nil
	})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if callCount != 2 {
		t.Errorf("expected still 2 calls (cached), got %d", callCount)
	}
}
