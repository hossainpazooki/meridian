package meridianv1

import (
	"reflect"
	"sort"
	"testing"
)

// The read-only guarantee is a fact about the proto: exactly these three
// methods, all unary, none of them a write. Any future RPC added to the
// service fails this test until the design is amended.
func TestReaderServiceIsReadOnly(t *testing.T) {
	sd := File_meridian_v1_read_proto.Services().ByName("Reader")
	if sd == nil {
		t.Fatal("service Reader missing from descriptor")
	}
	want := []string{"AsOf", "Head", "Reconcile"}
	var got []string
	for i := 0; i < sd.Methods().Len(); i++ {
		m := sd.Methods().Get(i)
		got = append(got, string(m.Name()))
		if m.IsStreamingClient() || m.IsStreamingServer() {
			t.Fatalf("method %s is streaming; the read API is unary only", m.Name())
		}
	}
	sort.Strings(got)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Reader methods = %v, want exactly %v", got, want)
	}
}
