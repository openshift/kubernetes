package ingress

import (
	"strings"
	"testing"

	configv1 "github.com/openshift/api/config/v1"
)

func TestValidateCreateSpec(t *testing.T) {
	tests := []struct {
		name        string
		hostname    string
		expectedErr string
	}{
		{
			name:        "empty",
			hostname:    "",
			expectedErr: `spec.componentRoutes[0].hostname: Invalid value: "": host must conform to DNS 1123 naming conventions: [spec.componentRoutes[0].hostname: Required value]`,
		},
		{
			name:        "new validation fails",
			hostname:    "host",
			expectedErr: `spec.componentRoutes[0].hostname: Invalid value: "host": host must conform to DNS 1123 naming conventions: [spec.componentRoutes[0].hostname: Invalid value: "host": should be a domain with at least two segments separated by dots]`,
		},
		{
			name:        "new validation passes",
			hostname:    "host.com",
			expectedErr: ``,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			actual := validateIngressSpecCreate(
				configv1.IngressSpec{
					ComponentRoutes: []configv1.ComponentRouteSpec{
						{
							Hostname: configv1.Hostname(tc.hostname),
						},
					},
				},
			)
			switch {
			case len(actual) == 0 && len(tc.expectedErr) == 0:
			case len(actual) == 0 && len(tc.expectedErr) != 0:
				t.Fatalf("didn't get expected error: %v", tc.expectedErr)
			case len(actual) != 0 && len(tc.expectedErr) == 0:
				t.Fatalf("unexpected error: %v", actual)
			case len(actual) != 0 && len(tc.expectedErr) != 0:
				found := false
				for _, actualErr := range actual {
					found = found || strings.Contains(actualErr.Error(), tc.expectedErr)
				}
				if !found {
					t.Fatalf("got %q, expected %q", actual, tc.expectedErr)
				}
			default:
			}

		})
	}
}

func TestValidateUpdateSpec(t *testing.T) {
	tests := []struct {
		name        string
		hostname    string
		oldHostname string
		noPrevious  bool
		expectedErr string
	}{
		{
			name:        "no change",
			hostname:    "111!!! invalid I think",
			oldHostname: "111!!! invalid I think",
			expectedErr: ``,
		},
		{
			name:        "change from invalid to valid",
			hostname:    "host.com",
			oldHostname: "host",
			expectedErr: ``,
		},
		{
			name:        "change from valid to used-to-be-valid",
			hostname:    "host",
			oldHostname: "host.com",
			expectedErr: `spec.componentRoutes[0].hostname: Invalid value: "host": host must conform to DNS 1123 naming conventions: [spec.componentRoutes[0].hostname: Invalid value: "host": should be a domain with at least two segments separated by dots]`,
		},
		{
			name:        "change from not-valid to still-not-valid",
			hostname:    "hoststillfails",
			oldHostname: "host",
			expectedErr: `spec.componentRoutes[0].hostname: Invalid value: "hoststillfails": host must conform to DNS 1123 naming conventions: [spec.componentRoutes[0].hostname: Invalid value: "hoststillfails": should be a domain with at least two segments separated by dots]`,
		},
		{
			name:        "no previous value and illegal",
			hostname:    "host",
			oldHostname: "nohost",
			noPrevious:  true,
			expectedErr: `spec.componentRoutes[0].hostname: Invalid value: "host": host must conform to DNS 1123 naming conventions: [spec.componentRoutes[0].hostname: Invalid value: "host": should be a domain with at least two segments separated by dots]`,
		},
		{
			name:        "no previous value and legal",
			hostname:    "host.com",
			oldHostname: "nohost",
			noPrevious:  true,
			expectedErr: ``,
		},
		{
			name:        "no previous value to empty, empty is illegal",
			hostname:    "",
			oldHostname: "nohost",
			noPrevious:  true,
			expectedErr: `spec.componentRoutes[0].hostname: Invalid value: "": host must conform to DNS 1123 naming conventions: [spec.componentRoutes[0].hostname: Required value]`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			oldName := "match"
			if tc.noPrevious {
				oldName = "no-match"
			}
			actual := validateIngressSpecUpdate(
				configv1.IngressSpec{
					ComponentRoutes: []configv1.ComponentRouteSpec{
						{
							Name:     "match",
							Hostname: configv1.Hostname(tc.hostname),
						},
					},
				},
				configv1.IngressSpec{
					ComponentRoutes: []configv1.ComponentRouteSpec{
						{
							Name:     oldName,
							Hostname: configv1.Hostname(tc.oldHostname),
						},
					},
				})

			switch {
			case len(actual) == 0 && len(tc.expectedErr) == 0:
			case len(actual) == 0 && len(tc.expectedErr) != 0:
				t.Fatalf("didn't get expected error: %v", tc.expectedErr)
			case len(actual) != 0 && len(tc.expectedErr) == 0:
				t.Fatalf("unexpected error: %v", actual)
			case len(actual) != 0 && len(tc.expectedErr) != 0:
				found := false
				for _, actualErr := range actual {
					found = found || strings.Contains(actualErr.Error(), tc.expectedErr)
				}
				if !found {
					t.Fatalf("got %q, expected %q", actual, tc.expectedErr)
				}
			default:
			}

		})
	}
}

func TestValidateUpdateStatus(t *testing.T) {
	tests := []struct {
		name                string
		defaultHostname     string
		oldDefaultHostname  string
		currentHostnames    []configv1.Hostname
		oldCurrentHostnames []configv1.Hostname
		noPrevious          bool
		expectedErr         string
	}{
		{
			name:               "defaultHostname: no change",
			defaultHostname:    "111!!! invalid I think",
			oldDefaultHostname: "111!!! invalid I think",
			expectedErr:        ``,
		},
		{
			name:               "defaultHostname: change from invalid to valid",
			defaultHostname:    "host.com",
			oldDefaultHostname: "host",
			expectedErr:        ``,
		},
		{
			name:               "defaultHostname: change from valid to used-to-be-valid",
			defaultHostname:    "host",
			oldDefaultHostname: "host.com",
			expectedErr:        `status.componentRoutes[0].defaultHostname: Invalid value: "host": host must conform to DNS 1123 naming conventions: [status.componentRoutes[0].defaultHostname: Invalid value: "host": should be a domain with at least two segments separated by dots]`,
		},
		{
			name:               "defaultHostname: change from not-valid to still-not-valid",
			defaultHostname:    "hoststillfails",
			oldDefaultHostname: "host",
			expectedErr:        `status.componentRoutes[0].defaultHostname: Invalid value: "hoststillfails": host must conform to DNS 1123 naming conventions: [status.componentRoutes[0].defaultHostname: Invalid value: "hoststillfails": should be a domain with at least two segments separated by dots]`,
		},
		{
			name:               "defaultHostname: no previous value and illegal",
			defaultHostname:    "host",
			oldDefaultHostname: "nohost",
			noPrevious:         true,
			expectedErr:        `status.componentRoutes[0].defaultHostname: Invalid value: "host": host must conform to DNS 1123 naming conventions: [status.componentRoutes[0].defaultHostname: Invalid value: "host": should be a domain with at least two segments separated by dots]`,
		},
		{
			name:               "defaultHostname: no previous value and legal",
			defaultHostname:    "host.com",
			oldDefaultHostname: "nohost",
			noPrevious:         true,
			expectedErr:        ``,
		},
		{
			name:               "defaultHostname: no previous value to empty, empty is illegal",
			defaultHostname:    "",
			oldDefaultHostname: "nohost",
			noPrevious:         true,
			expectedErr:        `status.componentRoutes[0].defaultHostname: Invalid value: "": host must conform to DNS 1123 naming conventions: [status.componentRoutes[0].defaultHostname: Required value]`,
		},

		{
			name:                "currentHostnames: no change",
			defaultHostname:     "host.com",
			currentHostnames:    []configv1.Hostname{configv1.Hostname("111!!! invalid I think")},
			oldCurrentHostnames: []configv1.Hostname{configv1.Hostname("111!!! invalid I think")},
			expectedErr:         ``,
		},
		{
			name:                "currentHostnames: change from invalid to valid",
			defaultHostname:     "host.com",
			currentHostnames:    []configv1.Hostname{configv1.Hostname("host.com")},
			oldCurrentHostnames: []configv1.Hostname{configv1.Hostname("host")},
			expectedErr:         ``,
		},
		{
			name:                "currentHostnames: change from valid to used-to-be-valid",
			defaultHostname:     "host.com",
			currentHostnames:    []configv1.Hostname{configv1.Hostname("host")},
			oldCurrentHostnames: []configv1.Hostname{configv1.Hostname("host.com")},
			expectedErr:         `status.componentRoutes[0].currentHostnames[0]: Invalid value: "host": host must conform to DNS 1123 naming conventions: [status.componentRoutes[0].currentHostnames[0]: Invalid value: "host": should be a domain with at least two segments separated by dots]`,
		},
		{
			name:                "currentHostnames: change from not-valid to still-not-valid",
			defaultHostname:     "host.com",
			currentHostnames:    []configv1.Hostname{configv1.Hostname("hoststillfails")},
			oldCurrentHostnames: []configv1.Hostname{configv1.Hostname("host")},
			expectedErr:         `status.componentRoutes[0].currentHostnames[0]: Invalid value: "hoststillfails": host must conform to DNS 1123 naming conventions: [status.componentRoutes[0].currentHostnames[0]: Invalid value: "hoststillfails": should be a domain with at least two segments separated by dots]`,
		},
		{
			name:                "currentHostnames: no previous value and illegal",
			defaultHostname:     "host.com",
			currentHostnames:    []configv1.Hostname{configv1.Hostname("host")},
			oldCurrentHostnames: []configv1.Hostname{configv1.Hostname("nohost")},
			noPrevious:          true,
			expectedErr:         `status.componentRoutes[0].currentHostnames[0]: Invalid value: "host": host must conform to DNS 1123 naming conventions: [status.componentRoutes[0].currentHostnames[0]: Invalid value: "host": should be a domain with at least two segments separated by dots]`,
		},
		{
			name:                "currentHostnames: no previous value and legal",
			defaultHostname:     "host.com",
			currentHostnames:    []configv1.Hostname{configv1.Hostname("host.com")},
			oldCurrentHostnames: []configv1.Hostname{configv1.Hostname("nohost")},
			noPrevious:          true,
			expectedErr:         ``,
		},
		{
			name:                "currentHostnames: no previous value to empty, empty is illegal",
			defaultHostname:     "host.com",
			currentHostnames:    []configv1.Hostname{configv1.Hostname("")},
			oldCurrentHostnames: []configv1.Hostname{configv1.Hostname("nohost")},
			noPrevious:          true,
			expectedErr:         `status.componentRoutes[0].currentHostnames[0]: Invalid value: "": host must conform to DNS 1123 naming conventions: [status.componentRoutes[0].currentHostnames[0]: Required value]`,
		},
		{
			name:                "currentHostnames: add new legal value with illegal value remaining",
			defaultHostname:     "host.com",
			currentHostnames:    []configv1.Hostname{configv1.Hostname("host.com"), configv1.Hostname("111!!! invalid I think")},
			oldCurrentHostnames: []configv1.Hostname{configv1.Hostname("111!!! invalid I think")},
			expectedErr:         ``,
		},
		{
			name:                "currentHostnames: add new illegal value with illegal value remaining",
			defaultHostname:     "host.com",
			currentHostnames:    []configv1.Hostname{configv1.Hostname("host"), configv1.Hostname("111!!! invalid I think")},
			oldCurrentHostnames: []configv1.Hostname{configv1.Hostname("111!!! invalid I think")},
			expectedErr:         `status.componentRoutes[0].currentHostnames[0]: Invalid value: "host": host must conform to DNS 1123 naming conventions: [status.componentRoutes[0].currentHostnames[0]: Invalid value: "host": should be a domain with at least two segments separated by dots]`,
		},
		{
			name:                "currentHostnames: add new illegal value to the end with illegal value remaining",
			defaultHostname:     "host.com",
			currentHostnames:    []configv1.Hostname{configv1.Hostname("111!!! invalid I think"), configv1.Hostname("host")},
			oldCurrentHostnames: []configv1.Hostname{configv1.Hostname("111!!! invalid I think")},
			expectedErr:         `status.componentRoutes[0].currentHostnames[1]: Invalid value: "host": host must conform to DNS 1123 naming conventions: [status.componentRoutes[0].currentHostnames[1]: Invalid value: "host": should be a domain with at least two segments separated by dots]`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			oldName := "match"
			if tc.noPrevious {
				oldName = "no-match"
			}
			actual := validateIngressStatusUpdate(
				configv1.IngressStatus{
					ComponentRoutes: []configv1.ComponentRouteStatus{
						{
							Name:             "match",
							DefaultHostname:  configv1.Hostname(tc.defaultHostname),
							CurrentHostnames: tc.currentHostnames,
						},
					},
				},
				configv1.IngressStatus{
					ComponentRoutes: []configv1.ComponentRouteStatus{
						{
							Name:             oldName,
							DefaultHostname:  configv1.Hostname(tc.oldDefaultHostname),
							CurrentHostnames: tc.oldCurrentHostnames,
						},
					},
				})

			switch {
			case len(actual) == 0 && len(tc.expectedErr) == 0:
			case len(actual) == 0 && len(tc.expectedErr) != 0:
				t.Fatalf("didn't get expected error: %v", tc.expectedErr)
			case len(actual) != 0 && len(tc.expectedErr) == 0:
				t.Fatalf("unexpected error: %v", actual)
			case len(actual) != 0 && len(tc.expectedErr) != 0:
				found := false
				for _, actualErr := range actual {
					found = found || strings.Contains(actualErr.Error(), tc.expectedErr)
				}
				if !found {
					t.Fatalf("got %q, expected %q", actual, tc.expectedErr)
				}
			default:
			}

		})
	}
}
