package label

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"

	et "github.com/openshift-eng/openshift-tests-extension/pkg/extension/extensiontests"
	"k8s.io/apimachinery/pkg/util/sets"
)

func Run(otePath string) {
	if len(os.Args) != 2 && len(os.Args) != 3 {
		fmt.Fprintf(os.Stderr, "error: requires exactly one argument\n")
		os.Exit(1)
	}
	filename := os.Args[len(os.Args)-1]

	oteCmd := exec.Command(otePath, "list", "tests")
	// We can't have OTE also add annotations to the spec names to map to labels, or they won't match the actual spec names
	oteCmd.Env = append(oteCmd.Env, "OMIT_ANNOTATIONS=true")
	oteCmd.Stderr = os.Stderr
	output, err := oteCmd.Output()
	if err != nil {
		fmt.Println("Error running ote list tests command:", err)
	}
	var specs et.ExtensionTestSpecs
	if err = json.Unmarshal(output, &specs); err != nil {
		fmt.Println("Error parsing ote list tests output:", err)
	}

	nameToLabels := make(map[string]sets.Set[string])
	for _, spec := range specs {
		existingLabels := nameToLabels[spec.Name]
		if existingLabels == nil {
			existingLabels = sets.New[string]()
		}
		for label := range spec.Labels {
			existingLabels.Insert(label)
		}
		nameToLabels[spec.Name] = existingLabels
	}

	var pairs []string
	for name, labels := range nameToLabels {
		labelList := labels.UnsortedList()
		sort.Strings(labelList)
		pairs = append(pairs, fmt.Sprintf("%q:%q", name, strings.Join(labelList, ",")))
	}
	sort.Strings(pairs)

	contents := fmt.Sprintf(`
package generated

var Labels = map[string]string{
%s,
}

`, strings.Join(pairs, ",\n\n"))

	if err := os.WriteFile(filename, []byte(contents), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v", err)
		os.Exit(1)
	}
	if _, err := exec.Command("gofmt", "-s", "-w", filename).Output(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v", err)
		os.Exit(1)
	}
}
