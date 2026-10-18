package cli

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"terrancc/internal/core"
	"terrancc/internal/model"
)

// slugs maps project ids to slugs for display.
func (a *app) slugs() (map[int64]string, error) {
	c, err := a.client()
	if err != nil {
		return nil, err
	}
	ps, err := c.ListProjects(true)
	if err != nil {
		return nil, err
	}
	m := map[int64]string{}
	for _, p := range ps {
		m[p.ID] = p.Slug
	}
	return m, nil
}

func (a *app) printItems(items []model.Item) error {
	return a.emit(items, func() {
		if len(items) == 0 {
			a.printf("nothing here\n")
			return
		}
		slugs, _ := a.slugs()
		for _, it := range items {
			a.printf("%s\n", itemLine(it, slugs))
		}
	})
}

func (a *app) printItem(it model.Item) error {
	return a.emit(it, func() {
		slugs, _ := a.slugs()
		a.printf("%s\n", itemLine(it, slugs))
	})
}

func (a *app) addItemCmds(root *cobra.Command) {
	root.AddCommand(a.addCmd(false), a.addCmd(true), a.lsCmd(), a.nextCmd(), a.showCmd(), a.editCmd(),
		a.startCmd(), a.noteCmd(), a.evidenceCmd(), a.attachCmd(), a.rmCmd())
	for _, action := range []string{"review", "done", "reopen"} {
		root.AddCommand(&cobra.Command{
			Use:   action + " <id>",
			Short: map[string]string{"review": "Move an item to Review", "done": "Mark an item done", "reopen": "Reopen a done item"}[action],
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				return a.itemAction(args[0], action, nil)
			},
		})
	}
}

func (a *app) itemAction(ref, action string, body any) error {
	id, err := parseID(ref)
	if err != nil {
		return err
	}
	c, err := a.client()
	if err != nil {
		return err
	}
	it, err := c.Action(id, action, body)
	if err != nil {
		return err
	}
	return a.printItem(it)
}

// projectID resolves a slug or id to a project id.
func (a *app) projectID(ref string) (*int64, error) {
	c, err := a.client()
	if err != nil {
		return nil, err
	}
	p, err := c.GetProject(ref)
	if err != nil {
		return nil, err
	}
	return &p.ID, nil
}

// addCmd builds "add" (a ticket or to-do) or, when todo is set, "todo" (a
// personal to-do for today).
func (a *app) addCmd(todo bool) *cobra.Command {
	var n core.NewItem
	var project, kind, when, ctx, parent string
	cmd := &cobra.Command{
		Use:   "add <title>",
		Short: "Add a ticket (with --project) or a to-do",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			n.Title = strings.Join(args, " ")
			n.Kind, n.When, n.Context = model.Kind(kind), model.When(when), model.Context(ctx)
			if project != "" {
				pid, err := a.projectID(project)
				if err != nil {
					return err
				}
				n.ProjectID = pid
			}
			if parent != "" {
				id, err := parseID(parent)
				if err != nil {
					return err
				}
				n.ParentID = &id
			}
			c, err := a.client()
			if err != nil {
				return err
			}
			it, err := c.AddItem(n)
			if err != nil {
				return err
			}
			return a.printItem(it)
		},
	}
	f := cmd.Flags()
	f.StringVar(&when, "when", "", "today|week|month|anytime")
	f.StringVar(&ctx, "context", "", "work|irl|osrs|tech")
	if todo {
		cmd.Use, cmd.Short = "todo <title>", "Add a personal to-do (for today unless --when says otherwise)"
		f.Lookup("when").DefValue, when = "today", "today"
		return cmd
	}
	f.StringVar(&project, "project", "", "project slug or id (makes it a ticket)")
	f.StringVar(&kind, "kind", "", "todo|bug|task|feature|idea")
	f.BoolVar(&n.Inbox, "inbox", false, "file it in the project's Inbox")
	f.IntVar(&n.Priority, "priority", 0, "higher sorts first in the backlog")
	f.StringVar(&parent, "parent", "", "parent feature id (makes this a sub-task)")
	f.StringVar(&n.Notes, "notes", "", "markdown notes")
	return cmd
}

func (a *app) lsCmd() *cobra.Command {
	var project, status, when, kind, ctx, query string
	var personal, all bool
	cmd := &cobra.Command{
		Use:   "ls",
		Short: "List items (hides done and someday unless --all or --status)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			q := url.Values{}
			for k, v := range map[string]string{"project": project, "when": when, "kind": kind, "context": ctx, "q": query} {
				if v != "" {
					q.Set(k, v)
				}
			}
			if personal {
				q.Set("personal", "1")
			}
			if status == "" && !all {
				status = "inbox,backlog,up_next,in_progress,review"
			}
			if status != "" {
				q.Set("status", status)
			}
			c, err := a.client()
			if err != nil {
				return err
			}
			items, err := c.ListItems(q)
			if err != nil {
				return err
			}
			return a.printItems(items)
		},
	}
	f := cmd.Flags()
	f.StringVar(&project, "project", "", "project slug or id")
	f.BoolVar(&personal, "personal", false, "only personal to-dos")
	f.StringVar(&status, "status", "", "comma-separated statuses")
	f.StringVar(&when, "when", "", "comma-separated: today,week,month,anytime")
	f.StringVar(&kind, "kind", "", "comma-separated kinds")
	f.StringVar(&ctx, "context", "", "work|irl|osrs|tech")
	f.StringVarP(&query, "query", "q", "", "title contains")
	f.BoolVar(&all, "all", false, "include done and someday")
	return cmd
}

func (a *app) nextCmd() *cobra.Command {
	var project string
	cmd := &cobra.Command{
		Use:   "next",
		Short: "Show Up next, in order",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			q := url.Values{"status": {"up_next"}}
			if project != "" {
				q.Set("project", project)
			}
			c, err := a.client()
			if err != nil {
				return err
			}
			items, err := c.ListItems(q)
			if err != nil {
				return err
			}
			return a.emit(items, func() {
				if len(items) == 0 {
					a.printf("nothing up next\n")
				}
				slugs, _ := a.slugs()
				for i, it := range items {
					a.printf("%d. %s\n", i+1, itemLine(it, slugs))
					if it.WhyFirst != "" {
						a.printf("     why first: %s\n", it.WhyFirst)
					}
					if len(it.DependsOn) > 0 {
						a.printf("     depends on: %s\n", ids(it.DependsOn))
					}
				}
			})
		},
	}
	cmd.Flags().StringVar(&project, "project", "", "project slug or id")
	return cmd
}

func ids(v []int64) string {
	s := make([]string, len(v))
	for i, n := range v {
		s[i] = "#" + strconv.FormatInt(n, 10)
	}
	return strings.Join(s, ", ")
}

func (a *app) showCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <id>",
		Short: "Show everything about an item",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseID(args[0])
			if err != nil {
				return err
			}
			c, err := a.client()
			if err != nil {
				return err
			}
			it, err := c.GetItem(id)
			if err != nil {
				return err
			}
			return a.emit(it, func() {
				slugs, _ := a.slugs()
				a.printf("#%d %s\n", it.ID, it.Title)
				field := func(name, v string) {
					if v != "" {
						a.printf("  %-12s %s\n", name+":", v)
					}
				}
				if it.ProjectID != nil {
					field("project", slugs[*it.ProjectID])
				}
				field("kind", string(it.Kind))
				field("status", string(it.Status))
				field("when", string(it.When))
				field("context", string(it.Context))
				if it.Priority != 0 {
					field("priority", strconv.Itoa(it.Priority))
				}
				field("why first", it.WhyFirst)
				if len(it.DependsOn) > 0 {
					field("depends on", ids(it.DependsOn))
				}
				if it.ParentID != nil {
					field("parent", "#"+strconv.FormatInt(*it.ParentID, 10))
				}
				field("model", it.Model)
				field("effort", it.Effort)
				field("branch", it.Branch)
				field("worktree", it.Worktree)
				if it.Session != "" {
					s := it.Session
					if it.SessionModel != "" {
						s += " (" + it.SessionModel + ")"
					}
					if it.SessionStartedAt != nil {
						s += " started " + it.SessionStartedAt.Local().Format("2006-01-02 15:04")
					}
					field("session", s)
				}
				for _, e := range it.Evidence {
					a.printf("  evidence:    %s %s\n", e.Type, e.Value)
				}
				for _, p := range it.Attachments {
					a.printf("  attachment:  %s\n", p)
				}
				if it.Notes != "" {
					a.printf("\n%s\n", it.Notes)
				}
			})
		},
	}
}

func (a *app) editCmd() *cobra.Command {
	str := map[string]*string{}
	ints := map[string]*int{}
	cmd := &cobra.Command{
		Use:   "edit <id>",
		Short: "Change an item's fields",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseID(args[0])
			if err != nil {
				return err
			}
			patch := map[string]any{}
			for flag, v := range str {
				if !cmd.Flags().Changed(flag) {
					continue
				}
				key := strings.ReplaceAll(flag, "-", "_")
				switch flag {
				case "project":
					if *v == "" {
						patch["project_id"] = nil
					} else {
						patch["project_id"] = *v
					}
				case "parent":
					if *v == "" {
						patch["parent_id"] = nil
					} else {
						pid, err := parseID(*v)
						if err != nil {
							return err
						}
						patch["parent_id"] = pid
					}
				case "depends-on":
					deps := []int64{}
					for _, part := range strings.Split(*v, ",") {
						if strings.TrimSpace(part) == "" {
							continue
						}
						d, err := parseID(part)
						if err != nil {
							return err
						}
						deps = append(deps, d)
					}
					patch["depends_on"] = deps
				default:
					patch[key] = *v
				}
			}
			for flag, v := range ints {
				if cmd.Flags().Changed(flag) {
					patch[flag] = *v
				}
			}
			if len(patch) == 0 {
				return fmt.Errorf("nothing to change; see tcc edit --help")
			}
			c, err := a.client()
			if err != nil {
				return err
			}
			it, err := c.EditItem(id, patch)
			if err != nil {
				return err
			}
			return a.printItem(it)
		},
	}
	for _, f := range [][2]string{
		{"title", ""}, {"notes", "replace the notes"}, {"kind", "todo|bug|task|feature|idea"},
		{"status", "inbox|backlog|up_next|in_progress|review|done|someday"}, {"when", "today|week|month|anytime"},
		{"context", "work|irl|osrs|tech"}, {"why-first", "why this goes first in Up next"},
		{"depends-on", "comma-separated ids; empty clears"}, {"project", "slug or id; empty makes it personal"},
		{"parent", "parent id; empty clears"}, {"branch", ""}, {"worktree", ""},
		{"model", "model for dispatch, e.g. \"Opus 5.5\" or \"Plan: Fable 5.1 high. Build: Sonnet 5.5 medium.\""},
		{"effort", "low|medium|high|xhigh|max"},
	} {
		str[f[0]] = cmd.Flags().String(f[0], "", f[1])
	}
	ints["priority"] = cmd.Flags().Int("priority", 0, "higher sorts first")
	ints["position"] = cmd.Flags().Int("position", 0, "order in Up next")
	return cmd
}

func (a *app) startCmd() *cobra.Command {
	var info core.StartInfo
	cmd := &cobra.Command{
		Use:   "start <id>",
		Short: "Move an item to In progress and record where the work happens",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.itemAction(args[0], "start", info)
		},
	}
	f := cmd.Flags()
	f.StringVar(&info.Branch, "branch", "", "git branch")
	f.StringVar(&info.Worktree, "worktree", "", "worktree path")
	f.StringVar(&info.Session, "session", "", "Claude session or herdr agent name")
	f.StringVar(&info.SessionModel, "session-model", "", "model id the session runs on")
	return cmd
}

func (a *app) noteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "note <id> <text>",
		Short: "Append a timestamped note",
		Args:  cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseID(args[0])
			if err != nil {
				return err
			}
			c, err := a.client()
			if err != nil {
				return err
			}
			it, err := c.Note(id, strings.Join(args[1:], " "))
			if err != nil {
				return err
			}
			return a.printItem(it)
		},
	}
}

func (a *app) evidenceCmd() *cobra.Command {
	vals := map[string]*[]string{}
	cmd := &cobra.Command{
		Use:   "evidence <id>",
		Short: "Attach proof of work: commits, PRs, screenshots, test runs, notes",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseID(args[0])
			if err != nil {
				return err
			}
			var ev []model.Evidence
			for _, typ := range model.EvidenceTypes {
				for _, v := range *vals[typ] {
					ev = append(ev, model.Evidence{Type: typ, Value: v})
				}
			}
			if len(ev) == 0 {
				return fmt.Errorf("give at least one of --commit --pr --screenshot --test --note")
			}
			c, err := a.client()
			if err != nil {
				return err
			}
			it, err := c.Evidence(id, ev)
			if err != nil {
				return err
			}
			return a.printItem(it)
		},
	}
	for _, typ := range model.EvidenceTypes {
		vals[typ] = cmd.Flags().StringArray(typ, nil, "repeatable")
	}
	return cmd
}

func (a *app) attachCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "attach <id> <file>",
		Short: "Upload a file (e.g. a screenshot) to an item",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseID(args[0])
			if err != nil {
				return err
			}
			c, err := a.client()
			if err != nil {
				return err
			}
			it, err := c.Attach(id, args[1])
			if err != nil {
				return err
			}
			return a.printItem(it)
		},
	}
}

func (a *app) rmCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "rm <id>",
		Short: "Delete an item",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := parseID(args[0])
			if err != nil {
				return err
			}
			c, err := a.client()
			if err != nil {
				return err
			}
			if err := c.DeleteItem(id); err != nil {
				return err
			}
			return a.emit(map[string]any{"deleted": id}, func() { a.printf("deleted #%d\n", id) })
		},
	}
}
