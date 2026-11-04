package versions

import (
	"reflect"
	"testing"
)

func TestParseGoReleases(t *testing.T) {
	data := `[{"version":"go1.27.3","stable":true,"files":[]},{"version":"go1.28rc1","stable":false},{"version":"go1.26.10","stable":true}]`
	got, err := ParseGoReleases([]byte(data))
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"1.27.3", "1.26.10"}; !reflect.DeepEqual(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
	if _, err := ParseGoReleases([]byte("{")); err == nil {
		t.Error("broken list parsed")
	}
}
