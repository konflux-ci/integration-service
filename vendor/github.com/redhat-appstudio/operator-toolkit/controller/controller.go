/*
Copyright 2023.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"github.com/go-logr/logr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cluster"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// CacheInitializer is an interface that should be implemented by operator controllers requiring to index fields.
type CacheInitializer interface {
	SetupCache(mgr ctrl.Manager) error
}

// Controller is an interface that should be implemented by operator controllers and that allows to have a cohesive way
// of defining them and register them.
type Controller interface {
	reconcile.Reconciler

	Register(mgr ctrl.Manager, log *logr.Logger, cluster cluster.Cluster) error
}

// SetupControllers invoke the Register function of every controller passed as an argument to this function. If a given
// Controller implements CacheInitializer, the cache will be initialized before registering the controller.
func SetupControllers(mgr manager.Manager, cluster cluster.Cluster, controllers ...Controller) error {
	log := ctrl.Log.WithName("controllers")

	for _, controller := range controllers {
		if cacheInitializer, ok := controller.(CacheInitializer); ok {
			err := cacheInitializer.SetupCache(mgr)
			if err != nil {
				return err
			}
		}

		err := controller.Register(mgr, &log, cluster)
		if err != nil {
			return err
		}
	}

	return nil
}
