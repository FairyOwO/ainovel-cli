package assets

import (
	"reflect"
	"strings"
	"testing"
)

func TestLoadIncludesReversalToolkitReference(t *testing.T) {
	refs := reflect.ValueOf(Load("default").References)
	field := refs.FieldByName("ReversalToolkit")
	if !field.IsValid() {
		t.Fatal("Load should expose ReversalToolkit reference")
	}
	if got := field.String(); !strings.Contains(got, "反转") {
		t.Fatalf("expected reversal toolkit content to mention 反转, got %q", got)
	}
}
