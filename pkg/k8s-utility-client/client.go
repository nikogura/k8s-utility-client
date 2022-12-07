/*
Copyright <2022> Nik Ogura <nik.ogura@gmail.com>

Permission is hereby granted, free of charge, to any person obtaining a copy of this software and associated documentation files (the "Software"), to deal in the Software without restriction, including without limitation the rights to use, copy, modify, merge, publish, distribute, sublicense, and/or sell copies of the Software, and to permit persons to whom the Software is furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
*/
package k8s_utility_client

import (
	"bytes"
	"context"
	"fmt"
	"github.com/pkg/errors"
	"io"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer/yaml"
	yamlutil "k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
	"log"
	"os"
)

// IN_POD_NAMESPACE_FILE  If this file exists, odds are you're running in a k8s pod.  From here we can determine both that we're in k8s, and what our current namespace is
const IN_POD_NAMESPACE_FILE = "/var/run/secrets/kubernetes.io/serviceaccount/namespace"

type K8sClients struct {
	InCluster     bool
	ClientSet     *kubernetes.Clientset
	DynamicClient dynamic.Interface
	K8SConfig     *rest.Config
	Namespace     string
}

// NewK8sClients  Creates both standard k8s Clientsets and a Dynamic Clientset for Unstructured resources.  Autodetcts whether it's running in a cluster, or outside.  Looks for default config files in the usual places and automagically does the right thing.
func NewK8sClients() (clients *K8sClients, err error) {
	clients = &K8sClients{
		InCluster:     false,
		ClientSet:     nil,
		DynamicClient: nil,
		K8SConfig:     nil,
		Namespace:     "",
	}
	// Initialize K8S Client
	// detect whether we're running in a k8s cluster or not.  If we're in a cluster, IN_POD_NAMESPACE_FILE will exist
	if _, err := os.Stat(IN_POD_NAMESPACE_FILE); !os.IsNotExist(err) {
		clients.InCluster = true

		// read the file.  The contents are our namespace
		nsb, err := os.ReadFile(IN_POD_NAMESPACE_FILE)
		if err != nil {
			log.Fatalf("failed reading in-pod namespace file: %s", IN_POD_NAMESPACE_FILE)
		}

		// set the namespace
		clients.Namespace = string(nsb)

		// create the client config for in-cluster work
		cc, err := rest.InClusterConfig()
		if err != nil {
			err = errors.Wrapf(err, "failed creating in-cluster k8s client config")
			return clients, err
		}

		clients.K8SConfig = cc

	} else { // We're not in a cluster, so look on the filesystem for the default k8s config file
		configFile := fmt.Sprintf("%s/.kube/config", homedir.HomeDir())

		// read the file
		if _, err := os.Stat(configFile); !os.IsNotExist(err) {
			config, err := clientcmd.NewDefaultClientConfigLoadingRules().Load()
			if err != nil {
				log.Fatalf("failed loading kubeconfig file: %s", configFile)
			}

			clients.Namespace = config.Contexts[config.CurrentContext].Namespace

			// create a config from the file
			cc, err := clientcmd.NewDefaultClientConfig(*config, &clientcmd.ConfigOverrides{}).ClientConfig()
			if err != nil {
				err = errors.Wrapf(err, "failed creating default kubernetes client config")
				return clients, err
			}

			clients.K8SConfig = cc
		} else { // error out if the k8s config doesn't exist
			err = errors.New(fmt.Sprintf("k8s config file %s does not exist.  Cannot continue", configFile))
			return clients, err
		}
	}

	// bail if we still don't have a client config
	if clients.K8SConfig == nil {
		err := errors.New("Failed creating k8s client config.  Cannot proceed with tests.")
		log.Fatal(err)
	}

	// create a k8s clientset
	cs, err := kubernetes.NewForConfig(clients.K8SConfig)
	if err != nil {
		log.Fatalf("failed creating k8s clientset: %s", err)
	}

	// set the global var
	clients.ClientSet = cs

	// create a dynamic clientset
	dc, err := dynamic.NewForConfig(clients.K8SConfig)
	if err != nil {
		log.Fatalf("failed k8s dynamic client: %s", err)
	}

	// set the global var
	clients.DynamicClient = dc

	// Bail if we don't have k8s clients
	if clients.ClientSet == nil {
		err := errors.New("Failed creating k8s clientset.  Cannot proceed with tests.")
		return clients, err
	}

	if clients.DynamicClient == nil {
		err := errors.New("Failed creating k8s dynamic client.  Cannot proceed with tests.")
		return clients, err
	}

	return clients, err

}

func (k *K8sClients) ResourcesAndObjectsFromFile(fileName string) (interfaces []dynamic.ResourceInterface, objects []*unstructured.Unstructured, err error) {

	b, err := os.ReadFile(fileName)
	if err != nil {
		err = errors.Wrapf(err, "failed reading file %s", fileName)
		return interfaces, objects, err
	}

	return k.ResourcesAndObjectsFromBytes(b)
}

// ResourcesAnd ObjectsFromYaml Reads k8s yaml files and converts them into Unstructured interfaces that can be applied to the cluster similar to `kubectl apply -f`
func (k *K8sClients) ResourcesAndObjectsFromBytes(yamlBytes []byte) (interfaces []dynamic.ResourceInterface, objects []*unstructured.Unstructured, err error) {
	interfaces = make([]dynamic.ResourceInterface, 0)
	objects = make([]*unstructured.Unstructured, 0)

	decoder := yamlutil.NewYAMLOrJSONDecoder(bytes.NewReader(yamlBytes), 100)
	for {
		var rawObj runtime.RawExtension
		if err = decoder.Decode(&rawObj); err != nil {
			break
		}

		obj, gvk, err := yaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme).Decode(rawObj.Raw, nil, nil)
		if err != nil {
			err = errors.Wrapf(err, "failed decoding resource file")
			return interfaces, objects, err
		}

		unstructuredMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
		if err != nil {
			err = errors.Wrapf(err, "failed converting object unstructured")
			return interfaces, objects, err
		}

		unstructuredObj := &unstructured.Unstructured{Object: unstructuredMap}

		gr, err := restmapper.GetAPIGroupResources(k.ClientSet.Discovery())
		if err != nil {
			err = errors.Wrapf(err, "failed getting api group resources")
			return interfaces, objects, err
		}

		mapper := restmapper.NewDiscoveryRESTMapper(gr)
		mapping, err := mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
		if err != nil {
			err = errors.Wrapf(err, "failed creating rest mapping")
			return interfaces, objects, err
		}

		var dri dynamic.ResourceInterface
		if mapping.Scope.Name() == meta.RESTScopeNameNamespace {
			if unstructuredObj.GetNamespace() == "" {
				unstructuredObj.SetNamespace("default")
			}
			dri = k.DynamicClient.Resource(mapping.Resource).Namespace(unstructuredObj.GetNamespace())
		} else {
			dri = k.DynamicClient.Resource(mapping.Resource)
		}

		if dri != nil && unstructuredObj != nil {
			interfaces = append(interfaces, dri)
			objects = append(objects, unstructuredObj)
		}
	}

	if err != io.EOF {
		err = errors.Wrapf(err, "encountered something weird when parsing yaml")
		return interfaces, objects, err
	}

	// reset err to nil, since io.EOF is actually expected
	err = nil

	return interfaces, objects, err
}

// ApplyResources  Takes a list of Unstructured interfaces and 'objects' and applies them to the cluster.  ApplyResources will try to Get the resources first, and if they already exist, it will Update them.
func (k *K8sClients) ApplyResources(ctx context.Context, interfaces []dynamic.ResourceInterface, objects []*unstructured.Unstructured) (err error) {
	for i, ri := range interfaces {
		obj := objects[i]

		// Try to get the resource from k8s.  If it exists, we'll have to update, and cope with the optimistic lock
		res, getErr := ri.Get(ctx, obj.GetName(), metav1.GetOptions{})
		if getErr == nil {
			rv := res.GetResourceVersion()
			obj.SetResourceVersion(rv)

			_, err := ri.Update(ctx, obj, metav1.UpdateOptions{})
			if err != nil {
				err = errors.Wrapf(err, "failed updating %s kind %s", obj.GetName(), obj.GetKind())
				return err
			}

		} else {
			_, err := ri.Create(ctx, obj, metav1.CreateOptions{})
			if err != nil {
				err = errors.Wrapf(err, "failed creating %s kind %s", obj.GetName(), obj.GetKind())
				return err
			}
		}
	}

	return err
}

// DeleteResources takes a list of Unstructured interfaces and 'objects' and performs a 'Foreground delete' upon them. See https://kubernetes.io/docs/concepts/architecture/garbage-collection/#foreground-deletion for more information about delete types.
func (k *K8sClients) DeleteResources(ctx context.Context, interfaces []dynamic.ResourceInterface, objects []*unstructured.Unstructured) (err error) {
	for i, ri := range interfaces {
		obj := objects[i]
		propagation := metav1.DeletePropagationForeground

		fmt.Printf("Deleting %s %s\n", obj.GetKind(), obj.GetName())
		err = ri.Delete(ctx, obj.GetName(), metav1.DeleteOptions{
			PropagationPolicy: &propagation,
		})
		if err != nil {
			err = errors.Wrapf(err, "failed deleting %s kind %s", obj.GetName(), obj.GetKind())
			return err
		}
	}

	return err
}
