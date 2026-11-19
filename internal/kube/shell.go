package kube

// Shell identifies the container the user selected in the cluster view.
type Shell struct {
	Kubectl, Context, Namespace, Pod, Container string
}

// Args returns the interactive kubectl command for that container.
func (s Shell) Args() []string {
	return []string{s.Kubectl, "--context=" + s.Context, "-n", s.Namespace, "exec", "-it", s.Pod, "-c", s.Container, "--", "sh"}
}
