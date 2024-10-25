package main

import (
	"encoding/json"
	"flag"
	"log/slog"
	"os"
	"slices"
	"strings"

	"github.com/tinyrange/tinyrange/experimental/planner2"
	"github.com/tinyrange/tinyrange/pkg/builder"
	"github.com/tinyrange/tinyrange/pkg/common"
	"github.com/tinyrange/tinyrange/pkg/database"
)

type condaRequirement string

// Key implements planner2.Condition.
func (req condaRequirement) Key() string {
	return string(req)
}

// Satisfies implements planner2.Condition.
func (req condaRequirement) Satisfies(name planner2.PackageName) (planner2.MatchResult, error) {
	if req.Matches(name.Version) {
		return planner2.MatchResultMatched, nil
	}

	return planner2.MatchResultNoMatch, nil
}

func (req condaRequirement) Matches(ver string) bool {
	if strings.HasPrefix(string(req), ">=") {
		reqString := strings.TrimPrefix(string(req), ">=")
		return strings.Compare(ver, reqString) >= 0
	} else if strings.HasPrefix(string(req), "<") {
		reqString := strings.TrimPrefix(string(req), "<")
		return strings.Compare(ver, reqString) < 0
	} else if strings.HasSuffix(string(req), "*") {
		reqString := strings.TrimSuffix(string(req), "*")
		return strings.HasPrefix(ver, reqString)
	} else {
		return ver == string(req)
	}
}

var (
	_ planner2.Condition = condaRequirement("")
)

type condaDepend string

func (dep condaDepend) Name() string {
	name, _, _ := strings.Cut(string(dep), " ")

	return name
}

func (dep condaDepend) Requirements() planner2.Condition {
	_, requirements, ok := strings.Cut(string(dep), " ")
	if !ok {
		return nil
	}

	// Remove build.
	requirements, _, _ = strings.Cut(requirements, " ")

	if requirements == "*" {
		return planner2.IdentityCondition{}
	} else if strings.Contains(requirements, ",") {
		var ret planner2.AndCondition

		for _, requirement := range strings.Split(requirements, ",") {
			ret = append(ret, condaRequirement(requirement))
		}

		return ret
	} else if strings.Contains(requirements, "|") {
		var ret planner2.OrCondition

		for _, requirement := range strings.Split(requirements, "|") {
			ret = append(ret, condaRequirement(requirement))
		}

		return ret
	} else {
		return planner2.AndCondition{condaRequirement(requirements)}
	}
}

func (dep condaDepend) Build() string {
	_, requirements, ok := strings.Cut(string(dep), " ")
	if !ok {
		return ""
	}

	_, build, _ := strings.Cut(requirements, " ")

	return build
}

type condaPackage struct {
	Build       string        `json:"build"`
	PropName    string        `json:"name"`
	PropVersion string        `json:"version"`
	License     string        `json:"license"`
	Depends     []condaDepend `json:"depends"`

	filename string
}

// Conflicts implements planner2.Installer.
func (c condaPackage) Conflicts() ([]planner2.PackageQuery, error) {
	return []planner2.PackageQuery{}, nil
}

// Dependencies implements planner2.Installer.
func (c condaPackage) Dependencies() ([]planner2.PackageOptions, error) {
	var ret []planner2.PackageOptions

	for _, dep := range c.Depends {
		ret = append(ret, planner2.PackageOptions{
			planner2.PackageQuery{
				Name:      dep.Name(),
				Condition: dep.Requirements(),
			},
		})
	}

	return ret, nil
}

// Directives implements planner2.Installer.
func (c condaPackage) Directives() ([]planner2.Directive, error) {
	return []planner2.Directive{}, nil
}

// Tags implements planner2.Installer.
func (c condaPackage) Tags() planner2.TagList {
	return planner2.TagList{}
}

// Aliases implements planner2.Package.
func (c condaPackage) Aliases() []planner2.PackageName {
	return []planner2.PackageName{c.Name()}
}

// Installers implements planner2.Package.
func (c condaPackage) Installers() ([]planner2.Installer, error) {
	return []planner2.Installer{c}, nil
}

// Name implements planner2.Package.
func (c condaPackage) Name() planner2.PackageName {
	return planner2.PackageName{
		Name:    c.PropName,
		Version: c.PropVersion,
	}
}

var (
	_ planner2.Package   = condaPackage{}
	_ planner2.Installer = condaPackage{}
)

type condaRepoData struct {
	Packages map[string]condaPackage `json:"packages"`

	index map[string][]condaPackage
}

func (repo *condaRepoData) createIndex() {
	repo.index = make(map[string][]condaPackage)

	for filename, pkg := range repo.Packages {
		pkg.filename = filename
		repo.index[pkg.PropName] = append(repo.index[pkg.PropName], pkg)
	}

	for name := range repo.index {
		idx := repo.index[name]

		slices.SortFunc(idx, func(a condaPackage, b condaPackage) int {
			return strings.Compare(a.PropVersion, b.PropVersion)
		})

		repo.index[name] = idx
	}
}

// Find implements planner2.PackageSource.
func (repo *condaRepoData) Find(q planner2.PackageQuery) ([]planner2.Package, error) {
	var ret []planner2.Package

	pkgs, ok := repo.index[q.Name]
	if !ok {
		return nil, nil
	}

	for _, pkg := range pkgs {
		if q.Condition != nil {
			match, err := q.Condition.Satisfies(pkg.Name())
			if err != nil {
				return nil, err
			}

			if match != planner2.MatchResultMatched {
				continue
			}
		}

		ret = append(ret, pkg)
	}

	// Sort the results to ensure the same order of results is always returned.
	slices.SortStableFunc(ret, func(a planner2.Package, b planner2.Package) int {
		return strings.Compare(a.Name().Name, b.Name().Name)
	})

	return ret, nil
}

var (
	_ planner2.PackageSource = &condaRepoData{}
)

func fromCondaQuery(query string) planner2.PackageQuery {
	q := condaDepend(query)

	return planner2.PackageQuery{
		Name:      q.Name(),
		Condition: q.Requirements(),
	}
}

var (
	doQuery = flag.String("query", "", "Query to run")
)

func appMain() error {
	flag.Parse()

	db := database.New("build/build")

	def := builder.NewFetchHttpBuildDefinition("https://conda.anaconda.org/conda-forge/linux-64/repodata.json", 0, nil)

	ctx := db.NewBuildContext(def)

	f, err := db.Build(ctx, def, common.BuildOptions{})
	if err != nil {
		return err
	}

	fh, err := f.Open()
	if err != nil {
		return err
	}
	defer fh.Close()

	var data condaRepoData

	if err := json.NewDecoder(fh).Decode(&data); err != nil {
		return err
	}

	data.createIndex()

	slog.Info("loaded", "pkgs", len(data.Packages))

	if *doQuery != "" {
		q := fromCondaQuery(*doQuery)

		pkgs, err := data.Find(q)
		if err != nil {
			return err
		}

		for _, pkg := range pkgs {
			slog.Info("found", "pkg", pkg)
		}

		return nil
	} else {
		// Make a plan.

		plan := NewPlan()

		for _, pkg := range flag.Args() {
			if err := plan.Add(
				[]planner2.PackageSource{&data},
				planner2.PackageOptions{fromCondaQuery(pkg)},
			); err != nil {
				return err
			}
		}

		if err := plan.ResolveConstraints(); err != nil {
			return err
		}

		// plan.DumpTree(os.Stdout)

		return nil
	}
}

func main() {
	if err := appMain(); err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}
