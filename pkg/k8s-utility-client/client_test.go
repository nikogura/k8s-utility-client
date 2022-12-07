/*
Copyright <2022> Nik Ogura <nik.ogura@gmail.com>

Permission is hereby granted, free of charge, to any person obtaining a copy of this software and associated documentation files (the "Software"), to deal in the Software without restriction, including without limitation the rights to use, copy, modify, merge, publish, distribute, sublicense, and/or sell copies of the Software, and to permit persons to whom the Software is furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
*/
package k8s_utility_client

import (
	"context"
	"fmt"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"log"
	"os"
	"os/exec"
	"testing"
	"time"
)

var tmpDir string

// TestMain runs before all tests.  Ensures that setUp() runs before all tests, and tearDown() runs after them.
func TestMain(m *testing.M) {
	setUp()
	code := m.Run()

	tearDown()

	os.Exit(code)

}

// Runs before all tests
func setUp() {
	t, err := os.MkdirTemp("", "sqwatchling")
	if err != nil {
		log.Fatalf("failed creating tempdir: %s", err)
	}

	tmpDir = t
	// NB: Tests will not run unless a k8s cluster is running and available, and ~/.kube/config is properly configured to talk to it.

	kubectl, err := exec.LookPath("kubectl")
	if err != nil {
		log.Fatal("kubectl command not found in path")
	}

	cmd := exec.Command(kubectl, "get", "node")

	err = cmd.Run()
	if err != nil {
		log.Fatalf("command `kubectl get node` failed with error: %q.  Usually this means there is no Kubernetes cluster available.  The tests require a working k8s cluster, and a properly set up config file in order to function.", err)
	}

}

func tearDown() {
	// Clean up the temp dir
	if _, err := os.Stat(tmpDir); !os.IsNotExist(err) {
		_ = os.RemoveAll(tmpDir)
	}
}

func TestResourceLoading(t *testing.T) {
	testCases := []struct {
		name     string
		fileName string
	}{
		{
			"basic resource",
			"test_fixtures/resources.yaml",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			client, err := NewK8sClients()
			if err != nil {
				t.Errorf("failed creating client: %s", err)
			}

			_, objects, err := client.ResourcesAndObjectsFromFile(tc.fileName)
			if err != nil {
				t.Errorf("failed to load yaml file %s: %s", tc.fileName, err)
			}

			// NB: Need a var for every type of resource we're going to try to load
			var deployment v1.Deployment

			var service corev1.Service

			for _, o := range objects {
				switch o.GetKind() {
				// NB: Need a case for every type of resource we're going to try to load
				case "Service":
					err = runtime.DefaultUnstructuredConverter.FromUnstructured(o.Object, &service)
					if err != nil {
						t.Errorf("failed converting unstructured resource to Service: %s", err)
					}
				case "Deployment":
					err = runtime.DefaultUnstructuredConverter.FromUnstructured(o.Object, &deployment)
					if err != nil {
						t.Errorf("failed converting unstructured resource to Deployment: %s", err)
					}
				}
			}
		})
	}
}

func TestApplyResources(t *testing.T) {
	testCases := []struct {
		name     string
		fileName string
	}{
		{
			"basic resource",
			"test_fixtures/resources.yaml",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			client, err := NewK8sClients()
			if err != nil {
				t.Errorf("failed creating client: %s", err)
			}

			interfaces, objects, err := client.ResourcesAndObjectsFromFile(tc.fileName)
			if err != nil {
				t.Errorf("failed to load yaml file %s: %s", tc.fileName, err)
			}

			ctx := context.TODO()

			fmt.Printf("Creating resources in k8s.\n")
			err = client.ApplyResources(ctx, interfaces, objects)
			if err != nil {
				t.Errorf("failed to apply resources: %s", err)
			}

			sleeptime := 10
			fmt.Printf("Sleeping %d seconds before attempting verification.\n", sleeptime)
			time.Sleep(time.Duration(sleeptime) * time.Second)

			fmt.Printf("Verifying resources in k8s.\n")
			for i, obj := range objects {
				ri := interfaces[i]

				o, err := ri.Get(ctx, obj.GetName(), metav1.GetOptions{})
				if err != nil {
					t.Errorf("failed getting resource %s kind %s", obj.GetName(), obj.GetKind())
				}

				assert.Equal(t, obj.GetName(), o.GetName(), "Created Resource name doesn't match expectation.")
				assert.Equal(t, obj.GetKind(), o.GetKind(), "Created Resource Kind does not match expectations.")
				assert.Equal(t, obj.GetNamespace(), o.GetNamespace(), "Created Resource Namespace does not match expectations.")

			}

			fmt.Printf("Cleaning up resources in k8s.\n")
			err = client.DeleteResources(ctx, interfaces, objects)
			if err != nil {
				t.Errorf("failed deleting resources: %s", err)
			}

			fmt.Printf("Sleeping %d seconds before attempting verification of clean up.\n", sleeptime)
			time.Sleep(time.Duration(sleeptime) * time.Second)

			for i, obj := range objects {
				ri := interfaces[i]

				_, err := ri.Get(ctx, obj.GetName(), metav1.GetOptions{})
				if err == nil {
					t.Errorf("Resource %s kind %s failed to clean up.", obj.GetName(), obj.GetKind())
				}
			}
		})
	}
}
