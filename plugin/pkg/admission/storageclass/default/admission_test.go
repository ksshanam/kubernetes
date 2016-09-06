/*
Copyright 2016 The Kubernetes Authors.

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

package admission

import (
	"testing"

	"github.com/golang/glog"

	"k8s.io/kubernetes/pkg/admission"
	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/api/unversioned"
	"k8s.io/kubernetes/pkg/apis/extensions"
	"k8s.io/kubernetes/pkg/conversion"
)

func TestAdmission(t *testing.T) {
	defaultClass1 := &extensions.StorageClass{
		TypeMeta: unversioned.TypeMeta{
			Kind: "StorageClass",
		},
		ObjectMeta: api.ObjectMeta{
			Name: "default1",
			Annotations: map[string]string{
				isDefaultAnnotation: "true",
			},
		},
		Provisioner: "default1",
	}
	defaultClass2 := &extensions.StorageClass{
		TypeMeta: unversioned.TypeMeta{
			Kind: "StorageClass",
		},
		ObjectMeta: api.ObjectMeta{
			Name: "default2",
			Annotations: map[string]string{
				isDefaultAnnotation: "true",
			},
		},
		Provisioner: "default2",
	}
	// Class that has explicit default = false
	classWithFalseDefault := &extensions.StorageClass{
		TypeMeta: unversioned.TypeMeta{
			Kind: "StorageClass",
		},
		ObjectMeta: api.ObjectMeta{
			Name: "nondefault1",
			Annotations: map[string]string{
				isDefaultAnnotation: "false",
			},
		},
		Provisioner: "nondefault1",
	}
	// Class with missing default annotation (=non-default)
	classWithNoDefault := &extensions.StorageClass{
		TypeMeta: unversioned.TypeMeta{
			Kind: "StorageClass",
		},
		ObjectMeta: api.ObjectMeta{
			Name: "nondefault2",
		},
		Provisioner: "nondefault1",
	}
	// Class with empty default annotation (=non-default)
	classWithEmptyDefault := &extensions.StorageClass{
		TypeMeta: unversioned.TypeMeta{
			Kind: "StorageClass",
		},
		ObjectMeta: api.ObjectMeta{
			Name: "nondefault2",
			Annotations: map[string]string{
				isDefaultAnnotation: "",
			},
		},
		Provisioner: "nondefault1",
	}

	claimWithClass := &api.PersistentVolumeClaim{
		TypeMeta: unversioned.TypeMeta{
			Kind: "PersistentVolumeClaim",
		},
		ObjectMeta: api.ObjectMeta{
			Name:      "claimWithClass",
			Namespace: "ns",
			Annotations: map[string]string{
				classAnnotation: "foo",
			},
		},
	}
	claimWithEmptyClass := &api.PersistentVolumeClaim{
		TypeMeta: unversioned.TypeMeta{
			Kind: "PersistentVolumeClaim",
		},
		ObjectMeta: api.ObjectMeta{
			Name:      "claimWithEmptyClass",
			Namespace: "ns",
			Annotations: map[string]string{
				classAnnotation: "",
			},
		},
	}
	claimWithNoClass := &api.PersistentVolumeClaim{
		TypeMeta: unversioned.TypeMeta{
			Kind: "PersistentVolumeClaim",
		},
		ObjectMeta: api.ObjectMeta{
			Name:      "claimWithNoClass",
			Namespace: "ns",
		},
	}

	tests := []struct {
		name              string
		classes           []*extensions.StorageClass
		claim             *api.PersistentVolumeClaim
		expectError       bool
		expectedClassName string
	}{
		{
			"no default, no modification of PVCs",
			[]*extensions.StorageClass{classWithFalseDefault, classWithNoDefault, classWithEmptyDefault},
			claimWithNoClass,
			false,
			"",
		},
		{
			"one default, modify PVC with class=nil",
			[]*extensions.StorageClass{defaultClass1, classWithFalseDefault, classWithNoDefault, classWithEmptyDefault},
			claimWithNoClass,
			false,
			"default1",
		},
		{
			"one default, no modification of PVC with class=''",
			[]*extensions.StorageClass{defaultClass1, classWithFalseDefault, classWithNoDefault, classWithEmptyDefault},
			claimWithEmptyClass,
			false,
			"",
		},
		{
			"one default, no modification of PVC with class='foo'",
			[]*extensions.StorageClass{defaultClass1, classWithFalseDefault, classWithNoDefault, classWithEmptyDefault},
			claimWithClass,
			false,
			"foo",
		},
		{
			"two defaults, error with PVC with class=nil",
			[]*extensions.StorageClass{defaultClass1, defaultClass2, classWithFalseDefault, classWithNoDefault, classWithEmptyDefault},
			claimWithNoClass,
			true,
			"",
		},
		{
			"two defaults, no modification of PVC with class=''",
			[]*extensions.StorageClass{defaultClass1, defaultClass2, classWithFalseDefault, classWithNoDefault, classWithEmptyDefault},
			claimWithEmptyClass,
			false,
			"",
		},
		{
			"two defaults, no modification of PVC with class='foo'",
			[]*extensions.StorageClass{defaultClass1, defaultClass2, classWithFalseDefault, classWithNoDefault, classWithEmptyDefault},
			claimWithClass,
			false,
			"foo",
		},
	}

	for _, test := range tests {
		glog.V(4).Infof("starting test %q", test.name)

		// clone the claim, it's going to be modified
		clone, err := conversion.NewCloner().DeepCopy(test.claim)
		if err != nil {
			t.Fatalf("Cannot clone claim: %v", err)
		}
		claim := clone.(*api.PersistentVolumeClaim)

		ctrl := newPlugin(nil)
		for _, c := range test.classes {
			ctrl.store.Add(c)
		}
		attrs := admission.NewAttributesRecord(
			claim, // new object
			nil,   // old object
			api.Kind("PersistentVolumeClaim").WithVersion("version"),
			claim.Namespace,
			claim.Name,
			api.Resource("persistentvolumeclaims").WithVersion("version"),
			"", // subresource
			admission.Create,
			nil, // userInfo
		)
		err = ctrl.Admit(attrs)
		glog.Infof("Got %v", err)
		if err != nil && !test.expectError {
			t.Errorf("Test %q: unexpected error received: %v", test.name, err)
		}
		if err == nil && test.expectError {
			t.Errorf("Test %q: expected error and no error recevied", test.name)
		}

		class := ""
		if claim.Annotations != nil {
			if value, ok := claim.Annotations[classAnnotation]; ok {
				class = value
			}
		}
		if test.expectedClassName != "" && test.expectedClassName != class {
			t.Errorf("Test %q: expected class name %q, got %q", test.name, test.expectedClassName, class)
		}
		if test.expectedClassName == "" && class != "" {
			t.Errorf("Test %q: expected class name %q, got %q", test.name, test.expectedClassName, class)
		}
	}
}
