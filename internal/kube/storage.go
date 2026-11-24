package kube

import (
	"cmp"
	"context"
	"slices"
	"strings"

	"github.com/Blinks44/hearth/internal/run"
	"github.com/Blinks44/hearth/internal/sysenv"
)

// Storage contains the storage facts provided by the Kubernetes API.
type Storage struct {
	Phase, Requested, Capacity, Class, Volume string
	VolumeMode, BindingMode, ReclaimPolicy    string
	AccessModes                               []string
	ClaimNamespace, ClaimName                 string
	Provisioner                               string
	Default, Expandable                       bool
}

type volumeSpec struct {
	StorageClassName, VolumeName, VolumeMode, PersistentVolumeReclaimPolicy string
	AccessModes                                                             []string
	Resources                                                               struct{ Requests map[string]string }
	Capacity                                                                map[string]string
	ClaimRef                                                                struct{ Namespace, Name string }
}

type storageClassFields struct {
	Provisioner, VolumeBindingMode, ReclaimPolicy string
	AllowVolumeExpansion                          bool
}

func (o resourceObject) storage() Storage {
	return Storage{
		Phase: o.Status.Phase, Requested: o.Spec.Resources.Requests["storage"],
		Capacity: cmp.Or(o.Status.Capacity["storage"], o.Spec.Capacity["storage"]), Class: o.Spec.StorageClassName, Volume: o.Spec.VolumeName,
		AccessModes: o.Spec.AccessModes, VolumeMode: o.Spec.VolumeMode,
		ClaimNamespace: o.Spec.ClaimRef.Namespace, ClaimName: o.Spec.ClaimRef.Name,
		Provisioner: o.Provisioner, BindingMode: o.VolumeBindingMode,
		ReclaimPolicy: cmp.Or(o.ReclaimPolicy, o.Spec.PersistentVolumeReclaimPolicy),
		Default: o.Metadata.Annotations["storageclass.kubernetes.io/is-default-class"] == "true" ||
			o.Metadata.Annotations["storageclass.beta.kubernetes.io/is-default-class"] == "true",
		Expandable: o.AllowVolumeExpansion,
	}
}

// ClaimKey identifies a claim within the selected cluster.
type ClaimKey struct{ Namespace, Name string }

// ClaimConsumer is a pod that refers to a claim in its volumes.
type ClaimConsumer struct {
	Namespace, Name, UID, Phase string
}

// ClaimDetails contains independent reads. An error keeps incomplete data
// from turning into a claim of absence.
type ClaimDetails struct {
	Classes                  []Resource
	Consumers                map[ClaimKey][]ClaimConsumer
	ClassesErr, ConsumersErr error
}

func readClaimDetails(ctx context.Context, r run.Runner, env sysenv.Env, t Target) *ClaimDetails {
	d := &ClaimDetails{Consumers: map[ClaimKey][]ClaimConsumer{}}
	d.Classes, d.ClassesErr = ReadResources(ctx, r, env, t, StorageClasses)
	for _, scope := range namespaceScopes(t.Namespaces) {
		var list struct{ Items []pod }
		if err := get(ctx, r, env, t, append([]string{"pods"}, scope...), &list); err != nil {
			d.ConsumersErr = err
			continue
		}
		for _, p := range list.Items {
			m := p.Metadata
			consumer := ClaimConsumer{Namespace: m.Namespace, UID: m.UID, Name: m.Name, Phase: p.Status.Phase}
			for _, name := range p.claims() {
				key := ClaimKey{m.Namespace, name}
				d.Consumers[key] = append(d.Consumers[key], consumer)
			}
		}
	}
	return d
}

func (p pod) claims() []string {
	var names []string
	for _, v := range p.Spec.Volumes {
		if name := v.claimName(p.Metadata.Name); name != "" && !slices.Contains(names, name) {
			names = append(names, name)
		}
	}
	return names
}

func (v podVolume) claimName(podName string) string {
	if v.PersistentVolumeClaim.ClaimName != "" {
		return v.PersistentVolumeClaim.ClaimName
	}
	if podName != "" && v.Ephemeral != nil && v.Ephemeral.VolumeClaimTemplate != nil {
		return podName + "-" + v.Name
	}
	return ""
}

// MissingClass returns whether a pending claim names a class absent from a
// successful read of the cluster's storage classes.
func (d *ClaimDetails) MissingClass(r Resource) bool {
	return d != nil && d.ClassesErr == nil && r.Storage.Phase == "Pending" && r.Storage.Class != "" &&
		!slices.ContainsFunc(d.Classes, func(c Resource) bool { return c.Name == r.Storage.Class })
}

// NoVolumeEvent returns the controller's report when it could not find an
// available volume. It avoids guessing from the phase or from PV counts.
func NoVolumeEvent(events []Event) string {
	for _, e := range events {
		if e.Reason == "FailedBinding" && strings.Contains(e.Message, "no persistent volumes available for this claim") {
			return e.Message
		}
	}
	return ""
}
