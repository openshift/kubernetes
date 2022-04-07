package infrastructure

import (
	"testing"

	apiv1 "github.com/openshift/api/config/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var testCases = []struct {
	name            string
	infra           *apiv1.Infrastructure
	expectedErrorNb int
}{
	{
		name: "infra with no resourceTags, no errors",
		infra: &apiv1.Infrastructure{
			ObjectMeta: metav1.ObjectMeta{
				Name:            "cluster",
				ResourceVersion: "1",
			},
			Spec: apiv1.InfrastructureSpec{
				CloudConfig: apiv1.ConfigMapFileReference{},
				PlatformSpec: apiv1.PlatformSpec{
					Type: "AWS",
					AWS:  &apiv1.AWSPlatformSpec{},
				},
			},
		},
		expectedErrorNb: 0,
	},
	{
		name: "infra with valid resourceTags, no errors",
		infra: &apiv1.Infrastructure{
			ObjectMeta: metav1.ObjectMeta{
				Name:            "cluster",
				ResourceVersion: "1",
			},
			Spec: apiv1.InfrastructureSpec{
				CloudConfig: apiv1.ConfigMapFileReference{},
				PlatformSpec: apiv1.PlatformSpec{
					Type: "AWS",
					AWS: &apiv1.AWSPlatformSpec{
						ResourceTags: []apiv1.AWSResourceTagSpec{
							{
								Key:   "Customer",
								Value: "ACME",
							},
						},
					},
				},
			},
		},
		expectedErrorNb: 0,
	},
	{
		name: "infra with single invalid resourceTags, error",
		infra: &apiv1.Infrastructure{
			ObjectMeta: metav1.ObjectMeta{
				Name:            "cluster",
				ResourceVersion: "1",
			},
			Spec: apiv1.InfrastructureSpec{
				CloudConfig: apiv1.ConfigMapFileReference{},
				PlatformSpec: apiv1.PlatformSpec{
					Type: "AWS",
					AWS: &apiv1.AWSPlatformSpec{
						ResourceTags: []apiv1.AWSResourceTagSpec{
							{
								Key:   "Customer",
								Value: "ACME",
							},
							{
								Key:   "shall.not.use.kubernetes.io",
								Value: "whatever",
							},
						},
					},
				},
			},
		},
		expectedErrorNb: 1,
	},
	{
		name: "infra with multiple invalid resourceTags, error",
		infra: &apiv1.Infrastructure{
			ObjectMeta: metav1.ObjectMeta{
				Name:            "cluster",
				ResourceVersion: "1",
			},
			Spec: apiv1.InfrastructureSpec{
				CloudConfig: apiv1.ConfigMapFileReference{},
				PlatformSpec: apiv1.PlatformSpec{
					Type: "AWS",
					AWS: &apiv1.AWSPlatformSpec{
						ResourceTags: []apiv1.AWSResourceTagSpec{
							{
								Key:   "shall.not.use.openshift.io",
								Value: "nonesense",
							},
							{
								Key:   "shall.not.use.kubernetes.io:anything",
								Value: "whatever",
							},
							{
								Key:   "can.use.Kubernetes.io:anything",
								Value: "whatever",
							},
							{
								Key:   "something.openshift.somethingelse:valid",
								Value: "whatever",
							},
							{
								Key:   "contains.dot.valid",
								Value: "whatever",
							},
							{
								Key:   "valid:_index",
								Value: "whatever",
							},
							{
								Key:   "aws:invalid",
								Value: "whatever",
							},
							{
								Key:   "aws.region.valid",
								Value: "whatever",
							},
						},
					},
				},
			},
		},
		expectedErrorNb: 3,
	},
	{
		name: "infra with duplicate resourceTags, error",
		infra: &apiv1.Infrastructure{
			ObjectMeta: metav1.ObjectMeta{
				Name:            "cluster",
				ResourceVersion: "1",
			},
			Spec: apiv1.InfrastructureSpec{
				CloudConfig: apiv1.ConfigMapFileReference{},
				PlatformSpec: apiv1.PlatformSpec{
					Type: "AWS",
					AWS: &apiv1.AWSPlatformSpec{
						ResourceTags: []apiv1.AWSResourceTagSpec{
							{
								Key:   "must.be.unique",
								Value: "once",
							},
							{
								Key:   "must.be.unique",
								Value: "twice",
							},
							{
								Key:   "customer_tag",
								Value: "whatever",
							},
						},
					},
				},
			},
		},
		expectedErrorNb: 1,
	},
}

func TestValidateCreate(t *testing.T) {

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			errs := infrastructureV1{}.ValidateCreate(tc.infra)
			if tc.expectedErrorNb != len(errs) {
				t.Errorf("Expected %d errors but got %d errors: %v", tc.expectedErrorNb, len(errs), errs)
			}
		})
	}
}

func TestValidateUpdate(t *testing.T) {

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			errs := infrastructureV1{}.ValidateUpdate(tc.infra, tc.infra)
			if tc.expectedErrorNb != len(errs) {
				t.Errorf("Expected %d errors but got %d errors: %v", tc.expectedErrorNb, len(errs), errs)
			}
		})
	}
}
