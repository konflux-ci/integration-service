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

package helpers

import (
	"github.com/go-logr/logr"
	applicationapiv1alpha1 "github.com/redhat-appstudio/application-api/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

type LogAction int

//go:generate stringer -type=LogAction -linecomment
const (
	LogActionView   LogAction = iota + 1 // VIEW
	LogActionAdd                         // ADD
	LogActionUpdate                      // UPDATE
	LogActionDelete                      // DELETE
)

type IntegrationLogger struct {
	logr.Logger
}

// loggerWithObjectMeta returns logger with object metadata key-value pairs
func loggerWithObjectMeta(obj runtime.Object, log logr.Logger) logr.Logger {
	name := ""
	namespace := ""
	kind := ""

	if obj != nil {
		metaObj, ok := obj.(metav1.ObjectMetaAccessor)
		if ok {
			name = metaObj.GetObjectMeta().GetName()
			namespace = metaObj.GetObjectMeta().GetNamespace()
		}
		kind = obj.GetObjectKind().GroupVersionKind().Kind
	}
	return log.WithValues("namespace", namespace, "name", name, "controllerKind", kind)
}

func (il *IntegrationLogger) setLogger(log logr.Logger) {
	il.Logger = log
}

// LogAuditEvent should be used for auditable events to log all required metadata
// msg is a user friendly log message
// obj is k8s runtime object
// audit action type
// and any extra keyAndValues being passed to logger
func (il *IntegrationLogger) LogAuditEvent(msg string, obj runtime.Object, action LogAction, keysAndValues ...interface{}) {
	log := il.WithCallDepth(1) // this is for logging of the real caller value from wrapper
	log = loggerWithObjectMeta(obj, log)
	// audit log must contain "audit": "true"
	log = log.WithValues("audit", "true", "action", action)

	log.Info(msg, keysAndValues...)
}

// WithApp returns a new logger with application.name and application.namespace key-values
func (il IntegrationLogger) WithApp(app applicationapiv1alpha1.Application) IntegrationLogger {
	log := il.Logger.WithValues("application.namespace", app.Namespace, "application.name", app.Name)
	il.setLogger(log)
	return il
}
