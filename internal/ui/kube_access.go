package ui

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/Blinks44/hearth/internal/kube"
	"github.com/Blinks44/hearth/locales"
)

const (
	accessRulesTab = iota
	accessAccountsTab
	accessRolesTab
	accessBindingsTab
)

type accessView struct {
	tab, detailTab int
	selected       string
	reveal         string
}

func (v *accessView) resetSelection() {
	v.selected, v.reveal, v.detailTab = "", "", 0
}

func (v *accessView) selectRow(key string) {
	if v.selected == key {
		v.selected = ""
	} else {
		v.selected, v.reveal = key, key
	}
	v.detailTab = 0
}

func (a *App) accessNamespace(context string) string {
	names := a.clusterNamespaces(context)
	preferred := "default"
	if k := a.kubeContext(context); k != nil {
		preferred = namespaceOf(*k)
	}
	if len(names) == 0 || slices.Contains(names, preferred) {
		return preferred
	}
	return names[0]
}

func accessResourceTab(kind kube.ResourceKind) int {
	switch kind {
	case kube.ServiceAccounts:
		return accessAccountsTab
	case kube.Roles, kube.ClusterRoles:
		return accessRolesTab
	default:
		return accessBindingsTab
	}
}

func accessKindName(kind kube.ResourceKind) string {
	switch kind {
	case kube.ServiceAccounts:
		return "ServiceAccount"
	case kube.Roles:
		return "Role"
	case kube.ClusterRoles:
		return "ClusterRole"
	case kube.RoleBindings:
		return "RoleBinding"
	default:
		return "ClusterRoleBinding"
	}
}

func (a *App) clusterAccessView(c *ui.Context, context string, review *kube.AccessReview) {
	s := &a.cluster
	v := &s.access
	tabs := []segment{{locales.KubeAccessCanI(), ""}, {locales.KubeAccessAccounts(), ""}, {locales.KubeAccessRoles(), ""}, {locales.KubeAccessBindings(), ""}}
	if review.RulesErr == nil {
		tabs[accessRulesTab].count = strconv.Itoa(len(review.Rules))
	}
	for tab := accessAccountsTab; tab <= accessBindingsTab; tab++ {
		count, known := 0, true
		for _, list := range review.Resources {
			if accessResourceTab(list.Kind) == tab {
				count += len(list.Items)
				known = known && list.Err == nil
			}
		}
		if known {
			tabs[tab].count = strconv.Itoa(count)
		}
	}
	ui.Row(c).Gap(10).AlignItems(ui.Center).Wrap().Shrink(0).Children(func() {
		tab := v.tab
		segments(c, "access-tabs", &v.tab, tabs)
		if tab != v.tab {
			v.resetSelection()
		}
		ui.Box(c).Grow(1)
		placeholder := locales.KubeAccessSearch()
		if v.tab != accessRulesTab {
			placeholder = locales.KubeDeploymentsSearch()
		}
		ui.SearchField(c.Key("access-search"), &s.query).Bind(&s.queryField).Height(controlHeight).Width(220).
			Label(placeholder).Placeholder(placeholder).OnChange(v.resetSelection)
	})
	ui.Column(c.Key("access-tab-" + strconv.Itoa(v.tab))).Grow(1).MinHeight(0).Children(func() {
		if v.tab == accessRulesTab {
			a.accessRulesView(c, context, review)
		} else {
			a.accessResourcesView(c, review)
		}
	})
}

func accessRuleKey(r kube.AccessRule) string {
	return fmt.Sprintf("access-rule-%q", []string{r.Resource, r.URL, r.Names, r.Verbs})
}

func accessRuleTitle(r kube.AccessRule) string {
	if r.Resource != "" {
		return r.Resource
	}
	return r.URL
}

func (a *App) accessRulesView(c *ui.Context, context string, review *kube.AccessReview) {
	v := &a.cluster.access
	if review.RulesErr != nil {
		a.accessReadError(c, "kubectl auth can-i --list", review.RulesErr)
		return
	}
	if review.RulesWarning != "" {
		ui.Text(c, review.RulesWarning).TextColor(Colors(c.Theme()).WarningText).FontSize(12).
			SingleLine().Tooltip(review.RulesWarning)
	}
	q := resourceQuery(a.cluster.query)
	var shown []kube.AccessRule
	for _, rule := range review.Rules {
		if strings.Contains(strings.ToLower(strings.Join([]string{rule.Resource, rule.URL, rule.Names, rule.Verbs}, " ")), q) {
			shown = append(shown, rule)
		}
	}
	if len(shown) == 0 {
		message := locales.KubeAccessNoRules()
		if q != "" {
			message = locales.SearchNoResults(a.cluster.query)
		}
		toolNote(c, message)
		return
	}
	selected := v.selected
	if selected == "" {
		selected = accessRuleKey(shown[0])
	}
	index := slices.IndexFunc(shown, func(rule kube.AccessRule) bool { return accessRuleKey(rule) == selected })
	if index < 0 {
		selected = ""
	}
	tracks := []ui.Track{ui.Fr(1.25), ui.Fr(1)}
	workloadPanes(c, func() {
		resourceHead(c, tracks, locales.KubeAccessResource(), locales.KubeAccessVerbs())
	}, func() {
		for _, rule := range shown {
			key := accessRuleKey(rule)
			v.tableRow(c, tracks, key, selected, accessRuleTitle(rule), func() {
				if selected == key {
					v.selected, v.detailTab = "-", 0
				} else {
					v.selectRow(key)
				}
			}, func() {
				accessCell(c, accessRuleTitle(rule))
				workloadStatus(c, rule.Verbs, Colors(c.Theme()).SuccessSoft, Colors(c.Theme()).SuccessText)
			})
		}
	}, selected, func() { a.accessRuleDetail(c, context, review, shown[index]) })
}

func (v *accessView) tableRow(c *ui.Context, tracks []ui.Track, key, selected, title string, click, cells func()) {
	pal := Colors(c.Theme())
	row := resourceGrid(ui.ButtonBase(c.Key(key)), tracks).
		Label(title).Height(33).Padding(0, 16).BorderWidth(0, 0, 1, 0).BorderColor(pal.Line).OnClick(click)
	if key == selected {
		row.Background(pal.ActiveRow)
	} else if row.Hovered() {
		row.Background(pal.Subtle)
	}
	if key == v.reveal {
		row.ScrollIntoView()
		v.reveal = ""
	}
	row.Children(cells)
}

func accessCell(c *ui.Context, text string) {
	clipped(c, text).Font(FontMono).FontSize(12)
}

func (a *App) accessRuleDetail(c *ui.Context, context string, review *kube.AccessReview, rule kube.AccessRule) {
	s := &a.cluster
	v := &s.access
	detailFrame(c, v.detailTab, false, func() {
		accessDetailHeader(c, accessRuleTitle(rule), context+" / "+review.Namespace, func() {
			v.selected, v.detailTab = "-", 0
		}, func() {
			kubeActionButton(c.Key("access-refresh"), "refresh-cw", locales.KubeClusterRefresh(), podActionHeight).AlignSelf(ui.Start).
				OnClick(func() { s.refresh++ })
			segments(c, "access-detail-tabs", &v.detailTab, []segment{{locales.KubeTabOverview(), ""}, {locales.KubeAccessBindings(), ""}})
		})
	}, func() {
		if v.detailTab == 0 {
			detailSection(c, locales.KubeTabOverview())
			accessFacts(c, [][2]string{
				{locales.KubeAccessUser(), review.Identity.Username},
				{locales.KubeAccessGroups(), strings.Join(review.Identity.Groups, ", ")},
				{locales.KubeContextLabel(), context},
				{locales.KubeNamespaceLabel(), review.Namespace},
				{locales.KubeAccessSource(), "kubectl auth can-i --list"},
			})
			detailSection(c, locales.KubeAccessVerbs())
			accessFacts(c, [][2]string{
				{locales.KubeAccessResource(), rule.Resource},
				{locales.KubeAccessURL(), rule.URL},
				{locales.KubeAccessVerbs(), rule.Verbs},
				{locales.KubeAccessNames(), rule.Names},
			})
		}
		a.accessMatchingBindings(c, review)
	})
}

func accessDetailHeader(c *ui.Context, title, subtitle string, close, actions func()) {
	ui.Column(c).Gap(8).Padding(14, 16).Children(func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, Icon("key-round")).Size(16, 16).Shrink(0)
			clipped(c, title).FontSize(15).FontWeight(600).Grow(1).MinWidth(0)
			iconButton(c, "x", locales.KubeCloseDetails()).OnClick(close)
		})
		ui.Text(c, subtitle).FontSize(12).TextColor(c.Theme().TextMuted).Wrap()
		if actions != nil {
			actions()
		}
	})
}

func accessFacts(c *ui.Context, facts [][2]string) {
	factRows(c, facts)
}

func (a *App) accessReadError(c *ui.Context, title string, err error) {
	text := title + " · " + a.accessWords(err)
	ui.Text(c, text).FontSize(12).TextColor(Colors(c.Theme()).WarningText).SingleLine().Tooltip(err.Error()).Margin(0, 16)
}

func (a *App) accessMatchingBindings(c *ui.Context, review *kube.AccessReview) {
	detailSection(c, locales.KubeAccessMatchingBindings())
	if review.IdentityErr != nil {
		a.accessReadError(c, "kubectl auth whoami", review.IdentityErr)
		return
	}
	found, complete := false, true
	for _, list := range review.Resources {
		if accessResourceTab(list.Kind) != accessBindingsTab {
			continue
		}
		if list.Err != nil {
			a.accessReadError(c, accessKindName(list.Kind), list.Err)
			complete = false
		}
		for _, item := range list.Items {
			if !slices.ContainsFunc(item.RBAC.Subjects, func(subject kube.RBACSubject) bool {
				return review.Identity.Matches(subject, item.Namespace)
			}) {
				continue
			}
			found = true
			a.accessResourceLink(c.Key("matching-binding-"+item.UID), item,
				item.RBAC.RoleRef.Kind+" "+item.RBAC.RoleRef.Name)
		}
	}
	if !found && complete {
		storageReadNote(c, locales.KubeAccessNoBindings())
	}
}

func accessObjectKey(item kube.Resource) string {
	return "access-object-" + item.UID
}

func (a *App) accessResourceLink(c *ui.Context, item kube.Resource, detail string) {
	ui.Row(c).Gap(8).AlignItems(ui.Center).Padding(4, 16).Children(func() {
		clipped(c, accessKindName(item.Kind)+" "+item.Name).FontSize(12).Grow(1).MinWidth(0)
		if detail != "" {
			clipped(c, detail).FontSize(11.5).TextColor(c.Theme().TextMuted).Grow(1).MinWidth(0)
		}
		kubeActionButton(c, "chevron-right", locales.KubeClusterOpen(), podActionHeight).OnClick(func() {
			v := &a.cluster.access
			a.cluster.query = ""
			v.tab, v.selected, v.reveal, v.detailTab = accessResourceTab(item.Kind), accessObjectKey(item), accessObjectKey(item), 0
		})
	})
}

func (a *App) accessResourcesView(c *ui.Context, review *kube.AccessReview) {
	v := &a.cluster.access
	q := resourceQuery(a.cluster.query)
	var shown []kube.Resource
	failed := false
	for _, list := range review.Resources {
		if accessResourceTab(list.Kind) != v.tab {
			continue
		}
		if list.Err != nil {
			a.accessReadError(c, accessKindName(list.Kind), list.Err)
			failed = true
		}
		for _, item := range list.Items {
			if strings.Contains(strings.ToLower(item.Name+" "+item.Namespace+" "+accessKindName(item.Kind)), q) {
				shown = append(shown, item)
			}
		}
	}
	if len(shown) == 0 {
		if q != "" {
			toolNote(c, locales.SearchNoResults(a.cluster.query))
		} else if !failed {
			toolNote(c, locales.KubeResourcesEmpty())
		}
		return
	}
	slices.SortStableFunc(shown, func(a, b kube.Resource) int { return strings.Compare(a.Name, b.Name) })
	index := slices.IndexFunc(shown, func(item kube.Resource) bool { return accessObjectKey(item) == v.selected })
	key := ""
	if index >= 0 {
		key = v.selected
	}
	tracks := []ui.Track{ui.Fr(1), ui.Fixed(122)}
	workloadPanes(c, func() {
		resourceHead(c, tracks, locales.KubeClusterName(), locales.KubeAccessResource())
	}, func() {
		for _, item := range shown {
			id := accessObjectKey(item)
			v.tableRow(c, tracks, id, key, item.Name, func() { v.selectRow(id) }, func() {
				accessCell(c, item.Name)
				accessCell(c, accessKindName(item.Kind))
			})
		}
	}, key, func() { a.accessObjectDetail(c, review, shown[index]) })
}

func (a *App) accessObjectDetail(c *ui.Context, review *kube.AccessReview, item kube.Resource) {
	v := &a.cluster.access
	scope := item.Namespace
	if scope == "" {
		scope = locales.KubeCustomClusterScoped()
	}
	detailFrame(c, 0, false, func() {
		accessDetailHeader(c, item.Name, accessKindName(item.Kind)+" · "+scope, v.resetSelection, nil)
	}, func() {
		detailSection(c, locales.KubeTabOverview())
		accessFacts(c, [][2]string{{locales.KubeClusterAge(), otherResourceAge(c, item)}})
		if item.RBAC == nil {
			return
		}
		if ref := item.RBAC.RoleRef; ref.Name != "" {
			detailSection(c, ref.Kind)
			role, ok := accessRole(review, item)
			if ok {
				a.accessResourceLink(c.Key("binding-role-"+role.UID), role, "")
			} else {
				storageReadNote(c, ref.Name)
			}
			detailSection(c, locales.KubeAccessSubjects())
			var subjects [][2]string
			for _, subject := range item.RBAC.Subjects {
				name := subject.Name
				if subject.Namespace != "" {
					name = subject.Namespace + "/" + name
				}
				subjects = append(subjects, [2]string{subject.Kind, name})
			}
			accessFacts(c, subjects)
			return
		}
		if len(item.RBAC.Rules) == 0 {
			storageReadNote(c, locales.KubeAccessNoRules())
		}
		for i, rule := range item.RBAC.Rules {
			detailSection(c, locales.KubeAccessRule(i+1))
			accessFacts(c, [][2]string{
				{locales.KubeAccessAPIGroups(), strings.Join(rule.APIGroups, ", ")},
				{locales.KubeAccessResource(), strings.Join(rule.Resources, ", ")},
				{locales.KubeAccessURL(), strings.Join(rule.NonResourceURLs, ", ")},
				{locales.KubeAccessVerbs(), strings.Join(rule.Verbs, ", ")},
				{locales.KubeAccessNames(), strings.Join(rule.ResourceNames, ", ")},
			})
		}
	})
}

func accessRole(review *kube.AccessReview, binding kube.Resource) (kube.Resource, bool) {
	ref := binding.RBAC.RoleRef
	for _, list := range review.Resources {
		if list.Kind != kube.Roles && list.Kind != kube.ClusterRoles || accessKindName(list.Kind) != ref.Kind {
			continue
		}
		for _, role := range list.Items {
			if role.Name == ref.Name && (role.Kind == kube.ClusterRoles || role.Namespace == binding.Namespace) {
				return role, true
			}
		}
	}
	return kube.Resource{}, false
}
