package ui

import (
	"slices"
	"strconv"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/kube"
	"github.com/Blinks44/hearth/internal/ops"
	"github.com/Blinks44/hearth/locales"
)

// The rows of the cluster's tables open, on a right-click, the actions
// their details have, so that a pod's log or a deletion is one click
// away from the list. Each item does what its button in the details does.

// podMenu is a pod row's context menu.
func (a *App) podMenu(c *ui.Context, row ui.Element, cl *kube.Cluster, p kube.Pod, crashed string) {
	s := &a.cluster
	row.ContextMenu(func(m *ui.Menu) {
		if a.StreamLogs != nil && len(p.Containers) > 0 {
			container := p.Containers[0].Name
			if crashed != "" {
				container = crashed
			}
			if m.Item(locales.KubeLogsOpen()).Chosen() {
				a.openLogs(p, container, false)
			}
		}
		if a.OpenKubeShell != nil && len(p.Containers) > 0 && m.Item(locales.KubeShellAction()+"…").Chosen() {
			container := crashed
			if container == "" {
				container = p.Containers[0].Name
			}
			s.pod, s.tab, s.revealWorkload = p.UID, 0, p.UID
			s.shell = &shellForm{uid: p.UID, container: container, open: true}
		}
		if a.RunForward != nil && a.CheckForwardPort != nil && p.Phase == "Running" && m.Item(locales.KubeForwardOpen()).Chosen() {
			s.pod, s.tab, s.revealWorkload = p.UID, 0, p.UID
			var ports []forwardPort
			for _, port := range p.TCPPorts {
				ports = append(ports, forwardPort{port, port, strconv.Itoa(port) + "/TCP"})
			}
			a.openForward(p.UID, "pod", p.Namespace, p.Name, ports)
		}
		if owner := podOwnerDeployment(cl, p); owner != nil && m.Item(locales.KubeOpenOwner(owner.Name)).Chosen() {
			s.kind, s.deployment, s.tab = kindDeployments, owner.UID, 0
		}
		if m.Item(locales.KubeCopyName()).Chosen() {
			copyName(c, p.Name)
		}
		a.resourceMenuItems(m, a.resourceRef("Pod", p.Namespace, p.Name, p.UID))
	})
}

// ownerMenu is the context menu of a group of pods' owner.
func (a *App) ownerMenu(c *ui.Context, row ui.Element, cl *kube.Cluster, g podGroup) {
	s := &a.cluster
	var ref ops.KubeResource
	if d := slices.IndexFunc(cl.Deployments, func(d kube.Deployment) bool {
		return g.owner.Kind == "Deployment" && d.Namespace == g.namespace && d.Name == g.owner.Name
	}); d >= 0 {
		ref = a.resourceRef("Deployment", g.namespace, g.owner.Name, cl.Deployments[d].UID)
	}
	row.ContextMenu(func(m *ui.Menu) {
		if ref.UID != "" {
			if m.Item(locales.KubeOpenOwner(g.owner.Name)).Chosen() {
				s.kind, s.deployment, s.tab = kindDeployments, ref.UID, 0
			}
			if a.StreamLogs != nil && a.WatchCluster != nil && m.Item(locales.KubeOwnerLogsAll()).Chosen() {
				a.openOwnerLogs(ref)
			}
		}
		label := locales.KubeOpenGroup()
		if a.groupOpen(g) {
			label = locales.KubeCloseGroup()
		}
		if m.Item(label).Chosen() {
			a.toggleGroup(g)
		}
		if m.Item(locales.KubeCopyName()).Chosen() {
			copyName(c, g.owner.Name)
		}
		if ref.UID != "" {
			a.resourceMenuItems(m, ref)
		}
	})
}

// resourceContextMenu gives a row the actions every resource has.
func (a *App) resourceContextMenu(c *ui.Context, row ui.Element, ref ops.KubeResource, extra func(*ui.Menu)) {
	row.ContextMenu(func(m *ui.Menu) {
		if extra != nil {
			extra(m)
		}
		if m.Item(locales.KubeCopyName()).Chosen() {
			copyName(c, ref.Name)
		}
		a.resourceMenuItems(m, ref)
	})
}

// resourceMenuItems are Edit YAML and Delete, the actions every resource
// with a UID has, after a separator.
func (a *App) resourceMenuItems(m *ui.Menu, ref ops.KubeResource) {
	if a.BuildPlan == nil || ref.UID == "" {
		return
	}
	m.Separator()
	if m.Item(locales.KubeResourceEdit()).Chosen() {
		a.openResourceEditor(ref, -1)
	}
	if m.Item(locales.KubeResourceDelete()).Chosen() {
		a.planResourceChange(a.resourceForm(ref), true)
	}
}

// podOwnerDeployment is the Deployment the pod belongs to, nil for none
// read.
func podOwnerDeployment(cl *kube.Cluster, p kube.Pod) *kube.Deployment {
	if p.Owner.Kind != "Deployment" {
		return nil
	}
	for i, d := range cl.Deployments {
		if d.Namespace == p.Namespace && d.Name == p.Owner.Name {
			return &cl.Deployments[i]
		}
	}
	return nil
}

// copyName copies a resource's name, as its menu's Copy Name does.
func copyName(c *ui.Context, name string) {
	c.WriteClipboard(name)
	c.Toast(locales.KubeNameCopied(name))
}
