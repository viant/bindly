package bindly

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/viant/bindly/locator"
	"github.com/viant/bindly/provider/body"
	"github.com/viant/bindly/state"
)

type replayInternalDetail struct{ Value string }
type replayInternalHas struct{ Name, Secret, Flag, Count, Private, Child bool }
type replayInternalRow struct {
	Name    string                `json:"name"`
	Secret  string                `internal:"true"`
	Flag    bool                  `internal:"true"`
	Count   int                   `internal:"true"`
	Private *replayInternalDetail `internal:"true"`
	Child   *replayInternalRow    `json:"child"`
	Has     *replayInternalHas    `json:"-" setMarker:"true"`
}
type replayInternalInput struct {
	Rows []*replayInternalRow
	Has  struct{ Rows bool } `setMarker:"true"`
}

func internalReplayPlan(t *testing.T) (*Injector, *Plan, *ReplayPlan) {
	t.Helper()
	i, err := NewInjector()
	if err != nil {
		t.Fatal(err)
	}
	p, err := i.CompilePlan(reflect.TypeFor[replayInternalInput](), BindingSpec{Name: "Rows", Path: "Rows", Location: state.Location{Kind: "body", In: "data"}})
	if err != nil {
		t.Fatal(err)
	}
	rp, err := p.Replay("Rows")
	if err != nil {
		t.Fatal(err)
	}
	return i, p, rp
}
func trustedInternalRow() *replayInternalRow {
	return &replayInternalRow{Name: "server", Secret: "private", Private: &replayInternalDetail{Value: "owned"}, Has: &replayInternalHas{true, true, true, true, true, true}}
}
func assertPublicInternalRow(t *testing.T, row *replayInternalRow) {
	t.Helper()
	if row.Secret != "" || row.Flag || row.Count != 0 || row.Private != nil || row.Has == nil || row.Has.Secret || row.Has.Flag || row.Has.Count || row.Has.Private || !row.Has.Name {
		t.Fatalf("raw replay inherited trust: %+v Has=%+v", row, row.Has)
	}
	if row.Child != nil {
		assertPublicInternalRow(t, row.Child)
	}
}
func TestReplayTrustedInternalCaptureAndRawReprojection(t *testing.T) {
	i, p, rp := internalReplayPlan(t)
	row := trustedInternalRow()
	row.Child = trustedInternalRow()
	input := &replayInternalInput{Rows: []*replayInternalRow{row, nil}}
	input.Has.Rows = true
	replay, err := rp.Capture(input)
	if err != nil {
		t.Fatal(err)
	}
	if replay.prepared != nil {
		t.Fatal("typed capture manufactured prepared proof")
	}
	decoded, err := replay.encodeField(rp.fields["Rows"], input.Rows)
	if err != nil {
		t.Fatal(err)
	}
	expected, _ := json.Marshal(input.Rows)
	if string(decoded) != string(expected) || string(replay.raw["Rows"]) != string(expected) || !strings.Contains(string(decoded), `"Count":0`) || !strings.Contains(string(decoded), `"Flag":false`) || !strings.Contains(string(decoded), `"private"`) {
		t.Fatalf("capture lost data: %s", decoded)
	}
	// Capture remains a raw source when replayed; no private mode travels with it.
	actual := &replayInternalInput{}
	if err = i.Bind(context.Background(), actual, WithPlan(p), WithReplay(ReplayBinding{Replay: replay, Only: true})); err != nil {
		t.Fatal(err)
	}
	if len(actual.Rows) != 2 || actual.Rows[1] != nil {
		t.Fatal("nil row lost")
	}
	assertPublicInternalRow(t, actual.Rows[0])
	if input.Rows[0].Secret != "private" || !input.Rows[0].Has.Secret || input.Rows[0].Child.Private.Value != "owned" {
		t.Fatal("capture mutated trusted input")
	}
}
func TestReplayTrustedInternalOmittedZeroStillRejectsLoss(t *testing.T) {
	type row struct {
		Zero int                  `json:"zero,omitempty" internal:"true"`
		Has  *struct{ Zero bool } `json:"-" setMarker:"true"`
	}
	type target struct {
		Row *row
		Has struct{ Row bool } `setMarker:"true"`
	}
	i, _ := NewInjector()
	p, err := i.CompilePlan(reflect.TypeFor[target](), BindingSpec{Name: "Row", Path: "Row", Location: state.Location{Kind: "body", In: "data"}})
	if err != nil {
		t.Fatal(err)
	}
	rp, _ := p.Replay("Row")
	input := &target{Row: &row{Has: &struct{ Zero bool }{true}}}
	input.Has.Row = true
	if _, err = rp.Capture(input); err == nil || !strings.Contains(err.Error(), "cannot preserve typed value/presence") {
		t.Fatalf("omitted internal zero accepted: %v", err)
	}
}

var replayInternalDecoderCalls int

type replayInternalDecoder struct{}

func (*replayInternalDecoder) UnmarshalJSON([]byte) error { replayInternalDecoderCalls++; return nil }
func TestReplayForgedInternalDecodeAndSourceCapture(t *testing.T) {
	type row struct {
		Name   string
		Secret string                                   `internal:"true"`
		Aux    *replayInternalDecoder                   `internal:"true"`
		Child  *row                                     `json:"child"`
		Has    *struct{ Name, Secret, Aux, Child bool } `json:"-" setMarker:"true"`
	}
	type target struct {
		Rows []*row
		Has  struct{ Rows bool } `setMarker:"true"`
	}
	i, _ := NewInjector()
	p, err := i.CompilePlan(reflect.TypeFor[target](), BindingSpec{Name: "Rows", Path: "Rows", Location: state.Location{Kind: "body", In: "data"}})
	if err != nil {
		t.Fatal(err)
	}
	rp, _ := p.Replay("Rows")
	for _, raw := range []string{`[{"Name":"ok","Secret":"forged","Aux":{},"Has":{"Secret":true},"child":{"Name":"child","Secret":null,"Aux":[]}}]`, `[{"Name":"ok","Secret":123,"Aux":null}]`} {
		for _, sourceCapture := range []bool{false, true} {
			replayInternalDecoderCalls = 0
			var replay *Replay
			if sourceCapture {
				s, _ := body.New([]byte(`{"data":`+raw+`}`), "application/json", nil)
				replay, err = rp.CaptureSources(context.Background(), []locator.Provider{s})
			} else {
				replay, err = rp.DecodeJSON([]byte(`{"Rows":` + raw + `}`))
			}
			if err != nil {
				t.Fatal(err)
			}
			if replay.prepared != nil {
				t.Fatal("raw capture manufactured prepared proof")
			}
			if string(replay.raw["Rows"]) != raw {
				t.Fatalf("raw capture altered: %s", replay.raw["Rows"])
			}
			actual := &target{}
			if err = i.Bind(context.Background(), actual, WithPlan(p), WithReplay(ReplayBinding{Replay: replay, Only: true})); err != nil {
				t.Fatal(err)
			}
			for r := actual.Rows[0]; r != nil; r = r.Child {
				if r.Secret != "" || r.Aux != nil || r.Has == nil || r.Has.Secret || r.Has.Aux || !r.Has.Name {
					t.Fatalf("forged input %+v Has=%+v", r, r.Has)
				}
			}
			if replayInternalDecoderCalls != 0 {
				t.Fatal("internal child decoder ran")
			}
		}
	}
}
func TestReplayPreparedInternalMaterializationRemainsDetached(t *testing.T) {
	i, p, rp := internalReplayPlan(t)
	replay, _ := rp.DecodeJSON([]byte(`{"Rows":[{"name":"public","child":null}]}`))
	actual := &replayInternalInput{}
	if err := i.Bind(context.Background(), actual, WithPlan(p), WithReplay(ReplayBinding{Replay: replay, Only: true, Gate: func(_ context.Context, target any) error {
		row := target.(*replayInternalInput).Rows[0]
		row.Secret = "server"
		row.Private = &replayInternalDetail{Value: "owned"}
		row.Has.Secret = true
		row.Has.Flag = true
		row.Has.Count = true
		row.Has.Private = true
		return nil
	}})); err != nil {
		t.Fatal(err)
	}
	if replay.prepared == nil {
		t.Fatal("native binding failed to prepare")
	}
	actual.Rows[0].Secret = "mutated"
	actual.Rows[0].Private.Value = "mutated"
	actual.Rows[0].Has.Secret = false
	providers, err := replay.Providers()
	if err != nil {
		t.Fatal(err)
	}
	scope, err := i.ForScope(providers...)
	if err != nil {
		t.Fatal(err)
	}
	next := &replayInternalInput{}
	if err = scope.Bind(context.Background(), next, WithPlan(p)); err != nil {
		t.Fatal(err)
	}
	expected := trustedInternalRow()
	expected.Name = "public"
	expected.Secret = "server"
	if !reflect.DeepEqual(next.Rows[0], expected) {
		t.Fatalf("prepared private value/presence lost: %+v Has=%+v", next.Rows[0], next.Rows[0].Has)
	}
	next.Rows[0].Private.Value = "second mutation"
	next.Rows[0].Has.Count = false
	again := &replayInternalInput{}
	if err = scope.Bind(context.Background(), again, WithPlan(p)); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(again.Rows[0], expected) {
		t.Fatal("prepared replay aliases a recipient")
	}
	serialized, _ := json.Marshal(replay)
	rawReplay, err := rp.DecodeJSON(serialized)
	if err != nil {
		t.Fatal(err)
	}
	public := &replayInternalInput{}
	if err = i.Bind(context.Background(), public, WithPlan(p), WithReplay(ReplayBinding{Replay: rawReplay, Only: true})); err != nil {
		t.Fatal(err)
	}
	assertPublicInternalRow(t, public.Rows[0])
}
