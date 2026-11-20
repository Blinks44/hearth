package docker

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/Blinks44/hearth/internal/run"
)

func TestDockerResources(t *testing.T) {
	container, network := strings.Repeat("a", 64), strings.Repeat("b", 64)
	for _, tc := range []struct {
		name   string
		kind   ResourceKind
		args   []string
		list   string
		format string
		id     string
		row    string
	}{
		{
			name: "volume", kind: VolumeResource, id: "shop_uploads", format: inspectVolume,
			args: []string{"volume", "ls", "--format", "{{json .}}"}, list: `{"Name":"shop_uploads"}`,
			row: `{"ID":"shop_uploads","Name":"shop_uploads","Project":"shop","Driver":"local"}`,
		},
		{
			name: "network", kind: NetworkResource, id: network, format: inspectNetwork,
			args: []string{"network", "ls", "--format", "{{json .}}", "--no-trunc"}, list: fmt.Sprintf(`{"ID":%q}`, network),
			row: fmt.Sprintf(`{"ID":%q,"Name":"shop_default","Project":"shop","Driver":"bridge","IPAM":[{"Subnet":"192.0.2.0/24"},{"Subnet":"2001:db8::/64"}]}`, network),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var r run.Fake
			r.Stdout(tc.list, colima.Command(nil, tc.args...).Args...)
			r.Stdout(tc.row, colima.Command(nil, string(tc.kind), "inspect", "--format", tc.format, "--", tc.id).Args...)
			r.Stdout(fmt.Sprintf(`{"ID":%q}`, container), colima.Command(nil, "ps", "--all", "--no-trunc", "--format", "{{json .}}").Args...)
			r.Stdout(fmt.Sprintf(`{"ID":%q,"Name":"/shop-api","State":{"Status":"exited"},"Compose":{"Name":"shop"},"Mounts":[{"Type":"volume","Name":"shop_uploads"},{"Type":"volume","Name":"shop_uploads"}],"Networks":{"shop_default":{"NetworkID":%q}}}`, container, network),
				colima.Command(nil, "container", "inspect", "--format", inspectContainer, "--", container).Args...)
			items, err := readResources(t.Context(), &r, nil, colima, tc.kind)
			if err != nil || len(items) != 1 {
				t.Fatalf("resources = %+v, err = %v", items, err)
			}
			item := items[0]
			if item.Project != "shop" || item.Size != -1 || len(item.Users) != 1 ||
				item.Users[0] != (ResourceUser{ID: container, Name: "shop-api", Project: "shop"}) {
				t.Fatalf("resource = %+v", item)
			}
			if tc.kind == NetworkResource && (len(item.Subnets) != 2 || item.Subnets[1] != "2001:db8::/64") {
				t.Fatalf("subnets = %v", item.Subnets)
			}
		})
	}
}

func TestDockerVolumeSizes(t *testing.T) {
	var r run.Fake
	r.Stdout(string(fixture(t, "volumes-df.json")),
		colima.Command(nil, "system", "df", "--verbose", "--format", "{{json .Volumes}}").Args...)
	items := []Resource{{Name: "shop_uploads", Size: -1}, {Name: "empty", Size: -1}, {Name: "external", Size: -1}}
	if err := readVolumeSizes(t.Context(), &r, nil, colima, items); err != nil {
		t.Fatal(err)
	}
	if items[0].Size != 1200000000 || items[1].Size != 0 || items[2].Size != -1 {
		t.Fatalf("sizes = %+v", items)
	}
}

func TestDockerResourceWatchCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	var r run.Fake
	WatchResources(ctx, &r, nil, colima, VolumeResource, func([]Resource, error, error) {
		t.Error("read after cancellation")
	})
	if len(r.Calls()) != 0 {
		t.Fatal("ran commands after cancellation")
	}
}
