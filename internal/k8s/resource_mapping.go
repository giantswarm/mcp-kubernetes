package k8s

import "k8s.io/apimachinery/pkg/runtime/schema"

// initBuiltinResources initializes the builtin resources mapping.
// This is shared between the main client and bearer token clients.
func initBuiltinResources() map[string]schema.GroupVersionResource {
	return map[string]schema.GroupVersionResource{
		// Core/v1 resources
		"pods":                   {Group: "", Version: "v1", Resource: "pods"},
		"pod":                    {Group: "", Version: "v1", Resource: "pods"},
		"services":               {Group: "", Version: "v1", Resource: "services"},
		"service":                {Group: "", Version: "v1", Resource: "services"},
		"svc":                    {Group: "", Version: "v1", Resource: "services"},
		"nodes":                  {Group: "", Version: "v1", Resource: "nodes"},
		"node":                   {Group: "", Version: "v1", Resource: "nodes"},
		"namespaces":             {Group: "", Version: "v1", Resource: "namespaces"},
		"namespace":              {Group: "", Version: "v1", Resource: "namespaces"},
		"ns":                     {Group: "", Version: "v1", Resource: "namespaces"},
		"configmaps":             {Group: "", Version: "v1", Resource: "configmaps"},
		"configmap":              {Group: "", Version: "v1", Resource: "configmaps"},
		"cm":                     {Group: "", Version: "v1", Resource: "configmaps"},
		"secrets":                {Group: "", Version: "v1", Resource: "secrets"},
		"secret":                 {Group: "", Version: "v1", Resource: "secrets"},
		"persistentvolumes":      {Group: "", Version: "v1", Resource: "persistentvolumes"},
		"persistentvolume":       {Group: "", Version: "v1", Resource: "persistentvolumes"},
		"pv":                     {Group: "", Version: "v1", Resource: "persistentvolumes"},
		"persistentvolumeclaims": {Group: "", Version: "v1", Resource: "persistentvolumeclaims"},
		"persistentvolumeclaim":  {Group: "", Version: "v1", Resource: "persistentvolumeclaims"},
		"pvc":                    {Group: "", Version: "v1", Resource: "persistentvolumeclaims"},

		// Apps/v1 resources
		"deployments":  {Group: "apps", Version: "v1", Resource: "deployments"},
		"deployment":   {Group: "apps", Version: "v1", Resource: "deployments"},
		"deploy":       {Group: "apps", Version: "v1", Resource: "deployments"},
		"replicasets":  {Group: "apps", Version: "v1", Resource: "replicasets"},
		"replicaset":   {Group: "apps", Version: "v1", Resource: "replicasets"},
		"rs":           {Group: "apps", Version: "v1", Resource: "replicasets"},
		"daemonsets":   {Group: "apps", Version: "v1", Resource: "daemonsets"},
		"daemonset":    {Group: "apps", Version: "v1", Resource: "daemonsets"},
		"ds":           {Group: "apps", Version: "v1", Resource: "daemonsets"},
		"statefulsets": {Group: "apps", Version: "v1", Resource: "statefulsets"},
		"statefulset":  {Group: "apps", Version: "v1", Resource: "statefulsets"},
		"sts":          {Group: "apps", Version: "v1", Resource: "statefulsets"},

		// Batch resources
		"jobs":     {Group: "batch", Version: "v1", Resource: "jobs"},
		"job":      {Group: "batch", Version: "v1", Resource: "jobs"},
		"cronjobs": {Group: "batch", Version: "v1", Resource: "cronjobs"},
		"cronjob":  {Group: "batch", Version: "v1", Resource: "cronjobs"},
		"cj":       {Group: "batch", Version: "v1", Resource: "cronjobs"},

		// Networking resources
		"ingresses": {Group: "networking.k8s.io", Version: "v1", Resource: "ingresses"},
		"ingress":   {Group: "networking.k8s.io", Version: "v1", Resource: "ingresses"},
		"ing":       {Group: "networking.k8s.io", Version: "v1", Resource: "ingresses"},

		// RBAC resources
		"roles":               {Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "roles"},
		"role":                {Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "roles"},
		"rolebindings":        {Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "rolebindings"},
		"rolebinding":         {Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "rolebindings"},
		"clusterroles":        {Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "clusterroles"},
		"clusterrole":         {Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "clusterroles"},
		"clusterrolebindings": {Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "clusterrolebindings"},
		"clusterrolebinding":  {Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "clusterrolebindings"},
		"serviceaccounts":     {Group: "", Version: "v1", Resource: "serviceaccounts"},
		"serviceaccount":      {Group: "", Version: "v1", Resource: "serviceaccounts"},
		"sa":                  {Group: "", Version: "v1", Resource: "serviceaccounts"},
	}
}
