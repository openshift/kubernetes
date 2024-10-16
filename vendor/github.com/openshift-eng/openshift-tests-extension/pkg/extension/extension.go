package extension

import (
	"fmt"
	"strings"

	et "github.com/openshift-eng/openshift-tests-extension/pkg/extension/extensiontests"
	"github.com/openshift-eng/openshift-tests-extension/pkg/version"
)

func NewExtension(product, kind, name string) *Extension {
	return &Extension{
		APIVersion: CurrentExtensionAPIVersion,
		Source: Source{
			Commit:       version.CommitFromGit,
			BuildDate:    version.BuildDate,
			GitTreeState: version.GitTreeState,
		},
		Component: Component{
			Product: product,
			Kind:    kind,
			Name:    name,
		},
	}
}

func (e *Extension) GetSuite(name string) (*Suite, error) {
	var suite *Suite

	for _, s := range e.Suites {
		if s.Name == name {
			suite = &s
			break
		}
	}

	if suite == nil {
		return nil, fmt.Errorf("no such suite: %s", name)
	}

	return suite, nil
}

func (e *Extension) GetSpecs() et.ExtensionTestSpecs {
	return e.specs
}

func (e *Extension) AddSpecs(specs et.ExtensionTestSpecs) {
	specs.Walk(func(spec *et.ExtensionTestSpec) {
		spec.Source = e.Component.Identifier()
	})

	e.specs = append(e.specs, specs...)
}

// IgnoreObsoleteTests allows removal of a test.
func (e *Extension) IgnoreObsoleteTests(testNames ...string) {
	e.obsoleteTests = append(e.obsoleteTests, testNames...)
}

// FindRemovedTestsWithoutRename compares two collections of specs, and if specs is missing a test from oldSpecs,
// including consideration of other names, we return an error.  Can be used to detect test renames or removals.
func (e *Extension) FindRemovedTestsWithoutRename(oldSpecs et.ExtensionTestSpecs) ([]string, error) {
	currentSpecs := e.GetSpecs()
	// It's neat we can do it with CEL but can it handle it when we've got 10K tests in there?
	potentiallyMissing, err := oldSpecs.Filter([]string{fmt.Sprintf(`!(name in %s)`, strSliceToCEL(currentSpecs.Names()))})
	if err != nil {
		return nil, err
	}

	actuallyMissing, err := potentiallyMissing.Filter([]string{fmt.Sprintf(`!(%s.exists(d, name == d))`,
		strSliceToCEL(currentSpecs.OtherNames()))})
	if err != nil {
		return nil, err
	}

	var unpermittedMissingTests []string
	for _, spec := range actuallyMissing {
		missing := true
		for _, allowed := range e.obsoleteTests {
			if spec.Name == allowed {
				missing = false
			}
		}
		if missing {
			unpermittedMissingTests = append(unpermittedMissingTests, spec.Name)
		}
	}

	if len(unpermittedMissingTests) > 0 {
		return unpermittedMissingTests, fmt.Errorf("some tests were not found")
	}

	return nil, nil
}

// AddGlobalSuite adds a suite whose qualifiers will apply to all tests,
// not just this one.  Allowing a developer to create a composed suite of
// tests from many sources.
func (e *Extension) AddGlobalSuite(suite Suite) *Extension {
	if e.Suites == nil {
		e.Suites = []Suite{suite}
	} else {
		e.Suites = append(e.Suites, suite)
	}

	return e
}

// AddSuite adds a suite whose qualifiers will only apply to tests present
// in its own extension.
func (e *Extension) AddSuite(suite Suite) *Extension {
	expr := fmt.Sprintf("source == %q", e.Component.Identifier())
	for i := range suite.Qualifiers {
		suite.Qualifiers[i] = fmt.Sprintf("(%s) && (%s)", expr, suite.Qualifiers[i])
	}
	e.AddGlobalSuite(suite)
	return e
}

func (e *Extension) FindSpecsByName(names ...string) (et.ExtensionTestSpecs, error) {
	var specs et.ExtensionTestSpecs
	var notFound []string

	for _, name := range names {
		found := false
		for i := range e.specs {
			if e.specs[i].Name == name {
				specs = append(specs, e.specs[i])
				found = true
				break
			}
		}
		if !found {
			notFound = append(notFound, name)
		}
	}

	if len(notFound) > 0 {
		return nil, fmt.Errorf("no such tests: %s", strings.Join(notFound, ", "))
	}

	return specs, nil
}

func (e *Component) Identifier() string {
	return fmt.Sprintf("%s:%s:%s", e.Product, e.Kind, e.Name)
}
