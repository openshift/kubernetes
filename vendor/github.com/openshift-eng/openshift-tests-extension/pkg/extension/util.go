package extension

import (
	"fmt"
	"strings"
)

func strSliceToCEL(s []string) string {
	return fmt.Sprintf(`["%s"]`, strings.Join(s, `","`))
}
