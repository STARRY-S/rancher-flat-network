/*
Copyright 2024 SUSE Rancher

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

// Code generated by main. DO NOT EDIT.

package v1

import (
	"context"
	"time"

	"github.com/rancher/wrangler/pkg/apply"
	"github.com/rancher/wrangler/pkg/condition"
	"github.com/rancher/wrangler/pkg/generic"
	"github.com/rancher/wrangler/pkg/kv"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// ServiceController interface for managing Service resources.
type ServiceController interface {
	generic.ControllerInterface[*v1.Service, *v1.ServiceList]
}

// ServiceClient interface for managing Service resources in Kubernetes.
type ServiceClient interface {
	generic.ClientInterface[*v1.Service, *v1.ServiceList]
}

// ServiceCache interface for retrieving Service resources in memory.
type ServiceCache interface {
	generic.CacheInterface[*v1.Service]
}

type ServiceStatusHandler func(obj *v1.Service, status v1.ServiceStatus) (v1.ServiceStatus, error)

type ServiceGeneratingHandler func(obj *v1.Service, status v1.ServiceStatus) ([]runtime.Object, v1.ServiceStatus, error)

func RegisterServiceStatusHandler(ctx context.Context, controller ServiceController, condition condition.Cond, name string, handler ServiceStatusHandler) {
	statusHandler := &serviceStatusHandler{
		client:    controller,
		condition: condition,
		handler:   handler,
	}
	controller.AddGenericHandler(ctx, name, generic.FromObjectHandlerToHandler(statusHandler.sync))
}

func RegisterServiceGeneratingHandler(ctx context.Context, controller ServiceController, apply apply.Apply,
	condition condition.Cond, name string, handler ServiceGeneratingHandler, opts *generic.GeneratingHandlerOptions) {
	statusHandler := &serviceGeneratingHandler{
		ServiceGeneratingHandler: handler,
		apply:                    apply,
		name:                     name,
		gvk:                      controller.GroupVersionKind(),
	}
	if opts != nil {
		statusHandler.opts = *opts
	}
	controller.OnChange(ctx, name, statusHandler.Remove)
	RegisterServiceStatusHandler(ctx, controller, condition, name, statusHandler.Handle)
}

type serviceStatusHandler struct {
	client    ServiceClient
	condition condition.Cond
	handler   ServiceStatusHandler
}

func (a *serviceStatusHandler) sync(key string, obj *v1.Service) (*v1.Service, error) {
	if obj == nil {
		return obj, nil
	}

	origStatus := obj.Status.DeepCopy()
	obj = obj.DeepCopy()
	newStatus, err := a.handler(obj, obj.Status)
	if err != nil {
		// Revert to old status on error
		newStatus = *origStatus.DeepCopy()
	}

	if a.condition != "" {
		if errors.IsConflict(err) {
			a.condition.SetError(&newStatus, "", nil)
		} else {
			a.condition.SetError(&newStatus, "", err)
		}
	}
	if !equality.Semantic.DeepEqual(origStatus, &newStatus) {
		if a.condition != "" {
			// Since status has changed, update the lastUpdatedTime
			a.condition.LastUpdated(&newStatus, time.Now().UTC().Format(time.RFC3339))
		}

		var newErr error
		obj.Status = newStatus
		newObj, newErr := a.client.UpdateStatus(obj)
		if err == nil {
			err = newErr
		}
		if newErr == nil {
			obj = newObj
		}
	}
	return obj, err
}

type serviceGeneratingHandler struct {
	ServiceGeneratingHandler
	apply apply.Apply
	opts  generic.GeneratingHandlerOptions
	gvk   schema.GroupVersionKind
	name  string
}

func (a *serviceGeneratingHandler) Remove(key string, obj *v1.Service) (*v1.Service, error) {
	if obj != nil {
		return obj, nil
	}

	obj = &v1.Service{}
	obj.Namespace, obj.Name = kv.RSplit(key, "/")
	obj.SetGroupVersionKind(a.gvk)

	return nil, generic.ConfigureApplyForObject(a.apply, obj, &a.opts).
		WithOwner(obj).
		WithSetID(a.name).
		ApplyObjects()
}

func (a *serviceGeneratingHandler) Handle(obj *v1.Service, status v1.ServiceStatus) (v1.ServiceStatus, error) {
	if !obj.DeletionTimestamp.IsZero() {
		return status, nil
	}

	objs, newStatus, err := a.ServiceGeneratingHandler(obj, status)
	if err != nil {
		return newStatus, err
	}

	return newStatus, generic.ConfigureApplyForObject(a.apply, obj, &a.opts).
		WithOwner(obj).
		WithSetID(a.name).
		ApplyObjects(objs...)
}
