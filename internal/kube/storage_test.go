package kube

import (
	"testing"

	"github.com/Blinks44/hearth/internal/run"
)

func TestStorageClaims(t *testing.T) {
	for _, allowClasses := range []bool{true, false} {
		var fake run.Fake
		target := Target{Kubectl: "kubectl", Context: "dev", Namespaces: []string{"shop", "tools"},
			Resource: PersistentVolumeClaims, Events: Ref{Namespace: "shop", UID: "claim"}}
		getArgs := func(kind string, scope ...string) []string {
			args := append([]string{"kubectl", "--context=dev", "get", kind}, scope...)
			return append(args, "--output=json", "--request-timeout=15s")
		}
		fake.Stdout(`{"items":[{"metadata":{"name":"data","namespace":"shop","uid":"claim"},"spec":{"storageClassName":"fast","resources":{"requests":{"storage":"10Gi"}},"accessModes":["ReadWriteOnce"]},"status":{"phase":"Pending"}}]}`,
			getArgs("persistentvolumeclaims", "--namespace=shop")...)
		fake.Stdout(`{"items":[]}`, getArgs("persistentvolumeclaims", "--namespace=tools")...)
		classes := getArgs("storageclasses.storage.k8s.io")
		if allowClasses {
			fake.Stdout(`{"items":[{"metadata":{"name":"standard","annotations":{"storageclass.kubernetes.io/is-default-class":"true"}},"provisioner":"example.test/storage","volumeBindingMode":"WaitForFirstConsumer"}]}`, classes...)
		} else {
			fake.Set(run.Result{ExitCode: 1, Stderr: []byte("Forbidden")}, nil, classes...)
		}
		fake.Stdout(`{"items":[{"metadata":{"name":"web","namespace":"shop","uid":"pod"},"spec":{"volumes":[{"persistentVolumeClaim":{"claimName":"data"}},{"persistentVolumeClaim":{"claimName":"data"}}]},"status":{"phase":"Pending"}}]}`,
			getArgs("pods", "--namespace=shop")...)
		fake.Set(run.Result{ExitCode: 1, Stderr: []byte("Forbidden")}, nil, getArgs("pods", "--namespace=tools")...)
		fake.Stdout(`{"items":[{"reason":"FailedBinding","message":"no persistent volumes available for this claim and no storage class is set"}]}`,
			getArgs("events", "--namespace=shop", "--field-selector=involvedObject.uid=claim")...)
		got, err := Read(t.Context(), &fake, nil, target)
		if err != nil || got.Resources == nil || len(got.Resources.Items) != 1 {
			t.Fatalf("claims = %+v, %v", got.Resources, err)
		}
		item, details := got.Resources.Items[0], got.Resources.Claims
		if item.Storage.Requested != "10Gi" || item.Storage.Phase != "Pending" || details.MissingClass(item) != allowClasses {
			t.Fatalf("claim = %+v, details = %+v", item.Storage, details)
		}
		if allowClasses && (!details.Classes[0].Storage.Default || details.Classes[0].Storage.Provisioner != "example.test/storage") {
			t.Fatalf("classes = %+v", details.Classes)
		}
		if users := details.Consumers[ClaimKey{"shop", "data"}]; len(users) != 1 || users[0].UID != "pod" ||
			len(details.Consumers[ClaimKey{"tools", "data"}]) != 0 || details.ConsumersErr == nil {
			t.Fatalf("consumer read = %+v", details)
		}
		if NoVolumeEvent(got.Events) == "" || NoVolumeEvent([]Event{{Reason: "ProvisioningFailed", Message: "no persistent volumes available for this claim"}}) != "" {
			t.Fatalf("unexpected binding diagnosis for %+v", got.Events)
		}
	}
}

func TestStorageVolumeClusterScope(t *testing.T) {
	var fake run.Fake
	target := Target{Kubectl: "kubectl", Context: "dev", Namespaces: []string{"shop", "tools"},
		Resource: PersistentVolumes, Events: Ref{UID: "volume"}}
	fake.Stdout(`{"items":[{"metadata":{"name":"local-volume","uid":"volume"},"spec":{"claimRef":{"namespace":"shop","name":"data"},"capacity":{"storage":"10Gi"},"persistentVolumeReclaimPolicy":"Retain"},"status":{"phase":"Bound"}}]}`,
		"kubectl", "--context=dev", "get", "persistentvolumes", "--output=json", "--request-timeout=15s")
	fake.Stdout(`{"items":[]}`, "kubectl", "--context=dev", "get", "events", "--all-namespaces",
		"--field-selector=involvedObject.uid=volume", "--output=json", "--request-timeout=15s")
	got, err := Read(t.Context(), &fake, nil, target)
	if err != nil || got.Resources == nil || len(got.Resources.Items) != 1 || got.Resources.EventsErr != nil {
		t.Fatalf("volumes = %+v, %v", got.Resources, err)
	}
	item := got.Resources.Items[0]
	if s := item.Storage; item.Namespace != "" || s.ClaimName != "data" || s.ClaimNamespace != "shop" ||
		s.Capacity != "10Gi" || s.ReclaimPolicy != "Retain" {
		t.Fatalf("volume = %+v", item)
	}
	const yaml = "kind: PersistentVolume\nmetadata:\n  uid: volume\n"
	fake.Stdout(yaml, "kubectl", "--context=dev", "get", "persistentvolumes", "--output=yaml",
		"--request-timeout=15s", "--", "local-volume")
	if manifest, err := ReadResourceYAML(t.Context(), &fake, nil, target, item); err != nil || manifest != yaml {
		t.Fatalf("manifest = %q, %v", manifest, err)
	}
}
