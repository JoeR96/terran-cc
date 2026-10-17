package cli

import (
	"net/url"
	"strconv"

	"github.com/spf13/cobra"

	"terrancc/internal/model"
)

func (a *app) projectCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "project", Short: "Manage projects"}

	var archived bool
	ls := &cobra.Command{
		Use:   "ls",
		Short: "List projects",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := a.client()
			if err != nil {
				return err
			}
			ps, err := c.ListProjects(archived)
			if err != nil {
				return err
			}
			return a.emit(ps, func() {
				for _, p := range ps {
					a.printf("%-20s %s\n", p.Slug, p.Name)
				}
			})
		},
	}
	ls.Flags().BoolVar(&archived, "archived", false, "include archived")

	var slug, repo, desc string
	add := &cobra.Command{
		Use:   "add <name>",
		Short: "Add a project",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := a.client()
			if err != nil {
				return err
			}
			p, err := c.AddProject(map[string]any{"name": args[0], "slug": slug, "repo_path": repo, "description": desc})
			if err != nil {
				return err
			}
			return a.emit(p, func() { a.printf("added %s (%s)\n", p.Name, p.Slug) })
		},
	}
	add.Flags().StringVar(&slug, "slug", "", "short name used in commands (default from the name)")
	add.Flags().StringVar(&repo, "repo", "", "git repo path, used when dispatching tickets")
	add.Flags().StringVar(&desc, "desc", "", "one-line description")

	edit := &cobra.Command{
		Use:   "edit <slug>",
		Short: "Change a project",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			patch := map[string]any{}
			for flag, key := range map[string]string{"name": "name", "slug": "slug", "repo": "repo_path", "desc": "description"} {
				if cmd.Flags().Changed(flag) {
					v, _ := cmd.Flags().GetString(flag)
					patch[key] = v
				}
			}
			if cmd.Flags().Changed("archive") {
				v, _ := cmd.Flags().GetBool("archive")
				patch["archived"] = v
			}
			c, err := a.client()
			if err != nil {
				return err
			}
			p, err := c.EditProject(args[0], patch)
			if err != nil {
				return err
			}
			return a.emit(p, func() { a.printf("updated %s (%s)\n", p.Name, p.Slug) })
		},
	}
	for _, f := range []string{"name", "slug", "repo", "desc"} {
		edit.Flags().String(f, "", "")
	}
	edit.Flags().Bool("archive", false, "archive (or --archive=false to restore)")

	show := &cobra.Command{
		Use:   "show <slug>",
		Short: "Show a project and how many items sit in each status",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := a.client()
			if err != nil {
				return err
			}
			p, err := c.GetProject(args[0])
			if err != nil {
				return err
			}
			items, err := c.ListItems(url.Values{"project": {strconv.FormatInt(p.ID, 10)}})
			if err != nil {
				return err
			}
			counts := map[model.Status]int{}
			for _, it := range items {
				counts[it.Status]++
			}
			out := struct {
				model.Project
				Counts map[model.Status]int `json:"counts"`
			}{p, counts}
			return a.emit(out, func() {
				a.printf("%s (%s)\n", p.Name, p.Slug)
				if p.RepoPath != "" {
					a.printf("  repo: %s\n", p.RepoPath)
				}
				for _, s := range model.Statuses {
					if counts[s] > 0 {
						a.printf("  %-12s %d\n", s, counts[s])
					}
				}
			})
		},
	}
	cmd.AddCommand(ls, add, edit, show)
	return cmd
}
