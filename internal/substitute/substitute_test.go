package substitute

import (
	"reflect"
	"testing"
)

func TestValidName(t *testing.T) {
	valid := []string{"A", "_", "_A", "A1_", "DOTFILES_THM_RED", "lower_ok"}
	invalid := []string{"", "1A", "A-B", "A.B", "A B", "A!", "É"}
	for _, s := range valid {
		if !ValidName(s) {
			t.Errorf("ValidName(%q) = false, want true", s)
		}
	}
	for _, s := range invalid {
		if ValidName(s) {
			t.Errorf("ValidName(%q) = true, want false", s)
		}
	}
}

func TestParsePins(t *testing.T) {
	pins, err := ParsePins([]string{"A=1", "B=", "C=x=y", "A=2"})
	if err != nil {
		t.Fatalf("ParsePins: %v", err)
	}
	want := map[string]string{"A": "2", "B": "", "C": "x=y"}
	if !reflect.DeepEqual(pins, want) {
		t.Errorf("pins = %v, want %v", pins, want)
	}
}

func TestParsePinsErrors(t *testing.T) {
	for _, entry := range []string{"NOEQUALS", "1BAD=x", "A-B=x", "=x"} {
		if _, err := ParsePins([]string{entry}); err == nil {
			t.Errorf("ParsePins(%q): expected error", entry)
		}
	}
}

func TestDeadPins(t *testing.T) {
	pins := map[string]string{"PRE_A": "1", "OTHER": "2", "PRE_B": "3"}
	if got := DeadPins(pins, []string{"PRE_"}); !reflect.DeepEqual(got, []string{"OTHER"}) {
		t.Errorf("DeadPins = %v, want [OTHER]", got)
	}
	got := DeadPins(pins, nil)
	want := []string{"OTHER", "PRE_A", "PRE_B"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("DeadPins(no prefixes) = %v, want %v", got, want)
	}
}

func newTestResolver() *Resolver {
	return NewResolver(
		[]string{"PRE_"},
		map[string]string{"PRE_PINNED": "pinned", "PRE_SHADOWED": "frompin", "PRE_UNUSED": "z"},
		[]string{"PRE_ENV=fromenv", "PRE_SHADOWED=fromenv", "PRE_EMPTY=", "HOME=/home/u"},
	)
}

func TestApplySubstitution(t *testing.T) {
	tests := []struct {
		name         string
		in           string
		want         string
		wantConsumed []string
		wantMissing  []string
	}{
		{"env value", "x=${PRE_ENV}", "x=fromenv", []string{"PRE_ENV"}, nil},
		{"pin beats env", "x=${PRE_SHADOWED}", "x=frompin", []string{"PRE_SHADOWED"}, nil},
		{"empty is a value", "x=[${PRE_EMPTY}]", "x=[]", []string{"PRE_EMPTY"}, nil},
		{"missing recorded", "${PRE_GONE}${PRE_GONE2}", "", []string{"PRE_GONE", "PRE_GONE2"}, []string{"PRE_GONE", "PRE_GONE2"}},
		{"dedup and sort", "${PRE_ENV}${PRE_ENV}${PRE_EMPTY}", "fromenvfromenv", []string{"PRE_EMPTY", "PRE_ENV"}, nil},
		{"unprefixed passthrough", "p=${HOME}", "p=${HOME}", nil, nil},
		{"bare dollar passthrough", "p=$PRE_ENV", "p=$PRE_ENV", nil, nil},
		{"non-posix passthrough", "${PRE_ENV:-d} ${a.b}", "${PRE_ENV:-d} ${a.b}", nil, nil},
		{"unclosed passthrough", "${PRE_ENV", "${PRE_ENV", nil, nil},
		{"empty braces passthrough", "${}", "${}", nil, nil},
		{"escape", "$${PRE_ENV}", "${PRE_ENV}", nil, nil},
		{"lone double dollar", "kill -9 $$", "kill -9 $$", nil, nil},
		{"escape unprefixed passthrough", "$${HOME}", "$${HOME}", nil, nil},
		{"dollar then escape", "$$${PRE_ENV}", "$${PRE_ENV}", nil, nil},
		{"two dollars then escape", "$$$${PRE_ENV}", "$$${PRE_ENV}", nil, nil},
		{"mixed", "a=${PRE_ENV} b=$${PRE_ENV} c=${HOME} d=$$", "a=fromenv b=${PRE_ENV} c=${HOME} d=$$", []string{"PRE_ENV"}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newTestResolver()
			out, res := r.Apply([]byte(tt.in))
			if string(out) != tt.want {
				t.Errorf("output = %q, want %q", out, tt.want)
			}
			if !reflect.DeepEqual(res.Consumed, tt.wantConsumed) {
				t.Errorf("consumed = %v, want %v", res.Consumed, tt.wantConsumed)
			}
			if !reflect.DeepEqual(res.Missing, tt.wantMissing) {
				t.Errorf("missing = %v, want %v", res.Missing, tt.wantMissing)
			}
		})
	}
}

func TestApplyValueNeverRescanned(t *testing.T) {
	r := NewResolver([]string{"PRE_"}, map[string]string{"PRE_A": "${PRE_B}"}, []string{"PRE_B=resolved"})
	out, res := r.Apply([]byte("${PRE_A}"))
	if string(out) != "${PRE_B}" {
		t.Errorf("output = %q, want literal ${PRE_B}", out)
	}
	if !reflect.DeepEqual(res.Consumed, []string{"PRE_A"}) {
		t.Errorf("consumed = %v, want [PRE_A]", res.Consumed)
	}
}

func TestApplyDisabledResolver(t *testing.T) {
	r := NewResolver(nil, nil, []string{"PRE_ENV=v"})
	if r.Enabled() {
		t.Fatal("resolver with no prefixes should be disabled")
	}
	in := "x=${PRE_ENV} y=$${PRE_ENV}"
	out, res := r.Apply([]byte(in))
	if string(out) != in {
		t.Errorf("disabled Apply changed content: %q", out)
	}
	if res.Consumed != nil || res.Missing != nil {
		t.Errorf("disabled Apply recorded vars: %+v", res)
	}
	var nilResolver *Resolver
	if nilResolver.Enabled() {
		t.Error("nil resolver should be disabled")
	}
}

func TestUnusedPins(t *testing.T) {
	r := newTestResolver()
	if _, res := r.Apply([]byte("${PRE_PINNED} ${PRE_ENV}")); len(res.Missing) != 0 {
		t.Fatalf("unexpected missing: %v", res.Missing)
	}
	r.Apply([]byte("${PRE_SHADOWED}"))
	want := []string{"PRE_UNUSED"}
	if got := r.UnusedPins(); !reflect.DeepEqual(got, want) {
		t.Errorf("UnusedPins = %v, want %v", got, want)
	}
}
