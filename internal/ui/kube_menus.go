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

func podLogContainer(p kube.Pod, crashed string) string {
	if crashed != "" || len(p.Containers) == 0 {
		return crashed
	}
	return p.Containers[0].Name
}

// podMenu is shared by right-click and the visible menu button.
func (a *App) podMenu(c *ui.Context, cl *kube.Cluster, p kube.Pod, log func(), crashed string) func(*ui.Menu) {
	s := &a.cluster
	return func(m *ui.Menu) {
		if log != nil && m.Item(locales.KubeLogsOpen()).Chosen() {
			log()
		}
		if a.OpenKubeShell != nil && len(p.Containers) > 0 && m.Item(locales.KubeShellAction()+"…").Chosen() {
			container := podLogContainer(p, crashed)
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
	}
}

func (a *App) podGroupRef(cl *kube.Cluster, g podGroup) ops.KubeResource {
	if d := slices.IndexFunc(cl.Deployments, func(d kube.Deployment) bool {
		return g.owner.Kind == "Deployment" && d.Namespace == g.namespace && d.Name == g.owner.Name
	}); d >= 0 {
		return a.resourceRef("Deployment", g.namespace, g.owner.Name, cl.Deployments[d].UID)
	}
	return ops.KubeResource{}
}

// ownerMenu is shared by right-click and the group's menu button.
func (a *App) ownerMenu(c *ui.Context, g podGroup, ref ops.KubeResource, log func()) func(*ui.Menu) {
	s := &a.cluster
	return func(m *ui.Menu) {
		if ref.UID != "" {
			if m.Item(locales.KubeOpenOwner(g.owner.Name)).Chosen() {
				s.kind, s.deployment, s.tab = kindDeployments, ref.UID, 0
			}
			if log != nil && m.Item(locales.KubeOwnerLogsAll()).Chosen() {
				log()
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
	}
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
