package body

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
)

func TestDeferredBodyDemandAndReplay(t *testing.T) {
	var loads atomic.Int32
	s := NewDeferred(func(context.Context) (*Source, error) {
		loads.Add(1)
		return New([]byte(`{"id":9007199254740993,"flag":false}`), "application/json", nil)
	})
	s.Kind()
	s.Priority()
	s.DefaultCacheable()
	s.Locate(nil)
	if loads.Load() != 0 {
		t.Fatal("enumeration initialized body")
	}
	raw, present, err := s.CaptureSource(context.Background(), reflect.TypeOf(int64(0)), "id")
	if err != nil || !present || string(raw.(json.RawMessage)) != "9007199254740993" {
		t.Fatalf("replay precision: %v %v %v", raw, present, err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			v, ok, err := s.Value(context.Background(), reflect.TypeOf(false), "flag")
			if err != nil || !ok || v != false {
				t.Errorf("false presence: %v %v %v", v, ok, err)
			}
		}()
	}
	wg.Wait()
	if loads.Load() != 1 {
		t.Fatalf("loads%d", loads.Load())
	}
	if _, _, err = s.Value(context.Background(), reflect.TypeOf(int(0)), "flag"); err == nil {
		t.Fatal("wrong target accepted")
	}
	if v, ok, err := s.Value(context.Background(), reflect.TypeOf(false), "flag"); err != nil || !ok || v != false {
		t.Fatal("target decode error poisoned other target")
	}
}
func TestDeferredBodyInitializationErrorAndCancellation(t *testing.T) {
	cause := errors.New("read failure")
	loads := 0
	s := NewDeferred(func(context.Context) (*Source, error) { loads++; return nil, cause })
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := s.Value(ctx, nil, ""); !errors.Is(err, context.Canceled) || loads != 0 {
		t.Fatal("cancellation initialized body")
	}
	for i := 0; i < 2; i++ {
		if _, _, err := s.Value(context.Background(), nil, ""); !errors.Is(err, cause) {
			t.Fatal(err)
		}
	}
	if loads != 1 {
		t.Fatal("initialization error retried")
	}
}
