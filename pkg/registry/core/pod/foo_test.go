package pod

import (
	"context"
	"encoding/json"
	"testing"

	v1 "k8s.io/kubernetes/pkg/apis/core/v1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	api "k8s.io/kubernetes/pkg/apis/core"

	corev1 "k8s.io/api/core/v1"
)

var podJSON = `{
  "apiVersion": "v1",
  "kind": "Pod",
  "metadata": {
    "annotations": {
      "ci-operator.openshift.io/container-sub-tests": "test",
      "ci-operator.openshift.io/save-container-logs": "true",
      "ci.openshift.io/job-spec": "{\"type\":\"presubmit\",\"job\":\"rehearse-16552-pull-ci-openshift-dptp-workflow-test-master-launch\",\"buildid\":\"1369706037107494912\",\"prowjobid\":\"6ed60414-81c8-11eb-9b46-0a580a832364\",\"refs\":{\"org\":\"openshift\",\"repo\":\"release\",\"repo_link\":\"https://github.com/openshift/release\",\"base_ref\":\"master\",\"base_sha\":\"b0250e342116e4944554d2a6be2514b7dcbe86ee\",\"base_link\":\"https://github.com/openshift/release/commit/b0250e342116e4944554d2a6be2514b7dcbe86ee\",\"pulls\":[{\"number\":16552,\"author\":\"jupierce\",\"sha\":\"471002836e36e5b284af4ae85733128f8aa32806\",\"link\":\"https://github.com/openshift/release/pull/16552\",\"commit_link\":\"https://github.com/openshift/release/pull/16552/commits/471002836e36e5b284af4ae85733128f8aa32806\",\"author_link\":\"https://github.com/jupierce\"}]},\"extra_refs\":[{\"org\":\"openshift\",\"repo\":\"dptp-workflow-test\",\"base_ref\":\"master\",\"workdir\":true}],\"decoration_config\":{\"timeout\":\"4h0m0s\",\"grace_period\":\"30m0s\",\"utility_images\":{\"clonerefs\":\"gcr.io/k8s-prow/clonerefs:v20210309-6b78fed7a6\",\"initupload\":\"gcr.io/k8s-prow/initupload:v20210309-6b78fed7a6\",\"entrypoint\":\"gcr.io/k8s-prow/entrypoint:v20210309-6b78fed7a6\",\"sidecar\":\"gcr.io/k8s-prow/sidecar:v20210309-6b78fed7a6\"},\"resources\":{\"clonerefs\":{\"limits\":{\"memory\":\"3Gi\"},\"requests\":{\"cpu\":\"100m\",\"memory\":\"500Mi\"}},\"initupload\":{\"limits\":{\"memory\":\"200Mi\"},\"requests\":{\"cpu\":\"100m\",\"memory\":\"50Mi\"}},\"place_entrypoint\":{\"limits\":{\"memory\":\"100Mi\"},\"requests\":{\"cpu\":\"100m\",\"memory\":\"25Mi\"}},\"sidecar\":{\"limits\":{\"memory\":\"2Gi\"},\"requests\":{\"cpu\":\"100m\",\"memory\":\"250Mi\"}}},\"gcs_configuration\":{\"bucket\":\"origin-ci-test\",\"path_strategy\":\"single\",\"default_org\":\"openshift\",\"default_repo\":\"origin\",\"mediaTypes\":{\"log\":\"text/plain\"}},\"gcs_credentials_secret\":\"gce-sa-credentials-gcs-publisher\",\"skip_cloning\":true}}",
      "k8s.v1.cni.cncf.io/network-status": "[{\n    \"name\": \"\",\n    \"interface\": \"eth0\",\n    \"ips\": [\n        \"10.130.13.25\"\n    ],\n    \"default\": true,\n    \"dns\": {}\n}]",
      "k8s.v1.cni.cncf.io/networks-status": "[{\n    \"name\": \"\",\n    \"interface\": \"eth0\",\n    \"ips\": [\n        \"10.130.13.25\"\n    ],\n    \"default\": true,\n    \"dns\": {}\n}]",
      "openshift.io/scc": "restricted"
    },
    "creationTimestamp": "2021-03-10T17:47:12Z",
    "labels": {
      "OPENSHIFT_CI": "true",
      "build-id": "1369706037107494912",
      "ci.openshift.io/multi-stage-test": "launch",
      "ci.openshift.io/refs.branch": "master",
      "ci.openshift.io/refs.org": "openshift",
      "ci.openshift.io/refs.repo": "release",
      "created-by-ci": "true",
      "job": "rehearse-16552-pull-ci-openshift-dptp-workflow-test-master-lXXX"
    },
    "name": "launch-osd-create-create",
    "namespace": "ci-op-nf6n8qpj",
    "ownerReferences": [
      {
        "apiVersion": "image.openshift.io/v1",
        "kind": "ImageStream",
        "name": "pipeline",
        "uid": "62510eda-4603-4d86-b2b5-604083551161"
      }
    ],
    "resourceVersion": "328089807",
    "selfLink": "/api/v1/namespaces/ci-op-nf6n8qpj/pods/launch-osd-create-create",
    "uid": "dc834071-5ebb-4cbb-8c6e-f7a30a895117"
  },
  "spec": {
    "containers": [
      {
        "args": [
          "/tools/entrypoint"
        ],
        "command": [
          "/tmp/entrypoint-wrapper/entrypoint-wrapper"
        ],
        "env": [
          {
            "name": "BUILD_ID",
            "value": "1369706037107494912"
          },
          {
            "name": "CI",
            "value": "true"
          },
          {
            "name": "JOB_NAME",
            "value": "rehearse-16552-pull-ci-openshift-dptp-workflow-test-master-launch"
          },
          {
            "name": "JOB_SPEC",
            "value": "{\"type\":\"presubmit\",\"job\":\"rehearse-16552-pull-ci-openshift-dptp-workflow-test-master-launch\",\"buildid\":\"1369706037107494912\",\"prowjobid\":\"6ed60414-81c8-11eb-9b46-0a580a832364\",\"refs\":{\"org\":\"openshift\",\"repo\":\"release\",\"repo_link\":\"https://github.com/openshift/release\",\"base_ref\":\"master\",\"base_sha\":\"b0250e342116e4944554d2a6be2514b7dcbe86ee\",\"base_link\":\"https://github.com/openshift/release/commit/b0250e342116e4944554d2a6be2514b7dcbe86ee\",\"pulls\":[{\"number\":16552,\"author\":\"jupierce\",\"sha\":\"471002836e36e5b284af4ae85733128f8aa32806\",\"link\":\"https://github.com/openshift/release/pull/16552\",\"commit_link\":\"https://github.com/openshift/release/pull/16552/commits/471002836e36e5b284af4ae85733128f8aa32806\",\"author_link\":\"https://github.com/jupierce\"}]},\"extra_refs\":[{\"org\":\"openshift\",\"repo\":\"dptp-workflow-test\",\"base_ref\":\"master\",\"workdir\":true}],\"decoration_config\":{\"timeout\":\"2h0m0s\",\"grace_period\":\"2m0s\",\"utility_images\":{\"clonerefs\":\"gcr.io/k8s-prow/clonerefs:v20210309-6b78fed7a6\",\"initupload\":\"gcr.io/k8s-prow/initupload:v20210309-6b78fed7a6\",\"entrypoint\":\"gcr.io/k8s-prow/entrypoint:v20210309-6b78fed7a6\",\"sidecar\":\"gcr.io/k8s-prow/sidecar:v20210309-6b78fed7a6\"},\"resources\":{\"clonerefs\":{\"limits\":{\"memory\":\"3Gi\"},\"requests\":{\"cpu\":\"100m\",\"memory\":\"500Mi\"}},\"initupload\":{\"limits\":{\"memory\":\"200Mi\"},\"requests\":{\"cpu\":\"100m\",\"memory\":\"50Mi\"}},\"place_entrypoint\":{\"limits\":{\"memory\":\"100Mi\"},\"requests\":{\"cpu\":\"100m\",\"memory\":\"25Mi\"}},\"sidecar\":{\"limits\":{\"memory\":\"2Gi\"},\"requests\":{\"cpu\":\"100m\",\"memory\":\"250Mi\"}}},\"gcs_configuration\":{\"bucket\":\"origin-ci-test\",\"path_strategy\":\"single\",\"default_org\":\"openshift\",\"default_repo\":\"origin\",\"mediaTypes\":{\"log\":\"text/plain\"}},\"gcs_credentials_secret\":\"gce-sa-credentials-gcs-publisher\",\"skip_cloning\":true}}"
          },
          {
            "name": "JOB_TYPE",
            "value": "presubmit"
          },
          {
            "name": "OPENSHIFT_CI",
            "value": "true"
          },
          {
            "name": "PROW_JOB_ID",
            "value": "6ed60414-81c8-11eb-9b46-0a580a832364"
          },
          {
            "name": "PULL_BASE_REF",
            "value": "master"
          },
          {
            "name": "PULL_BASE_SHA",
            "value": "b0250e342116e4944554d2a6be2514b7dcbe86ee"
          },
          {
            "name": "PULL_NUMBER",
            "value": "16552"
          },
          {
            "name": "PULL_PULL_SHA",
            "value": "471002836e36e5b284af4ae85733128f8aa32806"
          },
          {
            "name": "PULL_REFS",
            "value": "master:b0250e342116e4944554d2a6be2514b7dcbe86ee,16552:471002836e36e5b284af4ae85733128f8aa32806"
          },
          {
            "name": "REPO_NAME",
            "value": "release"
          },
          {
            "name": "REPO_OWNER",
            "value": "openshift"
          },
          {
            "name": "ENTRYPOINT_OPTIONS",
            "value": "{\"timeout\":7200000000000,\"grace_period\":120000000000,\"artifact_dir\":\"/logs/artifacts\",\"args\":[\"/bin/bash\",\"-c\",\"#!/bin/bash\\nset -eu\\n#!/bin/bash\\n\\nset -o nounset\\nset -o errexit\\nset -o pipefail\\n\\ntrap 'CHILDREN=$(jobs -p); if test -n \\\"${CHILDREN}\\\"; then kill ${CHILDREN} \\u0026\\u0026 wait; fi' TERM\\n\\nset -x\\n\\necho 'Allo!'\\nexport HOME=${SHARED_DIR}\\n\\necho 'Something' \\u003e ${HOME}/x\\necho 'Something else' \\u003e ${SHARED_DIR}/y\\n\\n\\necho \\\"Please job delete the pod associated with this prowjob on the build farm in the ci namespace. I'll wait here.\\\"\\nsleep 3000\\necho \\\"Ooops.. you didn't interrupt me. This will not reproduce the problem.\\\"\"],\"container_name\":\"test\",\"process_log\":\"/logs/process-log.txt\",\"marker_file\":\"/logs/marker-file.txt\",\"metadata_file\":\"/logs/artifacts/metadata.json\"}"
          },
          {
            "name": "ARTIFACT_DIR",
            "value": "/logs/artifacts"
          },
          {
            "name": "NAMESPACE",
            "value": "ci-op-nf6n8qpj"
          },
          {
            "name": "JOB_NAME_SAFE",
            "value": "launch"
          },
          {
            "name": "JOB_NAME_HASH",
            "value": "99c6a"
          },
          {
            "name": "LEASED_RESOURCE",
            "value": "7968f504-820d-4572-8891-df27d0912d06"
          },
          {
            "name": "RELEASE_IMAGE_LATEST",
            "value": "registry.build02.ci.openshift.org/ci-op-nf6n8qpj/release@sha256:8cb47ebccfbc7fa00588de784543950a5b3a8d15dfcf759420dc0f249ecf24c3"
          },
          {
            "name": "IMAGE_FORMAT",
            "value": "registry.build02.ci.openshift.org/ci-op-nf6n8qpj/stable:${component}"
          },
          {
            "name": "CLUSTER_VERSION",
            "value": "4.6.12"
          },
          {
            "name": "CLUSTER_NAME"
          },
          {
            "name": "COMPUTE_MACHINE_TYPE"
          },
          {
            "name": "CLUSTER_DURATION",
            "value": "300"
          },
          {
            "name": "COMPUTE_NODES",
            "value": "2"
          },
          {
            "name": "OCM_LOGIN_URL",
            "value": "staging"
          },
          {
            "name": "CLOUD_PROVIDER_REGION"
          },
          {
            "name": "CLUSTER_TYPE",
            "value": "osd-ephemeral"
          },
          {
            "name": "CLUSTER_PROFILE_DIR",
            "value": "/var/run/secrets/ci.openshift.io/cluster-profile"
          },
          {
            "name": "KUBECONFIG",
            "value": "/var/run/secrets/ci.openshift.io/multi-stage/kubeconfig"
          },
          {
            "name": "KUBEADMIN_PASSWORD_FILE",
            "value": "/var/run/secrets/ci.openshift.io/multi-stage/kubeadmin-password"
          },
          {
            "name": "SHARED_DIR",
            "value": "/var/run/secrets/ci.openshift.io/multi-stage"
          }
        ],
        "image": "image-registry.openshift-image-registry.svc:5000/ci-op-nf6n8qpj/pipeline@sha256:b1e014ba36672bf381d77de55af660b58e81fc6662918cc784390b2cbe94298f",
        "imagePullPolicy": "IfNotPresent",
        "name": "test",
        "resources": {
          "requests": {
            "cpu": "100m",
            "memory": "300Mi"
          }
        },
        "securityContext": {
          "capabilities": {
            "drop": [
              "KILL",
              "MKNOD",
              "SETGID",
              "SETUID"
            ]
          },
          "runAsUser": 1015360000
        },
        "terminationMessagePath": "/dev/termination-log",
        "terminationMessagePolicy": "FallbackToLogsOnError",
        "volumeMounts": [
          {
            "mountPath": "/logs",
            "name": "logs"
          },
          {
            "mountPath": "/tools",
            "name": "tools"
          },
          {
            "mountPath": "/alabama",
            "name": "home"
          },
          {
            "mountPath": "/tmp/entrypoint-wrapper",
            "name": "entrypoint-wrapper"
          },
          {
            "mountPath": "/var/run/secrets/ci.openshift.io/cluster-profile",
            "name": "cluster-profile"
          },
          {
            "mountPath": "/var/run/secrets/ci.openshift.io/multi-stage",
            "name": "launch"
          },
          {
            "mountPath": "/var/run/secrets/kubernetes.io/serviceaccount",
            "name": "launch-token-t5qhk",
            "readOnly": true
          }
        ]
      },
      {
        "command": [
          "/sidecar"
        ],
        "env": [
          {
            "name": "JOB_SPEC",
            "value": "{\"type\":\"presubmit\",\"job\":\"rehearse-16552-pull-ci-openshift-dptp-workflow-test-master-launch\",\"buildid\":\"1369706037107494912\",\"prowjobid\":\"6ed60414-81c8-11eb-9b46-0a580a832364\",\"refs\":{\"org\":\"openshift\",\"repo\":\"release\",\"repo_link\":\"https://github.com/openshift/release\",\"base_ref\":\"master\",\"base_sha\":\"b0250e342116e4944554d2a6be2514b7dcbe86ee\",\"base_link\":\"https://github.com/openshift/release/commit/b0250e342116e4944554d2a6be2514b7dcbe86ee\",\"pulls\":[{\"number\":16552,\"author\":\"jupierce\",\"sha\":\"471002836e36e5b284af4ae85733128f8aa32806\",\"link\":\"https://github.com/openshift/release/pull/16552\",\"commit_link\":\"https://github.com/openshift/release/pull/16552/commits/471002836e36e5b284af4ae85733128f8aa32806\",\"author_link\":\"https://github.com/jupierce\"}]},\"extra_refs\":[{\"org\":\"openshift\",\"repo\":\"dptp-workflow-test\",\"base_ref\":\"master\",\"workdir\":true}],\"decoration_config\":{\"timeout\":\"4h0m0s\",\"grace_period\":\"30m0s\",\"utility_images\":{\"clonerefs\":\"gcr.io/k8s-prow/clonerefs:v20210309-6b78fed7a6\",\"initupload\":\"gcr.io/k8s-prow/initupload:v20210309-6b78fed7a6\",\"entrypoint\":\"gcr.io/k8s-prow/entrypoint:v20210309-6b78fed7a6\",\"sidecar\":\"gcr.io/k8s-prow/sidecar:v20210309-6b78fed7a6\"},\"resources\":{\"clonerefs\":{\"limits\":{\"memory\":\"3Gi\"},\"requests\":{\"cpu\":\"100m\",\"memory\":\"500Mi\"}},\"initupload\":{\"limits\":{\"memory\":\"200Mi\"},\"requests\":{\"cpu\":\"100m\",\"memory\":\"50Mi\"}},\"place_entrypoint\":{\"limits\":{\"memory\":\"100Mi\"},\"requests\":{\"cpu\":\"100m\",\"memory\":\"25Mi\"}},\"sidecar\":{\"limits\":{\"memory\":\"2Gi\"},\"requests\":{\"cpu\":\"100m\",\"memory\":\"250Mi\"}}},\"gcs_configuration\":{\"bucket\":\"origin-ci-test\",\"path_strategy\":\"single\",\"default_org\":\"openshift\",\"default_repo\":\"origin\",\"mediaTypes\":{\"log\":\"text/plain\"}},\"gcs_credentials_secret\":\"gce-sa-credentials-gcs-publisher\",\"skip_cloning\":true}}"
          },
          {
            "name": "SIDECAR_OPTIONS",
            "value": "{\"gcs_options\":{\"items\":[\"/logs/artifacts\"],\"sub_dir\":\"artifacts/launch/osd-create-create\",\"bucket\":\"origin-ci-test\",\"path_strategy\":\"single\",\"default_org\":\"openshift\",\"default_repo\":\"origin\",\"mediaTypes\":{\"log\":\"text/plain\"},\"gcs_credentials_file\":\"/secrets/gcs/service-account.json\",\"dry_run\":false},\"entries\":[{\"args\":[\"/bin/bash\",\"-c\",\"#!/bin/bash\\nset -eu\\n#!/bin/bash\\n\\nset -o nounset\\nset -o errexit\\nset -o pipefail\\n\\ntrap 'CHILDREN=$(jobs -p); if test -n \\\"${CHILDREN}\\\"; then kill ${CHILDREN} \\u0026\\u0026 wait; fi' TERM\\n\\nset -x\\n\\necho 'Allo!'\\nexport HOME=${SHARED_DIR}\\n\\necho 'Something' \\u003e ${HOME}/x\\necho 'Something else' \\u003e ${SHARED_DIR}/y\\n\\n\\necho \\\"Please job delete the pod associated with this prowjob on the build farm in the ci namespace. I'll wait here.\\\"\\nsleep 3000\\necho \\\"Ooops.. you didn't interrupt me. This will not reproduce the problem.\\\"\"],\"container_name\":\"test\",\"process_log\":\"/logs/process-log.txt\",\"marker_file\":\"/logs/marker-file.txt\",\"metadata_file\":\"/logs/artifacts/metadata.json\"}],\"ignore_interrupts\":true}"
          }
        ],
        "image": "gcr.io/k8s-prow/sidecar:v20210309-6b78fed7a6",
        "imagePullPolicy": "IfNotPresent",
        "name": "sidecar",
        "resources": {
          "limits": {
            "memory": "2Gi"
          },
          "requests": {
            "cpu": "100m",
            "memory": "250Mi"
          }
        },
        "securityContext": {
          "capabilities": {
            "drop": [
              "KILL",
              "MKNOD",
              "SETGID",
              "SETUID"
            ]
          },
          "runAsUser": 1015360000
        },
        "terminationMessagePath": "/dev/termination-log",
        "terminationMessagePolicy": "File",
        "volumeMounts": [
          {
            "mountPath": "/logs",
            "name": "logs"
          },
          {
            "mountPath": "/secrets/gcs",
            "name": "gcs-credentials"
          },
          {
            "mountPath": "/var/run/secrets/kubernetes.io/serviceaccount",
            "name": "launch-token-t5qhk",
            "readOnly": true
          }
        ]
      }
    ],
    "dnsPolicy": "ClusterFirst",
    "enableServiceLinks": true,
    "imagePullSecrets": [
      {
        "name": "launch-dockercfg-wb2kr"
      }
    ],
    "initContainers": [
      {
        "args": [
          "/entrypoint",
          "/tools/entrypoint"
        ],
        "command": [
          "/bin/cp"
        ],
        "image": "gcr.io/k8s-prow/entrypoint:v20210309-6b78fed7a6",
        "imagePullPolicy": "IfNotPresent",
        "name": "place-entrypoint",
        "resources": {
          "limits": {
            "memory": "100Mi"
          },
          "requests": {
            "cpu": "100m",
            "memory": "25Mi"
          }
        },
        "securityContext": {
          "capabilities": {
            "drop": [
              "KILL",
              "MKNOD",
              "SETGID",
              "SETUID"
            ]
          },
          "runAsUser": 1015360000
        },
        "terminationMessagePath": "/dev/termination-log",
        "terminationMessagePolicy": "File",
        "volumeMounts": [
          {
            "mountPath": "/tools",
            "name": "tools"
          },
          {
            "mountPath": "/var/run/secrets/kubernetes.io/serviceaccount",
            "name": "launch-token-t5qhk",
            "readOnly": true
          }
        ]
      },
      {
        "args": [
          "/bin/entrypoint-wrapper",
          "/tmp/entrypoint-wrapper/entrypoint-wrapper"
        ],
        "command": [
          "cp"
        ],
        "image": "registry.ci.openshift.org/ci/entrypoint-wrapper:latest",
        "imagePullPolicy": "Always",
        "name": "cp-entrypoint-wrapper",
        "resources": {},
        "securityContext": {
          "capabilities": {
            "drop": [
              "KILL",
              "MKNOD",
              "SETGID",
              "SETUID"
            ]
          },
          "runAsUser": 1015360000
        },
        "terminationMessagePath": "/dev/termination-log",
        "terminationMessagePolicy": "FallbackToLogsOnError",
        "volumeMounts": [
          {
            "mountPath": "/tmp/entrypoint-wrapper",
            "name": "entrypoint-wrapper"
          },
          {
            "mountPath": "/var/run/secrets/kubernetes.io/serviceaccount",
            "name": "launch-token-t5qhk",
            "readOnly": true
          }
        ]
      }
    ],
    "nodeName": "build0-gstfj-w-b-5b6cl.c.openshift-ci-build-farm.internal",
    "preemptionPolicy": "PreemptLowerPriority",
    "priority": 0,
    "restartPolicy": "Never",
    "schedulerName": "default-scheduler",
    "securityContext": {
      "fsGroup": 1015360000,
      "seLinuxOptions": {
        "level": "s0:c124,c54"
      }
    },
    "serviceAccount": "launch",
    "serviceAccountName": "launch",
    "terminationGracePeriodSeconds": 150,
    "tolerations": [
      {
        "effect": "NoExecute",
        "key": "node.kubernetes.io/not-ready",
        "operator": "Exists",
        "tolerationSeconds": 300
      },
      {
        "effect": "NoExecute",
        "key": "node.kubernetes.io/unreachable",
        "operator": "Exists",
        "tolerationSeconds": 300
      },
      {
        "effect": "NoSchedule",
        "key": "node.kubernetes.io/memory-pressure",
        "operator": "Exists"
      }
    ],
    "volumes": [
      {
        "emptyDir": {},
        "name": "logs"
      },
      {
        "emptyDir": {},
        "name": "tools"
      },
      {
        "name": "gcs-credentials",
        "secret": {
          "defaultMode": 420,
          "secretName": "gce-sa-credentials-gcs-publisher"
        }
      },
      {
        "emptyDir": {},
        "name": "home"
      },
      {
        "emptyDir": {},
        "name": "entrypoint-wrapper"
      },
      {
        "name": "cluster-profile",
        "secret": {
          "defaultMode": 420,
          "secretName": "launch-cluster-profile"
        }
      },
      {
        "name": "launch",
        "secret": {
          "defaultMode": 420,
          "secretName": "launch"
        }
      },
      {
        "name": "launch-token-t5qhk",
        "secret": {
          "defaultMode": 420,
          "secretName": "launch-token-t5qhk"
        }
      }
    ]
  },
  "status": {
    "conditions": [
      {
        "lastProbeTime": null,
        "lastTransitionTime": "2021-03-10T17:47:17Z",
        "status": "True",
        "type": "Initialized"
      },
      {
        "lastProbeTime": null,
        "lastTransitionTime": "2021-03-10T17:47:26Z",
        "status": "True",
        "type": "Ready"
      },
      {
        "lastProbeTime": null,
        "lastTransitionTime": "2021-03-10T17:47:26Z",
        "status": "True",
        "type": "ContainersReady"
      },
      {
        "lastProbeTime": null,
        "lastTransitionTime": "2021-03-10T17:47:12Z",
        "status": "True",
        "type": "PodScheduled"
      }
    ],
    "containerStatuses": [
      {
        "containerID": "cri-o://e19f1dfcf3017eff90b12c8a89348c5cf1fd79d0207064d99b17d07784c13ad8",
        "image": "gcr.io/k8s-prow/sidecar:v20210309-6b78fed7a6",
        "imageID": "gcr.io/k8s-prow/sidecar@sha256:8615ade39ab086dfbcdeb982a1bcb506aa38a85edb752bc002d1ee1cd6f2a8bd",
        "lastState": {},
        "name": "sidecar",
        "ready": true,
        "restartCount": 0,
        "started": true,
        "state": {
          "running": {
            "startedAt": "2021-03-10T17:47:25Z"
          }
        }
      },
      {
        "containerID": "cri-o://ed42d4c0458078c1b0b6281f24d2fb3b464e4e333a07a35527fd1235aaf890fb",
        "image": "image-registry.openshift-image-registry.svc:5000/ci-op-nf6n8qpj/pipeline@sha256:b1e014ba36672bf381d77de55af660b58e81fc6662918cc784390b2cbe94298f",
        "imageID": "image-registry.openshift-image-registry.svc:5000/ci-op-nf6n8qpj/pipeline@sha256:b1e014ba36672bf381d77de55af660b58e81fc6662918cc784390b2cbe94298f",
        "lastState": {},
        "name": "test",
        "ready": true,
        "restartCount": 0,
        "started": true,
        "state": {
          "running": {
            "startedAt": "2021-03-10T17:47:24Z"
          }
        }
      }
    ],
    "hostIP": "10.0.32.95",
    "initContainerStatuses": [
      {
        "containerID": "cri-o://66316f587168a41962dad1ffe8b509d11dccbf2824078fc375bfcca462ce7785",
        "image": "gcr.io/k8s-prow/entrypoint:v20210309-6b78fed7a6",
        "imageID": "gcr.io/k8s-prow/entrypoint@sha256:5627d14f8c777b114fc188ee6059ae19d51530a6fc554eb63b383d0540cb9633",
        "lastState": {},
        "name": "place-entrypoint",
        "ready": true,
        "restartCount": 0,
        "state": {
          "terminated": {
            "containerID": "cri-o://66316f587168a41962dad1ffe8b509d11dccbf2824078fc375bfcca462ce7785",
            "exitCode": 0,
            "finishedAt": "2021-03-10T17:47:16Z",
            "reason": "Completed",
            "startedAt": "2021-03-10T17:47:16Z"
          }
        }
      },
      {
        "containerID": "cri-o://1081d0e39b871149be4ff5aadd59bab65b1cd073e82f5f32baaf5b884320d26b",
        "image": "registry.ci.openshift.org/ci/entrypoint-wrapper:latest",
        "imageID": "registry.ci.openshift.org/ci/entrypoint-wrapper@sha256:3a07adea00a77550952d8957b8858a2170a30498f6a40a92f142b67e72a4c00b",
        "lastState": {},
        "name": "cp-entrypoint-wrapper",
        "ready": true,
        "restartCount": 0,
        "state": {
          "terminated": {
            "containerID": "cri-o://1081d0e39b871149be4ff5aadd59bab65b1cd073e82f5f32baaf5b884320d26b",
            "exitCode": 0,
            "finishedAt": "2021-03-10T17:47:17Z",
            "reason": "Completed",
            "startedAt": "2021-03-10T17:47:17Z"
          }
        }
      }
    ],
    "phase": "Running",
    "podIP": "10.130.13.25",
    "podIPs": [
      {
        "ip": "10.130.13.25"
      }
    ],
    "qosClass": "Burstable",
    "startTime": "2021-03-10T17:47:12Z"
  }
}`

var deleteJSON = `{"kind":"DeleteOptions","apiVersion":"v1"}`

func TestGraceful(t *testing.T) {
	v1Pod := &corev1.Pod{}
	err := json.Unmarshal([]byte(podJSON), v1Pod)
	if err != nil {
		t.Fatal(err)
	}

	deleteOptions := &metav1.DeleteOptions{}
	err = json.Unmarshal([]byte(deleteJSON), deleteOptions)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%v", deleteOptions.GracePeriodSeconds)

	apiPod := &api.Pod{}
	err = v1.Convert_v1_Pod_To_core_Pod(v1Pod, apiPod, nil)
	if err != nil {
		t.Fatal(err)
	}

	strategy := podStrategy{}
	strategy.CheckGracefulDelete(context.TODO(), apiPod, deleteOptions)

	t.Logf("%v", *apiPod.Spec.TerminationGracePeriodSeconds)
	t.Fatalf("%v", apiPod.DeletionGracePeriodSeconds)

}
