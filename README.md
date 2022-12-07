# K8s Utility Client

A library creating a set of Kubernetes clients - both standard and dynamic that can be used within a K8s cluster, or without.

The [Operator Framework](https://sdk.operatorframework.io/) makes a k8s client that is insufficient to generating a Dynamic client, so I put this together to make all the clients I need.

It works the same whether it's running inside a cluster or without.  I don't have to care.

Sometimes you just want things to _just work_.

[![Current Release](https://img.shields.io/github/release/nikogura/k8s-utility-client.svg)](https://img.shields.io/github/release/nikogura/k8s-utility-client.svg)

[![Go Report Card](https://goreportcard.com/badge/github.com/nikogura/k8s-utility-client)](https://goreportcard.com/report/github.com/nikogura/k8s-utility-client)

[![Go Doc](https://img.shields.io/badge/godoc-reference-blue.svg?style=flat-square)](http://godoc.org/github.com/nikogura/k8s-utility-client/pkg/k8s-utility-client)

# Usage

## Creating a Client

Creating a clientis easy.  Just use:

        client, err := NewK8sClients()
        if err != nil {
            log.Fatalf("failed creating client: %s", err)
        }

It will look for your config file in ~/.kube/config.  The dynamic client must be able to reach a k8s cluster in order to do it's thing.


## Loading Resource Files

To load the files, run

        interfaces, objects, err := client.ResourcesAndObjectsFromFile(fileName)
        if err != nil {
            log.Fatalf("failed to load yaml file %s: %s", fileName, err)
        }

## Applying Resource Fields

        ctx := context.TODO()

        fmt.Printf("Creating resources in k8s.\n")
        err = client.ApplyResources(ctx, interfaces, objects)
        if err != nil {
            t.Errorf("failed to apply resources: %s", err)
        }
