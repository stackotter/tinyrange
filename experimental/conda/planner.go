package main

import (
	"fmt"
	"io"
	"log/slog"

	"github.com/tinyrange/tinyrange/experimental/planner2"
)

type ErrNoInstallationCandidates planner2.PackageOptions

// Error implements error.
func (e ErrNoInstallationCandidates) Error() string {
	return fmt.Sprintf("no installation candidates found for %s", planner2.PackageOptions(e))
}

var (
	_ error = ErrNoInstallationCandidates{}
)

var (
	ErrAlreadyInstalled = planner2.ErrAlreadyInstated
)

type packageOption struct {
	query planner2.PackageQuery
	pkg   planner2.Package
}

type InstallationContext struct {
	// A list of sources to find packages.
	Sources []planner2.PackageSource
	// A parent context for the installation.
	Parent *InstallationContext
	// The current node in the tree.
	Current *InstallationPlan
	// The root node in the tree.
	Root *InstallationPlan
}

func (ctx *InstallationContext) search(opts planner2.PackageOptions) ([]packageOption, error) {
	if ctx.Parent != nil {
		return ctx.Parent.search(opts)
	}

	var results []packageOption

	for _, source := range ctx.Sources {
		for _, opt := range opts {
			packages, err := source.Find(opt)
			if err != nil {
				return nil, err
			}

			for _, pkg := range packages {
				results = append(results, packageOption{
					query: opt,
					pkg:   pkg,
				})
			}
		}
	}

	return results, nil
}

func (ctx *InstallationContext) childContext(query planner2.PackageOptions) *InstallationContext {
	return &InstallationContext{
		Parent: ctx,
		Current: &InstallationPlan{
			QueryOptions: query,
		},
		Root: ctx.Root,
	}
}

func (ctx *InstallationContext) add() (*InstallationPlan, error) {
	// Find a list of candidates that could satisfy the query.
	err := ctx.Current.findCandidates(ctx)
	if err != nil {
		return nil, err
	}

	for _, candidate := range ctx.Current.Candidates {
		ctx.Current = &InstallationPlan{
			QueryOptions: ctx.Current.QueryOptions,
			Installer:    candidate.Installer,
			Query:        candidate.Query,
		}

		// Install all the package dependencies.
		depends, err := ctx.Current.Installer.Dependencies()
		if err != nil {
			return nil, err
		}

		for _, dep := range depends {
			childCtx := ctx.childContext(dep)

			child, err := childCtx.add()
			if err != nil {
				continue
			}

			ctx.Current.Children = append(ctx.Current.Children, child)
		}

		return ctx.Current, nil
	}

	return nil, ErrNoInstallationCandidates(ctx.Current.QueryOptions)
}

// Check if a given package query is already installed.
func (ctx *InstallationContext) getVersion(q planner2.PackageName) string {
	if ctx.Parent != nil {
		if version := ctx.Parent.getVersion(q); version != "" {
			return version
		}
	} else {
		if version := ctx.Root.getVersion(q); version != "" {
			return version
		}
	}

	if version := ctx.Current.getVersion(q); version != "" {
		return version
	}

	return ""
}

// Check if a given package query is installable (as in it doesn't have any conflicts).
func (ctx *InstallationContext) isInstallable(q planner2.PackageName) bool {
	return true
}

func newContext(sources []planner2.PackageSource, root *InstallationPlan) *InstallationContext {
	return &InstallationContext{
		Sources: sources,
		Root:    root,
		Current: root,
	}
}

type InstallerOption struct {
	Installer planner2.Installer
	Query     planner2.PackageQuery
}

// A installation planner for conda packages.
// Installation is represented as as a tree of packages.
// Each node in the tree represents a package that is to be installed.
// We have a single top level which is a virtual package that represents the
// list of user requested packages.
// Part of the design of this planner is it does not maintain global state so
// options for package installation can be tested in isolation.
type InstallationPlan struct {
	// The list of package options that could satisfy the query.
	QueryOptions planner2.PackageOptions
	// The query that the installer is based on.
	Query planner2.PackageQuery
	// The list of possible installation options.
	Candidates []InstallerOption
	// The concrete installer for the package (If the package was installed).
	Installer planner2.Installer
	// A list of dependencies that are required to install the package.
	Children []*InstallationPlan
}

func (plan *InstallationPlan) getVersion(name planner2.PackageName) string {
	if plan.Installer != nil {
		if plan.Installer.Name().Name == name.Name {
			return plan.Installer.Name().Version
		}
	}

	for _, child := range plan.Children {
		if version := child.getVersion(name); version != "" {
			return version
		}
	}

	return ""
}

func (plan *InstallationPlan) findCandidates(ctx *InstallationContext) error {
	// Search for a installer that matches the query.
	opts, err := ctx.search(plan.QueryOptions)
	if err != nil {
		return err
	}

	// Collect the list of installers.
	var installers []InstallerOption

	for _, opt := range opts {
		installerOptions, err := opt.pkg.Installers()
		if err != nil {
			return err
		}

		for _, installer := range installerOptions {
			installers = append(installers, InstallerOption{
				Installer: installer,
				Query:     opt.query,
			})
		}
	}

	plan.Candidates = installers

	return nil
}

func (plan *InstallationPlan) Add(sources []planner2.PackageSource, opts planner2.PackageOptions) error {
	ctx := newContext(sources, plan)

	childCtx := ctx.childContext(opts)

	child, err := childCtx.add()
	if err != nil {
		return err
	}

	ctx.Current.Children = append(ctx.Current.Children, child)

	return nil
}

func (plan *InstallationPlan) dumpTree(w io.Writer, prefix string) {
	fmt.Fprintf(w, "%s%s\n", prefix, plan.QueryOptions)
	for _, child := range plan.Children {
		child.dumpTree(w, prefix+"  ")
	}
}

func (plan *InstallationPlan) DumpTree(w io.Writer) {
	plan.dumpTree(w, "")
}

func (plan *InstallationPlan) ResolveConstraints() error {
	// A conflict is defined as two installed versions with the same name but incompatible requirements.

	installed := make(map[string][]planner2.PackageQuery)
	packageQueue := []*InstallationPlan{plan}

	for len(packageQueue) > 0 {
		packageNode := packageQueue[0]
		packageQueue = packageQueue[1:]

		installed[packageNode.Query.Name] = append(installed[packageNode.Query.Name], packageNode.Query)

		packageQueue = append(packageQueue, packageNode.Children...)
	}

	for name, queries := range installed {
		if name == "" {
			continue
		}

		conditions := make(map[string]planner2.Condition)

		for _, query := range queries {
			condition := query.Condition

			if condition == nil {
				continue
			}

			conditions[condition.Key()] = condition
		}

		// Get a combined condition for the package.
		var combined planner2.Condition
		for _, condition := range conditions {
			if combined == nil {
				combined = condition
			} else {
				combined = planner2.CombineConditions(combined, condition)
			}
		}

		slog.Info("", "name", name, "condition", combined)
	}

	return nil
}

func NewPlan() *InstallationPlan {
	return &InstallationPlan{}
}
